/**
 * Exercises detail subscription topology canvas behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';
import { getCanvasDetailDeviceId } from './detailSubscription';

describe('getCanvasDetailDeviceId', () => {
  it('returns device ID for deviceConfig', () => {
    expect(
      getCanvasDetailDeviceId({
        type: 'deviceConfig',
        data: { deviceId: 'dev-1' },
      }),
    ).toBe('dev-1');
  });

  it('returns device ID for deviceDetails', () => {
    expect(
      getCanvasDetailDeviceId({
        type: 'deviceDetails',
        data: { deviceId: 'dev-1' },
      }),
    ).toBe('dev-1');
  });

  it('does not subscribe detail telemetry for removed device-scoped interfaceStats', () => {
    expect(
      getCanvasDetailDeviceId({
        type: 'interfaceStats',
        data: { deviceId: 'dev-2' },
      }),
    ).toBeNull();
  });

  it('link-scoped interfaceStats returns null', () => {
    expect(
      getCanvasDetailDeviceId({
        type: 'interfaceStats',
        data: { linkId: 'link-1' },
      }),
    ).toBeNull();
  });

  it('returns null for non-device panels', () => {
    expect(getCanvasDetailDeviceId(null)).toBeNull();
    expect(getCanvasDetailDeviceId({ type: 'alerts' })).toBeNull();
    expect(getCanvasDetailDeviceId({ type: 'settings' })).toBeNull();
    expect(getCanvasDetailDeviceId({ type: 'addDevice' })).toBeNull();
    expect(getCanvasDetailDeviceId({ type: 'create-link' })).toBeNull();
    expect(getCanvasDetailDeviceId({ type: 'link-details' })).toBeNull();
    expect(getCanvasDetailDeviceId({ type: 'bulkEdit' })).toBeNull();
  });
});
