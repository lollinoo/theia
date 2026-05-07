import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { CanvasMap } from '../types/api';
import { MapSelector } from './MapSelector';

vi.mock('./MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span data-testid={`material-icon-${name}`} className="material-symbols-rounded">
      {name}
    </span>
  ),
}));

function mockMap(overrides: Partial<CanvasMap> = {}): CanvasMap {
  return {
    id: 'default',
    name: 'Default',
    description: '',
    source_area_id: null,
    filter: {},
    is_default: true,
    device_count: 0,
    link_count: 0,
    position_count: 0,
    created_at: '2026-05-07T00:00:00Z',
    updated_at: '2026-05-07T00:00:00Z',
    ...overrides,
  };
}

const maps = [
  mockMap(),
  mockMap({
    id: 'map-backbone',
    name: 'Backbone',
    description: 'Core network',
    source_area_id: 'area-core',
    filter: { area_id: 'area-core' },
    is_default: false,
    device_count: 3,
    link_count: 2,
    position_count: 3,
  }),
];

describe('MapSelector', () => {
  it('renders maps, shows the current map, and selects a saved map', () => {
    const onSelectMap = vi.fn();
    const onManageMaps = vi.fn();
    render(
      <MapSelector
        maps={maps}
        selectedMapId={null}
        selectedMapName="Default"
        onSelectMap={onSelectMap}
        onManageMaps={onManageMaps}
      />,
    );

    const button = screen.getByRole('button', { name: 'Select topology map' });
    expect(button).toHaveTextContent('Default');

    fireEvent.click(button);

    const menu = screen.getByRole('menu');
    expect(within(menu).getByRole('menuitem', { name: /Default/ })).toBeInTheDocument();
    fireEvent.click(within(menu).getByRole('menuitem', { name: /Backbone/ }));

    expect(onSelectMap).toHaveBeenCalledWith(maps[1]);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('closes the map menu with Escape', () => {
    render(
      <MapSelector
        maps={maps}
        selectedMapId="map-backbone"
        selectedMapName="Backbone"
        onSelectMap={vi.fn()}
        onManageMaps={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Select topology map' }));
    expect(screen.getByRole('menu')).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('uses a fallback default map and closes before managing maps', () => {
    const onManageMaps = vi.fn();
    render(
      <MapSelector
        maps={[maps[1]]}
        selectedMapId={null}
        selectedMapName="Default"
        onSelectMap={vi.fn()}
        onManageMaps={onManageMaps}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Select topology map' }));

    const menu = screen.getByRole('menu');
    expect(within(menu).getByRole('menuitem', { name: /Default/ })).toBeInTheDocument();
    fireEvent.click(within(menu).getByRole('menuitem', { name: /Manage maps/ }));

    expect(onManageMaps).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });
});
