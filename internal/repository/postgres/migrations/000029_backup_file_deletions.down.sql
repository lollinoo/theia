DROP TRIGGER retain_deleted_backup_file ON backup_files;
DROP FUNCTION retain_deleted_backup_file();
DROP INDEX idx_backup_files_file_path;
DROP TABLE backup_file_deletions;
