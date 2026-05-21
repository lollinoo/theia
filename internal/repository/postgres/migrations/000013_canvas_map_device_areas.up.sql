CREATE TABLE IF NOT EXISTS canvas_map_device_areas (
    map_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    area_id TEXT NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (map_id, device_id, area_id),
    FOREIGN KEY (map_id, device_id) REFERENCES canvas_map_devices(map_id, device_id) ON DELETE CASCADE,
    FOREIGN KEY (map_id, area_id) REFERENCES canvas_map_areas(map_id, area_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_canvas_map_device_areas_area_id
ON canvas_map_device_areas(map_id, area_id);
