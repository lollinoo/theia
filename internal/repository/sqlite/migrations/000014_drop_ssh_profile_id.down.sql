-- Reverse: re-add ssh_profile_id column (no data restoration needed;
-- device_credential_profiles join table is the source of truth).
ALTER TABLE devices ADD COLUMN ssh_profile_id TEXT DEFAULT NULL;
