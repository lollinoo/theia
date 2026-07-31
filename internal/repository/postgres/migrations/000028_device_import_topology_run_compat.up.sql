ALTER TABLE device_import_topology_runs
    ADD COLUMN IF NOT EXISTS layout_application_digest TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS failure_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS failure_message TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS failure_reference TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reconcile_attempts INTEGER NOT NULL DEFAULT 0;

ALTER TABLE device_import_topology_runs
    DROP CONSTRAINT IF EXISTS device_import_topology_runs_reconcile_attempts_check;

ALTER TABLE device_import_topology_runs
    ADD CONSTRAINT device_import_topology_runs_reconcile_attempts_check
    CHECK (reconcile_attempts >= 0 AND reconcile_attempts <= 2);

ALTER TABLE device_import_topology_runs
    DROP CONSTRAINT IF EXISTS device_import_topology_runs_state_check;

ALTER TABLE device_import_topology_runs
    ADD CONSTRAINT device_import_topology_runs_state_check
    CHECK (state IN ('importing', 'discovering', 'reconciling', 'followup', 'ready_for_layout', 'failed', 'completed', 'superseded'));

DROP INDEX IF EXISTS device_import_topology_runs_one_active_per_map;

CREATE UNIQUE INDEX device_import_topology_runs_one_active_per_map
    ON device_import_topology_runs(map_id)
    WHERE state IN ('importing', 'discovering', 'reconciling', 'followup', 'ready_for_layout', 'failed');
