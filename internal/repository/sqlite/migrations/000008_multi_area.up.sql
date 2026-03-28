-- Junction table for many-to-many device↔area relationship
CREATE TABLE IF NOT EXISTS device_areas (
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    area_id   TEXT NOT NULL REFERENCES areas(id) ON DELETE CASCADE,
    PRIMARY KEY (device_id, area_id)
);

-- Migrate existing single-area assignments into the junction table
INSERT INTO device_areas (device_id, area_id)
SELECT id, area_id FROM devices WHERE area_id IS NOT NULL AND area_id != '';

-- Drop the old single-area FK column
ALTER TABLE devices DROP COLUMN area_id;
