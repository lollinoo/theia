PRAGMA foreign_keys=off;

-- SQLite 12-step table recreation: drop ssh_profile_id column from devices.
-- Step 1: Create new table without ssh_profile_id
CREATE TABLE devices_new (
    id TEXT PRIMARY KEY,
    hostname TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    snmp_credentials_json TEXT NOT NULL DEFAULT '{}',
    device_type TEXT NOT NULL DEFAULT 'unknown',
    status TEXT NOT NULL DEFAULT 'unknown',
    sys_name TEXT NOT NULL DEFAULT '',
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

-- Step 2: Copy all rows by explicit column name (no SELECT *)
INSERT INTO devices_new SELECT
    id, hostname, ip, snmp_credentials_json, device_type, status,
    sys_name, sys_descr, sys_object_id, hardware_model, vendor, managed, tags_json,
    created_at, updated_at, metrics_source, prometheus_label_name, prometheus_label_value
FROM devices;

-- Step 3: Drop old table
DROP TABLE devices;

-- Step 4: Rename new table
ALTER TABLE devices_new RENAME TO devices;

PRAGMA foreign_keys=on;
