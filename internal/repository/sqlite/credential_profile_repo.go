package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// CredentialProfileRepo implements domain.CredentialProfileRepository using SQLite.
type CredentialProfileRepo struct {
	db *sql.DB
}

// NewCredentialProfileRepo creates a new CredentialProfileRepo.
func NewCredentialProfileRepo(db *sql.DB) *CredentialProfileRepo {
	return &CredentialProfileRepo{db: db}
}

// Create inserts a new credential profile.
func (r *CredentialProfileRepo) Create(profile *domain.CredentialProfile) error {
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now

	_, err := r.db.Exec(
		`INSERT INTO credential_profiles (id, name, description, username, port, auth_method, encrypted_secret, role, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		profile.ID.String(),
		profile.Name,
		profile.Description,
		profile.Username,
		profile.Port,
		string(profile.AuthMethod),
		profile.EncryptedSecret,
		profile.Role,
		now,
		now,
	)
	return err
}

// GetByID returns a credential profile by its UUID.
func (r *CredentialProfileRepo) GetByID(id uuid.UUID) (*domain.CredentialProfile, error) {
	row := r.db.QueryRow(
		`SELECT id, name, description, username, port, auth_method, encrypted_secret, role, created_at, updated_at
		 FROM credential_profiles WHERE id = ?`,
		id.String(),
	)
	return scanCredentialProfile(row)
}

// GetAll returns all credential profiles ordered by name.
func (r *CredentialProfileRepo) GetAll() ([]domain.CredentialProfile, error) {
	rows, err := r.db.Query(
		`SELECT id, name, description, username, port, auth_method, encrypted_secret, role, created_at, updated_at
		 FROM credential_profiles ORDER BY name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []domain.CredentialProfile
	for rows.Next() {
		p, err := scanCredentialProfileRow(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
	}
	return profiles, rows.Err()
}

// Update overwrites an existing credential profile.
func (r *CredentialProfileRepo) Update(profile *domain.CredentialProfile) error {
	profile.UpdatedAt = time.Now().UTC()

	res, err := r.db.Exec(
		`UPDATE credential_profiles SET name=?, description=?, username=?, port=?, auth_method=?, encrypted_secret=?, role=?, updated_at=? WHERE id=?`,
		profile.Name,
		profile.Description,
		profile.Username,
		profile.Port,
		string(profile.AuthMethod),
		profile.EncryptedSecret,
		profile.Role,
		profile.UpdatedAt,
		profile.ID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("credential profile %s not found", profile.ID)
	}
	return nil
}

// Delete removes a credential profile by its UUID.
func (r *CredentialProfileRepo) Delete(id uuid.UUID) error {
	res, err := r.db.Exec(`DELETE FROM credential_profiles WHERE id = ?`, id.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("credential profile %s not found", id)
	}
	return nil
}

// IsInUse checks whether any device references this credential profile.
// Uses the legacy ssh_profile_id FK column (preserved per D-06 until Phase 27).
func (r *CredentialProfileRepo) IsInUse(id uuid.UUID) (bool, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE ssh_profile_id = ?`, id.String()).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// --- helpers ---

func scanCredentialProfile(row *sql.Row) (*domain.CredentialProfile, error) {
	var idStr, authMethod string
	var p domain.CredentialProfile
	if err := row.Scan(&idStr, &p.Name, &p.Description, &p.Username, &p.Port, &authMethod, &p.EncryptedSecret, &p.Role, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("credential profile not found")
		}
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid credential profile id: %w", err)
	}
	p.ID = id
	p.AuthMethod = domain.SSHAuthMethod(authMethod)
	return &p, nil
}

func scanCredentialProfileRow(rows *sql.Rows) (*domain.CredentialProfile, error) {
	var idStr, authMethod string
	var p domain.CredentialProfile
	if err := rows.Scan(&idStr, &p.Name, &p.Description, &p.Username, &p.Port, &authMethod, &p.EncryptedSecret, &p.Role, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid credential profile id: %w", err)
	}
	p.ID = id
	p.AuthMethod = domain.SSHAuthMethod(authMethod)
	return &p, nil
}
