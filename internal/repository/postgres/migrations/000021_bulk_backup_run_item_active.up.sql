DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'backup_bulk_run_items'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%status%'
      AND pg_get_constraintdef(oid) LIKE '%checking%'
      AND pg_get_constraintdef(oid) LIKE '%cancelled%';

    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE backup_bulk_run_items DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

ALTER TABLE backup_bulk_run_items
    ADD CONSTRAINT backup_bulk_run_items_status_check
    CHECK (status IN ('checking', 'skipped', 'active', 'queued', 'running', 'success', 'failed', 'cancelled'));
