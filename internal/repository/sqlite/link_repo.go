package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// LinkRepo implements domain.LinkRepository using SQLite.
type LinkRepo struct {
	db       *DB
	onChange chan<- struct{}
}

// NewLinkRepo creates a new SQLite-backed link repository.
// The onChange channel, if non-nil, receives a non-blocking signal after
// every successful Create, Update, Delete, or Upsert operation.
func NewLinkRepo(db *sql.DB, onChange chan<- struct{}) *LinkRepo {
	return &LinkRepo{db: wrapDB(db), onChange: onChange}
}

// notify sends a non-blocking signal on the onChange channel to indicate
// that the underlying data has been modified.
func (r *LinkRepo) notify() {
	if r.onChange == nil {
		return
	}
	select {
	case r.onChange <- struct{}{}:
	default:
	}
}

// Create inserts a new link into the database.
func (r *LinkRepo) Create(link *domain.Link) error {
	return withSQLiteBusyRetry(func() error {
		return r.createOnce(link)
	})
}

func (r *LinkRepo) createOnce(link *domain.Link) error {
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
	r.notify()
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
	return withSQLiteBusyRetry(func() error {
		return r.updateOnce(link)
	})
}

func (r *LinkRepo) updateOnce(link *domain.Link) error {
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
	r.notify()
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
	return withSQLiteBusyRetry(func() error {
		return r.deleteOnce(id)
	})
}

func (r *LinkRepo) deleteOnce(id uuid.UUID) error {
	result, err := r.db.Exec(`DELETE FROM links WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting link: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("link not found: %s", id)
	}
	r.notify()
	return nil
}

// Upsert inserts a new link or merges with an existing one for the same physical
// interface pair. Unlike the old device-pair canonicalization, distinct parallel
// uplinks between the same two devices must remain separate rows.
//
// Same-direction matches require the same device IDs plus the same target interface
// and either the same source interface or an empty stored/incoming source interface
// so incomplete discoveries can be enriched in place. Reverse-direction matches
// require the mirrored device IDs plus the mirrored interface names for the same
// physical cable.
//
// All operations run inside a single transaction to prevent duplicate rows under
// concurrent SNMP discovery.
func (r *LinkRepo) Upsert(link *domain.Link) (bool, error) {
	var created bool
	err := withSQLiteBusyRetry(func() error {
		var innerErr error
		created, innerErr = r.upsertOnce(link)
		return innerErr
	})
	return created, err
}

func (r *LinkRepo) upsertOnce(link *domain.Link) (bool, error) {
	now := time.Now().UTC()
	link.UpdatedAt = now
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}

	tx, err := r.db.Begin()
	if err != nil {
		return false, fmt.Errorf("beginning upsert transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Check for an existing link in the same direction (A→B) for the same physical
	// interface pair. Empty source interface names are treated as incomplete data
	// that can match and be enriched in place.
	var existingID string
	var existingSrcIf, existingTgtIf string
	err = tx.QueryRow(
		`SELECT id, source_if_name, target_if_name FROM links
		 WHERE source_device_id = ? AND target_device_id = ?
		   AND target_if_name = ?
		   AND (source_if_name = ? OR source_if_name = '' OR ? = '')`,
		link.SourceDeviceID.String(), link.TargetDeviceID.String(),
		link.TargetIfName, link.SourceIfName, link.SourceIfName,
	).Scan(&existingID, &existingSrcIf, &existingTgtIf)

	if err == nil {
		// Same-direction match: refresh protocol/timestamp and fill any empty port names.
		link.ID = uuid.MustParse(existingID)
		newSrcIf := existingSrcIf
		if newSrcIf == "" && link.SourceIfName != "" {
			newSrcIf = link.SourceIfName
		}
		newTgtIf := existingTgtIf
		if newTgtIf == "" && link.TargetIfName != "" {
			newTgtIf = link.TargetIfName
		}
		if _, err = tx.Exec(
			`UPDATE links SET source_if_name = ?, target_if_name = ?,
			 discovery_protocol = ?, updated_at = ? WHERE id = ?`,
			newSrcIf, newTgtIf,
			string(link.DiscoveryProtocol), link.UpdatedAt, existingID,
		); err != nil {
			return false, fmt.Errorf("updating link: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return false, fmt.Errorf("committing link update: %w", err)
		}
		r.notify()
		return false, nil
	}
	if err != sql.ErrNoRows {
		return false, fmt.Errorf("checking existing link: %w", err)
	}

	// Check for a reverse-direction record (B→A) for the same physical cable.
	// Discovery can report the same port with different labels (if_name vs if_descr)
	// or leave one side blank, so reverse matching happens in Go using normalized
	// interface compatibility rather than exact SQL equality.
	reverse, err := findBestReverseLinkMatch(tx, link)
	if err != nil {
		return false, fmt.Errorf("checking reverse link: %w", err)
	}
	if reverse != nil {
		if shouldReorientReverseLink(reverse, link) {
			if _, err = tx.Exec(
				`UPDATE links SET source_device_id = ?, source_if_name = ?,
				 target_device_id = ?, target_if_name = ?,
				 discovery_protocol = ?, updated_at = ? WHERE id = ?`,
				link.SourceDeviceID.String(), link.SourceIfName,
				link.TargetDeviceID.String(), link.TargetIfName,
				string(link.DiscoveryProtocol), link.UpdatedAt, reverse.ID,
			); err != nil {
				return false, fmt.Errorf("reorienting reverse link: %w", err)
			}
			if err = tx.Commit(); err != nil {
				return false, fmt.Errorf("committing reverse link reorientation: %w", err)
			}
			r.notify()
			return false, nil
		}

		// Reverse-direction match found (B→A record exists). Enrich it:
		// The incoming link knows its own local port (link.SourceIfName) and the
		// remote's port (link.TargetIfName). The existing record's SourceIfName
		// is the remote's local port — fill it from link.TargetIfName if empty.
		// The existing TargetIfName is the local port from A's perspective —
		// fill it from link.SourceIfName if empty.
		link.ID = uuid.MustParse(reverse.ID)
		newSrcIf := reverse.SourceIfName
		if newSrcIf == "" && link.TargetIfName != "" {
			newSrcIf = link.TargetIfName
		}
		newTgtIf := reverse.TargetIfName
		if newTgtIf == "" && link.SourceIfName != "" {
			newTgtIf = link.SourceIfName
		}
		if _, err = tx.Exec(
			`UPDATE links SET source_if_name = ?, target_if_name = ?,
			 discovery_protocol = ?, updated_at = ? WHERE id = ?`,
			newSrcIf, newTgtIf,
			string(link.DiscoveryProtocol), link.UpdatedAt, reverse.ID,
		); err != nil {
			return false, fmt.Errorf("enriching reverse link: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return false, fmt.Errorf("committing reverse link update: %w", err)
		}
		r.notify()
		return false, nil
	}

	// No existing record in either direction — insert new.
	link.CreatedAt = now
	if _, err = tx.Exec(
		`INSERT INTO links (id, source_device_id, source_if_name,
			target_device_id, target_if_name, discovery_protocol,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		link.ID.String(), link.SourceDeviceID.String(), link.SourceIfName,
		link.TargetDeviceID.String(), link.TargetIfName,
		string(link.DiscoveryProtocol), link.CreatedAt, link.UpdatedAt,
	); err != nil {
		return false, fmt.Errorf("inserting link: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("committing link insert: %w", err)
	}
	r.notify()
	return true, nil
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

type reverseLinkMatch struct {
	ID           string
	SourceIfName string
	TargetIfName string
	score        int
}

func findBestReverseLinkMatch(tx *Tx, link *domain.Link) (*reverseLinkMatch, error) {
	rows, err := tx.Query(
		`SELECT id, source_if_name, target_if_name FROM links
		 WHERE source_device_id = ? AND target_device_id = ?
		 ORDER BY created_at`,
		link.TargetDeviceID.String(), link.SourceDeviceID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var best *reverseLinkMatch
	ambiguous := false

	for rows.Next() {
		var candidate reverseLinkMatch
		if err := rows.Scan(&candidate.ID, &candidate.SourceIfName, &candidate.TargetIfName); err != nil {
			return nil, fmt.Errorf("scanning reverse link candidate: %w", err)
		}

		srcScore, srcMatch := compatibleInterfaceScore(candidate.SourceIfName, link.TargetIfName)
		if !srcMatch {
			continue
		}
		tgtScore, tgtMatch := compatibleInterfaceScore(candidate.TargetIfName, link.SourceIfName)
		if !tgtMatch {
			continue
		}

		candidate.score = srcScore + tgtScore
		if best == nil || candidate.score > best.score {
			candidateCopy := candidate
			best = &candidateCopy
			ambiguous = false
			continue
		}
		if candidate.score == best.score {
			ambiguous = true
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating reverse link candidates: %w", err)
	}
	if ambiguous {
		return nil, nil
	}
	return best, nil
}

func compatibleInterfaceScore(existing, incoming string) (int, bool) {
	if existing == "" || incoming == "" {
		return 0, true
	}

	normalizedExisting := normalizeInterfaceName(existing)
	normalizedIncoming := normalizeInterfaceName(incoming)
	if normalizedExisting == normalizedIncoming {
		return 3, true
	}

	existingAnchor := physicalInterfaceAnchor(existing)
	incomingAnchor := physicalInterfaceAnchor(incoming)
	if existingAnchor != "" && existingAnchor == incomingAnchor {
		return 2, true
	}

	return 0, false
}

func shouldReorientReverseLink(reverse *reverseLinkMatch, link *domain.Link) bool {
	return reverse.SourceIfName == "" && link.SourceIfName != "" && link.TargetIfName == ""
}

func normalizeInterfaceName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func physicalInterfaceAnchor(name string) string {
	normalized := normalizeInterfaceName(name)
	if normalized == "" {
		return ""
	}

	virtualHints := []string{
		"vlan", "vrf", "vpn", "bridge", "br-", "bond", "loopback", "lo",
		"gre", "eoip", "wg", "wireguard", "pppoe", "ppp", "sstp", "ovpn",
		"l2tp", "vxlan", "veth", "tap", "tun",
	}
	for _, hint := range virtualHints {
		if strings.Contains(normalized, hint) {
			return ""
		}
	}

	physicalPatterns := []string{
		"ether", "eth", "sfp-sfpplus", "sfp", "qsfp", "ens", "eno", "enp",
		"gigabitethernet", "tengigabitethernet", "fastethernet", "ge-", "xe-", "et-",
	}
	for _, pattern := range physicalPatterns {
		if idx := strings.Index(normalized, pattern); idx >= 0 {
			anchor := normalized[idx:]
			for i, r := range anchor {
				if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '/') {
					anchor = anchor[:i]
					break
				}
			}
			anchor = strings.Trim(anchor, "- /")
			if hasDigit(anchor) {
				return anchor
			}
		}
	}

	shortPrefixes := []string{"gi", "te", "fo", "port"}
	for _, prefix := range shortPrefixes {
		if strings.HasPrefix(normalized, prefix) && hasDigit(normalized) {
			return normalized
		}
	}

	return ""
}

func hasDigit(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}
