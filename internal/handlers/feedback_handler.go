package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bookify-rooms/backend/internal/models"
	"github.com/bookify-rooms/backend/internal/realtime"
	"github.com/bookify-rooms/backend/internal/utils"
)

type FeedbackHandler struct {
	db      *sql.DB
	manager *realtime.Manager
}

func NewFeedbackHandler(db *sql.DB, manager *realtime.Manager) *FeedbackHandler {
	return &FeedbackHandler{db: db, manager: manager}
}

// CreateFeedback creates a new feedback for a booking
// POST /api/bookings/:id/feedback
func (h *FeedbackHandler) CreateFeedback(c *gin.Context) {
	bookingID := c.Param("id")
	userID := c.GetString("userID")
	role := c.GetString("role")

	if userID == "" {
		utils.Error(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req models.CreateFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if req.SatisfactionLevel == string(models.SatisfactionUnsatisfied) {
		complaintOther := ""
		if req.ComplaintOther != nil {
			complaintOther = strings.TrimSpace(*req.ComplaintOther)
		}
		if len(req.ComplaintItems) == 0 && complaintOther == "" {
			utils.Error(c, http.StatusBadRequest, "unsatisfied feedback must include at least one complaint item or other note")
			return
		}
	}

	// Verify booking exists and ensure it is eligible for feedback
	var booking models.Booking
	var actualCheckOutTime sql.NullString
	var bookingStatus models.BookingStatus
	var roomAmenities models.StringSlice
	err := h.db.QueryRowContext(context.Background(),
		`SELECT b.id, b.user_id, b.status, b.actual_check_out_time, r.amenities
		 FROM bookings b
		 INNER JOIN rooms r ON r.id = b.room_id
		 WHERE b.id = ?`, bookingID).
		Scan(&booking.ID, &booking.UserID, &bookingStatus, &actualCheckOutTime, &roomAmenities)

	if err == sql.ErrNoRows {
		utils.Error(c, http.StatusNotFound, "Booking not found")
		return
	}
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}
	if bookingStatus != models.StatusCompleted || !actualCheckOutTime.Valid || actualCheckOutTime.String == "" {
		utils.Error(c, http.StatusBadRequest, "feedback can only be submitted after checkout and completed booking")
		return
	}

	if req.SatisfactionLevel == string(models.SatisfactionUnsatisfied) && len(req.ComplaintItems) > 0 {
		allowed := make(map[string]struct{}, len(roomAmenities))
		for _, amenity := range roomAmenities {
			normalized := strings.ToLower(strings.TrimSpace(amenity))
			if normalized != "" {
				allowed[normalized] = struct{}{}
			}
		}

		invalidItems := make([]string, 0)
		for _, item := range req.ComplaintItems {
			normalized := strings.ToLower(strings.TrimSpace(item))
			if normalized == "" {
				continue
			}
			if _, ok := allowed[normalized]; !ok {
				invalidItems = append(invalidItems, item)
			}
		}

		if len(invalidItems) > 0 {
			utils.Error(c, http.StatusBadRequest, "complaint items must match room facilities: "+strings.Join(invalidItems, ", "))
			return
		}
	}

	// Regular users can only submit feedback for their own booking.
	// Kiosk/admin roles may submit feedback after checkout on behalf of the booking.
	isPrivilegedRole := role == "booking" || role == "admin" || role == "superadmin"
	if !isPrivilegedRole && booking.UserID != userID {
		utils.Error(c, http.StatusForbidden, "You can only submit feedback for your own booking")
		return
	}

	// Check if feedback already exists
	var existingFeedback string
	err = h.db.QueryRowContext(context.Background(),
		"SELECT id FROM feedbacks WHERE booking_id = ?", bookingID).
		Scan(&existingFeedback)
	if err == nil {
		utils.Error(c, http.StatusConflict, "Feedback for this booking already exists")
		return
	} else if err != sql.ErrNoRows {
		utils.Error(c, http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}

	// Create feedback
	feedbackID := uuid.New().String()
	now := time.Now().UnixMilli()
	complaintItemsJSON, err := json.Marshal(req.ComplaintItems)
	if err != nil {
		utils.Error(c, http.StatusBadRequest, "invalid complaint items")
		return
	}

	var complaintOtherValue *string
	if req.ComplaintOther != nil {
		trimmed := strings.TrimSpace(*req.ComplaintOther)
		if trimmed != "" {
			complaintOtherValue = &trimmed
		}
	}

	_, err = h.db.ExecContext(context.Background(),
		`INSERT INTO feedbacks (id, booking_id, user_id, satisfaction_level, reason, complaint_items, complaint_other, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		feedbackID, bookingID, userID, req.SatisfactionLevel, req.Reason, string(complaintItemsJSON), complaintOtherValue, now)

	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "Failed to create feedback: "+err.Error())
		return
	}

	// Broadcast updated bookings (include feedback info)
	h.broadcastBookings()

	utils.SuccessMessage(c, http.StatusCreated, "Feedback submitted successfully", gin.H{
		"id":                  feedbackID,
		"bookingId":           bookingID,
		"userId":              userID,
		"satisfactionLevel":   req.SatisfactionLevel,
		"reason":              req.Reason,
		"complaintItems":      req.ComplaintItems,
		"complaintOther":      complaintOtherValue,
		"createdAt":           now,
	})
}

// GetFeedback retrieves feedback for a specific booking
// GET /api/bookings/:id/feedback
func (h *FeedbackHandler) GetFeedback(c *gin.Context) {
	bookingID := c.Param("id")

	feedback, err := loadFeedbackByBookingID(h.db, bookingID)

	if err == sql.ErrNoRows {
		utils.Success(c, http.StatusOK, nil)
		return
	}
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}

	utils.Success(c, http.StatusOK, feedback)
}

// ListFeedbacks retrieves all feedbacks (admin only)
// GET /api/feedbacks
func (h *FeedbackHandler) ListFeedbacks(c *gin.Context) {
	page := 1
	limit := 20

	pageParam := c.DefaultQuery("page", "1")
	if p, err := strconv.Atoi(pageParam); err == nil && p > 0 {
		page = p
	}

	limitParam := c.DefaultQuery("limit", "20")
	if l, err := strconv.Atoi(limitParam); err == nil && l > 0 {
		limit = l
	}

	offset := (page - 1) * limit

	rows, err := h.db.QueryContext(context.Background(),
		`SELECT id, booking_id, user_id, satisfaction_level, reason, complaint_items, complaint_other, created_at
		 FROM feedbacks ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset)

	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}
	defer rows.Close()

	feedbacks := []models.Feedback{}
	for rows.Next() {
		if f, err := scanFeedbackRow(rows); err == nil {
			feedbacks = append(feedbacks, *f)
		}
	}

	// Get total count
	var total int
	h.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM feedbacks").Scan(&total)

	utils.Success(c, http.StatusOK, gin.H{
		"data":       feedbacks,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + limit - 1) / limit,
	})
}

// GetSatisfactionStats returns satisfaction statistics (admin only)
// GET /api/feedbacks/stats
func (h *FeedbackHandler) GetSatisfactionStats(c *gin.Context) {
	var satisfied int
	var unsatisfied int

	h.db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM feedbacks WHERE satisfaction_level = 'satisfied'").Scan(&satisfied)
	h.db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM feedbacks WHERE satisfaction_level = 'unsatisfied'").Scan(&unsatisfied)

	total := satisfied + unsatisfied
	satisfactionRate := 0.0
	if total > 0 {
		satisfactionRate = float64(satisfied) / float64(total) * 100
	}

	utils.Success(c, http.StatusOK, gin.H{
		"satisfied":        satisfied,
		"unsatisfied":      unsatisfied,
		"total":            total,
		"satisfactionRate": satisfactionRate,
	})
}

func (h *FeedbackHandler) broadcastBookings() {
	rows, err := h.db.QueryContext(context.Background(),
		"SELECT "+bookingCols+" FROM bookings ORDER BY created_at DESC")
	if err != nil {
		return
	}
	defer rows.Close()

	bookings := []models.Booking{}
	for rows.Next() {
		var b models.Booking
		if err := scanBooking(rows, &b); err == nil {
			bookings = append(bookings, b)
		}
	}
	h.manager.Bookings.Broadcast(bookings)
}

// LoadFeedbackForBooking loads feedback for a single booking
func (h *FeedbackHandler) LoadFeedbackForBooking(db *sql.DB, bookingID string) (*models.Feedback, error) {
	feedback, err := loadFeedbackByBookingID(db, bookingID)

	if err == sql.ErrNoRows {
		return nil, nil // No feedback yet
	}
	if err != nil {
		return nil, err
	}
	return feedback, nil
}
