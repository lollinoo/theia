CREATE INDEX IF NOT EXISTS idx_topology_observations_ingest_lookup
    ON topology_observations(local_device_id, remote_identity, local_port, remote_port, protocol);

CREATE INDEX IF NOT EXISTS idx_unresolved_neighbors_resolution_lookup
    ON unresolved_neighbors(local_device_id, remote_identity, protocol, resolved_at);
