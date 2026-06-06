package postgres

// This file defines topology observation repo persistence behavior, ordering guarantees, and not-found conventions.

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/topology"
)

// TopologyObservationRepo represents topology observation repo data used by the persistence boundary.
type TopologyObservationRepo struct {
	db *DB
}

// NewTopologyObservationRepo constructs topology observation repo state for the persistence boundary.
func NewTopologyObservationRepo(db *sql.DB) *TopologyObservationRepo {
	return &TopologyObservationRepo{db: wrapDB(db)}
}

func (r *TopologyObservationRepo) UpsertObservation(observation *topology.Observation) error {
	return withWriteRetry(func() error {
		return r.upsertObservationOnce(observation)
	})
}

func (r *TopologyObservationRepo) upsertObservationOnce(observation *topology.Observation) error {
	now := time.Now().UTC()
	if observation.ID == uuid.Nil {
		observation.ID = uuid.New()
	}
	if observation.FirstObservedAt.IsZero() {
		observation.FirstObservedAt = observation.LastObservedAt
	}
	if observation.FirstObservedAt.IsZero() {
		observation.FirstObservedAt = now
	}
	if observation.LastObservedAt.IsZero() {
		observation.LastObservedAt = now
	}
	observation.UpdatedAt = now
	if observation.CreatedAt.IsZero() {
		observation.CreatedAt = now
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning observation upsert transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var existingID string
	var firstObservedAt time.Time
	err = tx.QueryRow(
		`SELECT id, first_observed_at FROM topology_observations
		 WHERE local_device_id = ? AND remote_identity = ? AND local_port = ? AND remote_port = ? AND protocol = ?`,
		observation.LocalDeviceID.String(),
		observation.RemoteIdentity,
		observation.LocalPort,
		observation.RemotePort,
		string(observation.Protocol),
	).Scan(&existingID, &firstObservedAt)
	if err == nil {
		observation.ID = uuid.MustParse(existingID)
		observation.FirstObservedAt = firstObservedAt
		_, err = tx.Exec(
			`UPDATE topology_observations
			 SET remote_device_id = ?, is_self_neighbor = ?, first_observed_at = ?, last_observed_at = ?, updated_at = ?
			 WHERE id = ?`,
			formatUUID(observation.RemoteDeviceID),
			observation.SelfNeighbor,
			observation.FirstObservedAt,
			observation.LastObservedAt,
			observation.UpdatedAt,
			observation.ID.String(),
		)
		if err != nil {
			return fmt.Errorf("updating topology observation: %w", err)
		}
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("querying topology observation: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO topology_observations
		 (id, local_device_id, remote_identity, remote_device_id, local_port, remote_port, protocol, is_self_neighbor,
		  first_observed_at, last_observed_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observation.ID.String(),
		observation.LocalDeviceID.String(),
		observation.RemoteIdentity,
		formatUUID(observation.RemoteDeviceID),
		observation.LocalPort,
		observation.RemotePort,
		string(observation.Protocol),
		observation.SelfNeighbor,
		observation.FirstObservedAt,
		observation.LastObservedAt,
		observation.CreatedAt,
		observation.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting topology observation: %w", err)
	}
	return tx.Commit()
}

// PruneLocalObservations deletes local observations for the selected protocols
// that are absent from the supplied current observation keys.
func (r *TopologyObservationRepo) PruneLocalObservations(localDeviceID uuid.UUID, protocols []domain.DiscoveryProtocol, keep []topology.Observation) (int, error) {
	deleted := 0
	err := withWriteRetry(func() error {
		var pruneErr error
		deleted, pruneErr = r.pruneLocalObservationsOnce(localDeviceID, protocols, keep)
		return pruneErr
	})
	return deleted, err
}

func (r *TopologyObservationRepo) pruneLocalObservationsOnce(localDeviceID uuid.UUID, protocols []domain.DiscoveryProtocol, keep []topology.Observation) (int, error) {
	if localDeviceID == uuid.Nil || len(protocols) == 0 {
		return 0, nil
	}

	normalizedProtocols := normalizeObservationProtocols(protocols)
	if len(normalizedProtocols) == 0 {
		return 0, nil
	}

	args := make([]interface{}, 0, 1+len(normalizedProtocols)+len(keep)*4)
	args = append(args, localDeviceID.String())

	protocolPlaceholders := make([]string, 0, len(normalizedProtocols))
	protocolSet := make(map[domain.DiscoveryProtocol]struct{}, len(normalizedProtocols))
	for _, protocol := range normalizedProtocols {
		protocolPlaceholders = append(protocolPlaceholders, "?")
		args = append(args, string(protocol))
		protocolSet[protocol] = struct{}{}
	}

	keepClauses := make([]string, 0, len(keep))
	seenKeep := make(map[string]struct{}, len(keep))
	for _, observation := range keep {
		if observation.LocalDeviceID != localDeviceID {
			continue
		}
		if _, ok := protocolSet[observation.Protocol]; !ok {
			continue
		}
		key := observation.RemoteIdentity + "\x00" + observation.LocalPort + "\x00" + observation.RemotePort + "\x00" + string(observation.Protocol)
		if _, ok := seenKeep[key]; ok {
			continue
		}
		seenKeep[key] = struct{}{}
		keepClauses = append(keepClauses, "(remote_identity = ? AND local_port = ? AND remote_port = ? AND protocol = ?)")
		args = append(args, observation.RemoteIdentity, observation.LocalPort, observation.RemotePort, string(observation.Protocol))
	}

	query := fmt.Sprintf(
		`DELETE FROM topology_observations
		 WHERE local_device_id = ? AND protocol IN (%s)`,
		strings.Join(protocolPlaceholders, ", "),
	)
	if len(keepClauses) > 0 {
		query += " AND NOT (" + strings.Join(keepClauses, " OR ") + ")"
	}

	result, err := r.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("pruning topology observations: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	return int(rowsAffected), nil
}

// ListObservationsForDevices lists observations for devices data from the persistence boundary.
func (r *TopologyObservationRepo) ListObservationsForDevices(deviceIDs []uuid.UUID) ([]topology.Observation, error) {
	if len(deviceIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(deviceIDs))
	args := make([]interface{}, 0, len(deviceIDs)*2)
	for _, id := range deviceIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id.String())
	}
	args = append(args, args[:len(deviceIDs)]...)

	rows, err := r.db.Query(
		fmt.Sprintf(
			`SELECT id, local_device_id, remote_identity, remote_device_id, local_port, remote_port, protocol, is_self_neighbor,
			        first_observed_at, last_observed_at, created_at, updated_at
			 FROM topology_observations
			 WHERE local_device_id IN (%s) OR remote_device_id IN (%s)
			 ORDER BY protocol, local_device_id, remote_identity, local_port, remote_port`,
			strings.Join(placeholders, ", "),
			strings.Join(placeholders, ", "),
		),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying topology observations: %w", err)
	}
	defer rows.Close()

	observations := make([]topology.Observation, 0)
	for rows.Next() {
		observation, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating topology observations: %w", err)
	}
	return observations, nil
}

func normalizeObservationProtocols(protocols []domain.DiscoveryProtocol) []domain.DiscoveryProtocol {
	seen := make(map[domain.DiscoveryProtocol]struct{}, len(protocols))
	for _, protocol := range protocols {
		if protocol == "" {
			continue
		}
		seen[protocol] = struct{}{}
	}
	normalized := make([]domain.DiscoveryProtocol, 0, len(seen))
	for protocol := range seen {
		normalized = append(normalized, protocol)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})
	return normalized
}

func (r *TopologyObservationRepo) UpsertUnresolvedNeighbor(neighbor *topology.UnresolvedNeighbor) error {
	return withWriteRetry(func() error {
		return r.upsertUnresolvedNeighborOnce(neighbor)
	})
}

func (r *TopologyObservationRepo) upsertUnresolvedNeighborOnce(neighbor *topology.UnresolvedNeighbor) error {
	now := time.Now().UTC()
	if neighbor.ID == uuid.Nil {
		neighbor.ID = uuid.New()
	}
	if neighbor.FirstObservedAt.IsZero() {
		neighbor.FirstObservedAt = neighbor.LastObservedAt
	}
	if neighbor.FirstObservedAt.IsZero() {
		neighbor.FirstObservedAt = now
	}
	if neighbor.LastObservedAt.IsZero() {
		neighbor.LastObservedAt = now
	}
	if neighbor.Occurrences <= 0 {
		neighbor.Occurrences = 1
	}
	neighbor.UpdatedAt = now
	if neighbor.CreatedAt.IsZero() {
		neighbor.CreatedAt = now
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning unresolved neighbor upsert transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var existingID string
	var existingOccurrences int
	var firstObservedAt time.Time
	err = tx.QueryRow(
		`SELECT id, occurrences, first_observed_at FROM unresolved_neighbors
		 WHERE local_device_id = ? AND remote_identity = ? AND protocol = ?`,
		neighbor.LocalDeviceID.String(),
		neighbor.RemoteIdentity,
		string(neighbor.Protocol),
	).Scan(&existingID, &existingOccurrences, &firstObservedAt)
	if err == nil {
		neighbor.ID = uuid.MustParse(existingID)
		neighbor.Occurrences = existingOccurrences + neighbor.Occurrences
		neighbor.FirstObservedAt = firstObservedAt
		_, err = tx.Exec(
			`UPDATE unresolved_neighbors
			 SET occurrences = ?, first_observed_at = ?, last_observed_at = ?, resolved_at = NULL, updated_at = ?
			 WHERE id = ?`,
			neighbor.Occurrences,
			neighbor.FirstObservedAt,
			neighbor.LastObservedAt,
			neighbor.UpdatedAt,
			neighbor.ID.String(),
		)
		if err != nil {
			return fmt.Errorf("updating unresolved neighbor: %w", err)
		}
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("querying unresolved neighbor: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO unresolved_neighbors
		 (id, local_device_id, remote_identity, protocol, occurrences, first_observed_at, last_observed_at, resolved_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		neighbor.ID.String(),
		neighbor.LocalDeviceID.String(),
		neighbor.RemoteIdentity,
		string(neighbor.Protocol),
		neighbor.Occurrences,
		neighbor.FirstObservedAt,
		neighbor.LastObservedAt,
		nil,
		neighbor.CreatedAt,
		neighbor.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting unresolved neighbor: %w", err)
	}
	return tx.Commit()
}

func (r *TopologyObservationRepo) ResolveUnresolvedNeighbor(localDeviceID uuid.UUID, remoteIdentity string, protocol domain.DiscoveryProtocol, resolvedAt time.Time) error {
	if remoteIdentity == "" {
		return nil
	}
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	_, err := r.db.Exec(
		`UPDATE unresolved_neighbors
		 SET resolved_at = ?, updated_at = ?
		 WHERE local_device_id = ? AND remote_identity = ? AND protocol = ?`,
		resolvedAt,
		resolvedAt,
		localDeviceID.String(),
		remoteIdentity,
		string(protocol),
	)
	if err != nil {
		return fmt.Errorf("resolving unresolved neighbor: %w", err)
	}
	return nil
}

// GetUnresolvedNeighborsByDeviceID retrieves unresolved neighbors by device id data from the persistence boundary.
func (r *TopologyObservationRepo) GetUnresolvedNeighborsByDeviceID(localDeviceID uuid.UUID) ([]topology.UnresolvedNeighbor, error) {
	rows, err := r.db.Query(
		`SELECT id, local_device_id, remote_identity, protocol, occurrences, first_observed_at, last_observed_at, resolved_at, created_at, updated_at
		 FROM unresolved_neighbors
		 WHERE local_device_id = ? AND resolved_at IS NULL
		 ORDER BY protocol, remote_identity`,
		localDeviceID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying unresolved neighbors: %w", err)
	}
	defer rows.Close()

	neighbors := make([]topology.UnresolvedNeighbor, 0)
	for rows.Next() {
		neighbor, err := scanUnresolvedNeighbor(rows)
		if err != nil {
			return nil, err
		}
		neighbors = append(neighbors, neighbor)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unresolved neighbors: %w", err)
	}
	return neighbors, nil
}

type observationScanner interface {
	Scan(dest ...interface{}) error
}

func scanObservation(scanner observationScanner) (topology.Observation, error) {
	var observation topology.Observation
	var idValue string
	var localDeviceID string
	var remoteDeviceID string
	var protocol string
	var selfNeighbor bool
	if err := scanner.Scan(
		&idValue,
		&localDeviceID,
		&observation.RemoteIdentity,
		&remoteDeviceID,
		&observation.LocalPort,
		&observation.RemotePort,
		&protocol,
		&selfNeighbor,
		&observation.FirstObservedAt,
		&observation.LastObservedAt,
		&observation.CreatedAt,
		&observation.UpdatedAt,
	); err != nil {
		return topology.Observation{}, fmt.Errorf("scanning topology observation: %w", err)
	}
	observation.ID = uuid.MustParse(idValue)
	observation.LocalDeviceID = uuid.MustParse(localDeviceID)
	observation.RemoteDeviceID = parseOptionalUUID(remoteDeviceID)
	observation.Protocol = domain.DiscoveryProtocol(protocol)
	observation.SelfNeighbor = selfNeighbor
	return observation, nil
}

func scanUnresolvedNeighbor(scanner observationScanner) (topology.UnresolvedNeighbor, error) {
	var neighbor topology.UnresolvedNeighbor
	var idValue string
	var localDeviceID string
	var protocol string
	var resolvedAt sql.NullTime
	if err := scanner.Scan(
		&idValue,
		&localDeviceID,
		&neighbor.RemoteIdentity,
		&protocol,
		&neighbor.Occurrences,
		&neighbor.FirstObservedAt,
		&neighbor.LastObservedAt,
		&resolvedAt,
		&neighbor.CreatedAt,
		&neighbor.UpdatedAt,
	); err != nil {
		return topology.UnresolvedNeighbor{}, fmt.Errorf("scanning unresolved neighbor: %w", err)
	}
	neighbor.ID = uuid.MustParse(idValue)
	neighbor.LocalDeviceID = uuid.MustParse(localDeviceID)
	neighbor.Protocol = domain.DiscoveryProtocol(protocol)
	if resolvedAt.Valid {
		neighbor.ResolvedAt = &resolvedAt.Time
	}
	return neighbor, nil
}

func formatUUID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func parseOptionalUUID(value string) uuid.UUID {
	if strings.TrimSpace(value) == "" {
		return uuid.Nil
	}
	return uuid.MustParse(value)
}
