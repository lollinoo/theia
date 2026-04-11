CREATE TABLE IF NOT EXISTS devices (
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
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    metrics_source TEXT NOT NULL DEFAULT 'prometheus',
    prometheus_label_name TEXT NOT NULL DEFAULT 'instance',
    prometheus_label_value TEXT NOT NULL DEFAULT '',
    sys_name_lookup TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_ip ON devices(ip) WHERE ip <> '';
CREATE INDEX IF NOT EXISTS idx_devices_sys_name_lookup ON devices(sys_name_lookup) WHERE sys_name_lookup <> '';

CREATE TABLE IF NOT EXISTS interfaces (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    if_index INTEGER NOT NULL,
    if_name TEXT NOT NULL DEFAULT '',
    if_descr TEXT NOT NULL DEFAULT '',
    speed BIGINT NOT NULL DEFAULT 0,
    admin_status TEXT NOT NULL DEFAULT 'unknown',
    oper_status TEXT NOT NULL DEFAULT 'unknown',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_interfaces_device_id_if_index ON interfaces(device_id, if_index);

CREATE TABLE IF NOT EXISTS links (
    id TEXT PRIMARY KEY,
    source_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    source_if_name TEXT NOT NULL DEFAULT '',
    target_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    target_if_name TEXT NOT NULL DEFAULT '',
    discovery_protocol TEXT NOT NULL DEFAULT 'manual',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE(source_device_id, source_if_name, target_device_id, target_if_name)
);
CREATE INDEX IF NOT EXISTS idx_links_target_device_created_at ON links(target_device_id, created_at);
CREATE INDEX IF NOT EXISTS idx_links_pair_lookup ON links(source_device_id, target_device_id, target_if_name, source_if_name);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS device_positions (
    device_id TEXT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    x DOUBLE PRECISION NOT NULL DEFAULT 0,
    y DOUBLE PRECISION NOT NULL DEFAULT 0,
    pinned INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS snmp_profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    credentials_json TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_snmp_profiles_name ON snmp_profiles(name);

CREATE TABLE IF NOT EXISTS vendor_configs (
    name TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    config_json TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS areas (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '#00E676',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_areas_name ON areas(name);

CREATE TABLE IF NOT EXISTS device_areas (
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    area_id TEXT NOT NULL REFERENCES areas(id) ON DELETE CASCADE,
    PRIMARY KEY (device_id, area_id)
);
CREATE INDEX IF NOT EXISTS idx_device_areas_area_id ON device_areas(area_id);

CREATE TABLE IF NOT EXISTS credential_profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT 'admin',
    port INTEGER NOT NULL DEFAULT 22,
    auth_method TEXT NOT NULL DEFAULT 'password',
    encrypted_secret TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    role TEXT NOT NULL DEFAULT 'Admin'
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_credential_profiles_name ON credential_profiles(name);

CREATE TABLE IF NOT EXISTS device_credential_profiles (
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES credential_profiles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    is_winbox BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (device_id, profile_id)
);

CREATE TABLE IF NOT EXISTS backup_jobs (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_device_id ON backup_jobs(device_id);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_created_at ON backup_jobs(created_at DESC);

CREATE TABLE IF NOT EXISTS backup_files (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES backup_jobs(id) ON DELETE CASCADE,
    file_type TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_hash TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_backup_files_job_id ON backup_files(job_id);

CREATE TABLE IF NOT EXISTS instance_backups (
    id TEXT PRIMARY KEY,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    migration_version INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'running',
    error_message TEXT NOT NULL DEFAULT '',
    trigger_type TEXT NOT NULL DEFAULT 'manual',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_instance_backups_created_at ON instance_backups(created_at DESC);
