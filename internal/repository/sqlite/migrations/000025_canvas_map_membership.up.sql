CREATE TABLE IF NOT EXISTS canvas_map_devices (
    map_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('base', 'ghost')),
    added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (map_id, device_id),
    FOREIGN KEY (map_id) REFERENCES canvas_maps(id) ON DELETE CASCADE,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_canvas_map_devices_device_id
ON canvas_map_devices(device_id);

CREATE TABLE IF NOT EXISTS canvas_map_links (
    map_id TEXT NOT NULL,
    link_id TEXT NOT NULL,
    added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (map_id, link_id),
    FOREIGN KEY (map_id) REFERENCES canvas_maps(id) ON DELETE CASCADE,
    FOREIGN KEY (link_id) REFERENCES links(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_canvas_map_links_link_id
ON canvas_map_links(link_id);

CREATE TABLE IF NOT EXISTS canvas_map_areas (
    map_id TEXT NOT NULL,
    area_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '',
    added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (map_id, area_id),
    FOREIGN KEY (map_id) REFERENCES canvas_maps(id) ON DELETE CASCADE
);
