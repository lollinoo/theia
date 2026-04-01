import { describe, it, expect } from 'vitest';

describe('Canvas context menu filtering', () => {
  // Replicate the filtering logic from Canvas.tsx
  const allItems = [
    { id: 'webfig', label: 'Open WebFig' },
    { id: 'grafana', label: 'Open in Grafana' },
    { id: 'interface-stats', label: 'Per-Interface Stats' },
    { id: 'configure', label: 'Configure' },
  ];
  const virtualItemIds = new Set(['grafana', 'configure']);

  function filterMenuItems(deviceType: string) {
    const isVirtual = deviceType === 'virtual';
    return isVirtual
      ? allItems.filter((item) => virtualItemIds.has(item.id))
      : allItems;
  }

  it('shows only Grafana and Configure for virtual devices (VIRT-16)', () => {
    const items = filterMenuItems('virtual');
    expect(items).toHaveLength(2);
    expect(items.map((i) => i.id)).toEqual(['grafana', 'configure']);
  });

  it('shows all 4 items for physical devices', () => {
    const items = filterMenuItems('router');
    expect(items).toHaveLength(4);
    expect(items.map((i) => i.id)).toEqual(['webfig', 'grafana', 'interface-stats', 'configure']);
  });

  it('shows all 4 items for switch devices', () => {
    const items = filterMenuItems('switch');
    expect(items).toHaveLength(4);
  });
});
