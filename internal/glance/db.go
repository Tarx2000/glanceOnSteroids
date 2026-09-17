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
	Store      SettingsStore
)

// SettingsStore defines the interface for reading and writing runtime settings.
type SettingsStore interface {
	GetSetting(key, defaultValue string) (string, error)
	SetSetting(key, value string) error
}

// SQLiteStore implements SettingsStore using a SQLite database backend with an in-memory read cache.
type SQLiteStore struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string]string
}

func (s *SQLiteStore) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := s.db.Exec(`
		INSERT INTO settings (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value;
	`, key, value)
	if err == nil {
		if s.cache == nil {
			s.cache = make(map[string]string)
		}
		s.cache[key] = value
	}
	return err
}

func (s *SQLiteStore) GetSetting(key, defaultValue string) (string, error) {
	s.mu.RLock()
	if s.cache != nil {
		if val, ok := s.cache[key]; ok {
			s.mu.RUnlock()
			return val, nil
		}
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return defaultValue, fmt.Errorf("database not initialized")
	}
	// Double check cache after acquiring write lock
	if s.cache != nil {
		if val, ok := s.cache[key]; ok {
			return val, nil
		}
	} else {
		s.cache = make(map[string]string)
	}

	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?;", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			s.cache[key] = defaultValue
			return defaultValue, nil
		}
		return defaultValue, err
	}
	s.cache[key] = value
	return value, nil
}

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
	Store = &SQLiteStore{db: db, cache: make(map[string]string)}
	return nil
}

// dbSetSetting sets a key-value pair in the SQLite database (compatibility wrapper).
func dbSetSetting(key, value string) error {
	if Store == nil {
		return fmt.Errorf("store not initialized")
	}
	return Store.SetSetting(key, value)
}

// dbGetSetting retrieves a value from the SQLite database by key (compatibility wrapper).
func dbGetSetting(key, defaultValue string) (string, error) {
	if Store == nil {
		return defaultValue, fmt.Errorf("store not initialized")
	}
	return Store.GetSetting(key, defaultValue)
}