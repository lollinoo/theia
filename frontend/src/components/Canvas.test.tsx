import { describe, it, expect } from 'vitest';

describe('Canvas context menu filtering', () => {
  // Replicate the filtering logic from Canvas.tsx
  const allItems = [
    { id: 'webfig', label: 'Open WebFig' },
    { id: 'grafana', label: 'Open in Grafana' },
    { id: 'interface-stats', label: 'Per-Interface Stats' },
    { id: 'configure', label: 'Configure' },
  ];

  function filterMenuItems(deviceType: string, ip?: string) {
    const isVirtual = deviceType === 'virtual';
    const virtualItemIds = new Set(ip ? ['grafana', 'configure'] : ['configure']);
    return isVirtual
      ? allItems.filter((item) => virtualItemIds.has(item.id))
      : allItems;
  }

  it('shows only Grafana and Configure for virtual devices with IP (VIRT-16)', () => {
    const items = filterMenuItems('virtual', '10.0.0.1');
    expect(items).toHaveLength(2);
    expect(items.map((i) => i.id)).toEqual(['grafana', 'configure']);
  });

  it('shows only Configure for virtual devices without IP', () => {
    const items = filterMenuItems('virtual', '');
    expect(items).toHaveLength(1);
    expect(items.map((i) => i.id)).toEqual(['configure']);
  });

  it('shows only Configure for virtual devices with undefined IP', () => {
    const items = filterMenuItems('virtual');
    expect(items).toHaveLength(1);
    expect(items.map((i) => i.id)).toEqual(['configure']);
  });

  it('shows all 4 items for physical devices', () => {
    const items = filterMenuItems('router', '10.0.0.1');
    expect(items).toHaveLength(4);
    expect(items.map((i) => i.id)).toEqual(['webfig', 'grafana', 'interface-stats', 'configure']);
  });

  it('shows all 4 items for switch devices', () => {
    const items = filterMenuItems('switch', '10.0.0.2');
    expect(items).toHaveLength(4);
  });
});
