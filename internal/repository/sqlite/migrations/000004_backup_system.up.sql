CREATE TABLE IF NOT EXISTS backup_jobs (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_device_id ON backup_jobs(device_id);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_created_at ON backup_jobs(created_at DESC);

CREATE TABLE IF NOT EXISTS backup_files (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    file_type TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_hash TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (job_id) REFERENCES backup_jobs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_backup_files_job_id ON backup_files(job_id);
