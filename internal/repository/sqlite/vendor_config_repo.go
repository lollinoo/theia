package sqlite

import (
	"database/sql"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

// VendorConfigRepo implements domain.VendorConfigRepository using SQLite.
type VendorConfigRepo struct {
	db *DB
}

// NewVendorConfigRepo creates a new VendorConfigRepo.
func NewVendorConfigRepo(db *sql.DB) *VendorConfigRepo {
	return &VendorConfigRepo{db: wrapDB(db)}
}

// GetAll returns all vendor configs.
func (r *VendorConfigRepo) GetAll() ([]domain.VendorConfigRecord, error) {
	rows, err := r.db.Query(
		`SELECT name, display_name, config_json, created_at, updated_at FROM vendor_configs ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []domain.VendorConfigRecord
	for rows.Next() {
		var rec domain.VendorConfigRecord
		if err := rows.Scan(&rec.Name, &rec.DisplayName, &rec.ConfigJSON, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// GetByName returns a single vendor config by name.
func (r *VendorConfigRepo) GetByName(name string) (*domain.VendorConfigRecord, error) {
	var rec domain.VendorConfigRecord
	err := r.db.QueryRow(
		`SELECT name, display_name, config_json, created_at, updated_at FROM vendor_configs WHERE name = ?`,
		name,
	).Scan(&rec.Name, &rec.DisplayName, &rec.ConfigJSON, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// Upsert inserts or replaces a vendor config record.
func (r *VendorConfigRepo) Upsert(record *domain.VendorConfigRecord) error {
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now

	_, err := r.db.Exec(
		`INSERT INTO vendor_configs (name, display_name, config_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   display_name = excluded.display_name,
		   config_json = excluded.config_json,
		   updated_at = excluded.updated_at`,
		record.Name, record.DisplayName, record.ConfigJSON, record.CreatedAt, record.UpdatedAt,
	)
	return err
}
