ALTER TABLE devices
    DROP COLUMN IF EXISTS poll_interval_override;

ALTER TABLE devices
    DROP COLUMN IF EXISTS poll_class;
