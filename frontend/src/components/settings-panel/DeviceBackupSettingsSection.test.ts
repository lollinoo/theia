/**
 * Exercises device backup settings section settings behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import { deviceBackupNextBackupText, formatDeviceInterval } from './DeviceBackupSettingsSection';

describe('device backup settings helpers', () => {
  it('formats configured intervals with the existing display labels', () => {
    expect(formatDeviceInterval(6)).toBe('6 hours');
    expect(formatDeviceInterval(24)).toBe('24 hours');
    expect(formatDeviceInterval(48)).toBe('48 hours');
    expect(formatDeviceInterval(168)).toBe('7 days');
  });

  it('describes disabled and enabled schedules', () => {
    expect(deviceBackupNextBackupText('0')).toBe('Scheduling disabled');
    expect(deviceBackupNextBackupText('24')).toBe('Backups run every 24 hours');
  });
});
