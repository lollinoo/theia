/**
 * Exercises canvas component behavior so refactors preserve the documented contract.
 */
import { describe, expect, it, vi } from 'vitest';
import { buildDeviceContextMenuItems, buildPositionPayload } from './canvas/canvasHelpers';
import type { DeviceNode } from './DeviceCard';

function buildItems(isVirtual: boolean) {
  return buildDeviceContextMenuItems({
    isVirtual,
    grafanaEnabled: true,
    winboxDisabled: false,
    onOpenWinbox: vi.fn(),
    onOpenGrafana: vi.fn(),
    onConfigure: vi.fn(),
  });
}

describe('Canvas context menu filtering', () => {
  it('shows only Configure for virtual devices', () => {
    const items = buildItems(true);

    expect(items.map((item) => item.id)).toEqual(['configure']);
  });

  it('does not expose a device-scoped Per-Interface Stats item for physical devices', () => {
    const items = buildItems(false);

    expect(items.map((item) => item.id)).toEqual(['winbox', 'grafana', 'configure']);
    expect(items.map((item) => item.label)).not.toContain('Per-Interface Stats');
  });
});

describe('Canvas position payload', () => {
  it('excludes ghost nodes from persisted positions', () => {
    const realNode = {
      id: 'real-device',
      position: { x: 10, y: 20 },
      data: { pinned: true },
    } as DeviceNode;
    const ghostNode = {
      id: 'ghost-device',
      position: { x: 30, y: 40 },
      data: { pinned: false, kind: 'ghost-device', isGhost: true },
    } as DeviceNode;

    expect(buildPositionPayload([realNode, ghostNode])).toEqual([
      { device_id: 'real-device', x: 10, y: 20, pinned: true },
    ]);
  });
});
