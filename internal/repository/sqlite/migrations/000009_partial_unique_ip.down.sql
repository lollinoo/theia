DROP INDEX IF EXISTS idx_devices_ip;
CREATE UNIQUE INDEX idx_devices_ip ON devices(ip);
