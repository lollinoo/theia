CREATE TABLE device_credential_profiles_backup (
    device_id  TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    PRIMARY KEY (device_id, profile_id),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (profile_id) REFERENCES credential_profiles(id) ON DELETE CASCADE
);
INSERT INTO device_credential_profiles_backup SELECT device_id, profile_id, created_at FROM device_credential_profiles;
DROP TABLE device_credential_profiles;
ALTER TABLE device_credential_profiles_backup RENAME TO device_credential_profiles;
