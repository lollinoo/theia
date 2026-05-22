CREATE TABLE IF NOT EXISTS user_settings (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    locale TEXT NOT NULL DEFAULT 'en-US',
    bridge_port INTEGER NOT NULL DEFAULT 1337 CHECK (bridge_port BETWEEN 1 AND 65535),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bridge_credentials (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    secret_hash TEXT NOT NULL,
    secret_prefix TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rotated_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    last_used_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NULL,
    created_by_user_id TEXT NULL REFERENCES users(id) ON DELETE SET NULL,
    rotation_reason TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_bridge_credentials_user_id
ON bridge_credentials(user_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bridge_credentials_secret_prefix
ON bridge_credentials(secret_prefix);

CREATE INDEX IF NOT EXISTS idx_bridge_credentials_status
ON bridge_credentials(status);

CREATE INDEX IF NOT EXISTS idx_bridge_credentials_last_used_at
ON bridge_credentials(last_used_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bridge_credentials_one_active_per_user
ON bridge_credentials(user_id)
WHERE status = 'active';

CREATE TABLE IF NOT EXISTS bridge_launch_requests (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ NULL,
    consumed_by_credential_id TEXT NULL REFERENCES bridge_credentials(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_bridge_launch_requests_user_id
ON bridge_launch_requests(user_id);

CREATE INDEX IF NOT EXISTS idx_bridge_launch_requests_device_id
ON bridge_launch_requests(device_id);

CREATE INDEX IF NOT EXISTS idx_bridge_launch_requests_expires_at
ON bridge_launch_requests(expires_at);

CREATE TABLE IF NOT EXISTS bridge_connector_downloads (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connector_version TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL DEFAULT '',
    downloaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_bridge_connector_downloads_user_id
ON bridge_connector_downloads(user_id);

CREATE INDEX IF NOT EXISTS idx_bridge_connector_downloads_downloaded_at
ON bridge_connector_downloads(downloaded_at DESC);

DELETE FROM settings WHERE key = 'bridge_secret';
