ALTER TABLE devices
    DROP COLUMN IF EXISTS last_topology_discovery_result;

ALTER TABLE devices
    DROP COLUMN IF EXISTS last_topology_discovery_at;

ALTER TABLE devices
    DROP COLUMN IF EXISTS topology_bootstrap_state;

ALTER TABLE devices
    DROP COLUMN IF EXISTS topology_discovery_mode;
