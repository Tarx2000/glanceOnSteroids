package glance

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	dbInstance *sql.DB
	dbMu       sync.Mutex
)

// initDB initializes the SQLite connection and runs schema setup.
// It places the database in the same directory as the config file (e.g. `./glance.db`).
func initDB(configPath string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	if dbInstance != nil {
		return nil
	}

	configDir := filepath.Dir(configPath)
	dbPath := filepath.Join(configDir, "glance.db")
	log.Printf("[DB] Initializing SQLite database at %s", dbPath)

	// Ensure the parent directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("opening SQLite database: %w", err)
	}

	// Validate the connection immediately
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("pinging SQLite database: %w", err)
	}

	// Optimize SQLite settings for performance
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
	`); err != nil {
		db.Close()
		return fmt.Errorf("configuring SQLite pragma: %w", err)
	}

	// Create settings table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	if err != nil {
		db.Close()
		return fmt.Errorf("creating settings table: %w", err)
	}

	dbInstance = db
	return nil
}

// dbSetSetting sets a key-value pair in the SQLite database.
func dbSetSetting(key, value string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	if dbInstance == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := dbInstance.Exec(`
		INSERT INTO settings (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value;
	`, key, value)
	return err
}

// dbGetSetting retrieves a value from the SQLite database by key.
// If not found, it returns the provided default value.
func dbGetSetting(key, defaultValue string) (string, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	if dbInstance == nil {
		return defaultValue, fmt.Errorf("database not initialized")
	}

	var value string
	err := dbInstance.QueryRow("SELECT value FROM settings WHERE key = ?;", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return defaultValue, nil
		}
		return defaultValue, err
	}

	return value, nil
}