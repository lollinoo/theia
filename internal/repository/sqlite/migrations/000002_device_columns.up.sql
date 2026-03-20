ALTER TABLE devices ADD COLUMN metrics_source TEXT NOT NULL DEFAULT 'prometheus';
ALTER TABLE devices ADD COLUMN prometheus_label_name TEXT NOT NULL DEFAULT 'instance';
ALTER TABLE devices ADD COLUMN prometheus_label_value TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN vendor TEXT NOT NULL DEFAULT 'default';
