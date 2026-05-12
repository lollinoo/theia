CREATE TABLE IF NOT EXISTS canvas_maps (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    source_area_id TEXT NULL,
    filter_json TEXT NOT NULL DEFAULT '{}',
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (source_area_id) REFERENCES areas(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_canvas_maps_default
ON canvas_maps(is_default)
WHERE is_default = 1;

CREATE INDEX IF NOT EXISTS idx_canvas_maps_source_area_id
ON canvas_maps(source_area_id);

CREATE TABLE IF NOT EXISTS canvas_map_positions (
    map_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    x REAL NOT NULL,
    y REAL NOT NULL,
    pinned INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (map_id, device_id),
    FOREIGN KEY (map_id) REFERENCES canvas_maps(id) ON DELETE CASCADE,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_canvas_map_positions_device_id
ON canvas_map_positions(device_id);
