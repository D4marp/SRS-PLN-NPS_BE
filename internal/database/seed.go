package database

import (
	"database/sql"
	"log"
	"time"

	"github.com/bookify-rooms/backend/internal/utils"
	"github.com/google/uuid"
)

func seedUserIfMissing(db *sql.DB, email, password, name, role string) error {
	if email == "" || password == "" {
		return nil
	}

	var exists bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)", email).Scan(&exists); err != nil {
		return err
	}
	if exists {
		log.Printf("[seed] %s %s already exists, skipping", role, email)
		return nil
	}

	hashed, err := utils.HashPassword(password)
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	_, err = db.Exec(
		`INSERT INTO users (id, name, email, password, role, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), name, email, hashed, role, now,
	)
	if err != nil {
		return err
	}

	log.Printf("[seed] %s %s created successfully", role, email)
	return nil
}

func SeedSuperAdmin(db *sql.DB, email, password, name string) error {
	if email == "" || password == "" {
		log.Println("[seed] SUPERADMIN_EMAIL or SUPERADMIN_PASSWORD not set, skipping")
		return nil
	}
	return seedUserIfMissing(db, email, password, name, "superadmin")
}

func SeedAdmin(db *sql.DB, email, password, name string) error {
	if email == "" || password == "" {
		log.Println("[seed] ADMIN_EMAIL or ADMIN_PASSWORD not set, skipping")
		return nil
	}
	if name == "" {
		name = "Admin"
	}
	return seedUserIfMissing(db, email, password, name, "admin")
}

func SeedBooking(db *sql.DB, email, password, name string) error {
	if email == "" || password == "" {
		log.Println("[seed] BOOKING_EMAIL or BOOKING_PASSWORD not set, skipping")
		return nil
	}
	if name == "" {
		name = "Petugas Booking"
	}
	return seedUserIfMissing(db, email, password, name, "booking")
}

// SeedSuperAdminExtra creates an additional superadmin (SUPERADMIN2_*), if configured.
func SeedSuperAdminExtra(db *sql.DB, email, password, name string) error {
	if email == "" || password == "" {
		return nil
	}
	if name == "" {
		name = "Super Admin"
	}
	return seedUserIfMissing(db, email, password, name, "superadmin")
}
