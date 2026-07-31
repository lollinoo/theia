CREATE TABLE IF NOT EXISTS device_import_topology_runs (
    id TEXT PRIMARY KEY,
    map_id TEXT NOT NULL REFERENCES canvas_maps(id) ON DELETE CASCADE,
    actor_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_digest TEXT NOT NULL,
    layout_scope TEXT NOT NULL CHECK (layout_scope IN ('preserve', 'reorganize')),
    state TEXT NOT NULL CHECK (state IN ('importing', 'discovering', 'reconciling', 'followup', 'ready_for_layout', 'failed', 'completed', 'superseded')),
    auto_layout_allowed BOOLEAN NOT NULL DEFAULT TRUE,
    backgrounded BOOLEAN NOT NULL DEFAULT FALSE,
    layout_application_digest TEXT NOT NULL DEFAULT '',
    failure_code TEXT NOT NULL DEFAULT '',
    failure_message TEXT NOT NULL DEFAULT '',
    failure_reference TEXT NOT NULL DEFAULT '',
    reconcile_attempts INTEGER NOT NULL DEFAULT 0 CHECK (reconcile_attempts >= 0 AND reconcile_attempts <= 2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS device_import_topology_runs_one_active_per_map
    ON device_import_topology_runs(map_id)
    WHERE state IN ('importing', 'discovering', 'reconciling', 'followup', 'ready_for_layout', 'failed');

CREATE INDEX IF NOT EXISTS device_import_topology_runs_actor_map_created_idx
    ON device_import_topology_runs(actor_user_id, map_id, created_at DESC);

CREATE TABLE IF NOT EXISTS device_import_topology_run_items (
    run_id TEXT NOT NULL REFERENCES device_import_topology_runs(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'succeeded', 'warning', 'failed')),
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    result_code TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    error_reference TEXT NOT NULL DEFAULT '',
    neighbor_count INTEGER NOT NULL DEFAULT 0 CHECK (neighbor_count >= 0),
    links_created INTEGER NOT NULL DEFAULT 0 CHECK (links_created >= 0),
    unresolved_neighbors INTEGER NOT NULL DEFAULT 0 CHECK (unresolved_neighbors >= 0),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, device_id)
);

CREATE INDEX IF NOT EXISTS device_import_topology_run_items_run_state_idx
    ON device_import_topology_run_items(run_id, state, device_id);

CREATE INDEX IF NOT EXISTS device_import_topology_run_items_device_active_idx
    ON device_import_topology_run_items(device_id, state)
    WHERE state IN ('queued', 'running');
