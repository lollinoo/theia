DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'backup_bulk_runs'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%status%'
      AND pg_get_constraintdef(oid) LIKE '%cancelling%';

    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE backup_bulk_runs DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

ALTER TABLE backup_bulk_runs
    ADD CONSTRAINT backup_bulk_runs_status_check
    CHECK (status IN ('running', 'pausing', 'paused', 'success', 'partial', 'failed', 'cancelled', 'cancelling'));

DROP INDEX IF EXISTS backup_bulk_runs_one_active;

CREATE UNIQUE INDEX backup_bulk_runs_one_active
    ON backup_bulk_runs ((TRUE))
    WHERE status IN ('running', 'pausing', 'paused', 'cancelling');
