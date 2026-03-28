-- Re-add the single area_id column
ALTER TABLE devices ADD COLUMN area_id TEXT REFERENCES areas(id) ON DELETE SET NULL;

-- Copy back first area assignment (best-effort; multi-area info is lost)
UPDATE devices SET area_id = (
    SELECT area_id FROM device_areas WHERE device_areas.device_id = devices.id LIMIT 1
);

DROP TABLE IF EXISTS device_areas;
