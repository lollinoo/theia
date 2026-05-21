package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// DeviceCredentialProfileRow is the join-table row returned by ListAssignedProfiles.
type DeviceCredentialProfileRow struct {
	ProfileID  uuid.UUID
	Name       string
	Username   string
	Port       int
	AuthMethod domain.SSHAuthMethod
	Role       string
	IsWinbox   bool
	CreatedAt  time.Time
}

// WinboxAssignmentRow holds the minimal data needed to launch WinBox for a device.
type WinboxAssignmentRow struct {
	ProfileID       uuid.UUID
	Username        string
	EncryptedSecret string
}

// CredentialProfileRepo implements domain.CredentialProfileRepository using PostgreSQL.
type CredentialProfileRepo struct {
	db *DB
}

// NewCredentialProfileRepo creates a new CredentialProfileRepo.
func NewCredentialProfileRepo(db *sql.DB) *CredentialProfileRepo {
	return &CredentialProfileRepo{db: wrapDB(db)}
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
// Checks the device_credential_profiles join table (D-14).
func (r *CredentialProfileRepo) IsInUse(id uuid.UUID) (bool, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM device_credential_profiles WHERE profile_id = ?`, id.String()).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListAssignedProfiles returns all credential profiles assigned to the given device,
// ordered by profile name.
func (r *CredentialProfileRepo) ListAssignedProfiles(deviceID uuid.UUID) ([]DeviceCredentialProfileRow, error) {
	rows, err := r.db.Query(
		`SELECT cp.id, cp.name, cp.username, cp.port, cp.auth_method, cp.role, dcp.is_winbox, dcp.created_at
		 FROM device_credential_profiles dcp
		 JOIN credential_profiles cp ON cp.id = dcp.profile_id
		 WHERE dcp.device_id = ?
		 ORDER BY cp.name ASC`,
		deviceID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DeviceCredentialProfileRow
	for rows.Next() {
		var row DeviceCredentialProfileRow
		var idStr, authMethod string
		if err := rows.Scan(&idStr, &row.Name, &row.Username, &row.Port, &authMethod, &row.Role, &row.IsWinbox, &row.CreatedAt); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid profile id in join row: %w", err)
		}
		row.ProfileID = id
		row.AuthMethod = domain.SSHAuthMethod(authMethod)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []DeviceCredentialProfileRow{}
	}
	return result, nil
}

// AssignProfile inserts a row into device_credential_profiles linking a device to a profile.
// Returns an error if the pair already exists (UNIQUE constraint).
func (r *CredentialProfileRepo) AssignProfile(deviceID, profileID uuid.UUID) error {
	_, err := r.db.Exec(
		`INSERT INTO device_credential_profiles (device_id, profile_id, is_winbox, created_at) VALUES (?, ?, ?, ?)`,
		deviceID.String(), profileID.String(), false, time.Now().UTC(),
	)
	return err
}

// UnassignProfile removes a device-profile assignment.
// Returns an error if the profile was not assigned to the device.
func (r *CredentialProfileRepo) UnassignProfile(deviceID, profileID uuid.UUID) error {
	res, err := r.db.Exec(
		`DELETE FROM device_credential_profiles WHERE device_id = ? AND profile_id = ?`,
		deviceID.String(), profileID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("profile not assigned to device")
	}
	return nil
}

// SetWinboxProfile designates one profile as the WinBox profile for a device (D-04, D-08).
// Clears is_winbox on all other assignments first, then sets the target to 1.
// Returns an error if the profile is not assigned to the device.
func (r *CredentialProfileRepo) SetWinboxProfile(deviceID, profileID uuid.UUID) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Clear all winbox flags for this device
	if _, err := tx.Exec(
		`UPDATE device_credential_profiles SET is_winbox = ? WHERE device_id = ?`,
		false, deviceID.String(),
	); err != nil {
		return err
	}

	// Set the target profile as winbox
	res, err := tx.Exec(
		`UPDATE device_credential_profiles SET is_winbox = ? WHERE device_id = ? AND profile_id = ?`,
		true, deviceID.String(), profileID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("profile not assigned to device")
	}

	return tx.Commit()
}

// ClearWinboxProfile sets is_winbox=0 for all profiles assigned to the device (D-09).
// Idempotent — no error if no winbox profile is currently set.
func (r *CredentialProfileRepo) ClearWinboxProfile(deviceID uuid.UUID) error {
	_, err := r.db.Exec(
		`UPDATE device_credential_profiles SET is_winbox = ? WHERE device_id = ?`,
		false, deviceID.String(),
	)
	return err
}

// GetWinboxAssignment returns the profile designated as the WinBox profile for a device.
// Returns an error containing "no WinBox profile designated" if none is set.
func (r *CredentialProfileRepo) GetWinboxAssignment(deviceID uuid.UUID) (*WinboxAssignmentRow, error) {
	var row WinboxAssignmentRow
	var idStr string
	err := r.db.QueryRow(
		`SELECT cp.id, cp.username, cp.encrypted_secret
		 FROM device_credential_profiles dcp
		 JOIN credential_profiles cp ON cp.id = dcp.profile_id
		 WHERE dcp.device_id = ? AND dcp.is_winbox = ?`,
		deviceID.String(), true,
	).Scan(&idStr, &row.Username, &row.EncryptedSecret)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no WinBox profile designated")
		}
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid profile id in winbox row: %w", err)
	}
	row.ProfileID = id
	return &row, nil
}

// GetBackupProfileForDevice returns the first non-WinBox credential profile
// assigned to the given device. If no non-WinBox profile exists, it falls back
// to any assigned profile. Returns an error if no profile is assigned at all.
func (r *CredentialProfileRepo) GetBackupProfileForDevice(deviceID uuid.UUID) (*domain.CredentialProfile, error) {
	var p domain.CredentialProfile
	var idStr, encSecret string
	err := r.db.QueryRow(
		`SELECT cp.id, cp.name, cp.description, cp.username, cp.port,
		        cp.auth_method, cp.role, cp.encrypted_secret, cp.created_at, cp.updated_at
		 FROM device_credential_profiles dcp
		 JOIN credential_profiles cp ON cp.id = dcp.profile_id
		 WHERE dcp.device_id = ?
		 ORDER BY dcp.is_winbox ASC, cp.name ASC
		 LIMIT 1`,
		deviceID.String(),
	).Scan(&idStr, &p.Name, &p.Description, &p.Username, &p.Port,
		&p.AuthMethod, &p.Role, &encSecret, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no credential profile assigned to device %s", deviceID)
		}
		return nil, err
	}
	p.ID = uuid.MustParse(idStr)
	p.EncryptedSecret = encSecret
	return &p, nil
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
