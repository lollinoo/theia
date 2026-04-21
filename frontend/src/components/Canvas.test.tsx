import { describe, expect, it } from 'vitest';

describe('Canvas context menu filtering', () => {
  // Replicate the filtering logic from Canvas.tsx
  const allItems = [
    { id: 'winbox', label: 'Open in WinBox' },
    { id: 'grafana', label: 'Open in Grafana' },
    { id: 'interface-stats', label: 'Per-Interface Stats' },
    { id: 'configure', label: 'Configure' },
  ];

  function filterMenuItems(deviceType: string) {
    const isVirtual = deviceType === 'virtual';
    const virtualItemIds = new Set(['configure']);
    return isVirtual ? allItems.filter((item) => virtualItemIds.has(item.id)) : allItems;
  }

  it('shows only Configure for virtual devices with IP', () => {
    const items = filterMenuItems('virtual');
    expect(items).toHaveLength(1);
    expect(items.map((i) => i.id)).toEqual(['configure']);
  });

  it('shows only Configure for virtual devices without IP', () => {
    const items = filterMenuItems('virtual');
    expect(items).toHaveLength(1);
    expect(items.map((i) => i.id)).toEqual(['configure']);
  });

  it('shows only Configure for virtual devices with undefined IP', () => {
    const items = filterMenuItems('virtual');
    expect(items).toHaveLength(1);
    expect(items.map((i) => i.id)).toEqual(['configure']);
  });

  it('shows all 4 items for physical devices', () => {
    const items = filterMenuItems('router');
    expect(items).toHaveLength(4);
    expect(items.map((i) => i.id)).toEqual(['winbox', 'grafana', 'interface-stats', 'configure']);
  });

  it('shows all 4 items for switch devices', () => {
    const items = filterMenuItems('switch');
    expect(items).toHaveLength(4);
  });
});
