-- Reverse: drop join table
DROP TABLE IF EXISTS device_credential_profiles;

-- Reverse: drop role column (SQLite < 3.35 cannot DROP COLUMN, use 12-step recreation)
-- For simplicity and safety, create a temp table without role, copy data, swap
CREATE TABLE credential_profiles_backup (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT 'admin',
    port INTEGER NOT NULL DEFAULT 22,
    auth_method TEXT NOT NULL DEFAULT 'password',
    encrypted_secret TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
INSERT INTO credential_profiles_backup SELECT id, name, description, username, port, auth_method, encrypted_secret, created_at, updated_at FROM credential_profiles;
DROP TABLE credential_profiles;
ALTER TABLE credential_profiles_backup RENAME TO ssh_profiles;

-- Recreate original index
CREATE UNIQUE INDEX IF NOT EXISTS idx_ssh_profiles_name ON ssh_profiles(name);
