ALTER TABLE devices ADD COLUMN sys_name_lookup TEXT NOT NULL DEFAULT '';

UPDATE devices
SET sys_name_lookup = CASE
    WHEN TRIM(sys_name) = '' THEN ''
    ELSE
        CASE
            WHEN INSTR(RTRIM(LOWER(TRIM(sys_name)), '.'), '.') > 0 THEN
                SUBSTR(
                    RTRIM(LOWER(TRIM(sys_name)), '.'),
                    1,
                    INSTR(RTRIM(LOWER(TRIM(sys_name)), '.'), '.') - 1
                )
            ELSE RTRIM(LOWER(TRIM(sys_name)), '.')
        END
END;

CREATE INDEX IF NOT EXISTS idx_devices_sys_name_lookup
    ON devices(sys_name_lookup) WHERE sys_name_lookup != '';

DROP INDEX IF EXISTS idx_interfaces_device_id;
CREATE INDEX IF NOT EXISTS idx_interfaces_device_id_if_index
    ON interfaces(device_id, if_index);

CREATE INDEX IF NOT EXISTS idx_device_areas_area_id
    ON device_areas(area_id);

CREATE INDEX IF NOT EXISTS idx_links_target_device_created_at
    ON links(target_device_id, created_at);

CREATE INDEX IF NOT EXISTS idx_links_pair_lookup
    ON links(source_device_id, target_device_id, target_if_name, source_if_name);
