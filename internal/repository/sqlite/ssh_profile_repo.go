package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// SSHProfileRepo implements domain.SSHProfileRepository using SQLite.
type SSHProfileRepo struct {
	db *sql.DB
}

// NewSSHProfileRepo creates a new SSHProfileRepo.
func NewSSHProfileRepo(db *sql.DB) *SSHProfileRepo {
	return &SSHProfileRepo{db: db}
}

// Create inserts a new SSH profile.
func (r *SSHProfileRepo) Create(profile *domain.SSHProfile) error {
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now

	_, err := r.db.Exec(
		`INSERT INTO ssh_profiles (id, name, description, username, port, auth_method, encrypted_secret, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		profile.ID.String(),
		profile.Name,
		profile.Description,
		profile.Username,
		profile.Port,
		string(profile.AuthMethod),
		profile.EncryptedSecret,
		now,
		now,
	)
	return err
}

// GetByID returns an SSH profile by its UUID.
func (r *SSHProfileRepo) GetByID(id uuid.UUID) (*domain.SSHProfile, error) {
	row := r.db.QueryRow(
		`SELECT id, name, description, username, port, auth_method, encrypted_secret, created_at, updated_at
		 FROM ssh_profiles WHERE id = ?`,
		id.String(),
	)
	return scanSSHProfile(row)
}

// GetAll returns all SSH profiles ordered by name.
func (r *SSHProfileRepo) GetAll() ([]domain.SSHProfile, error) {
	rows, err := r.db.Query(
		`SELECT id, name, description, username, port, auth_method, encrypted_secret, created_at, updated_at
		 FROM ssh_profiles ORDER BY name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []domain.SSHProfile
	for rows.Next() {
		p, err := scanSSHProfileRow(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
	}
	return profiles, rows.Err()
}

// Update overwrites an existing SSH profile.
func (r *SSHProfileRepo) Update(profile *domain.SSHProfile) error {
	profile.UpdatedAt = time.Now().UTC()

	res, err := r.db.Exec(
		`UPDATE ssh_profiles SET name=?, description=?, username=?, port=?, auth_method=?, encrypted_secret=?, updated_at=? WHERE id=?`,
		profile.Name,
		profile.Description,
		profile.Username,
		profile.Port,
		string(profile.AuthMethod),
		profile.EncryptedSecret,
		profile.UpdatedAt,
		profile.ID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ssh profile %s not found", profile.ID)
	}
	return nil
}

// Delete removes an SSH profile by its UUID.
func (r *SSHProfileRepo) Delete(id uuid.UUID) error {
	res, err := r.db.Exec(`DELETE FROM ssh_profiles WHERE id = ?`, id.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ssh profile %s not found", id)
	}
	return nil
}

// IsInUse checks whether any device references this SSH profile.
func (r *SSHProfileRepo) IsInUse(id uuid.UUID) (bool, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE ssh_profile_id = ?`, id.String()).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// --- helpers ---

func scanSSHProfile(row *sql.Row) (*domain.SSHProfile, error) {
	var idStr, authMethod string
	var p domain.SSHProfile
	if err := row.Scan(&idStr, &p.Name, &p.Description, &p.Username, &p.Port, &authMethod, &p.EncryptedSecret, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("ssh profile not found")
		}
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ssh profile id: %w", err)
	}
	p.ID = id
	p.AuthMethod = domain.SSHAuthMethod(authMethod)
	return &p, nil
}

func scanSSHProfileRow(rows *sql.Rows) (*domain.SSHProfile, error) {
	var idStr, authMethod string
	var p domain.SSHProfile
	if err := rows.Scan(&idStr, &p.Name, &p.Description, &p.Username, &p.Port, &authMethod, &p.EncryptedSecret, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ssh profile id: %w", err)
	}
	p.ID = id
	p.AuthMethod = domain.SSHAuthMethod(authMethod)
	return &p, nil
}
