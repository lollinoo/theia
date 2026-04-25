import { describe, expect, it, vi } from 'vitest';

import { buildDeviceContextMenuItems } from './canvas/canvasHelpers';

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
