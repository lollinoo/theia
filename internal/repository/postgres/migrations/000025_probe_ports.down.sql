ALTER TABLE device_addresses
    DROP COLUMN IF EXISTS probe_ports;

ALTER TABLE devices
    DROP COLUMN IF EXISTS probe_ports;
