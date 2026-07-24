CREATE TABLE IF NOT EXISTS canvas_map_link_routes (
    map_id TEXT NOT NULL REFERENCES canvas_maps(id) ON DELETE CASCADE,
    link_id TEXT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    route_version INTEGER NOT NULL CHECK (route_version = 1),
    waypoints_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (map_id, link_id)
);

CREATE INDEX IF NOT EXISTS idx_canvas_map_link_routes_link_id
ON canvas_map_link_routes(link_id);
