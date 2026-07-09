package main

import (
	"log"

	"github.com/bookify-rooms/backend/internal/config"
	"github.com/bookify-rooms/backend/internal/database"
	"github.com/bookify-rooms/backend/internal/server"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	if err := database.SeedSuperAdmin(db, cfg.SuperAdminEmail, cfg.SuperAdminPassword, "Super Admin"); err != nil {
		log.Fatalf("Failed to seed superadmin: %v", err)
	}
	if err := database.SeedAdmin(db, cfg.AdminEmail, cfg.AdminPassword, cfg.AdminName); err != nil {
		log.Fatalf("Failed to seed admin: %v", err)
	}
	if err := database.SeedBooking(db, cfg.BookingEmail, cfg.BookingPassword, cfg.BookingName); err != nil {
		log.Fatalf("Failed to seed booking user: %v", err)
	}
	if err := database.SeedSuperAdminExtra(db, cfg.SuperAdmin2Email, cfg.SuperAdmin2Password, cfg.SuperAdmin2Name); err != nil {
		log.Fatalf("Failed to seed superadmin2: %v", err)
	}

	srv := server.New(cfg, db)
	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
