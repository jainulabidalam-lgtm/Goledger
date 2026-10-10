package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

// InitDB opens a SQLite database at the specified path and runs migrations.
func InitDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Connection limits suitable for SQLite
	db.SetMaxOpenConns(1) // SQLite writer lock needs 1 connection for writes to avoid BUSY errors easily
	db.SetMaxIdleConns(1)

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	// Ensure migrations table exists
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY
	);`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			currency TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS journal_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date DATE NOT NULL,
			description TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS journal_lines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			journal_entry_id INTEGER NOT NULL,
			account_id INTEGER NOT NULL,
			debit_amount INTEGER NOT NULL DEFAULT 0,
			credit_amount INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(journal_entry_id) REFERENCES journal_entries(id),
			FOREIGN KEY(account_id) REFERENCES accounts(id),
			CHECK (debit_amount >= 0 AND credit_amount >= 0),
			CHECK ((debit_amount > 0 AND credit_amount = 0) OR (debit_amount = 0 AND credit_amount > 0))
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
	}

	for i, migration := range migrations {
		var version int
		err := db.QueryRow("SELECT version FROM schema_migrations WHERE version = ?", i+1).Scan(&version)
		if err == sql.ErrNoRows {
			log.Printf("Running migration version %d", i+1)
			if _, err := db.Exec(migration); err != nil {
				return fmt.Errorf("failed executing migration %d: %w", i+1, err)
			}
			if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", i+1); err != nil {
				return fmt.Errorf("failed to record migration %d: %w", i+1, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check migration version %d: %w", i+1, err)
		}
	}
	return nil
}
