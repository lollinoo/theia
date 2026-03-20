CREATE TABLE IF NOT EXISTS ssh_profiles (
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
CREATE UNIQUE INDEX IF NOT EXISTS idx_ssh_profiles_name ON ssh_profiles(name);
ALTER TABLE devices ADD COLUMN ssh_profile_id TEXT DEFAULT NULL;
