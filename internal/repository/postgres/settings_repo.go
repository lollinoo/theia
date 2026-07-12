package postgres

// This file defines settings repo persistence behavior, ordering guarantees, and not-found conventions.

import (
	"database/sql"
	"fmt"
	"time"
)

// SettingsRepo implements domain.SettingsRepository using PostgreSQL.
type SettingsRepo struct {
	db *DB
}

// NewSettingsRepo creates a new PostgreSQL-backed settings repository.
func NewSettingsRepo(db *sql.DB) *SettingsRepo {
	return &SettingsRepo{db: wrapDB(db)}
}

// Get retrieves a single setting value by key.
// Returns an error if the key is not found.
func (r *SettingsRepo) Get(key string) (string, error) {
	var value string
	err := r.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("setting not found: %s", key)
		}
		return "", fmt.Errorf("querying setting %s: %w", key, err)
	}
	return value, nil
}

// Set creates or updates a setting value.
func (r *SettingsRepo) Set(key, value string) error {
	now := time.Now().UTC()
	_, err := r.db.Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, now,
	)
	if err != nil {
		return fmt.Errorf("setting %s: %w", key, err)
	}
	return nil
}

// Update locks an existing setting row and replaces its value transactionally.
func (r *SettingsRepo) Update(key string, update func(current string) (string, error)) (string, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return "", fmt.Errorf("beginning setting %s update: %w", key, err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is harmless

	var current string
	if err := tx.QueryRow(`SELECT value FROM settings WHERE key = ? FOR UPDATE`, key).Scan(&current); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("setting not found: %s", key)
		}
		return "", fmt.Errorf("locking setting %s: %w", key, err)
	}

	next, err := update(current)
	if err != nil {
		return "", fmt.Errorf("updating setting %s: %w", key, err)
	}
	if _, err := tx.Exec(
		`UPDATE settings SET value = ?, updated_at = ? WHERE key = ?`,
		next, time.Now().UTC(), key,
	); err != nil {
		return "", fmt.Errorf("setting %s: %w", key, err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing setting %s update: %w", key, err)
	}
	return next, nil
}

// GetAll retrieves all settings as a key-value map.
func (r *SettingsRepo) GetAll() (map[string]string, error) {
	rows, err := r.db.Query(`SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("querying all settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scanning setting: %w", err)
		}
		settings[key] = value
	}

	return settings, rows.Err()
}
