CREATE TABLE IF NOT EXISTS canvas_map_devices (
    map_id TEXT NOT NULL REFERENCES canvas_maps(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('base', 'ghost')),
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (map_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_canvas_map_devices_device_id
ON canvas_map_devices(device_id);

CREATE TABLE IF NOT EXISTS canvas_map_links (
    map_id TEXT NOT NULL REFERENCES canvas_maps(id) ON DELETE CASCADE,
    link_id TEXT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (map_id, link_id)
);

CREATE INDEX IF NOT EXISTS idx_canvas_map_links_link_id
ON canvas_map_links(link_id);

CREATE TABLE IF NOT EXISTS canvas_map_areas (
    map_id TEXT NOT NULL REFERENCES canvas_maps(id) ON DELETE CASCADE,
    area_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '',
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (map_id, area_id)
);
