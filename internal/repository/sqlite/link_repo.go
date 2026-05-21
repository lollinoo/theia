package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
)

// LinkRepo implements domain.LinkRepository using SQLite.
type LinkRepo struct {
	db             *DB
	onChange       chan<- struct{}
	manualCreateMu sync.Mutex
	subscribersMu  sync.RWMutex
	subscribers    map[chan domain.LinkChangeEvent]struct{}
	repairPending  atomic.Bool
}

// NewLinkRepo creates a new SQLite-backed link repository.
// The onChange channel, if non-nil, receives a non-blocking signal after
// every successful Create, Update, Delete, or Upsert operation.
func NewLinkRepo(db *sql.DB, onChange chan<- struct{}) *LinkRepo {
	return &LinkRepo{
		db:          wrapDB(db),
		onChange:    onChange,
		subscribers: make(map[chan domain.LinkChangeEvent]struct{}),
	}
}

func (r *LinkRepo) LinkChanges() <-chan domain.LinkChangeEvent {
	return r.SubscribeLinkChanges(256)
}

func (r *LinkRepo) SubscribeLinkChanges(buffer int) <-chan domain.LinkChangeEvent {
	if buffer <= 0 {
		buffer = 256
	}

	ch := make(chan domain.LinkChangeEvent, buffer)
	r.subscribersMu.Lock()
	r.subscribers[ch] = struct{}{}
	r.subscribersMu.Unlock()
	return ch
}

func (r *LinkRepo) DrainLinkRepair() bool {
	return r.repairPending.Swap(false)
}

// notify sends a non-blocking signal on the onChange channel to indicate
// that the underlying data has been modified.
func (r *LinkRepo) notify() {
	if r.onChange == nil {
		return
	}
	select {
	case r.onChange <- struct{}{}:
		observability.Default().IncCacheInvalidation("link_repo")
	default:
	}
}

func (r *LinkRepo) publishChange(kind domain.ChangeKind, linkID uuid.UUID) {
	event := domain.LinkChangeEvent{
		Kind:   kind,
		LinkID: linkID,
	}

	r.subscribersMu.RLock()
	defer r.subscribersMu.RUnlock()
	for subscriber := range r.subscribers {
		select {
		case subscriber <- event:
		default:
			r.repairPending.Store(true)
		}
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
	r.publishChange(domain.ChangeKindCreated, link.ID)
	return nil
}

// CreateManualIdempotent inserts a manual link or returns the stored equivalent
// link without mutating existing discovery-owned rows.
func (r *LinkRepo) CreateManualIdempotent(link *domain.Link, browserLocalStorageMigration bool) (*domain.Link, bool, error) {
	var stored *domain.Link
	var created bool
	err := withSQLiteBusyRetry(func() error {
		r.manualCreateMu.Lock()
		defer r.manualCreateMu.Unlock()

		var innerErr error
		stored, created, innerErr = r.createManualIdempotentOnce(link, browserLocalStorageMigration)
		return innerErr
	})
	return stored, created, err
}

func (r *LinkRepo) createManualIdempotentOnce(link *domain.Link, browserLocalStorageMigration bool) (*domain.Link, bool, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, false, fmt.Errorf("beginning manual link create transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	existing, err := findManualCreateEquivalentLink(tx, link, browserLocalStorageMigration)
	if err != nil {
		return nil, false, fmt.Errorf("checking equivalent manual link: %w", err)
	}
	if existing != nil {
		membershipChanged, err := r.addLinkToMaterializedBaseEndpointMaps(tx, existing, now)
		if err != nil {
			return nil, false, err
		}
		if membershipChanged {
			if err = tx.Commit(); err != nil {
				return nil, false, fmt.Errorf("committing manual link map membership repair: %w", err)
			}
			r.notify()
			r.publishChange(domain.ChangeKindUpdated, existing.ID)
		}
		return existing, false, nil
	}

	link.CreatedAt = now
	link.UpdatedAt = now
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}

	if _, err = tx.Exec(
		`INSERT INTO links (id, source_device_id, source_if_name,
			target_device_id, target_if_name, discovery_protocol,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		link.ID.String(), link.SourceDeviceID.String(), link.SourceIfName,
		link.TargetDeviceID.String(), link.TargetIfName,
		string(link.DiscoveryProtocol), link.CreatedAt, link.UpdatedAt,
	); err != nil {
		return nil, false, fmt.Errorf("inserting manual link: %w", err)
	}
	if _, err = r.addLinkToMaterializedBaseEndpointMaps(tx, link, now); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("committing manual link insert: %w", err)
	}

	r.notify()
	r.publishChange(domain.ChangeKindCreated, link.ID)
	inserted := *link
	return &inserted, true, nil
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
	r.publishChange(domain.ChangeKindUpdated, link.ID)
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

// GetByIDs retrieves the requested links.
func (r *LinkRepo) GetByIDs(ids []uuid.UUID) ([]domain.Link, error) {
	if len(ids) == 0 {
		return []domain.Link{}, nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id.String())
	}

	rows, err := r.db.Query(
		`SELECT id, source_device_id, source_if_name,
			target_device_id, target_if_name, discovery_protocol,
			created_at, updated_at
		FROM links
		WHERE id IN (`+strings.Join(placeholders, ", ")+`)
		ORDER BY created_at`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying links by ids: %w", err)
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
	r.publishChange(domain.ChangeKindDeleted, id)
	return nil
}

// Upsert inserts a new link or merges with an existing one for the same physical
// interface pair. Unlike the old device-pair canonicalization, distinct parallel
// uplinks between the same two devices must remain separate rows.
//
// Same-direction matches require the same device IDs plus compatible interface
// names, where empty stored/incoming source or target interfaces are treated as
// incomplete data that can be enriched in place. Reverse-direction matches require
// the mirrored device IDs plus the mirrored interface names for the same physical
// cable.
//
// All operations run inside a single transaction to prevent duplicate rows under
// concurrent SNMP discovery.
func (r *LinkRepo) Upsert(link *domain.Link) (bool, error) {
	result, err := r.UpsertDetailed(link)
	return result.Created, err
}

func (r *LinkRepo) UpsertDetailed(link *domain.Link) (domain.LinkUpsertResult, error) {
	var result domain.LinkUpsertResult
	err := withSQLiteBusyRetry(func() error {
		var innerErr error
		result, innerErr = r.upsertOnce(link)
		return innerErr
	})
	return result, err
}

func (r *LinkRepo) upsertOnce(link *domain.Link) (domain.LinkUpsertResult, error) {
	now := time.Now().UTC()
	link.UpdatedAt = now
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}

	tx, err := r.db.Begin()
	if err != nil {
		return domain.LinkUpsertResult{}, fmt.Errorf("beginning upsert transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Check for an existing link in the same direction (A→B) for the same physical
	// interface pair. Empty interface names are treated as incomplete data that can
	// match and be enriched in place, but ambiguous partial observations must not
	// attach to an arbitrary parallel link.
	sameDirection, err := findBestSameDirectionLinkMatch(tx, link)
	if err != nil {
		return domain.LinkUpsertResult{}, fmt.Errorf("checking same-direction link: %w", err)
	}
	if sameDirection != nil {
		// Same-direction match: fill any empty port names and update protocol only
		// when the stored row materially changes. Identical rediscovery is a no-op.
		link.ID = uuid.MustParse(sameDirection.ID)
		newSrcIf := sameDirection.SourceIfName
		if newSrcIf == "" && link.SourceIfName != "" {
			newSrcIf = link.SourceIfName
		}
		newTgtIf := sameDirection.TargetIfName
		if newTgtIf == "" && link.TargetIfName != "" {
			newTgtIf = link.TargetIfName
		}
		link.SourceIfName = newSrcIf
		link.TargetIfName = newTgtIf

		portsChanged := newSrcIf != sameDirection.SourceIfName || newTgtIf != sameDirection.TargetIfName
		protocolChanged := sameDirection.Protocol != string(link.DiscoveryProtocol)
		if !portsChanged && !protocolChanged {
			membershipChanged, err := r.addLinkToMaterializedBaseEndpointMaps(tx, link, now)
			if err != nil {
				return domain.LinkUpsertResult{}, err
			}
			if membershipChanged {
				if err = tx.Commit(); err != nil {
					return domain.LinkUpsertResult{}, fmt.Errorf("committing link map membership repair: %w", err)
				}
				r.notify()
				r.publishChange(domain.ChangeKindUpdated, link.ID)
			}
			result := domain.LinkUpsertResult{
				Created: false,
				Changed: membershipChanged,
				Kind:    domain.LinkUpsertKindNoop,
			}
			r.recordUpsert(result, link.DiscoveryProtocol)
			return result, nil
		}
		if _, err = tx.Exec(
			`UPDATE links SET source_if_name = ?, target_if_name = ?,
			 discovery_protocol = ?, updated_at = ? WHERE id = ?`,
			newSrcIf, newTgtIf,
			string(link.DiscoveryProtocol), link.UpdatedAt, sameDirection.ID,
		); err != nil {
			return domain.LinkUpsertResult{}, fmt.Errorf("updating link: %w", err)
		}
		if _, err = r.addLinkToMaterializedBaseEndpointMaps(tx, link, now); err != nil {
			return domain.LinkUpsertResult{}, err
		}
		if err = tx.Commit(); err != nil {
			return domain.LinkUpsertResult{}, fmt.Errorf("committing link update: %w", err)
		}
		r.notify()
		r.publishChange(domain.ChangeKindUpdated, link.ID)
		kind := domain.LinkUpsertKindUpdated
		if portsChanged {
			kind = domain.LinkUpsertKindEnriched
		}
		result := domain.LinkUpsertResult{Created: false, Changed: true, Kind: kind}
		r.recordUpsert(result, link.DiscoveryProtocol)
		return result, nil
	}

	// Check for a reverse-direction record (B→A) for the same physical cable.
	// Discovery can report the same port with different labels (if_name vs if_descr)
	// or leave one side blank, so reverse matching happens in Go using normalized
	// interface compatibility rather than exact SQL equality.
	reverse, err := findBestReverseLinkMatch(tx, link)
	if err != nil {
		return domain.LinkUpsertResult{}, fmt.Errorf("checking reverse link: %w", err)
	}
	if reverse != nil {
		if shouldReorientReverseLink(reverse, link) {
			link.ID = uuid.MustParse(reverse.ID)
			if _, err = tx.Exec(
				`UPDATE links SET source_device_id = ?, source_if_name = ?,
				 target_device_id = ?, target_if_name = ?,
				 discovery_protocol = ?, updated_at = ? WHERE id = ?`,
				link.SourceDeviceID.String(), link.SourceIfName,
				link.TargetDeviceID.String(), link.TargetIfName,
				string(link.DiscoveryProtocol), link.UpdatedAt, reverse.ID,
			); err != nil {
				return domain.LinkUpsertResult{}, fmt.Errorf("reorienting reverse link: %w", err)
			}
			if _, err = r.addLinkToMaterializedBaseEndpointMaps(tx, link, now); err != nil {
				return domain.LinkUpsertResult{}, err
			}
			if err = tx.Commit(); err != nil {
				return domain.LinkUpsertResult{}, fmt.Errorf("committing reverse link reorientation: %w", err)
			}
			r.notify()
			r.publishChange(domain.ChangeKindUpdated, link.ID)
			result := domain.LinkUpsertResult{Created: false, Changed: true, Kind: domain.LinkUpsertKindReoriented}
			r.recordUpsert(result, link.DiscoveryProtocol)
			return result, nil
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
		portsChanged := newSrcIf != reverse.SourceIfName || newTgtIf != reverse.TargetIfName
		protocolChanged := reverse.Protocol != string(link.DiscoveryProtocol)
		if !portsChanged && !protocolChanged {
			link.ID = uuid.MustParse(reverse.ID)
			membershipChanged, err := r.addLinkToMaterializedBaseEndpointMaps(tx, link, now)
			if err != nil {
				return domain.LinkUpsertResult{}, err
			}
			if membershipChanged {
				if err = tx.Commit(); err != nil {
					return domain.LinkUpsertResult{}, fmt.Errorf("committing reverse link map membership repair: %w", err)
				}
				r.notify()
				r.publishChange(domain.ChangeKindUpdated, link.ID)
			}
			result := domain.LinkUpsertResult{
				Created: false,
				Changed: membershipChanged,
				Kind:    domain.LinkUpsertKindNoop,
			}
			r.recordUpsert(result, link.DiscoveryProtocol)
			return result, nil
		}
		if _, err = tx.Exec(
			`UPDATE links SET source_if_name = ?, target_if_name = ?,
			 discovery_protocol = ?, updated_at = ? WHERE id = ?`,
			newSrcIf, newTgtIf,
			string(link.DiscoveryProtocol), link.UpdatedAt, reverse.ID,
		); err != nil {
			return domain.LinkUpsertResult{}, fmt.Errorf("enriching reverse link: %w", err)
		}
		if _, err = r.addLinkToMaterializedBaseEndpointMaps(tx, link, now); err != nil {
			return domain.LinkUpsertResult{}, err
		}
		if err = tx.Commit(); err != nil {
			return domain.LinkUpsertResult{}, fmt.Errorf("committing reverse link update: %w", err)
		}
		r.notify()
		r.publishChange(domain.ChangeKindUpdated, link.ID)
		kind := domain.LinkUpsertKindUpdated
		if portsChanged {
			kind = domain.LinkUpsertKindEnriched
		}
		result := domain.LinkUpsertResult{Created: false, Changed: true, Kind: kind}
		r.recordUpsert(result, link.DiscoveryProtocol)
		return result, nil
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
		return domain.LinkUpsertResult{}, fmt.Errorf("inserting link: %w", err)
	}
	if _, err = r.addLinkToMaterializedBaseEndpointMaps(tx, link, now); err != nil {
		return domain.LinkUpsertResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.LinkUpsertResult{}, fmt.Errorf("committing link insert: %w", err)
	}
	r.notify()
	r.publishChange(domain.ChangeKindCreated, link.ID)
	result := domain.LinkUpsertResult{Created: true, Changed: true, Kind: domain.LinkUpsertKindCreated}
	r.recordUpsert(result, link.DiscoveryProtocol)
	return result, nil
}

func (r *LinkRepo) addLinkToMaterializedBaseEndpointMaps(tx *Tx, link *domain.Link, now time.Time) (bool, error) {
	result, err := tx.Exec(
		`INSERT INTO canvas_map_links (map_id, link_id, added_at)
		 SELECT source_members.map_id, ?, ?
		 FROM canvas_map_devices source_members
		 JOIN canvas_map_devices target_members
		   ON target_members.map_id = source_members.map_id
		  AND target_members.device_id = ?
		 JOIN canvas_maps cm
		   ON cm.id = source_members.map_id
		  AND cm.membership_materialized = ?
		 WHERE source_members.device_id = ?
		   AND (source_members.role = ? OR target_members.role = ?)
		 ON CONFLICT(map_id, link_id) DO NOTHING`,
		link.ID.String(),
		now,
		link.TargetDeviceID.String(),
		true,
		link.SourceDeviceID.String(),
		string(domain.CanvasMapDeviceRoleBase),
		string(domain.CanvasMapDeviceRoleBase),
	)
	if err != nil {
		return false, fmt.Errorf("adding link to materialized canvas maps: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return false, nil
	}
	if _, err := tx.Exec(
		`UPDATE canvas_maps
		 SET updated_at = ?
		 WHERE id IN (
			SELECT map_id FROM canvas_map_links WHERE link_id = ?
		 )`,
		now,
		link.ID.String(),
	); err != nil {
		return false, fmt.Errorf("touching canvas maps for link membership: %w", err)
	}
	return true, nil
}

func (r *LinkRepo) recordUpsert(result domain.LinkUpsertResult, protocol domain.DiscoveryProtocol) {
	observability.Default().IncLinkUpsert(protocol, result.Kind)
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

type linkRowScanner interface {
	Scan(dest ...any) error
}

func findManualCreateEquivalentLink(tx *Tx, link *domain.Link, browserLocalStorageMigration bool) (*domain.Link, error) {
	if browserLocalStorageMigration {
		return scanLinkRow(tx.QueryRow(
			`SELECT id, source_device_id, source_if_name,
				target_device_id, target_if_name, discovery_protocol,
				created_at, updated_at
			FROM links
			WHERE (source_device_id = ? AND target_device_id = ?)
			   OR (source_device_id = ? AND target_device_id = ?)
			ORDER BY created_at
			LIMIT 1`,
			link.SourceDeviceID.String(), link.TargetDeviceID.String(),
			link.TargetDeviceID.String(), link.SourceDeviceID.String(),
		))
	}

	return scanLinkRow(tx.QueryRow(
		`SELECT id, source_device_id, source_if_name,
			target_device_id, target_if_name, discovery_protocol,
			created_at, updated_at
		FROM links
		WHERE (source_device_id = ? AND source_if_name = ?
		       AND target_device_id = ? AND target_if_name = ?)
		   OR (source_device_id = ? AND source_if_name = ?
		       AND target_device_id = ? AND target_if_name = ?)
		ORDER BY created_at
		LIMIT 1`,
		link.SourceDeviceID.String(), link.SourceIfName,
		link.TargetDeviceID.String(), link.TargetIfName,
		link.TargetDeviceID.String(), link.TargetIfName,
		link.SourceDeviceID.String(), link.SourceIfName,
	))
}

type sameDirectionLinkMatch struct {
	ID           string
	SourceIfName string
	TargetIfName string
	Protocol     string
	score        int
}

func findBestSameDirectionLinkMatch(tx *Tx, link *domain.Link) (*sameDirectionLinkMatch, error) {
	rows, err := tx.Query(
		`SELECT id, source_if_name, target_if_name, discovery_protocol FROM links
		 WHERE source_device_id = ? AND target_device_id = ?
		 ORDER BY created_at`,
		link.SourceDeviceID.String(), link.TargetDeviceID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var best *sameDirectionLinkMatch
	ambiguous := false

	for rows.Next() {
		var candidate sameDirectionLinkMatch
		if err := rows.Scan(&candidate.ID, &candidate.SourceIfName, &candidate.TargetIfName, &candidate.Protocol); err != nil {
			return nil, fmt.Errorf("scanning same-direction link candidate: %w", err)
		}

		srcScore, srcMatch := sameDirectionInterfaceScore(candidate.SourceIfName, link.SourceIfName)
		if !srcMatch {
			continue
		}
		tgtScore, tgtMatch := sameDirectionInterfaceScore(candidate.TargetIfName, link.TargetIfName)
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
		return nil, fmt.Errorf("iterating same-direction link candidates: %w", err)
	}
	if ambiguous {
		return nil, nil
	}
	return best, nil
}

func sameDirectionInterfaceScore(existing, incoming string) (int, bool) {
	if existing == incoming {
		if existing == "" {
			return 1, true
		}
		return 3, true
	}
	if existing == "" || incoming == "" {
		return 0, true
	}
	return 0, false
}

func scanLinkRow(row linkRowScanner) (*domain.Link, error) {
	var link domain.Link
	var idStr, srcDeviceID, tgtDeviceID, protocol string

	err := row.Scan(
		&idStr, &srcDeviceID, &link.SourceIfName,
		&tgtDeviceID, &link.TargetIfName, &protocol,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	link.ID = uuid.MustParse(idStr)
	link.SourceDeviceID = uuid.MustParse(srcDeviceID)
	link.TargetDeviceID = uuid.MustParse(tgtDeviceID)
	link.DiscoveryProtocol = domain.DiscoveryProtocol(protocol)
	return &link, nil
}

type reverseLinkMatch struct {
	ID           string
	SourceIfName string
	TargetIfName string
	Protocol     string
	score        int
}

func findBestReverseLinkMatch(tx *Tx, link *domain.Link) (*reverseLinkMatch, error) {
	rows, err := tx.Query(
		`SELECT id, source_if_name, target_if_name, discovery_protocol FROM links
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
		if err := rows.Scan(&candidate.ID, &candidate.SourceIfName, &candidate.TargetIfName, &candidate.Protocol); err != nil {
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
