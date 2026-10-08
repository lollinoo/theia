package postgres

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/topology"
)

const discoveryBatchSize = 128

type discoveryObservationKey struct {
	local                           uuid.UUID
	identity, localPort, remotePort string
	protocol                        domain.DiscoveryProtocol
}

type discoveryNeighborKey struct {
	local    uuid.UUID
	identity string
	protocol domain.DiscoveryProtocol
}

// UpsertDiscoveryObservations stores a discovery's observations and unresolved
// neighbor changes atomically, in bounded statements. Duplicate observation
// keys keep the last result; unresolved occurrences still count every sighting.
func (r *TopologyObservationRepo) UpsertDiscoveryObservations(observations []topology.Observation) error {
	if len(observations) == 0 {
		return nil
	}
	return r.upsertDiscoveryObservationsOnce(observations)
}

func (r *TopologyObservationRepo) upsertDiscoveryObservationsOnce(observations []topology.Observation) error {
	now := time.Now().UTC()
	unique := make([]topology.Observation, 0, len(observations))
	indices := make(map[discoveryObservationKey]int, len(observations))
	unresolved := make(map[discoveryNeighborKey]topology.UnresolvedNeighbor)
	lastNeighbor := make(map[discoveryNeighborKey]topology.Observation)
	for _, observation := range observations {
		if observation.ID == uuid.Nil {
			observation.ID = uuid.New()
		}
		if observation.LastObservedAt.IsZero() {
			observation.LastObservedAt = now
		}
		if observation.FirstObservedAt.IsZero() {
			observation.FirstObservedAt = observation.LastObservedAt
		}
		if observation.CreatedAt.IsZero() {
			observation.CreatedAt = now
		}
		observation.UpdatedAt = now
		key := discoveryObservationKey{observation.LocalDeviceID, observation.RemoteIdentity, observation.LocalPort, observation.RemotePort, observation.Protocol}
		if index, ok := indices[key]; ok {
			observation.ID = unique[index].ID
			observation.FirstObservedAt = unique[index].FirstObservedAt
			observation.CreatedAt = unique[index].CreatedAt
			unique[index] = observation
		} else {
			indices[key] = len(unique)
			unique = append(unique, observation)
		}
		neighborKey := discoveryNeighborKey{observation.LocalDeviceID, observation.RemoteIdentity, observation.Protocol}
		lastNeighbor[neighborKey] = observation
		if observation.RemoteDeviceID == uuid.Nil {
			neighbor, exists := unresolved[neighborKey]
			if !exists {
				neighbor = topology.UnresolvedNeighbor{ID: uuid.New(), LocalDeviceID: observation.LocalDeviceID, RemoteIdentity: observation.RemoteIdentity, Protocol: observation.Protocol, FirstObservedAt: observation.FirstObservedAt, CreatedAt: now}
			}
			neighbor.Occurrences++
			neighbor.LastObservedAt = observation.LastObservedAt
			neighbor.UpdatedAt = now
			unresolved[neighborKey] = neighbor
		}
	}
	sort.Slice(unique, func(i, j int) bool {
		a, b := unique[i], unique[j]
		if a.LocalDeviceID != b.LocalDeviceID {
			return a.LocalDeviceID.String() < b.LocalDeviceID.String()
		}
		if a.RemoteIdentity != b.RemoteIdentity {
			return a.RemoteIdentity < b.RemoteIdentity
		}
		if a.LocalPort != b.LocalPort {
			return a.LocalPort < b.LocalPort
		}
		if a.RemotePort != b.RemotePort {
			return a.RemotePort < b.RemotePort
		}
		return a.Protocol < b.Protocol
	})

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning discovery batch: %w", err)
	}
	defer tx.Rollback()
	if err := execDiscoveryJSONBatches(tx, unique, `INSERT INTO topology_observations
		(id,local_device_id,remote_identity,remote_device_id,local_port,remote_port,protocol,is_self_neighbor,first_observed_at,last_observed_at,created_at,updated_at)
		SELECT "ID","LocalDeviceID","RemoteIdentity",CASE WHEN "RemoteDeviceID" = '00000000-0000-0000-0000-000000000000' THEN '' ELSE "RemoteDeviceID" END,"LocalPort","RemotePort","Protocol","SelfNeighbor","FirstObservedAt","LastObservedAt","CreatedAt","UpdatedAt"
		FROM jsonb_to_recordset(?::jsonb) AS input("ID" text,"LocalDeviceID" text,"RemoteIdentity" text,"RemoteDeviceID" text,"LocalPort" text,"RemotePort" text,"Protocol" text,"SelfNeighbor" boolean,"FirstObservedAt" timestamptz,"LastObservedAt" timestamptz,"CreatedAt" timestamptz,"UpdatedAt" timestamptz)
		ON CONFLICT(local_device_id,remote_identity,local_port,remote_port,protocol) DO UPDATE
		SET remote_device_id=EXCLUDED.remote_device_id,is_self_neighbor=EXCLUDED.is_self_neighbor,last_observed_at=EXCLUDED.last_observed_at,updated_at=EXCLUDED.updated_at`); err != nil {
		return fmt.Errorf("upserting discovery observations: %w", err)
	}
	neighbors := make([]topology.UnresolvedNeighbor, 0, len(lastNeighbor))
	for key, last := range lastNeighbor {
		neighbor, ok := unresolved[key]
		if !ok {
			neighbor = topology.UnresolvedNeighbor{ID: uuid.New(), LocalDeviceID: last.LocalDeviceID, RemoteIdentity: last.RemoteIdentity, Protocol: last.Protocol, FirstObservedAt: now, LastObservedAt: now, CreatedAt: now}
		}
		if last.RemoteDeviceID != uuid.Nil {
			resolvedAt := last.LastObservedAt
			neighbor.ResolvedAt = &resolvedAt
			neighbor.UpdatedAt = resolvedAt
		}
		neighbors = append(neighbors, neighbor)
	}
	// Known-only resolutions and unknown sightings acquire neighbor row locks
	// in one phase and in the same key order, even for overlapping discoveries.
	sort.Slice(neighbors, func(i, j int) bool {
		a, b := neighbors[i], neighbors[j]
		if a.LocalDeviceID != b.LocalDeviceID {
			return a.LocalDeviceID.String() < b.LocalDeviceID.String()
		}
		if a.RemoteIdentity != b.RemoteIdentity {
			return a.RemoteIdentity < b.RemoteIdentity
		}
		return a.Protocol < b.Protocol
	})
	if err := execDiscoveryJSONBatches(tx, neighbors, `INSERT INTO unresolved_neighbors
		(id,local_device_id,remote_identity,protocol,occurrences,first_observed_at,last_observed_at,resolved_at,created_at,updated_at)
		SELECT input."ID",input."LocalDeviceID",input."RemoteIdentity",input."Protocol",input."Occurrences",input."FirstObservedAt",input."LastObservedAt",input."ResolvedAt",input."CreatedAt",input."UpdatedAt"
		FROM jsonb_to_recordset(?::jsonb) AS input("ID" text,"LocalDeviceID" text,"RemoteIdentity" text,"Protocol" text,"Occurrences" integer,"FirstObservedAt" timestamptz,"LastObservedAt" timestamptz,"ResolvedAt" timestamptz,"CreatedAt" timestamptz,"UpdatedAt" timestamptz)
		WHERE input."Occurrences" > 0 OR EXISTS(SELECT 1 FROM unresolved_neighbors n WHERE n.local_device_id=input."LocalDeviceID" AND n.remote_identity=input."RemoteIdentity" AND n.protocol=input."Protocol")
		ORDER BY input."LocalDeviceID",input."RemoteIdentity",input."Protocol"
		ON CONFLICT(local_device_id,remote_identity,protocol) DO UPDATE
		SET occurrences=unresolved_neighbors.occurrences+EXCLUDED.occurrences,
		last_observed_at=CASE WHEN EXCLUDED.occurrences>0 THEN EXCLUDED.last_observed_at ELSE unresolved_neighbors.last_observed_at END,
		resolved_at=EXCLUDED.resolved_at,updated_at=EXCLUDED.updated_at`); err != nil {
		return fmt.Errorf("upserting discovery neighbor changes: %w", err)
	}
	return tx.Commit()
}

func execDiscoveryJSONBatches[T any](tx *Tx, values []T, query string) error {
	for start := 0; start < len(values); start += discoveryBatchSize {
		payload, err := json.Marshal(values[start:min(start+discoveryBatchSize, len(values))])
		if err != nil {
			return err
		}
		if _, err := tx.Exec(query, string(payload)); err != nil {
			return err
		}
	}
	return nil
}
