ALTER TABLE devices ADD COLUMN topology_discovery_mode TEXT NOT NULL DEFAULT 'inherit';
ALTER TABLE devices ADD COLUMN topology_bootstrap_state TEXT NOT NULL DEFAULT 'idle';
ALTER TABLE devices ADD COLUMN last_topology_discovery_at DATETIME;
ALTER TABLE devices ADD COLUMN last_topology_discovery_result TEXT NOT NULL DEFAULT '';
