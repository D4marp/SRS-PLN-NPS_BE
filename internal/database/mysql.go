package database

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// ConnectWithRetry attempts to connect to the database with exponential backoff retry logic.
// This is essential in containerized environments where MySQL might not be ready immediately.
func ConnectWithRetry(dsn string, maxRetries int, initialWaitTime time.Duration) (*sql.DB, error) {
	var lastErr error
	waitTime := initialWaitTime

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("Database connection attempt %d/%d...", attempt, maxRetries)

		db, err := sql.Open("mysql", dsn)
		if err != nil {
			lastErr = err
			log.Printf("Failed to open database (attempt %d): %v", attempt, err)
			if attempt < maxRetries {
				time.Sleep(waitTime)
				waitTime = time.Duration(math.Min(float64(waitTime*2), 30)) * time.Second
			}
			continue
		}

		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
		db.SetConnMaxIdleTime(5 * time.Minute)

		if err := db.Ping(); err != nil {
			lastErr = err
			log.Printf("Failed to ping database (attempt %d): %v, retrying in %v...", attempt, err, waitTime)
			db.Close()
			if attempt < maxRetries {
				time.Sleep(waitTime)
				waitTime = time.Duration(math.Min(float64(waitTime*2), 30)) * time.Second
			}
			continue
		}

		log.Printf("✓ Database connected successfully on attempt %d", attempt)
		return db, nil
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, lastErr)
}

// Connect attempts to connect to the database with default retry settings (8 retries, 1s initial wait).
// For production, prefer ConnectWithRetry with custom parameters.
func Connect(dsn string) (*sql.DB, error) {
	// Default retry settings: 8 attempts with exponential backoff starting at 1 second
	// This gives ~4 minutes total wait time in worst case, suitable for most scenarios
	return ConnectWithRetry(dsn, 8, 1*time.Second)
}

func RunMigrations(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename VARCHAR(255) PRIMARY KEY,
			applied_at BIGINT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		var count int
		db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE filename = ?",
			entry.Name()).Scan(&count)
		if count > 0 {
			continue
		}

		content, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		// Split on semicolons to handle multiple statements
		statements := strings.Split(string(content), ";")
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err = db.Exec(stmt); err != nil {
				return fmt.Errorf("run migration %s: %w", entry.Name(), err)
			}
		}

		db.Exec("INSERT INTO schema_migrations (filename, applied_at) VALUES (?, ?)",
			entry.Name(), time.Now().UnixMilli())

		fmt.Printf("✓ Migration applied: %s\n", entry.Name())
	}

	return nil
}
