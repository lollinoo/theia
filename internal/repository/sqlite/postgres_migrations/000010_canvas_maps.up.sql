CREATE TABLE IF NOT EXISTS canvas_maps (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    source_area_id TEXT NULL REFERENCES areas(id) ON DELETE SET NULL,
    filter_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_canvas_maps_default
ON canvas_maps(is_default)
WHERE is_default = TRUE;

CREATE INDEX IF NOT EXISTS idx_canvas_maps_source_area_id
ON canvas_maps(source_area_id);

CREATE TABLE IF NOT EXISTS canvas_map_positions (
    map_id TEXT NOT NULL REFERENCES canvas_maps(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    x DOUBLE PRECISION NOT NULL,
    y DOUBLE PRECISION NOT NULL,
    pinned BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (map_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_canvas_map_positions_device_id
ON canvas_map_positions(device_id);
