package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port              string
	DatabaseURL       string
	JWTSecret         string
	JWTExpiry         string // e.g. "168h"
	UploadsDir        string
	BaseURL           string
	AllowedOrigins    []string
	SuperAdminEmail   string
	SuperAdminPassword string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Try to build DATABASE_URL from individual components (DB_HOST, DB_PORT, etc)
	databaseURL := getEnv("DATABASE_URL", "")
	if databaseURL == "" {
		databaseURL = buildDatabaseURL()
	}

	// Fallback if still empty
	if databaseURL == "" {
		databaseURL = "root:@tcp(127.0.0.1:3306)/bookify?parseTime=true&charset=utf8mb4"
	}

	return &Config{
		Port:              getEnv("PORT", "8080"),
		DatabaseURL:       databaseURL,
		JWTSecret:         getEnv("JWT_SECRET", "change-this-secret-in-production"),
		JWTExpiry:         getEnv("JWT_EXPIRY", "168h"),
		UploadsDir:        getEnv("UPLOADS_DIR", "./uploads"),
		BaseURL:           getEnv("BASE_URL", "http://localhost:8080"),
		AllowedOrigins:    []string{getEnv("ALLOWED_ORIGINS", "*")},
		SuperAdminEmail:   getEnv("SUPERADMIN_EMAIL", ""),
		SuperAdminPassword: getEnv("SUPERADMIN_PASSWORD", ""),
	}
}

// buildDatabaseURL constructs a MySQL DSN from individual DB_* environment variables
func buildDatabaseURL() string {
	host := getEnv("DB_HOST", "")
	port := getEnv("DB_PORT", "")
	name := getEnv("DB_NAME", "")
	user := getEnv("DB_USER", "")
	password := getEnv("DB_PASSWORD", "")

	// Only build if at least host and name are provided
	if host == "" || name == "" {
		return ""
	}

	if port == "" {
		port = "3306" // MySQL default
	}

	// Format: user:password@tcp(host:port)/dbname?parseTime=true&charset=utf8mb4
	if password != "" {
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4", user, password, host, port, name)
	}
	// Without password
	return fmt.Sprintf("%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4", user, host, port, name)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
