CREATE EXTENSION IF NOT EXISTS btree_gist;

DROP INDEX IF EXISTS idx_devices_ip;
CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_ip
ON devices (lower(btrim(ip)))
WHERE btrim(ip) <> '' AND device_type <> 'virtual';

CREATE INDEX IF NOT EXISTS idx_devices_ip_virtualness_lookup
ON devices (lower(btrim(ip)), ((CASE WHEN device_type = 'virtual' THEN 1 ELSE 0 END)))
WHERE btrim(ip) <> '';

ALTER TABLE devices
DROP CONSTRAINT IF EXISTS devices_ip_physical_virtual_excl;

ALTER TABLE devices
ADD CONSTRAINT devices_ip_physical_virtual_excl
EXCLUDE USING gist (
    (lower(btrim(ip))) WITH =,
    ((CASE WHEN device_type = 'virtual' THEN 1 ELSE 0 END)) WITH <>
)
WHERE (btrim(ip) <> '');
