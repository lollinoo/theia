CREATE TABLE IF NOT EXISTS backup_bulk_runs (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('running', 'success', 'partial', 'failed', 'cancelled', 'cancelling')),
    batch_size INTEGER NOT NULL DEFAULT 10 CHECK (batch_size > 0),
    total_count INTEGER NOT NULL DEFAULT 0 CHECK (total_count >= 0),
    queued_count INTEGER NOT NULL DEFAULT 0 CHECK (queued_count >= 0),
    success_count INTEGER NOT NULL DEFAULT 0 CHECK (success_count >= 0),
    failed_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    skipped_count INTEGER NOT NULL DEFAULT 0 CHECK (skipped_count >= 0),
    cancelled_count INTEGER NOT NULL DEFAULT 0 CHECK (cancelled_count >= 0),
    error_message TEXT NOT NULL DEFAULT '',
    cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS backup_bulk_runs_one_active
    ON backup_bulk_runs ((TRUE))
    WHERE status IN ('running', 'cancelling');

CREATE INDEX IF NOT EXISTS backup_bulk_runs_created_at_idx
    ON backup_bulk_runs(created_at DESC);

CREATE TABLE IF NOT EXISTS backup_bulk_run_items (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES backup_bulk_runs(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    device_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('checking', 'skipped', 'queued', 'running', 'success', 'failed', 'cancelled')),
    reason TEXT NOT NULL DEFAULT '',
    backup_job_id TEXT REFERENCES backup_jobs(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    UNIQUE (run_id, device_id)
);

CREATE INDEX IF NOT EXISTS backup_bulk_run_items_run_status_idx
    ON backup_bulk_run_items(run_id, status, created_at);

CREATE INDEX IF NOT EXISTS backup_bulk_run_items_backup_job_idx
    ON backup_bulk_run_items(backup_job_id)
    WHERE backup_job_id IS NOT NULL;
