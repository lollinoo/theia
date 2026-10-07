-- Keep cleanup paths outside the FK cascade: disk removal can fail or be interrupted.
CREATE TABLE backup_file_deletions (
    id TEXT PRIMARY KEY,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_backup_file_deletions_pending ON backup_file_deletions(next_attempt_at, id);
CREATE INDEX idx_backup_files_file_path ON backup_files(file_path);

CREATE FUNCTION retain_deleted_backup_file() RETURNS trigger AS $$
BEGIN
    IF btrim(OLD.file_path) <> '' THEN
        INSERT INTO backup_file_deletions(id, file_name, file_path)
        VALUES (OLD.id, OLD.file_name, OLD.file_path)
        ON CONFLICT (id) DO NOTHING;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER retain_deleted_backup_file
AFTER DELETE ON backup_files
FOR EACH ROW EXECUTE FUNCTION retain_deleted_backup_file();
