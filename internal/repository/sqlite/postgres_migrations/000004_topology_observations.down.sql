DROP INDEX IF EXISTS idx_unresolved_neighbors_active;
DROP INDEX IF EXISTS idx_unresolved_neighbors_local_device_id;
DROP TABLE IF EXISTS unresolved_neighbors;

DROP INDEX IF EXISTS idx_topology_observations_remote_identity_protocol;
DROP INDEX IF EXISTS idx_topology_observations_remote_device_id;
DROP INDEX IF EXISTS idx_topology_observations_local_device_id;
DROP TABLE IF EXISTS topology_observations;
