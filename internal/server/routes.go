package server

import (
	"github.com/gin-gonic/gin"

	"github.com/bookify-rooms/backend/internal/handlers"
	"github.com/bookify-rooms/backend/internal/middleware"
	"github.com/bookify-rooms/backend/internal/realtime"
)

func (s *Server) registerRoutes(r *gin.Engine) {
	authMw := middleware.Auth(s.cfg.JWTSecret)
	adminMw := middleware.RequireRole("admin", "superadmin")
	superMw := middleware.RequireRole("superadmin")

	// Shared realtime manager
	rtManager := realtime.NewManager()

	// -------------------------------------------------------------------------
	// Health Check (for Docker/K8s health checks)
	// -------------------------------------------------------------------------
	// -------------------------------------------------------------------------
	// Auth
	// -------------------------------------------------------------------------
	authH := handlers.NewAuthHandler(s.db, s.cfg.JWTSecret, s.cfg.JWTExpiry)
	auth := r.Group("/api/auth")
	{
		auth.POST("/register", authH.Register)
		auth.POST("/login", authH.Login)
		auth.POST("/forgot-password", authH.ForgotPassword)

		// Protected
		auth.GET("/me", authMw, authH.Me)
		auth.PUT("/me", authMw, authH.UpdateMe)
		auth.PATCH("/me/city", authMw, authH.UpdateCity)
		auth.PATCH("/me/password", authMw, authH.ChangePassword)
		auth.DELETE("/me", authMw, authH.DeleteAccount)
		auth.POST("/logout", authMw, authH.Logout)
	}

	// -------------------------------------------------------------------------
	// Rooms
	// -------------------------------------------------------------------------
	roomH := handlers.NewRoomHandler(s.db, rtManager)
	bookingH := handlers.NewBookingHandler(s.db, rtManager)
	facilityH := handlers.NewFacilityHandler(s.db)
	rooms := r.Group("/api/rooms")
	{
		rooms.GET("", roomH.ListRooms)
		rooms.GET("/:id", roomH.GetRoom)
		rooms.GET("/:id/bookings", bookingH.GetRoomBookings)

		// Admin/superadmin only
		rooms.POST("", authMw, adminMw, roomH.CreateRoom)
		rooms.PUT("/:id", authMw, adminMw, roomH.UpdateRoom)
		rooms.DELETE("/:id", authMw, adminMw, roomH.DeleteRoom)

		// Image upload (admin only)
		storageH := handlers.NewStorageHandler(s.db, s.cfg.UploadsDir, s.cfg.BaseURL)
		rooms.POST("/:id/images", authMw, adminMw, storageH.UploadRoomImage)
		rooms.DELETE("/:id/images", authMw, adminMw, storageH.DeleteRoomImage)
	}

	// -------------------------------------------------------------------------
	// Facilities (public list, admin create/update/delete)
	// -------------------------------------------------------------------------
	facilities := r.Group("/api/facilities")
	{
		facilities.GET("", facilityH.ListFacilities)

		// Admin/superadmin only
		facilities.POST("", authMw, adminMw, facilityH.CreateFacility)
		facilities.PUT("/:id", authMw, adminMw, facilityH.UpdateFacility)
		facilities.DELETE("/:id", authMw, adminMw, facilityH.DeleteFacility)
	}

	// -------------------------------------------------------------------------
	// Bookings
	// -------------------------------------------------------------------------
	feedbackH := handlers.NewFeedbackHandler(s.db, rtManager)
	r.POST("/api/bookings/:id/feedback", feedbackH.CreateFeedback)
	r.GET("/api/bookings/:id/feedback", feedbackH.GetFeedback)
	r.GET("/api/bookings/:id", bookingH.GetBooking)
	r.PATCH("/api/bookings/:id/checkin-checkout", bookingH.UpdateCheckInCheckOut)

	bookings := r.Group("/api/bookings")
	bookings.Use(authMw)
	{
		bookings.GET("", bookingH.ListBookings)
		bookings.GET("/pending", adminMw, bookingH.GetPendingBookings) // admin shortcut
		bookings.PUT("/:id", adminMw, bookingH.UpdateBooking)
		bookings.POST("", bookingH.CreateBooking)                          // → status: pending
		bookings.POST("/:id/approve", adminMw, bookingH.ApproveBooking)    // → confirmed
		bookings.POST("/:id/reject", adminMw, bookingH.RejectBooking)      // → rejected
		bookings.PATCH("/:id/cancel", bookingH.CancelBooking)              // → cancelled
		bookings.PATCH("/:id/complete", adminMw, bookingH.CompleteBooking) // → completed

	}

	// -------------------------------------------------------------------------
	// Feedbacks (admin panel)
	// -------------------------------------------------------------------------
	r.POST("/api/feedbacks", feedbackH.CreateFeedbackFromBody)

	feedbacks := r.Group("/api/feedbacks")
	feedbacks.Use(authMw, adminMw)
	{
		feedbacks.GET("", feedbackH.ListFeedbacks)
		feedbacks.GET("/stats", feedbackH.GetSatisfactionStats)
	}

	// -------------------------------------------------------------------------
	// Admin Panel
	// -------------------------------------------------------------------------
	adminH := handlers.NewAdminHandler(s.db, rtManager)
	admin := r.Group("/api/admin")
	admin.Use(authMw)
	{
		admin.GET("/stats", adminMw, adminH.GetStats)
		admin.GET("/bookings", adminMw, adminH.GetAdminBookings)

		// Superadmin: user management
		admin.GET("/users", superMw, adminH.ListUsers)
		admin.POST("/users", superMw, adminH.CreateUser)
		admin.GET("/users/:id", superMw, adminH.GetUser)
		admin.PATCH("/users/:id/role", superMw, adminH.ChangeUserRole)
		admin.DELETE("/users/:id", superMw, adminH.DeleteUser)
	}

	// -------------------------------------------------------------------------
	// WebSocket
	// -------------------------------------------------------------------------
	wsH := handlers.NewWSHandler(s.db, rtManager, s.cfg.JWTSecret)
	r.GET("/ws/rooms", wsH.WatchRooms)
	r.GET("/ws/bookings", wsH.WatchBookings) // token via query param ?token=<jwt>
}
