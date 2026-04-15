-- Phase 39: Add poll classification columns to devices table.
-- poll_class is NOT NULL with a SQL default of 'standard' so existing rows
-- receive a safe value before the Go-level data migration in
-- migrateDevicePollClass refines them via domain.ClassifyPollClass.
ALTER TABLE devices ADD COLUMN poll_class TEXT NOT NULL DEFAULT 'standard';

-- poll_interval_override is nullable (seconds). NULL = use class default;
-- a non-null integer overrides the performance interval for this device.
-- Validation (e.g., minimum 10s) belongs to the API layer in Phase 40+.
ALTER TABLE devices ADD COLUMN poll_interval_override INTEGER;
