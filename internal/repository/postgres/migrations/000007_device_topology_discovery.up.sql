ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS topology_discovery_mode TEXT NOT NULL DEFAULT 'inherit';

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS topology_bootstrap_state TEXT NOT NULL DEFAULT 'idle';

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS last_topology_discovery_at TIMESTAMPTZ;

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS last_topology_discovery_result TEXT NOT NULL DEFAULT '';
