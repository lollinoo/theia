PRAGMA foreign_keys=off;

-- SQLite 12-step table recreation: remove Phase 39 polling columns.
-- Step 1: Create new table without the Phase 39 columns.
CREATE TABLE devices_new (
    id TEXT PRIMARY KEY,
    hostname TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    snmp_credentials_json TEXT NOT NULL DEFAULT '{}',
    device_type TEXT NOT NULL DEFAULT 'unknown',
    status TEXT NOT NULL DEFAULT 'unknown',
    sys_name TEXT NOT NULL DEFAULT '',
    sys_name_lookup TEXT NOT NULL DEFAULT '',
    sys_descr TEXT NOT NULL DEFAULT '',
    sys_object_id TEXT NOT NULL DEFAULT '',
    hardware_model TEXT NOT NULL DEFAULT '',
    vendor TEXT NOT NULL DEFAULT 'default',
    managed INTEGER NOT NULL DEFAULT 0,
    tags_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    metrics_source TEXT NOT NULL DEFAULT 'prometheus',
    prometheus_label_name TEXT NOT NULL DEFAULT 'instance',
    prometheus_label_value TEXT NOT NULL DEFAULT ''
);

-- Step 2: Copy rows by explicit column name (no SELECT *)
INSERT INTO devices_new SELECT
    id, hostname, ip, snmp_credentials_json, device_type, status,
    sys_name, sys_name_lookup, sys_descr, sys_object_id, hardware_model,
    vendor, managed, tags_json, created_at, updated_at,
    metrics_source, prometheus_label_name, prometheus_label_value
FROM devices;

-- Step 3: Drop old table
DROP TABLE devices;

-- Step 4: Rename new table
ALTER TABLE devices_new RENAME TO devices;

-- Step 5: Recreate the indexes that lived on the original devices table.
-- The sys_name_lookup partial index was added in 000015 and must be
-- restored after the rebuild or GetBySysName regresses to a table scan.
-- IMPORTANT: This index definition must match
-- 000015_scale_indexes.up.sql exactly, including the partial-index WHERE
-- clause. If 000015 changes, update this rollback copy in lockstep.
CREATE INDEX IF NOT EXISTS idx_devices_sys_name_lookup
    ON devices(sys_name_lookup) WHERE sys_name_lookup != '';

PRAGMA foreign_keys=on;
