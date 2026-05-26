UPDATE backup_bulk_run_items
SET status = 'checking'
WHERE status = 'active';

ALTER TABLE backup_bulk_run_items
    DROP CONSTRAINT IF EXISTS backup_bulk_run_items_status_check;

ALTER TABLE backup_bulk_run_items
    ADD CONSTRAINT backup_bulk_run_items_status_check
    CHECK (status IN ('checking', 'skipped', 'queued', 'running', 'success', 'failed', 'cancelled'));
