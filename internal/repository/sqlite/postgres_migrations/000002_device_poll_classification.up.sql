ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS poll_class TEXT NOT NULL DEFAULT 'standard';

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS poll_interval_override INTEGER;
