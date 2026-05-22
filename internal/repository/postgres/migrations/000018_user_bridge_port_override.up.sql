ALTER TABLE user_settings
    ADD COLUMN bridge_port_override INTEGER NULL CHECK (
        bridge_port_override IS NULL OR bridge_port_override BETWEEN 1 AND 65535
    );

UPDATE user_settings
SET bridge_port_override = bridge_port
WHERE bridge_port <> 1337;
