CREATE TABLE IF NOT EXISTS ssh_credentials (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL DEFAULT 'admin',
    port INTEGER NOT NULL DEFAULT 22,
    auth_method TEXT NOT NULL DEFAULT 'password',
    encrypted_secret TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ssh_credentials_device_id ON ssh_credentials(device_id);

CREATE TABLE IF NOT EXISTS config_backups (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    method TEXT NOT NULL DEFAULT 'ssh',
    config_text TEXT NOT NULL DEFAULT '',
    config_hash TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    backup_type TEXT NOT NULL DEFAULT 'running',
    created_at DATETIME NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_config_backups_device_id ON config_backups(device_id);
CREATE INDEX IF NOT EXISTS idx_config_backups_created_at ON config_backups(created_at DESC);

CREATE TABLE IF NOT EXISTS vendor_configs (
    name TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    config_json TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
