CREATE TABLE IF NOT EXISTS topology_observations (
    id TEXT PRIMARY KEY,
    local_device_id TEXT NOT NULL,
    remote_identity TEXT NOT NULL DEFAULT '',
    remote_device_id TEXT NOT NULL DEFAULT '',
    local_port TEXT NOT NULL DEFAULT '',
    remote_port TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL DEFAULT 'lldp',
    is_self_neighbor INTEGER NOT NULL DEFAULT 0,
    first_observed_at DATETIME NOT NULL,
    last_observed_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE(local_device_id, remote_identity, local_port, remote_port, protocol),
    FOREIGN KEY (local_device_id) REFERENCES devices(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_topology_observations_local_device_id ON topology_observations(local_device_id);
CREATE INDEX IF NOT EXISTS idx_topology_observations_remote_device_id ON topology_observations(remote_device_id);
CREATE INDEX IF NOT EXISTS idx_topology_observations_remote_identity_protocol ON topology_observations(remote_identity, protocol);

CREATE TABLE IF NOT EXISTS unresolved_neighbors (
    id TEXT PRIMARY KEY,
    local_device_id TEXT NOT NULL,
    remote_identity TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL DEFAULT 'lldp',
    occurrences INTEGER NOT NULL DEFAULT 1,
    first_observed_at DATETIME NOT NULL,
    last_observed_at DATETIME NOT NULL,
    resolved_at DATETIME NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE(local_device_id, remote_identity, protocol),
    FOREIGN KEY (local_device_id) REFERENCES devices(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_unresolved_neighbors_local_device_id ON unresolved_neighbors(local_device_id);
CREATE INDEX IF NOT EXISTS idx_unresolved_neighbors_active ON unresolved_neighbors(local_device_id, resolved_at);
