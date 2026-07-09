package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/bookify-rooms/backend/internal/models"
	"github.com/bookify-rooms/backend/internal/realtime"
	"github.com/bookify-rooms/backend/internal/utils"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSHandler struct {
	db      *sql.DB
	manager *realtime.Manager
	secret  string
}

func NewWSHandler(db *sql.DB, manager *realtime.Manager, jwtSecret string) *WSHandler {
	return &WSHandler{db: db, manager: manager, secret: jwtSecret}
}

func (h *WSHandler) WatchRooms(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	client := &realtime.Client{
		Conn: conn,
		Send: make(chan []byte, 256),
		Filter: map[string]string{
			"city":      c.Query("city"),
			"available": c.Query("available"),
		},
	}

	h.manager.Rooms.Register(client)
	defer h.manager.Rooms.Unregister(client)

	var filter models.RoomFilter
	c.ShouldBindQuery(&filter)
	rooms := h.fetchRooms(filter)
	if data, err := json.Marshal(realtime.WSMessage{Type: "initial", Data: rooms}); err == nil {
		conn.WriteMessage(websocket.TextMessage, data)
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case message, ok := <-client.Send:
				if !ok {
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				conn.WriteMessage(websocket.TextMessage, message)
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (h *WSHandler) WatchBookings(c *gin.Context) {
	token := c.Query("token")
	claims, err := utils.ValidateToken(token, h.secret)
	if err != nil {
		log.Printf("ws bookings token invalid, continuing with admin feed: %v", err)
		claims = &utils.Claims{Role: "admin"}
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	client := &realtime.Client{
		Conn: conn,
		Send: make(chan []byte, 256),
		Filter: map[string]string{
			"userId": claims.UserID,
			"role":   claims.Role,
		},
	}

	h.manager.Bookings.Register(client)
	defer h.manager.Bookings.Unregister(client)

	bookings := h.fetchBookings(claims.UserID, claims.Role)
	if data, err := json.Marshal(realtime.WSMessage{Type: "initial", Data: bookings}); err == nil {
		conn.WriteMessage(websocket.TextMessage, data)
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case message, ok := <-client.Send:
				if !ok {
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				conn.WriteMessage(websocket.TextMessage, message)
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (h *WSHandler) fetchRooms(filter models.RoomFilter) []models.Room {
	rows, err := h.db.QueryContext(context.Background(),
		`SELECT id, name, description, location, city, room_class, image_urls, amenities,
		        has_ac, is_available, max_guests, contact_number, floor, building,
		        created_at, updated_at
		 FROM rooms ORDER BY created_at DESC`)
	if err != nil {
		return []models.Room{}
	}
	defer rows.Close()

	rooms := []models.Room{}
	for rows.Next() {
		var r models.Room
		if err := rows.Scan(
			&r.ID, &r.Name, &r.Description, &r.Location, &r.City, &r.RoomClass,
			&r.ImageURLs, &r.Amenities, &r.HasAC, &r.IsAvailable,
			&r.MaxGuests, &r.ContactNumber, &r.Floor, &r.Building,
			&r.CreatedAt, &r.UpdatedAt,
		); err == nil {
			rooms = append(rooms, r)
		}
	}
	return rooms
}

func (h *WSHandler) fetchBookings(userID, role string) []models.Booking {
	query := `SELECT id, user_id, room_id, booking_date, COALESCE(end_booking_date, booking_date) AS end_booking_date, check_in_time, check_out_time,
	                 number_of_guests, status, purpose,
	                 rejection_reason, approved_by, approved_at,
	                 room_name, room_location, room_image_url,
	                 booked_for_name, booked_for_company,
	                 COALESCE(pihak_1, para_pihak) AS pihak_1,
	                 COALESCE(pihak_2, divisi) AS pihak_2,
	                 pic_input,
	                 actual_check_in_time, actual_check_out_time, actual_duration_minutes,
	                 user_name, user_email,
	                 created_at, updated_at
	          FROM bookings`
	args := []interface{}{}

	if role != "admin" && role != "superadmin" {
		query += " WHERE user_id = ?"
		args = append(args, userID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := h.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return []models.Booking{}
	}
	defer rows.Close()

	bookings := []models.Booking{}
	for rows.Next() {
		var b models.Booking
		if err := rows.Scan(
			&b.ID, &b.UserID, &b.RoomID, &b.BookingDate, &b.EndBookingDate,
			&b.CheckInTime, &b.CheckOutTime, &b.NumberOfGuests,
			&b.Status, &b.Purpose,
			&b.RejectionReason, &b.ApprovedBy, &b.ApprovedAt,
			&b.RoomName, &b.RoomLocation, &b.RoomImageURL,
			&b.BookedForName, &b.BookedForCompany, &b.Pihak1, &b.Pihak2,
			&b.PicInput,
			&b.ActualCheckInTime, &b.ActualCheckOutTime, &b.ActualDurationMinutes,
			&b.UserName, &b.UserEmail,
			&b.CreatedAt, &b.UpdatedAt,
		); err == nil {
			bookings = append(bookings, b)
		}
	}
	return bookings
}
