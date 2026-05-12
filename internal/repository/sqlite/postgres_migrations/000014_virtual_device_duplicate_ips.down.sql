DROP INDEX IF EXISTS idx_devices_ip;
CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_ip
ON devices(ip)
WHERE ip <> '';
