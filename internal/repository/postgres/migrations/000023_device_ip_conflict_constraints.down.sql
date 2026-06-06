ALTER TABLE devices
DROP CONSTRAINT IF EXISTS devices_ip_physical_virtual_excl;

DROP INDEX IF EXISTS idx_devices_ip_virtualness_lookup;
DROP INDEX IF EXISTS idx_devices_ip;

CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_ip
ON devices(ip)
WHERE ip <> '' AND device_type <> 'virtual';
