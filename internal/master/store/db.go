package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "github.com/glebarez/go-sqlite"
)

// DB wraps a sql.DB with application-specific methods.
type DB struct {
	*sql.DB
	log *slog.Logger
}

// New opens the SQLite database and runs migrations.
func New(dbPath string, log *slog.Logger) (*DB, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// Open with pure-Go SQLite driver (no CGO required)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Set connection pool (SQLite is single-writer, but readers can be concurrent)
	db.SetMaxOpenConns(1)

	// Enable WAL mode and foreign keys via PRAGMA
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("execute %s: %w", p, err)
		}
	}

	store := &DB{DB: db, log: log}

	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	log.Info("database initialized", "path", dbPath)
	return store, nil
}

// migrate runs all SQL migration files in order.
func (d *DB) migrate() error {
	// Create migrations tracking table
	_, err := d.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Check current version
	var currentVersion int
	row := d.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	d.log.Info("current schema version", "version", currentVersion)

	// Run embedded migrations
	migrations := getMigrations()
	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		d.log.Info("applying migration", "version", m.version, "name", m.name)

		tx, err := d.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %d (%s): %w", m.version, m.name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}

		d.log.Info("migration applied", "version", m.version)
	}

	return nil
}

type migration struct {
	version int
	name    string
	sql     string
}
