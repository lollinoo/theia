CREATE TABLE IF NOT EXISTS device_addresses (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    address TEXT NOT NULL DEFAULT '',
    normalized_address TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'other' CHECK (role IN ('primary', 'management', 'backup', 'monitoring', 'other')),
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    priority INTEGER NOT NULL DEFAULT 100,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (device_id, normalized_address)
);

CREATE INDEX IF NOT EXISTS idx_device_addresses_device_id
ON device_addresses(device_id);

CREATE INDEX IF NOT EXISTS idx_device_addresses_normalized_address
ON device_addresses(normalized_address)
WHERE normalized_address <> '';

CREATE INDEX IF NOT EXISTS idx_device_addresses_device_role_priority
ON device_addresses(device_id, role, priority);

CREATE UNIQUE INDEX IF NOT EXISTS idx_device_addresses_one_primary
ON device_addresses(device_id)
WHERE is_primary = TRUE;

INSERT INTO device_addresses (
    id, device_id, address, normalized_address, label, role, is_primary, priority, created_at, updated_at
)
SELECT
    id,
    id,
    btrim(ip),
    lower(btrim(ip)),
    'Primary',
    'primary',
    TRUE,
    0,
    created_at,
    updated_at
FROM devices
WHERE btrim(ip) <> ''
ON CONFLICT (device_id, normalized_address) DO NOTHING;
