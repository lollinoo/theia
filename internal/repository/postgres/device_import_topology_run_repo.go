package postgres

// This file persists resumable, map-scoped topology bootstrap runs created by node import.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"hash"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lollinoo/theia/internal/domain"
)

var (
	// ErrDeviceImportTopologyRunNotFound hides whether a run belongs to another actor or map.
	ErrDeviceImportTopologyRunNotFound = domain.ErrDeviceImportTopologyRunNotFound
	// ErrDeviceImportTopologyRunConflict reports an active run or stale state transition.
	ErrDeviceImportTopologyRunConflict = domain.ErrDeviceImportTopologyRunConflict
)

// DeviceImportTopologyRunRepo persists run state independently of browser lifetime.
type DeviceImportTopologyRunRepo struct {
	db         *DB
	deviceRepo *DeviceRepo
	now        func() time.Time
}

const (
	deviceImportTopologyRunStaleAfter             = 24 * time.Hour
	deviceImportTopologyMaxReconciliationAttempts = 2
)

// NewDeviceImportTopologyRunRepo creates the PostgreSQL topology-run repository.
// Supplying the shared device repository lets direct transactional device-state
// updates publish the same post-commit cache events as normal device mutations.
func NewDeviceImportTopologyRunRepo(db *sql.DB, deviceRepos ...*DeviceRepo) *DeviceImportTopologyRunRepo {
	var deviceRepo *DeviceRepo
	if len(deviceRepos) > 0 {
		deviceRepo = deviceRepos[0]
	}
	return &DeviceImportTopologyRunRepo{db: wrapDB(db), deviceRepo: deviceRepo, now: time.Now}
}

func (r *DeviceImportTopologyRunRepo) publishDeviceUpdates(deviceIDs []uuid.UUID) {
	if r == nil || r.deviceRepo == nil {
		return
	}
	deviceIDs = normalizedDeviceImportTopologyUUIDs(deviceIDs)
	if len(deviceIDs) == 0 {
		return
	}
	r.deviceRepo.notify()
	for _, deviceID := range deviceIDs {
		r.deviceRepo.publishChange(domain.ChangeKindUpdated, deviceID)
	}
}

// Create starts an importing run and serializes automatic layout ownership per map.
func (r *DeviceImportTopologyRunRepo) Create(ctx context.Context, run *domain.DeviceImportTopologyRun) error {
	if r == nil || r.db == nil || run == nil || run.ID == uuid.Nil || run.MapID == uuid.Nil || run.ActorUserID == uuid.Nil {
		return fmt.Errorf("creating topology run: invalid input")
	}
	now := r.now().UTC()
	run.LayoutScope = domain.NormalizeDeviceImportTopologyLayoutScope(run.LayoutScope)
	run.State = domain.DeviceImportTopologyRunStateImporting
	run.AutoLayoutAllowed = true
	run.CreatedAt = now
	run.UpdatedAt = now
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting topology run creation: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs
		 SET state = ?, completed_at = ?, updated_at = ?
		 WHERE map_id = ? AND state IN (?, ?, ?, ?, ?, ?) AND updated_at < ?`,
		string(domain.DeviceImportTopologyRunStateSuperseded), now, now, run.MapID.String(),
		string(domain.DeviceImportTopologyRunStateImporting),
		string(domain.DeviceImportTopologyRunStateDiscovering),
		string(domain.DeviceImportTopologyRunStateReconciling),
		string(domain.DeviceImportTopologyRunStateFollowup),
		string(domain.DeviceImportTopologyRunStateReadyForLayout),
		string(domain.DeviceImportTopologyRunStateFailed),
		now.Add(-deviceImportTopologyRunStaleAfter),
	); err != nil {
		return fmt.Errorf("expiring abandoned topology runs: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO device_import_topology_runs
		 (id, map_id, actor_user_id, file_digest, layout_scope, state, auto_layout_allowed,
		  backgrounded, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, FALSE, ?, ?)`,
		run.ID.String(), run.MapID.String(), run.ActorUserID.String(), run.FileDigest,
		string(run.LayoutScope), string(run.State), run.AutoLayoutAllowed, now, now,
	)
	if err != nil {
		if isDeviceImportTopologyRunActiveConstraint(err) {
			return ErrDeviceImportTopologyRunConflict
		}
		return fmt.Errorf("creating topology run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing topology run creation: %w", err)
	}
	return nil
}

func isDeviceImportTopologyRunActiveConstraint(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" &&
			strings.EqualFold(pgErr.ConstraintName, "device_import_topology_runs_one_active_per_map")
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "device_import_topology_runs_one_active_per_map") &&
		(strings.Contains(message, "unique") || strings.Contains(message, "duplicate"))
}

// AddItem attaches an imported map member to the run in queued state.
func (r *DeviceImportTopologyRunRepo) AddItem(ctx context.Context, runID, deviceID uuid.UUID) error {
	if runID == uuid.Nil || deviceID == uuid.Nil {
		return fmt.Errorf("adding topology run item: invalid input")
	}
	now := r.now().UTC()
	result, err := r.db.Exec(
		`INSERT INTO device_import_topology_run_items (run_id, device_id, state, updated_at)
		 SELECT r.id, d.device_id, ?, ?
		 FROM device_import_topology_runs r
		 JOIN canvas_map_devices d ON d.map_id = r.map_id AND d.device_id = ?
		 WHERE r.id = ? AND r.state = ?
		 ON CONFLICT (run_id, device_id) DO NOTHING`,
		string(domain.DeviceImportTopologyItemStateQueued), now, deviceID.String(), runID.String(),
		string(domain.DeviceImportTopologyRunStateImporting),
	)
	if err != nil {
		return fmt.Errorf("adding topology run item: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return ErrDeviceImportTopologyRunConflict
	}
	return nil
}

// FinalizeImport transitions a populated run to discovery, or completes an empty run.
func (r *DeviceImportTopologyRunRepo) FinalizeImport(ctx context.Context, runID uuid.UUID) error {
	if runID == uuid.Nil {
		return fmt.Errorf("finalizing topology run: run id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting topology run finalize: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var currentStateRaw string
	if err := tx.QueryRowContext(ctx,
		`SELECT state FROM device_import_topology_runs WHERE id = ? FOR UPDATE`, runID.String(),
	).Scan(&currentStateRaw); errors.Is(err, sql.ErrNoRows) {
		return ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return fmt.Errorf("locking topology run finalize: %w", err)
	}
	currentState := domain.DeviceImportTopologyRunState(currentStateRaw)
	if currentState == domain.DeviceImportTopologyRunStateDiscovering || currentState == domain.DeviceImportTopologyRunStateCompleted {
		return nil
	}
	if currentState != domain.DeviceImportTopologyRunStateImporting {
		return ErrDeviceImportTopologyRunConflict
	}

	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM device_import_topology_run_items WHERE run_id = ?`, runID.String(),
	).Scan(&count); err != nil {
		return fmt.Errorf("counting topology run items: %w", err)
	}
	now := r.now().UTC()
	state := domain.DeviceImportTopologyRunStateDiscovering
	var completedAt any
	if count == 0 {
		state = domain.DeviceImportTopologyRunStateCompleted
		completedAt = now
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs
		 SET state = ?, started_at = COALESCE(started_at, ?), completed_at = ?, updated_at = ?
		 WHERE id = ? AND state = ?`,
		string(state), now, completedAt, now, runID.String(), string(domain.DeviceImportTopologyRunStateImporting),
	)
	if err != nil {
		return fmt.Errorf("finalizing topology run: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return ErrDeviceImportTopologyRunConflict
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing topology run finalize: %w", err)
	}
	return nil
}

// Get returns an authoritative snapshot ordered by device ID.
func (r *DeviceImportTopologyRunRepo) Get(ctx context.Context, runID uuid.UUID) (domain.DeviceImportTopologyRunSnapshot, error) {
	var snapshot domain.DeviceImportTopologyRunSnapshot
	if runID == uuid.Nil {
		return snapshot, ErrDeviceImportTopologyRunNotFound
	}
	if err := scanDeviceImportTopologyRun(r.db.QueryRow(
		`SELECT id, map_id, actor_user_id, file_digest, layout_scope, state,
		        auto_layout_allowed, backgrounded, failure_code, failure_message, failure_reference,
		        reconcile_attempts, created_at, started_at, completed_at, updated_at
		 FROM device_import_topology_runs WHERE id = ?`, runID.String(),
	), &snapshot.Run); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return snapshot, ErrDeviceImportTopologyRunNotFound
		}
		return snapshot, fmt.Errorf("reading topology run: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT run_id, device_id, state, attempt, result_code, message, error_reference,
		        neighbor_count, links_created, unresolved_neighbors, started_at, completed_at, updated_at
		 FROM device_import_topology_run_items WHERE run_id = ? ORDER BY device_id`, runID.String(),
	)
	if err != nil {
		return snapshot, fmt.Errorf("querying topology run items: %w", err)
	}
	defer rows.Close()
	snapshot.Items = make([]domain.DeviceImportTopologyRunItem, 0)
	for rows.Next() {
		var item domain.DeviceImportTopologyRunItem
		if err := scanDeviceImportTopologyRunItem(rows, &item); err != nil {
			return snapshot, fmt.Errorf("reading topology run item: %w", err)
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	if err := rows.Err(); err != nil {
		return snapshot, fmt.Errorf("iterating topology run items: %w", err)
	}
	if snapshot.Run.State == domain.DeviceImportTopologyRunStateReadyForLayout {
		snapshot.Run.LayoutInputToken, err = deviceImportTopologyLayoutInputToken(ctx, r.db, snapshot.Run.MapID)
		if err != nil {
			return domain.DeviceImportTopologyRunSnapshot{}, err
		}
	}
	return snapshot, nil
}

type deviceImportTopologyLayoutQueryer interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

// ApplyLayout validates one optimistic topology snapshot and persists positions,
// route resets, and run completion in a single serializable transaction.
func (r *DeviceImportTopologyRunRepo) ApplyLayout(
	ctx context.Context,
	runID, actorUserID uuid.UUID,
	request domain.DeviceImportTopologyLayoutApply,
) error {
	if r == nil || r.db == nil || runID == uuid.Nil || actorUserID == uuid.Nil ||
		strings.TrimSpace(request.InputToken) == "" {
		return ErrDeviceImportTopologyRunNotFound
	}
	positions, err := normalizeDeviceImportTopologyLayoutPositions(request.Positions)
	if err != nil {
		return err
	}
	applicationDigest := deviceImportTopologyLayoutApplicationDigest(request.InputToken, positions)

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("starting topology layout transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var mapIDRaw, stateRaw, layoutScopeRaw string
	var autoLayoutAllowed bool
	var appliedDigest string
	err = tx.QueryRowContext(ctx,
		`SELECT map_id, state, auto_layout_allowed, layout_application_digest, layout_scope
		 FROM device_import_topology_runs
		 WHERE id = ? AND actor_user_id = ?
		 FOR UPDATE`,
		runID.String(), actorUserID.String(),
	).Scan(&mapIDRaw, &stateRaw, &autoLayoutAllowed, &appliedDigest, &layoutScopeRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDeviceImportTopologyRunNotFound
	}
	if err != nil {
		return fmt.Errorf("locking topology layout run: %w", err)
	}
	state := domain.DeviceImportTopologyRunState(stateRaw)
	if state == domain.DeviceImportTopologyRunStateCompleted {
		if appliedDigest == applicationDigest {
			return nil
		}
		return ErrDeviceImportTopologyRunConflict
	}
	if state != domain.DeviceImportTopologyRunStateReadyForLayout || !autoLayoutAllowed {
		return ErrDeviceImportTopologyRunConflict
	}
	mapID, err := uuid.Parse(mapIDRaw)
	if err != nil {
		return fmt.Errorf("parsing topology layout map: %w", err)
	}
	currentToken, err := deviceImportTopologyLayoutInputToken(ctx, tx, mapID)
	if err != nil {
		return err
	}
	if currentToken != request.InputToken {
		return domain.ErrDeviceImportTopologyLayoutStale
	}
	resetRouteIDs, err := validateDeviceImportTopologyLayoutContract(
		ctx,
		tx,
		mapID,
		runID,
		domain.NormalizeDeviceImportTopologyLayoutScope(domain.DeviceImportTopologyLayoutScope(layoutScopeRaw)),
		positions,
	)
	if err != nil {
		return err
	}

	now := r.now().UTC()
	for _, position := range positions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO canvas_map_positions (map_id, device_id, x, y, pinned, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(map_id, device_id) DO UPDATE SET
			   x = excluded.x, y = excluded.y, pinned = excluded.pinned, updated_at = excluded.updated_at`,
			mapID.String(), position.DeviceID.String(), position.X, position.Y, position.Pinned, now,
		); err != nil {
			return fmt.Errorf("saving topology layout position %s: %w", position.DeviceID, err)
		}
	}

	for _, linkID := range resetRouteIDs {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM canvas_map_link_routes WHERE map_id = ? AND link_id = ?`,
			mapID.String(), linkID.String(),
		); err != nil {
			return fmt.Errorf("resetting topology layout route %s: %w", linkID, err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE canvas_maps SET updated_at = ? WHERE id = ?`, now, mapID.String(),
	); err != nil {
		return fmt.Errorf("touching topology layout map: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs
		 SET state = ?, layout_application_digest = ?, completed_at = ?, updated_at = ?
		 WHERE id = ?`,
		string(domain.DeviceImportTopologyRunStateCompleted), applicationDigest, now, now, runID.String(),
	); err != nil {
		return fmt.Errorf("completing topology layout run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing topology layout: %w", err)
	}
	return nil
}

func normalizeDeviceImportTopologyLayoutPositions(
	positions []domain.DevicePosition,
) ([]domain.DevicePosition, error) {
	normalized := append([]domain.DevicePosition(nil), positions...)
	seen := make(map[uuid.UUID]struct{}, len(normalized))
	for _, position := range normalized {
		if position.DeviceID == uuid.Nil || math.IsNaN(position.X) || math.IsInf(position.X, 0) ||
			math.IsNaN(position.Y) || math.IsInf(position.Y, 0) {
			return nil, ErrDeviceImportTopologyRunConflict
		}
		if _, duplicate := seen[position.DeviceID]; duplicate {
			return nil, ErrDeviceImportTopologyRunConflict
		}
		seen[position.DeviceID] = struct{}{}
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].DeviceID.String() < normalized[j].DeviceID.String()
	})
	return normalized, nil
}

const (
	deviceImportTopologyLayoutMinHorizontalGap = 466.0
	deviceImportTopologyLayoutMinVerticalGap   = 256.0
)

type storedDeviceImportTopologyPosition struct {
	x sql.NullFloat64
	y sql.NullFloat64
}

func validateDeviceImportTopologyLayoutContract(
	ctx context.Context,
	tx *Tx,
	mapID, runID uuid.UUID,
	scope domain.DeviceImportTopologyLayoutScope,
	positions []domain.DevicePosition,
) ([]uuid.UUID, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT d.device_id, p.x, p.y
		 FROM canvas_map_devices d
		 LEFT JOIN canvas_map_positions p ON p.map_id = d.map_id AND p.device_id = d.device_id
		 WHERE d.map_id = ?
		 ORDER BY d.device_id
		 FOR UPDATE OF d`,
		mapID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("locking topology layout map devices: %w", err)
	}
	mapPositions := make(map[uuid.UUID]storedDeviceImportTopologyPosition)
	for rows.Next() {
		var deviceIDRaw string
		var position storedDeviceImportTopologyPosition
		if err := rows.Scan(&deviceIDRaw, &position.x, &position.y); err != nil {
			rows.Close()
			return nil, fmt.Errorf("reading topology layout map device: %w", err)
		}
		deviceID, err := uuid.Parse(deviceIDRaw)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("parsing topology layout map device: %w", err)
		}
		mapPositions[deviceID] = position
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing topology layout map devices: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating topology layout map devices: %w", err)
	}

	expected := make(map[uuid.UUID]struct{})
	if scope == domain.DeviceImportTopologyLayoutScopeReorganize {
		for deviceID := range mapPositions {
			expected[deviceID] = struct{}{}
		}
	} else {
		itemRows, err := tx.QueryContext(ctx,
			`SELECT device_id FROM device_import_topology_run_items WHERE run_id = ? ORDER BY device_id`,
			runID.String(),
		)
		if err != nil {
			return nil, fmt.Errorf("querying topology layout imported devices: %w", err)
		}
		for itemRows.Next() {
			var deviceIDRaw string
			if err := itemRows.Scan(&deviceIDRaw); err != nil {
				itemRows.Close()
				return nil, fmt.Errorf("reading topology layout imported device: %w", err)
			}
			deviceID, err := uuid.Parse(deviceIDRaw)
			if err != nil {
				itemRows.Close()
				return nil, fmt.Errorf("parsing topology layout imported device: %w", err)
			}
			if _, belongsToMap := mapPositions[deviceID]; !belongsToMap {
				itemRows.Close()
				return nil, ErrDeviceImportTopologyRunConflict
			}
			expected[deviceID] = struct{}{}
		}
		if err := itemRows.Close(); err != nil {
			return nil, fmt.Errorf("closing topology layout imported devices: %w", err)
		}
		if err := itemRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating topology layout imported devices: %w", err)
		}
	}
	if len(positions) != len(expected) {
		return nil, ErrDeviceImportTopologyRunConflict
	}
	candidates := make(map[uuid.UUID]domain.DevicePosition, len(positions))
	for _, position := range positions {
		if _, allowed := expected[position.DeviceID]; !allowed {
			return nil, ErrDeviceImportTopologyRunConflict
		}
		candidates[position.DeviceID] = position
	}
	for deviceID := range expected {
		if _, present := candidates[deviceID]; !present {
			return nil, ErrDeviceImportTopologyRunConflict
		}
	}

	for leftIndex := range positions {
		for rightIndex := leftIndex + 1; rightIndex < len(positions); rightIndex++ {
			if deviceImportTopologyPositionsOverlap(positions[leftIndex], positions[rightIndex]) {
				return nil, ErrDeviceImportTopologyRunConflict
			}
		}
	}
	if scope == domain.DeviceImportTopologyLayoutScopePreserve {
		for deviceID, current := range mapPositions {
			if _, moving := expected[deviceID]; moving || !current.x.Valid || !current.y.Valid {
				continue
			}
			fixed := domain.DevicePosition{DeviceID: deviceID, X: current.x.Float64, Y: current.y.Float64}
			for _, candidate := range positions {
				if deviceImportTopologyPositionsOverlap(candidate, fixed) {
					return nil, ErrDeviceImportTopologyRunConflict
				}
			}
		}
	}

	moved := make(map[uuid.UUID]struct{}, len(positions))
	for _, position := range positions {
		current := mapPositions[position.DeviceID]
		if !current.x.Valid || !current.y.Valid || current.x.Float64 != position.X || current.y.Float64 != position.Y {
			moved[position.DeviceID] = struct{}{}
		}
	}
	linkRows, err := tx.QueryContext(ctx,
		`SELECT ml.link_id, l.source_device_id, l.target_device_id
		 FROM canvas_map_links ml
		 JOIN links l ON l.id = ml.link_id
		 WHERE ml.map_id = ?
		 ORDER BY ml.link_id`,
		mapID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying topology layout route resets: %w", err)
	}
	resetRouteIDs := make([]uuid.UUID, 0)
	for linkRows.Next() {
		var linkIDRaw, sourceIDRaw, targetIDRaw string
		if err := linkRows.Scan(&linkIDRaw, &sourceIDRaw, &targetIDRaw); err != nil {
			linkRows.Close()
			return nil, fmt.Errorf("reading topology layout route reset: %w", err)
		}
		linkID, linkErr := uuid.Parse(linkIDRaw)
		sourceID, sourceErr := uuid.Parse(sourceIDRaw)
		targetID, targetErr := uuid.Parse(targetIDRaw)
		if linkErr != nil || sourceErr != nil || targetErr != nil {
			linkRows.Close()
			return nil, fmt.Errorf("parsing topology layout route reset endpoints")
		}
		_, sourceMoved := moved[sourceID]
		_, targetMoved := moved[targetID]
		if sourceMoved || targetMoved {
			resetRouteIDs = append(resetRouteIDs, linkID)
		}
	}
	if err := linkRows.Close(); err != nil {
		return nil, fmt.Errorf("closing topology layout route resets: %w", err)
	}
	if err := linkRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating topology layout route resets: %w", err)
	}
	return resetRouteIDs, nil
}

func deviceImportTopologyPositionsOverlap(left, right domain.DevicePosition) bool {
	return math.Abs(left.X-right.X) < deviceImportTopologyLayoutMinHorizontalGap &&
		math.Abs(left.Y-right.Y) < deviceImportTopologyLayoutMinVerticalGap
}

func deviceImportTopologyLayoutApplicationDigest(
	inputToken string,
	positions []domain.DevicePosition,
) string {
	digest := sha256.New()
	writeDeviceImportTopologyLayoutHash(digest, "application-v1", inputToken)
	for _, position := range positions {
		writeDeviceImportTopologyLayoutHash(
			digest,
			position.DeviceID.String(),
			strconv.FormatFloat(position.X, 'g', -1, 64),
			strconv.FormatFloat(position.Y, 'g', -1, 64),
			strconv.FormatBool(position.Pinned),
		)
	}
	return fmt.Sprintf("sha256:%x", digest.Sum(nil))
}

func deviceImportTopologyLayoutInputToken(
	ctx context.Context,
	queryer deviceImportTopologyLayoutQueryer,
	mapID uuid.UUID,
) (string, error) {
	digest := sha256.New()
	writeDeviceImportTopologyLayoutHash(digest, "topology-layout-v1", mapID.String())
	queries := []struct {
		name  string
		query string
		scan  func(*sql.Rows) ([]string, error)
	}{
		{
			name: "devices",
			query: `SELECT md.device_id, md.role, COALESCE(md.visual_color, ''),
			               d.hostname, d.ip, d.sys_name, d.device_type, d.vendor
			        FROM canvas_map_devices md
			        JOIN devices d ON d.id = md.device_id
			        WHERE md.map_id = ? ORDER BY md.device_id`,
			scan: scanDeviceImportTopologyLayoutStrings(8),
		},
		{
			name: "interfaces",
			query: `SELECT i.device_id, i.id, i.if_index::text, i.if_name, i.if_descr,
			               i.speed::text, i.admin_status, i.oper_status
			        FROM interfaces i
			        JOIN canvas_map_devices md ON md.device_id = i.device_id
			        WHERE md.map_id = ? ORDER BY i.device_id, i.if_index, i.id`,
			scan: scanDeviceImportTopologyLayoutStrings(8),
		},
		{
			name: "links",
			query: `SELECT l.id, l.source_device_id, l.source_if_name,
			               l.target_device_id, l.target_if_name, l.discovery_protocol
			        FROM canvas_map_links ml
			        JOIN links l ON l.id = ml.link_id
			        WHERE ml.map_id = ? ORDER BY l.id`,
			scan: scanDeviceImportTopologyLayoutStrings(6),
		},
		{
			name: "positions",
			query: `SELECT device_id, x::text, y::text, pinned::text
			        FROM canvas_map_positions WHERE map_id = ? ORDER BY device_id`,
			scan: scanDeviceImportTopologyLayoutStrings(4),
		},
		{
			name: "routes",
			query: `SELECT link_id, route_version::text, waypoints_json::text
			        FROM canvas_map_link_routes WHERE map_id = ? ORDER BY link_id`,
			scan: scanDeviceImportTopologyLayoutStrings(3),
		},
	}
	for _, query := range queries {
		writeDeviceImportTopologyLayoutHash(digest, query.name)
		rows, err := queryer.QueryContext(ctx, query.query, mapID.String())
		if err != nil {
			return "", fmt.Errorf("querying topology layout %s: %w", query.name, err)
		}
		for rows.Next() {
			values, scanErr := query.scan(rows)
			if scanErr != nil {
				rows.Close()
				return "", fmt.Errorf("reading topology layout %s: %w", query.name, scanErr)
			}
			writeDeviceImportTopologyLayoutHash(digest, values...)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return "", fmt.Errorf("iterating topology layout %s: %w", query.name, err)
		}
		if err := rows.Close(); err != nil {
			return "", fmt.Errorf("closing topology layout %s: %w", query.name, err)
		}
	}
	return fmt.Sprintf("sha256:%x", digest.Sum(nil)), nil
}

func scanDeviceImportTopologyLayoutStrings(count int) func(*sql.Rows) ([]string, error) {
	return func(rows *sql.Rows) ([]string, error) {
		values := make([]string, count)
		destinations := make([]any, count)
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}
		return values, nil
	}
}

func writeDeviceImportTopologyLayoutHash(digest hash.Hash, values ...string) {
	for _, value := range values {
		_, _ = fmt.Fprintf(digest, "%d:", len(value))
		_, _ = digest.Write([]byte(value))
		_, _ = digest.Write([]byte{0})
	}
}

// MarkItemRunning claims the newest queued active run item for a bootstrap task.
func (r *DeviceImportTopologyRunRepo) MarkItemRunning(
	ctx context.Context,
	deviceID uuid.UUID,
	startedAt time.Time,
) (uuid.UUID, bool, error) {
	if deviceID == uuid.Nil {
		return uuid.Nil, false, nil
	}
	if startedAt.IsZero() {
		startedAt = r.now().UTC()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("starting topology item claim: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var runIDRaw string
	err = tx.QueryRowContext(ctx,
		`SELECT i.run_id
		 FROM device_import_topology_run_items i
		 JOIN device_import_topology_runs r ON r.id = i.run_id
		 WHERE i.device_id = ? AND i.state = ? AND r.state IN (?, ?)
		 ORDER BY r.created_at DESC
		 LIMIT 1
		 FOR UPDATE OF i SKIP LOCKED`,
		deviceID.String(), string(domain.DeviceImportTopologyItemStateQueued),
		string(domain.DeviceImportTopologyRunStateDiscovering), string(domain.DeviceImportTopologyRunStateFollowup),
	).Scan(&runIDRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("claiming topology run item: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_run_items
		 SET state = ?, attempt = attempt + 1, result_code = '', message = '', error_reference = '',
		     started_at = ?, completed_at = NULL, updated_at = ?
		 WHERE run_id = ? AND device_id = ?`,
		string(domain.DeviceImportTopologyItemStateRunning), startedAt.UTC(), startedAt.UTC(), runIDRaw, deviceID.String(),
	); err != nil {
		return uuid.Nil, false, fmt.Errorf("marking topology run item running: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, false, fmt.Errorf("committing topology item claim: %w", err)
	}
	runID, err := uuid.Parse(runIDRaw)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("parsing topology run id: %w", err)
	}
	return runID, true, nil
}

// CompleteItem records one sanitized terminal result, closes its one-shot device window, and
// reports whether the run is terminal. Retry and follow-up transitions reopen that window.
func (r *DeviceImportTopologyRunRepo) CompleteItem(
	ctx context.Context,
	runID, deviceID uuid.UUID,
	completion domain.DeviceImportTopologyItemCompletion,
) (bool, error) {
	if runID == uuid.Nil || deviceID == uuid.Nil || !completion.State.Terminal() {
		return false, fmt.Errorf("completing topology run item: invalid input")
	}
	if completion.CompletedAt.IsZero() {
		completion.CompletedAt = r.now().UTC()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("starting topology item completion: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var runState string
	if err := tx.QueryRowContext(ctx,
		`SELECT state FROM device_import_topology_runs WHERE id = ? FOR UPDATE`, runID.String(),
	).Scan(&runState); errors.Is(err, sql.ErrNoRows) {
		return false, ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return false, fmt.Errorf("locking topology run item completion: %w", err)
	}
	if runState != string(domain.DeviceImportTopologyRunStateDiscovering) &&
		runState != string(domain.DeviceImportTopologyRunStateFollowup) {
		return false, ErrDeviceImportTopologyRunConflict
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_run_items
		 SET state = ?, result_code = ?, message = ?, error_reference = ?, neighbor_count = ?,
		     links_created = ?, unresolved_neighbors = ?, completed_at = ?, updated_at = ?
		 WHERE run_id = ? AND device_id = ? AND state = ?`,
		string(completion.State), string(completion.ResultCode), completion.Message, completion.Reference,
		max(completion.NeighborCount, 0), max(completion.LinksCreated, 0), max(completion.UnresolvedNeighbors, 0),
		completion.CompletedAt.UTC(), completion.CompletedAt.UTC(), runID.String(), deviceID.String(),
		string(domain.DeviceImportTopologyItemStateRunning),
	)
	if err != nil {
		return false, fmt.Errorf("completing topology run item: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return false, ErrDeviceImportTopologyRunConflict
	}
	deviceResult, err := tx.ExecContext(ctx,
		`UPDATE devices
		 SET topology_bootstrap_state = ?, updated_at = ?
		 WHERE id = ? AND topology_bootstrap_state IN (?, ?)`,
		string(domain.TopologyBootstrapStateCompleted), r.now().UTC(), deviceID.String(),
		string(domain.TopologyBootstrapStatePending), string(domain.TopologyBootstrapStateFollowupScheduled),
	)
	if err != nil {
		return false, fmt.Errorf("closing topology bootstrap window after item completion: %w", err)
	}
	deviceRows, _ := deviceResult.RowsAffected()
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs SET updated_at = ? WHERE id = ?`,
		completion.CompletedAt.UTC(), runID.String(),
	); err != nil {
		return false, fmt.Errorf("touching topology run after item completion: %w", err)
	}
	var remaining int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM device_import_topology_run_items
		 WHERE run_id = ? AND state IN (?, ?)`,
		runID.String(), string(domain.DeviceImportTopologyItemStateQueued), string(domain.DeviceImportTopologyItemStateRunning),
	).Scan(&remaining); err != nil {
		return false, fmt.Errorf("counting remaining topology run items: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("committing topology item completion: %w", err)
	}
	if deviceRows > 0 {
		r.publishDeviceUpdates([]uuid.UUID{deviceID})
	}
	return remaining == 0, nil
}

// ApplyReconciliationResults atomically replaces provisional discovery counters with the final
// map-scoped link outcome for every item in a reconciling run.
func (r *DeviceImportTopologyRunRepo) ApplyReconciliationResults(
	ctx context.Context,
	runID uuid.UUID,
	results []domain.DeviceImportTopologyItemReconciliation,
) error {
	if runID == uuid.Nil {
		return fmt.Errorf("applying topology reconciliation results: run id is required")
	}
	byDevice := make(map[uuid.UUID]domain.DeviceImportTopologyItemReconciliation, len(results))
	for _, result := range results {
		if result.DeviceID == uuid.Nil || result.LinksCreated < 0 || result.UnresolvedNeighbors < 0 {
			return fmt.Errorf("applying topology reconciliation results: invalid item")
		}
		if _, duplicate := byDevice[result.DeviceID]; duplicate {
			return fmt.Errorf("applying topology reconciliation results: duplicate device %s", result.DeviceID)
		}
		byDevice[result.DeviceID] = result
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting topology reconciliation result update: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var runStateRaw string
	if err := tx.QueryRowContext(ctx,
		`SELECT state FROM device_import_topology_runs WHERE id = ? FOR UPDATE`,
		runID.String(),
	).Scan(&runStateRaw); errors.Is(err, sql.ErrNoRows) {
		return ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return fmt.Errorf("locking topology reconciliation result update: %w", err)
	}
	if domain.DeviceImportTopologyRunState(runStateRaw) != domain.DeviceImportTopologyRunStateReconciling {
		return ErrDeviceImportTopologyRunConflict
	}

	type storedItem struct {
		deviceID   uuid.UUID
		state      domain.DeviceImportTopologyItemState
		resultCode domain.DeviceImportTopologyResultCode
		message    string
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT device_id, state, result_code, message
		 FROM device_import_topology_run_items WHERE run_id = ? ORDER BY device_id FOR UPDATE`,
		runID.String(),
	)
	if err != nil {
		return fmt.Errorf("locking topology reconciliation items: %w", err)
	}
	items := make([]storedItem, 0, len(results))
	for rows.Next() {
		var rawDeviceID, stateRaw, resultCodeRaw string
		var item storedItem
		if err := rows.Scan(&rawDeviceID, &stateRaw, &resultCodeRaw, &item.message); err != nil {
			rows.Close()
			return fmt.Errorf("reading topology reconciliation item: %w", err)
		}
		item.deviceID, err = uuid.Parse(rawDeviceID)
		if err != nil {
			rows.Close()
			return fmt.Errorf("parsing topology reconciliation item device: %w", err)
		}
		item.state = domain.DeviceImportTopologyItemState(stateRaw)
		item.resultCode = domain.DeviceImportTopologyResultCode(resultCodeRaw)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterating topology reconciliation items: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("closing topology reconciliation items: %w", err)
	}
	if len(items) != len(byDevice) {
		return ErrDeviceImportTopologyRunConflict
	}
	for _, item := range items {
		if _, ok := byDevice[item.deviceID]; !ok {
			return ErrDeviceImportTopologyRunConflict
		}
	}

	now := r.now().UTC()
	for _, item := range items {
		result := byDevice[item.deviceID]
		state := item.state
		resultCode := item.resultCode
		message := item.message
		if state != domain.DeviceImportTopologyItemStateFailed {
			switch {
			case result.UnresolvedNeighbors > 0:
				state = domain.DeviceImportTopologyItemStateWarning
				resultCode = domain.DeviceImportTopologyResultUnresolvedNeighbors
				message = "Some discovered neighbors could not be linked automatically."
			case resultCode == domain.DeviceImportTopologyResultUnresolvedNeighbors:
				state = domain.DeviceImportTopologyItemStateSucceeded
				resultCode = domain.DeviceImportTopologyResultDiscovered
				message = "Topology discovery completed."
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE device_import_topology_run_items
			 SET state = ?, result_code = ?, message = ?, links_created = ?, unresolved_neighbors = ?, updated_at = ?
			 WHERE run_id = ? AND device_id = ?`,
			string(state), string(resultCode), message,
			result.LinksCreated, result.UnresolvedNeighbors, now,
			runID.String(), item.deviceID.String(),
		); err != nil {
			return fmt.Errorf("updating topology reconciliation item %s: %w", item.deviceID, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs SET updated_at = ? WHERE id = ?`,
		now, runID.String(),
	); err != nil {
		return fmt.Errorf("touching topology run after reconciliation results: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing topology reconciliation results: %w", err)
	}
	return nil
}

// RecoverInterrupted requeues in-flight items and finalizes stale importing runs after restart.
func (r *DeviceImportTopologyRunRepo) RecoverInterrupted(ctx context.Context) ([]uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting topology run recovery: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	now := r.now().UTC()
	rows, err := tx.Query(
		`UPDATE device_import_topology_run_items i
		 SET state = ?, started_at = NULL, completed_at = NULL, updated_at = ?
		 FROM device_import_topology_runs r
		 WHERE r.id = i.run_id AND r.state IN (?, ?, ?) AND i.state = ?
		 RETURNING i.device_id`,
		string(domain.DeviceImportTopologyItemStateQueued), now,
		string(domain.DeviceImportTopologyRunStateDiscovering), string(domain.DeviceImportTopologyRunStateReconciling),
		string(domain.DeviceImportTopologyRunStateFollowup), string(domain.DeviceImportTopologyItemStateRunning),
	)
	if err != nil {
		return nil, fmt.Errorf("requeueing interrupted topology items: %w", err)
	}
	var recovered []uuid.UUID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, fmt.Errorf("reading recovered topology item: %w", err)
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("parsing recovered topology item: %w", err)
		}
		recovered = append(recovered, id)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing recovered topology items: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs r
		 SET state = CASE WHEN EXISTS (
		       SELECT 1 FROM device_import_topology_run_items i WHERE i.run_id = r.id
		     ) THEN ? ELSE ? END,
		     started_at = COALESCE(started_at, CAST(? AS TIMESTAMPTZ)),
		     completed_at = CASE WHEN EXISTS (
		       SELECT 1 FROM device_import_topology_run_items i WHERE i.run_id = r.id
		     ) THEN NULL ELSE CAST(? AS TIMESTAMPTZ) END,
		     updated_at = ?
		 WHERE state = ?`,
		string(domain.DeviceImportTopologyRunStateDiscovering), string(domain.DeviceImportTopologyRunStateCompleted),
		now, now, now, string(domain.DeviceImportTopologyRunStateImporting),
	); err != nil {
		return nil, fmt.Errorf("recovering importing topology runs: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE devices d
		 SET topology_discovery_mode = ?, topology_bootstrap_state = ?, updated_at = ?
		 FROM device_import_topology_run_items i
		 JOIN device_import_topology_runs r ON r.id = i.run_id
		 WHERE d.id = i.device_id
		   AND r.state IN (?, ?)
		   AND i.state = ?`,
		string(domain.TopologyDiscoveryModeBootstrapOnce), string(domain.TopologyBootstrapStatePending), now,
		string(domain.DeviceImportTopologyRunStateDiscovering),
		string(domain.DeviceImportTopologyRunStateFollowup),
		string(domain.DeviceImportTopologyItemStateQueued),
	); err != nil {
		return nil, fmt.Errorf("restoring recovered topology devices to pending: %w", err)
	}
	queuedRows, err := tx.Query(
		`SELECT i.device_id
		 FROM device_import_topology_run_items i
		 JOIN device_import_topology_runs r ON r.id = i.run_id
		 WHERE r.state IN (?, ?) AND i.state = ?
		 ORDER BY i.device_id`,
		string(domain.DeviceImportTopologyRunStateDiscovering),
		string(domain.DeviceImportTopologyRunStateFollowup),
		string(domain.DeviceImportTopologyItemStateQueued),
	)
	if err != nil {
		return nil, fmt.Errorf("querying queued topology recovery items: %w", err)
	}
	for queuedRows.Next() {
		var raw string
		if err := queuedRows.Scan(&raw); err != nil {
			queuedRows.Close()
			return nil, fmt.Errorf("reading queued topology recovery item: %w", err)
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			queuedRows.Close()
			return nil, fmt.Errorf("parsing queued topology recovery item: %w", err)
		}
		recovered = append(recovered, id)
	}
	if err := queuedRows.Close(); err != nil {
		return nil, fmt.Errorf("closing queued topology recovery items: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing topology run recovery: %w", err)
	}
	recovered = normalizedDeviceImportTopologyUUIDs(recovered)
	r.publishDeviceUpdates(recovered)
	return recovered, nil
}

// RecoverReconciling returns runs whose idempotent reconciliation was interrupted.
// It also closes the restart window after the last discovery item committed but
// before the coordinator could advance the run to reconciliation.
func (r *DeviceImportTopologyRunRepo) RecoverReconciling(ctx context.Context) ([]uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting topology reconciliation recovery: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	now := r.now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs r
		 SET state = ?, updated_at = ?
		 WHERE r.state IN (?, ?)
		   AND EXISTS (
		     SELECT 1 FROM device_import_topology_run_items i WHERE i.run_id = r.id
		   )
		   AND NOT EXISTS (
		     SELECT 1 FROM device_import_topology_run_items i
		     WHERE i.run_id = r.id AND i.state IN (?, ?)
		   )`,
		string(domain.DeviceImportTopologyRunStateReconciling), now,
		string(domain.DeviceImportTopologyRunStateDiscovering), string(domain.DeviceImportTopologyRunStateFollowup),
		string(domain.DeviceImportTopologyItemStateQueued), string(domain.DeviceImportTopologyItemStateRunning),
	); err != nil {
		return nil, fmt.Errorf("advancing terminal topology runs for reconciliation recovery: %w", err)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM device_import_topology_runs WHERE state = ? ORDER BY created_at, id`,
		string(domain.DeviceImportTopologyRunStateReconciling),
	)
	if err != nil {
		return nil, fmt.Errorf("querying interrupted topology reconciliation runs: %w", err)
	}
	defer rows.Close()
	runIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("reading interrupted topology reconciliation run: %w", err)
		}
		runID, err := uuid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parsing interrupted topology reconciliation run: %w", err)
		}
		runIDs = append(runIDs, runID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating interrupted topology reconciliation runs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing interrupted topology reconciliation runs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing topology reconciliation recovery: %w", err)
	}
	return runIDs, nil
}

// TransitionRun advances the durable state machine without permitting stale regressions.
func (r *DeviceImportTopologyRunRepo) TransitionRun(
	ctx context.Context,
	runID uuid.UUID,
	next domain.DeviceImportTopologyRunState,
) error {
	if runID == uuid.Nil {
		return ErrDeviceImportTopologyRunNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting topology run transition: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var currentRaw string
	var autoLayoutAllowed bool
	if err := tx.QueryRowContext(ctx,
		`SELECT state, auto_layout_allowed FROM device_import_topology_runs WHERE id = ? FOR UPDATE`, runID.String(),
	).Scan(&currentRaw, &autoLayoutAllowed); errors.Is(err, sql.ErrNoRows) {
		return ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return fmt.Errorf("reading topology run state: %w", err)
	}
	current := domain.DeviceImportTopologyRunState(currentRaw)
	if !validDeviceImportTopologyRunTransition(current, next) {
		return ErrDeviceImportTopologyRunConflict
	}
	if next == domain.DeviceImportTopologyRunStateReadyForLayout && !autoLayoutAllowed {
		next = domain.DeviceImportTopologyRunStateCompleted
	}
	now := r.now().UTC()
	var completedAt any
	if next == domain.DeviceImportTopologyRunStateCompleted || next == domain.DeviceImportTopologyRunStateSuperseded {
		completedAt = now
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs
		 SET state = ?, completed_at = ?, updated_at = ?,
		     failure_code = '', failure_message = '', failure_reference = '', reconcile_attempts = 0
		 WHERE id = ?`,
		string(next), completedAt, now, runID.String(),
	); err != nil {
		return fmt.Errorf("transitioning topology run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing topology run transition: %w", err)
	}
	return nil
}

// RecordReconciliationFailure persists a sanitized failure and allows one bounded retry.
func (r *DeviceImportTopologyRunRepo) RecordReconciliationFailure(
	ctx context.Context,
	runID uuid.UUID,
	failure domain.DeviceImportTopologyRunFailure,
) (bool, error) {
	if runID == uuid.Nil {
		return false, ErrDeviceImportTopologyRunNotFound
	}
	failure = normalizeDeviceImportTopologyRunFailure(failure)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("starting topology reconciliation failure record: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var stateRaw string
	var attempts int
	if err := tx.QueryRowContext(ctx,
		`SELECT state, reconcile_attempts FROM device_import_topology_runs WHERE id = ? FOR UPDATE`,
		runID.String(),
	).Scan(&stateRaw, &attempts); errors.Is(err, sql.ErrNoRows) {
		return false, ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return false, fmt.Errorf("locking topology reconciliation failure: %w", err)
	}
	if domain.DeviceImportTopologyRunState(stateRaw) != domain.DeviceImportTopologyRunStateReconciling {
		return false, ErrDeviceImportTopologyRunConflict
	}
	attempts++
	retry := attempts < deviceImportTopologyMaxReconciliationAttempts
	nextState := domain.DeviceImportTopologyRunStateReconciling
	if !retry {
		nextState = domain.DeviceImportTopologyRunStateFailed
	}
	now := r.now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs
		 SET state = ?, failure_code = ?, failure_message = ?, failure_reference = ?,
		     reconcile_attempts = ?, updated_at = ?
		 WHERE id = ?`,
		string(nextState), string(failure.Code), failure.Message, failure.Reference,
		attempts, now, runID.String(),
	); err != nil {
		return false, fmt.Errorf("recording topology reconciliation failure: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("committing topology reconciliation failure: %w", err)
	}
	return retry, nil
}

// FailImport persists a sanitized post-commit finalization failure for manual recovery.
func (r *DeviceImportTopologyRunRepo) FailImport(
	ctx context.Context,
	runID uuid.UUID,
	failure domain.DeviceImportTopologyRunFailure,
) error {
	if runID == uuid.Nil {
		return ErrDeviceImportTopologyRunNotFound
	}
	failure = normalizeDeviceImportTopologyRunFailure(failure)
	result, err := r.db.Exec(
		`UPDATE device_import_topology_runs
		 SET state = ?, failure_code = ?, failure_message = ?, failure_reference = ?, updated_at = ?
		 WHERE id = ? AND state = ?`,
		string(domain.DeviceImportTopologyRunStateFailed), string(failure.Code), failure.Message, failure.Reference,
		r.now().UTC(), runID.String(), string(domain.DeviceImportTopologyRunStateImporting),
	)
	if err != nil {
		return fmt.Errorf("recording topology import finalization failure: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return ErrDeviceImportTopologyRunConflict
	}
	return nil
}

func normalizeDeviceImportTopologyRunFailure(
	failure domain.DeviceImportTopologyRunFailure,
) domain.DeviceImportTopologyRunFailure {
	if failure.Code == domain.DeviceImportTopologyResultNone {
		failure.Code = domain.DeviceImportTopologyResultInternal
	}
	failure.Message = strings.TrimSpace(failure.Message)
	if failure.Message == "" {
		failure.Message = "Topology Bootstrap-Once could not complete automatically."
	}
	if len(failure.Message) > 240 {
		failure.Message = failure.Message[:240]
	}
	failure.Reference = strings.TrimSpace(failure.Reference)
	if len(failure.Reference) > 128 {
		failure.Reference = failure.Reference[:128]
	}
	return failure
}

// RunMapID returns the map scope used for invalidation without exposing run ownership.
func (r *DeviceImportTopologyRunRepo) RunMapID(ctx context.Context, runID uuid.UUID) (uuid.UUID, error) {
	if runID == uuid.Nil {
		return uuid.Nil, ErrDeviceImportTopologyRunNotFound
	}
	var raw string
	if err := r.db.QueryRow(
		`SELECT map_id FROM device_import_topology_runs WHERE id = ?`, runID.String(),
	).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return uuid.Nil, fmt.Errorf("reading topology run map: %w", err)
	}
	mapID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parsing topology run map: %w", err)
	}
	return mapID, nil
}

// GetForActor returns a run only to the administrator who created it.
func (r *DeviceImportTopologyRunRepo) GetForActor(
	ctx context.Context,
	runID, actorUserID uuid.UUID,
) (domain.DeviceImportTopologyRunSnapshot, error) {
	snapshot, err := r.Get(ctx, runID)
	if err != nil {
		return domain.DeviceImportTopologyRunSnapshot{}, err
	}
	if actorUserID == uuid.Nil || snapshot.Run.ActorUserID != actorUserID {
		return domain.DeviceImportTopologyRunSnapshot{}, ErrDeviceImportTopologyRunNotFound
	}
	return snapshot, nil
}

// GetActiveForMap returns the actor's current resumable run for one map.
func (r *DeviceImportTopologyRunRepo) GetActiveForMap(
	ctx context.Context,
	mapID, actorUserID uuid.UUID,
) (domain.DeviceImportTopologyRunSnapshot, error) {
	if mapID == uuid.Nil || actorUserID == uuid.Nil {
		return domain.DeviceImportTopologyRunSnapshot{}, ErrDeviceImportTopologyRunNotFound
	}
	var runIDRaw string
	if err := r.db.QueryRow(
		`SELECT id FROM device_import_topology_runs
		 WHERE map_id = ? AND actor_user_id = ?
		   AND state IN (?, ?, ?, ?, ?, ?)
		 ORDER BY created_at DESC LIMIT 1`,
		mapID.String(), actorUserID.String(),
		string(domain.DeviceImportTopologyRunStateImporting),
		string(domain.DeviceImportTopologyRunStateDiscovering),
		string(domain.DeviceImportTopologyRunStateReconciling),
		string(domain.DeviceImportTopologyRunStateFollowup),
		string(domain.DeviceImportTopologyRunStateReadyForLayout),
		string(domain.DeviceImportTopologyRunStateFailed),
	).Scan(&runIDRaw); errors.Is(err, sql.ErrNoRows) {
		return domain.DeviceImportTopologyRunSnapshot{}, ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return domain.DeviceImportTopologyRunSnapshot{}, fmt.Errorf("reading active topology run: %w", err)
	}
	runID, err := uuid.Parse(runIDRaw)
	if err != nil {
		return domain.DeviceImportTopologyRunSnapshot{}, fmt.Errorf("parsing active topology run: %w", err)
	}
	return r.GetForActor(ctx, runID, actorUserID)
}

// SetBackgrounded persists the user's decision to continue with a partial map.
func (r *DeviceImportTopologyRunRepo) SetBackgrounded(ctx context.Context, runID, actorUserID uuid.UUID) error {
	if runID == uuid.Nil || actorUserID == uuid.Nil {
		return ErrDeviceImportTopologyRunNotFound
	}
	now := r.now().UTC()
	result, err := r.db.Exec(
		`UPDATE device_import_topology_runs
		 SET backgrounded = TRUE,
		     auto_layout_allowed = CASE WHEN state = ? THEN FALSE ELSE auto_layout_allowed END,
		     completed_at = CASE WHEN state = ? THEN ? ELSE completed_at END,
		     state = CASE WHEN state = ? THEN ? ELSE state END,
		     updated_at = ?
		 WHERE id = ? AND actor_user_id = ? AND state IN (?, ?, ?, ?, ?, ?)`,
		string(domain.DeviceImportTopologyRunStateFailed),
		string(domain.DeviceImportTopologyRunStateFailed), now,
		string(domain.DeviceImportTopologyRunStateFailed), string(domain.DeviceImportTopologyRunStateCompleted),
		now, runID.String(), actorUserID.String(),
		string(domain.DeviceImportTopologyRunStateImporting),
		string(domain.DeviceImportTopologyRunStateDiscovering),
		string(domain.DeviceImportTopologyRunStateReconciling),
		string(domain.DeviceImportTopologyRunStateFollowup),
		string(domain.DeviceImportTopologyRunStateReadyForLayout),
		string(domain.DeviceImportTopologyRunStateFailed),
	)
	if err != nil {
		return fmt.Errorf("backgrounding topology run: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return ErrDeviceImportTopologyRunNotFound
	}
	return nil
}

// DisableAutoLayout prevents a late discovery completion from overwriting manual canvas edits.
func (r *DeviceImportTopologyRunRepo) DisableAutoLayout(ctx context.Context, runID, actorUserID uuid.UUID) error {
	if runID == uuid.Nil || actorUserID == uuid.Nil {
		return ErrDeviceImportTopologyRunNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting topology manual edit: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var stateRaw string
	if err := tx.QueryRowContext(ctx,
		`SELECT state FROM device_import_topology_runs
		 WHERE id = ? AND actor_user_id = ? FOR UPDATE`,
		runID.String(), actorUserID.String(),
	).Scan(&stateRaw); errors.Is(err, sql.ErrNoRows) {
		return ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return fmt.Errorf("locking topology manual edit: %w", err)
	}
	state := domain.DeviceImportTopologyRunState(stateRaw)
	if !state.Active() {
		return ErrDeviceImportTopologyRunNotFound
	}
	now := r.now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs
		 SET auto_layout_allowed = FALSE, state = ?, completed_at = ?, updated_at = ?
		 WHERE id = ?`,
		string(domain.DeviceImportTopologyRunStateCompleted), now, now, runID.String(),
	); err != nil {
		return fmt.Errorf("disabling topology auto layout: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing topology manual edit: %w", err)
	}
	return nil
}

// RequeueFollowupItems schedules at most one targeted second discovery attempt.
func (r *DeviceImportTopologyRunRepo) RequeueFollowupItems(
	ctx context.Context,
	runID uuid.UUID,
	candidates []uuid.UUID,
) ([]uuid.UUID, error) {
	if runID == uuid.Nil {
		return nil, ErrDeviceImportTopologyRunNotFound
	}
	deviceIDs := normalizedDeviceImportTopologyUUIDs(candidates)
	if len(deviceIDs) == 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting topology follow-up: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var stateRaw string
	if err := tx.QueryRowContext(ctx,
		`SELECT state FROM device_import_topology_runs WHERE id = ? FOR UPDATE`, runID.String(),
	).Scan(&stateRaw); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return nil, fmt.Errorf("locking topology follow-up run: %w", err)
	}
	if stateRaw != string(domain.DeviceImportTopologyRunStateReconciling) {
		return nil, ErrDeviceImportTopologyRunConflict
	}

	now := r.now().UTC()
	requeued := make([]uuid.UUID, 0, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		result, err := tx.ExecContext(ctx,
			`UPDATE device_import_topology_run_items
			 SET state = ?, result_code = '', message = '', error_reference = '', neighbor_count = 0,
			     links_created = 0, unresolved_neighbors = 0, started_at = NULL, completed_at = NULL, updated_at = ?
			 WHERE run_id = ? AND device_id = ? AND attempt < 2 AND state IN (?, ?, ?)`,
			string(domain.DeviceImportTopologyItemStateQueued), now, runID.String(), deviceID.String(),
			string(domain.DeviceImportTopologyItemStateSucceeded),
			string(domain.DeviceImportTopologyItemStateWarning),
			string(domain.DeviceImportTopologyItemStateFailed),
		)
		if err != nil {
			return nil, fmt.Errorf("requeueing topology follow-up item %s: %w", deviceID, err)
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE devices SET topology_discovery_mode = ?, topology_bootstrap_state = ?, updated_at = ? WHERE id = ?`,
			string(domain.TopologyDiscoveryModeBootstrapOnce), string(domain.TopologyBootstrapStatePending), now, deviceID.String(),
		); err != nil {
			return nil, fmt.Errorf("reopening topology follow-up bootstrap for %s: %w", deviceID, err)
		}
		requeued = append(requeued, deviceID)
	}
	if len(requeued) > 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE device_import_topology_runs SET state = ?, updated_at = ? WHERE id = ?`,
			string(domain.DeviceImportTopologyRunStateFollowup), now, runID.String(),
		); err != nil {
			return nil, fmt.Errorf("transitioning topology follow-up run: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing topology follow-up: %w", err)
	}
	r.publishDeviceUpdates(requeued)
	return requeued, nil
}

// RequeueItems resets selected terminal items and their one-shot device state for manual retry.
func (r *DeviceImportTopologyRunRepo) RequeueItems(
	ctx context.Context,
	runID, actorUserID uuid.UUID,
	requested []uuid.UUID,
) ([]uuid.UUID, error) {
	if runID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, ErrDeviceImportTopologyRunNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting topology retry: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var stateRaw string
	if err := tx.QueryRowContext(ctx,
		`SELECT state FROM device_import_topology_runs
		 WHERE id = ? AND actor_user_id = ? FOR UPDATE`, runID.String(), actorUserID.String(),
	).Scan(&stateRaw); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeviceImportTopologyRunNotFound
	} else if err != nil {
		return nil, fmt.Errorf("reading topology retry run: %w", err)
	}
	if !domain.DeviceImportTopologyRunState(stateRaw).Active() || stateRaw == string(domain.DeviceImportTopologyRunStateImporting) {
		return nil, ErrDeviceImportTopologyRunConflict
	}

	deviceIDs := normalizedDeviceImportTopologyUUIDs(requested)
	if len(deviceIDs) == 0 {
		rows, err := tx.Query(
			`SELECT device_id FROM device_import_topology_run_items
			 WHERE run_id = ? AND state IN (?, ?) ORDER BY device_id FOR UPDATE`,
			runID.String(), string(domain.DeviceImportTopologyItemStateWarning), string(domain.DeviceImportTopologyItemStateFailed),
		)
		if err != nil {
			return nil, fmt.Errorf("querying topology retry items: %w", err)
		}
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return nil, fmt.Errorf("reading topology retry item: %w", err)
			}
			id, err := uuid.Parse(raw)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("parsing topology retry item: %w", err)
			}
			deviceIDs = append(deviceIDs, id)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("closing topology retry items: %w", err)
		}
	}
	if len(deviceIDs) == 0 {
		return nil, ErrDeviceImportTopologyRunConflict
	}

	now := r.now().UTC()
	requeued := make([]uuid.UUID, 0, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		result, err := tx.ExecContext(ctx,
			`UPDATE device_import_topology_run_items
			 SET state = ?, result_code = '', message = '', error_reference = '', neighbor_count = 0,
			     links_created = 0, unresolved_neighbors = 0, started_at = NULL, completed_at = NULL, updated_at = ?
			 WHERE run_id = ? AND device_id = ? AND state IN (?, ?)`,
			string(domain.DeviceImportTopologyItemStateQueued), now, runID.String(), deviceID.String(),
			string(domain.DeviceImportTopologyItemStateWarning), string(domain.DeviceImportTopologyItemStateFailed),
		)
		if err != nil {
			return nil, fmt.Errorf("requeueing topology item %s: %w", deviceID, err)
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE devices SET topology_discovery_mode = ?, topology_bootstrap_state = ?, updated_at = ? WHERE id = ?`,
			string(domain.TopologyDiscoveryModeBootstrapOnce), string(domain.TopologyBootstrapStatePending), now, deviceID.String(),
		); err != nil {
			return nil, fmt.Errorf("reopening topology bootstrap for %s: %w", deviceID, err)
		}
		requeued = append(requeued, deviceID)
	}
	if len(requeued) == 0 {
		return nil, ErrDeviceImportTopologyRunConflict
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE device_import_topology_runs
		 SET state = ?, completed_at = NULL, updated_at = ? WHERE id = ?`,
		string(domain.DeviceImportTopologyRunStateDiscovering), now, runID.String(),
	); err != nil {
		return nil, fmt.Errorf("reopening topology run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing topology retry: %w", err)
	}
	r.publishDeviceUpdates(requeued)
	return requeued, nil
}

func normalizedDeviceImportTopologyUUIDs(values []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(values))
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if value == uuid.Nil {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].String() < result[j].String() })
	return result
}

func validDeviceImportTopologyRunTransition(current, next domain.DeviceImportTopologyRunState) bool {
	if current == next {
		return true
	}
	if next == domain.DeviceImportTopologyRunStateSuperseded {
		return current.Active()
	}
	switch current {
	case domain.DeviceImportTopologyRunStateDiscovering, domain.DeviceImportTopologyRunStateFollowup:
		return next == domain.DeviceImportTopologyRunStateReconciling
	case domain.DeviceImportTopologyRunStateReconciling:
		return next == domain.DeviceImportTopologyRunStateFollowup || next == domain.DeviceImportTopologyRunStateReadyForLayout
	case domain.DeviceImportTopologyRunStateReadyForLayout:
		return next == domain.DeviceImportTopologyRunStateCompleted
	default:
		return false
	}
}

func scanDeviceImportTopologyRun(scanner rowScanner, run *domain.DeviceImportTopologyRun) error {
	var idRaw, mapIDRaw, actorIDRaw, layoutScope, state, failureCode string
	var startedAt, completedAt sql.NullTime
	if err := scanner.Scan(
		&idRaw, &mapIDRaw, &actorIDRaw, &run.FileDigest, &layoutScope, &state,
		&run.AutoLayoutAllowed, &run.Backgrounded, &failureCode, &run.FailureMessage, &run.FailureReference,
		&run.ReconcileAttempts, &run.CreatedAt, &startedAt, &completedAt, &run.UpdatedAt,
	); err != nil {
		return err
	}
	var err error
	if run.ID, err = uuid.Parse(idRaw); err != nil {
		return err
	}
	if run.MapID, err = uuid.Parse(mapIDRaw); err != nil {
		return err
	}
	if run.ActorUserID, err = uuid.Parse(actorIDRaw); err != nil {
		return err
	}
	run.LayoutScope = domain.DeviceImportTopologyLayoutScope(layoutScope)
	run.State = domain.DeviceImportTopologyRunState(state)
	run.FailureCode = domain.DeviceImportTopologyResultCode(failureCode)
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}
	return nil
}

func scanDeviceImportTopologyRunItem(scanner rowScanner, item *domain.DeviceImportTopologyRunItem) error {
	var runIDRaw, deviceIDRaw, state, resultCode string
	var startedAt, completedAt sql.NullTime
	if err := scanner.Scan(
		&runIDRaw, &deviceIDRaw, &state, &item.Attempt, &resultCode, &item.Message, &item.Reference,
		&item.NeighborCount, &item.LinksCreated, &item.UnresolvedNeighbors, &startedAt, &completedAt, &item.UpdatedAt,
	); err != nil {
		return err
	}
	var err error
	if item.RunID, err = uuid.Parse(runIDRaw); err != nil {
		return err
	}
	if item.DeviceID, err = uuid.Parse(deviceIDRaw); err != nil {
		return err
	}
	item.State = domain.DeviceImportTopologyItemState(state)
	item.ResultCode = domain.DeviceImportTopologyResultCode(resultCode)
	if startedAt.Valid {
		item.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}
	return nil
}
