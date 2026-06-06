DROP INDEX IF EXISTS backup_bulk_runs_processing_lease_idx;

ALTER TABLE backup_bulk_runs
    DROP COLUMN IF EXISTS processing_lease_expires_at,
    DROP COLUMN IF EXISTS processing_owner;
