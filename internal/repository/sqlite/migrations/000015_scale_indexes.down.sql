DROP INDEX IF EXISTS idx_links_pair_lookup;
DROP INDEX IF EXISTS idx_links_target_device_created_at;
DROP INDEX IF EXISTS idx_device_areas_area_id;

DROP INDEX IF EXISTS idx_interfaces_device_id_if_index;
CREATE INDEX IF NOT EXISTS idx_interfaces_device_id ON interfaces(device_id);

DROP INDEX IF EXISTS idx_devices_sys_name_lookup;
ALTER TABLE devices DROP COLUMN sys_name_lookup;
