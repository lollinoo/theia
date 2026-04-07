-- SQLite does not support DROP COLUMN before 3.35.0; recreate table without trigger_type
CREATE TABLE instance_backups_backup AS SELECT id, file_name, file_path, size_bytes, sha256, app_version, migration_version, status, error_message, created_at FROM instance_backups;
DROP TABLE instance_backups;
ALTER TABLE instance_backups_backup RENAME TO instance_backups;
CREATE INDEX IF NOT EXISTS idx_instance_backups_created_at ON instance_backups(created_at DESC);
