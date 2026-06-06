/**
 * Exercises device card variant component behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import { resolveDeviceCardRenderModel, resolveDeviceCardVariant } from './deviceCardVariant';

describe('deviceCardVariant', () => {
  it('keeps physical devices on the physical card model', () => {
    expect(resolveDeviceCardVariant({ device_type: 'router', ip: '10.0.0.1' })).toBe('physical');

    expect(
      resolveDeviceCardRenderModel({
        device: { device_type: 'router', ip: '10.0.0.1' },
        monitoringState: 'monitorable',
        addressState: 'address',
        hasFreshnessMeta: true,
      }),
    ).toMatchObject({
      variant: 'physical',
      showOperationalReadouts: true,
      showFreshnessMeta: true,
    });
  });

  it('maps no-ip virtual nodes to the unmonitored card model', () => {
    expect(
      resolveDeviceCardRenderModel({
        device: { device_type: 'virtual', ip: '' },
        monitoringState: 'unmonitored',
        addressState: 'unmonitored',
        hasFreshnessMeta: true,
      }),
    ).toMatchObject({
      variant: 'virtual-unmonitored',
      showOperationalReadouts: false,
      showFreshnessMeta: false,
      showVirtualStatusBadge: false,
    });
  });

  it('maps virtual nodes with IP to the status-first virtual card model', () => {
    expect(
      resolveDeviceCardRenderModel({
        device: { device_type: 'virtual', ip: '192.168.1.1' },
        monitoringState: 'monitorable',
        addressState: 'address',
        hasFreshnessMeta: true,
      }),
    ).toMatchObject({
      variant: 'virtual-monitorable',
      showOperationalReadouts: false,
      showFreshnessMeta: true,
      showVirtualAddressChip: true,
      showVirtualStatusBadge: true,
    });
  });
});
