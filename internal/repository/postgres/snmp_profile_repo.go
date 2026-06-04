package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
)

// SNMPProfileRepo implements domain.SNMPProfileRepository using PostgreSQL.
type SNMPProfileRepo struct {
	db      *DB
	keyring *crypto.Keyring
}

// NewSNMPProfileRepo creates a new SNMPProfileRepo.
func NewSNMPProfileRepo(db *sql.DB, keyring *crypto.Keyring) *SNMPProfileRepo {
	return &SNMPProfileRepo{db: wrapDB(db), keyring: keyring}
}

// Create inserts a new SNMP profile.
func (r *SNMPProfileRepo) Create(profile *domain.SNMPProfile) error {
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now

	// Deep copy credentials for encryption (don't modify the original)
	credsCopy := deepCopySNMPCredentials(profile.Credentials)
	if err := encryptSNMPCredentials(&credsCopy, r.keyring); err != nil {
		return fmt.Errorf("encrypting snmp credentials: %w", err)
	}
	credsJSON, err := json.Marshal(credsCopy)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	_, err = r.db.Exec(
		`INSERT INTO snmp_profiles (id, name, description, credentials_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		profile.ID.String(),
		profile.Name,
		profile.Description,
		string(credsJSON),
		now,
		now,
	)
	return err
}

// GetByID returns a profile by its UUID.
func (r *SNMPProfileRepo) GetByID(id uuid.UUID) (*domain.SNMPProfile, error) {
	row := r.db.QueryRow(
		`SELECT id, name, description, credentials_json, created_at, updated_at
		 FROM snmp_profiles WHERE id = ?`,
		id.String(),
	)
	return scanProfile(row, r.keyring)
}

// GetAll returns all profiles ordered by name.
func (r *SNMPProfileRepo) GetAll() ([]domain.SNMPProfile, error) {
	rows, err := r.db.Query(
		`SELECT id, name, description, credentials_json, created_at, updated_at
		 FROM snmp_profiles ORDER BY name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []domain.SNMPProfile
	for rows.Next() {
		p, err := scanProfileRow(rows, r.keyring)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
	}
	return profiles, rows.Err()
}

// Update overwrites an existing profile.
func (r *SNMPProfileRepo) Update(profile *domain.SNMPProfile) error {
	profile.UpdatedAt = time.Now().UTC()

	// Deep copy credentials for encryption (don't modify the original)
	credsCopy := deepCopySNMPCredentials(profile.Credentials)
	if err := encryptSNMPCredentials(&credsCopy, r.keyring); err != nil {
		return fmt.Errorf("encrypting snmp credentials: %w", err)
	}
	credsJSON, err := json.Marshal(credsCopy)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	res, err := r.db.Exec(
		`UPDATE snmp_profiles SET name=?, description=?, credentials_json=?, updated_at=? WHERE id=?`,
		profile.Name,
		profile.Description,
		string(credsJSON),
		profile.UpdatedAt,
		profile.ID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("snmp profile %s not found", profile.ID)
	}
	return nil
}

// Delete removes a profile by its UUID.
func (r *SNMPProfileRepo) Delete(id uuid.UUID) error {
	res, err := r.db.Exec(`DELETE FROM snmp_profiles WHERE id = ?`, id.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("snmp profile %s not found", id)
	}
	return nil
}

// --- helpers ---

type profileScanner interface {
	Scan(dest ...interface{}) error
}

func scanProfile(row *sql.Row, keyring *crypto.Keyring) (*domain.SNMPProfile, error) {
	var idStr, credsJSON string
	var p domain.SNMPProfile
	if err := row.Scan(&idStr, &p.Name, &p.Description, &credsJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("snmp profile not found")
		}
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid profile id: %w", err)
	}
	p.ID = id
	if err := json.Unmarshal([]byte(credsJSON), &p.Credentials); err != nil {
		return nil, fmt.Errorf("unmarshal credentials: %w", err)
	}
	if err := decryptSNMPCredentials(&p.Credentials, keyring); err != nil {
		return nil, fmt.Errorf("decrypting snmp profile credentials %s: %w", p.ID, err)
	}
	return &p, nil
}

func scanProfileRow(rows *sql.Rows, keyring *crypto.Keyring) (*domain.SNMPProfile, error) {
	var idStr, credsJSON string
	var p domain.SNMPProfile
	if err := rows.Scan(&idStr, &p.Name, &p.Description, &credsJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid profile id: %w", err)
	}
	p.ID = id
	if err := json.Unmarshal([]byte(credsJSON), &p.Credentials); err != nil {
		return nil, fmt.Errorf("unmarshal credentials: %w", err)
	}
	if err := decryptSNMPCredentials(&p.Credentials, keyring); err != nil {
		return nil, fmt.Errorf("decrypting snmp profile credentials %s: %w", p.ID, err)
	}
	return &p, nil
}
