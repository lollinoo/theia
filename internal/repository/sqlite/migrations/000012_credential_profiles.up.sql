-- Step 1: Rename table (per D-01)
ALTER TABLE ssh_profiles RENAME TO credential_profiles;

-- Step 2: Add role column with default 'Admin' (per CRED-01, CRED-04)
ALTER TABLE credential_profiles ADD COLUMN role TEXT NOT NULL DEFAULT 'Admin';

-- Step 3: Recreate unique index with new name (per D-07)
DROP INDEX IF EXISTS idx_ssh_profiles_name;
CREATE UNIQUE INDEX idx_credential_profiles_name ON credential_profiles(name);

-- Step 4: Create join table (per D-08, D-09, CRED-02)
CREATE TABLE device_credential_profiles (
    device_id TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    PRIMARY KEY (device_id, profile_id),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (profile_id) REFERENCES credential_profiles(id) ON DELETE CASCADE
);

-- Step 5: Seed join table from existing FK (per D-05 step 4)
INSERT INTO device_credential_profiles (device_id, profile_id, created_at)
SELECT id, ssh_profile_id, CURRENT_TIMESTAMP FROM devices WHERE ssh_profile_id IS NOT NULL;
