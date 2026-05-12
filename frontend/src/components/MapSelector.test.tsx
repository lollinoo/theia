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
  mockMap({
    id: 'map-edge',
    name: 'Edge',
    description: 'Edge network',
    source_area_id: 'area-edge',
    filter: { area_id: 'area-edge' },
    is_default: false,
    device_count: 2,
    link_count: 1,
    position_count: 2,
  }),
];

describe('MapSelector', () => {
  it('includes the current map in the trigger accessible name while remaining discoverable', () => {
    render(
      <MapSelector
        maps={maps}
        selectedMapId="map-backbone"
        selectedMapName="Backbone"
        onSelectMap={vi.fn()}
        onManageMaps={vi.fn()}
      />,
    );

    const button = screen.getByRole('button', { name: /select topology map/i });

    expect(button).toHaveAccessibleName(/current map Backbone/i);
  });

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

    const button = screen.getByRole('button', { name: /select topology map/i });
    expect(button).toHaveTextContent('Default');

    fireEvent.click(button);

    const listbox = screen.getByRole('listbox');
    expect(within(listbox).getByRole('option', { name: /Default/ })).toBeInTheDocument();
    fireEvent.click(within(listbox).getByRole('option', { name: /Backbone/ }));

    expect(onSelectMap).toHaveBeenCalledWith(maps[1]);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  it('focuses the selected map option when opened', () => {
    render(
      <MapSelector
        maps={maps}
        selectedMapId="map-backbone"
        selectedMapName="Backbone"
        onSelectMap={vi.fn()}
        onManageMaps={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /select topology map/i }));

    const selectedOption = screen.getByRole('option', { name: /Backbone/ });
    expect(selectedOption).toHaveAttribute('aria-selected', 'true');
    expect(selectedOption).toHaveFocus();
  });

  it('moves focus across map options with ArrowDown, ArrowUp, Home, and End', () => {
    render(
      <MapSelector
        maps={maps}
        selectedMapId="map-backbone"
        selectedMapName="Backbone"
        onSelectMap={vi.fn()}
        onManageMaps={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /select topology map/i }));

    const defaultOption = screen.getByRole('option', { name: /Default/ });
    const backboneOption = screen.getByRole('option', { name: /Backbone/ });
    const edgeOption = screen.getByRole('option', { name: /Edge/ });

    expect(backboneOption).toHaveFocus();

    fireEvent.keyDown(backboneOption, { key: 'ArrowDown' });
    expect(edgeOption).toHaveFocus();

    fireEvent.keyDown(edgeOption, { key: 'ArrowUp' });
    expect(backboneOption).toHaveFocus();

    fireEvent.keyDown(backboneOption, { key: 'Home' });
    expect(defaultOption).toHaveFocus();

    fireEvent.keyDown(defaultOption, { key: 'End' });
    expect(edgeOption).toHaveFocus();
  });

  it.each([
    ['Enter', 'Enter'],
    ['Space', ' '],
  ])('selects the focused map with %s and closes', (_label, key) => {
    const onSelectMap = vi.fn();
    render(
      <MapSelector
        maps={maps}
        selectedMapId="map-backbone"
        selectedMapName="Backbone"
        onSelectMap={onSelectMap}
        onManageMaps={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /select topology map/i }));
    const backboneOption = screen.getByRole('option', { name: /Backbone/ });
    fireEvent.keyDown(backboneOption, { key: 'ArrowDown' });

    const edgeOption = screen.getByRole('option', { name: /Edge/ });
    expect(edgeOption).toHaveFocus();

    fireEvent.keyDown(edgeOption, { key });

    expect(onSelectMap).toHaveBeenCalledWith(maps[2]);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
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

    fireEvent.click(screen.getByRole('button', { name: /select topology map/i }));
    expect(screen.getByRole('listbox')).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
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

    fireEvent.click(screen.getByRole('button', { name: /select topology map/i }));

    const listbox = screen.getByRole('listbox');
    expect(within(listbox).getByRole('option', { name: /Default/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Manage maps/ }));

    expect(onManageMaps).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  it('preserves the desktop trigger cap while clamping the dropdown for narrow viewports', () => {
    render(
      <MapSelector
        maps={maps}
        selectedMapId="map-backbone"
        selectedMapName="Backbone"
        onSelectMap={vi.fn()}
        onManageMaps={vi.fn()}
      />,
    );

    const button = screen.getByRole('button', { name: /select topology map/i });
    expect(button.className).toContain('max-w-[min(15rem,calc(100vw-6rem))]');

    fireEvent.click(button);

    expect(screen.getByRole('listbox').parentElement?.className).toContain(
      'w-[min(16rem,calc(100vw-6rem))]',
    );
  });
});
