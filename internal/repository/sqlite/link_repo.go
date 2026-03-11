package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/google/uuid"
)

// LinkRepo implements domain.LinkRepository using SQLite.
type LinkRepo struct {
	db *sql.DB
}

// NewLinkRepo creates a new SQLite-backed link repository.
func NewLinkRepo(db *sql.DB) *LinkRepo {
	return &LinkRepo{db: db}
}

// Create inserts a new link into the database.
func (r *LinkRepo) Create(link *domain.Link) error {
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}

	_, err := r.db.Exec(
		`INSERT INTO links (id, source_device_id, source_if_name,
			target_device_id, target_if_name, discovery_protocol,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		link.ID.String(), link.SourceDeviceID.String(), link.SourceIfName,
		link.TargetDeviceID.String(), link.TargetIfName,
		string(link.DiscoveryProtocol), link.CreatedAt, link.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting link: %w", err)
	}
	return nil
}

// GetByID retrieves a single link by UUID.
func (r *LinkRepo) GetByID(id uuid.UUID) (*domain.Link, error) {
	row := r.db.QueryRow(
		`SELECT id, source_device_id, source_if_name,
			target_device_id, target_if_name, discovery_protocol,
			created_at, updated_at
		FROM links WHERE id = ?`, id.String(),
	)

	var link domain.Link
	var idStr, srcDeviceID, tgtDeviceID, protocol string

	err := row.Scan(
		&idStr, &srcDeviceID, &link.SourceIfName,
		&tgtDeviceID, &link.TargetIfName, &protocol,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("link not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying link by id: %w", err)
	}

	link.ID = uuid.MustParse(idStr)
	link.SourceDeviceID = uuid.MustParse(srcDeviceID)
	link.TargetDeviceID = uuid.MustParse(tgtDeviceID)
	link.DiscoveryProtocol = domain.DiscoveryProtocol(protocol)

	return &link, nil
}

// Update modifies the interface names of an existing link.
func (r *LinkRepo) Update(link *domain.Link) error {
	link.UpdatedAt = time.Now().UTC()

	result, err := r.db.Exec(
		`UPDATE links SET source_if_name = ?, target_if_name = ?, updated_at = ? WHERE id = ?`,
		link.SourceIfName, link.TargetIfName, link.UpdatedAt, link.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("updating link: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("link not found: %s", link.ID)
	}
	return nil
}

// GetByDeviceID retrieves all links where the device is either source or target.
func (r *LinkRepo) GetByDeviceID(deviceID uuid.UUID) ([]domain.Link, error) {
	rows, err := r.db.Query(
		`SELECT id, source_device_id, source_if_name,
			target_device_id, target_if_name, discovery_protocol,
			created_at, updated_at
		FROM links
		WHERE source_device_id = ? OR target_device_id = ?
		ORDER BY created_at`,
		deviceID.String(), deviceID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying links by device: %w", err)
	}
	defer rows.Close()

	return r.scanLinks(rows)
}

// GetAll retrieves all links.
func (r *LinkRepo) GetAll() ([]domain.Link, error) {
	rows, err := r.db.Query(
		`SELECT id, source_device_id, source_if_name,
			target_device_id, target_if_name, discovery_protocol,
			created_at, updated_at
		FROM links ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying all links: %w", err)
	}
	defer rows.Close()

	return r.scanLinks(rows)
}

// Delete removes a link by UUID.
func (r *LinkRepo) Delete(id uuid.UUID) error {
	result, err := r.db.Exec(`DELETE FROM links WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting link: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("link not found: %s", id)
	}
	return nil
}

// Upsert inserts a new link or updates an existing one matching
// the same (source_device_id, source_if_name, target_device_id, target_if_name).
func (r *LinkRepo) Upsert(link *domain.Link) error {
	now := time.Now().UTC()
	link.UpdatedAt = now
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}

	// Check if a matching link already exists (either direction)
	var existingID string
	err := r.db.QueryRow(
		`SELECT id FROM links
		WHERE (source_device_id = ? AND source_if_name = ? AND target_device_id = ? AND target_if_name = ?)
		   OR (source_device_id = ? AND source_if_name = ? AND target_device_id = ? AND target_if_name = ?)`,
		link.SourceDeviceID.String(), link.SourceIfName,
		link.TargetDeviceID.String(), link.TargetIfName,
		link.TargetDeviceID.String(), link.TargetIfName,
		link.SourceDeviceID.String(), link.SourceIfName,
	).Scan(&existingID)

	if err == sql.ErrNoRows {
		// Insert new
		link.CreatedAt = now
		_, err = r.db.Exec(
			`INSERT INTO links (id, source_device_id, source_if_name,
				target_device_id, target_if_name, discovery_protocol,
				created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			link.ID.String(), link.SourceDeviceID.String(), link.SourceIfName,
			link.TargetDeviceID.String(), link.TargetIfName,
			string(link.DiscoveryProtocol), link.CreatedAt, link.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("inserting link: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking existing link: %w", err)
	}

	// Update existing
	link.ID = uuid.MustParse(existingID)
	_, err = r.db.Exec(
		`UPDATE links SET discovery_protocol = ?, updated_at = ? WHERE id = ?`,
		string(link.DiscoveryProtocol), link.UpdatedAt, existingID,
	)
	if err != nil {
		return fmt.Errorf("updating link: %w", err)
	}
	return nil
}

// scanLinks scans multiple link rows.
func (r *LinkRepo) scanLinks(rows *sql.Rows) ([]domain.Link, error) {
	var links []domain.Link
	for rows.Next() {
		var link domain.Link
		var idStr, srcDeviceID, tgtDeviceID, protocol string

		err := rows.Scan(
			&idStr, &srcDeviceID, &link.SourceIfName,
			&tgtDeviceID, &link.TargetIfName, &protocol,
			&link.CreatedAt, &link.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}

		link.ID = uuid.MustParse(idStr)
		link.SourceDeviceID = uuid.MustParse(srcDeviceID)
		link.TargetDeviceID = uuid.MustParse(tgtDeviceID)
		link.DiscoveryProtocol = domain.DiscoveryProtocol(protocol)
		links = append(links, link)
	}

	return links, rows.Err()
}
