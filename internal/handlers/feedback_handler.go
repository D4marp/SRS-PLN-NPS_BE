package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
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

	// Verify booking exists and ensure it is eligible for feedback
	var booking models.Booking
	var actualCheckOutTime sql.NullString
	err := h.db.QueryRowContext(context.Background(),
		"SELECT id, user_id, actual_check_out_time FROM bookings WHERE id = ?", bookingID).
		Scan(&booking.ID, &booking.UserID, &actualCheckOutTime)

	if err == sql.ErrNoRows {
		utils.Error(c, http.StatusNotFound, "Booking not found")
		return
	}
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}
	if !actualCheckOutTime.Valid || actualCheckOutTime.String == "" {
		utils.Error(c, http.StatusBadRequest, "feedback can only be submitted after checkout")
		return
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

	_, err = h.db.ExecContext(context.Background(),
		`INSERT INTO feedbacks (id, booking_id, user_id, satisfaction_level, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		feedbackID, bookingID, userID, req.SatisfactionLevel, req.Reason, now)

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
		"createdAt":           now,
	})
}

// GetFeedback retrieves feedback for a specific booking
// GET /api/bookings/:id/feedback
func (h *FeedbackHandler) GetFeedback(c *gin.Context) {
	bookingID := c.Param("id")

	var feedback models.Feedback
	err := h.db.QueryRowContext(context.Background(),
		`SELECT id, booking_id, user_id, satisfaction_level, reason, created_at
		 FROM feedbacks WHERE booking_id = ?`, bookingID).
		Scan(&feedback.ID, &feedback.BookingID, &feedback.UserID, &feedback.SatisfactionLevel, &feedback.Reason, &feedback.CreatedAt)

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
		`SELECT id, booking_id, user_id, satisfaction_level, reason, created_at
		 FROM feedbacks ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset)

	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}
	defer rows.Close()

	feedbacks := []models.Feedback{}
	for rows.Next() {
		var f models.Feedback
		if err := rows.Scan(&f.ID, &f.BookingID, &f.UserID, &f.SatisfactionLevel, &f.Reason, &f.CreatedAt); err == nil {
			feedbacks = append(feedbacks, f)
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
	var feedback models.Feedback
	err := db.QueryRowContext(context.Background(),
		`SELECT id, booking_id, user_id, satisfaction_level, reason, created_at
		 FROM feedbacks WHERE booking_id = ?`, bookingID).
		Scan(&feedback.ID, &feedback.BookingID, &feedback.UserID, &feedback.SatisfactionLevel, &feedback.Reason, &feedback.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil // No feedback yet
	}
	if err != nil {
		return nil, err
	}
	return &feedback, nil
}
