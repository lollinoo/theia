CREATE INDEX IF NOT EXISTS idx_unresolved_neighbors_active_lookup
    ON unresolved_neighbors(local_device_id, remote_identity, protocol)
    WHERE resolved_at IS NULL;
