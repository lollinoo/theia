DROP INDEX IF EXISTS backup_bulk_runs_one_active;

UPDATE backup_bulk_runs
SET status = 'running'
WHERE status IN ('pausing', 'paused');

ALTER TABLE backup_bulk_runs
    DROP CONSTRAINT IF EXISTS backup_bulk_runs_status_check;

ALTER TABLE backup_bulk_runs
    ADD CONSTRAINT backup_bulk_runs_status_check
    CHECK (status IN ('running', 'success', 'partial', 'failed', 'cancelled', 'cancelling'));

CREATE UNIQUE INDEX backup_bulk_runs_one_active
    ON backup_bulk_runs ((TRUE))
    WHERE status IN ('running', 'cancelling');
