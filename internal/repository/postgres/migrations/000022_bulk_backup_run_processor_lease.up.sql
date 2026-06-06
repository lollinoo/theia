ALTER TABLE backup_bulk_runs
    ADD COLUMN IF NOT EXISTS processing_owner TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS processing_lease_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS backup_bulk_runs_processing_lease_idx
    ON backup_bulk_runs(processing_lease_expires_at)
    WHERE processing_owner <> '';
