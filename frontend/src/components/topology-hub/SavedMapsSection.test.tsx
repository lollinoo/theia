/**
 * Exercises saved maps section topology hub behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { CanvasMap } from '../../types/api';
import { SavedMapsSection } from './SavedMapsSection';

function mockMap(overrides: Partial<CanvasMap> = {}): CanvasMap {
  return {
    id: 'map-1',
    name: 'Backbone',
    description: '',
    source_area_id: 'area-1',
    filter: { area_id: 'area-1' },
    is_default: false,
    device_count: 8,
    link_count: 3,
    position_count: 8,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    ...overrides,
  };
}

describe('SavedMapsSection', () => {
  it('renders loading error and empty states', () => {
    const props = {
      maps: [],
      selectedMapId: null,
      onCreateEmptyMap: vi.fn(),
      onSelectMap: vi.fn(),
      onOpenMap: vi.fn(),
      onRenameMap: vi.fn(),
      onDuplicateMap: vi.fn(),
      onDeleteMap: vi.fn(),
    };

    const { rerender } = render(<SavedMapsSection {...props} loading={true} error={null} />);
    expect(screen.getByText('Loading maps')).toBeInTheDocument();

    rerender(<SavedMapsSection {...props} loading={false} error="Could not load maps" />);
    expect(screen.getByText('Could not load maps')).toBeInTheDocument();

    rerender(<SavedMapsSection {...props} loading={false} error={null} />);
    expect(screen.getByText('No saved maps')).toBeInTheDocument();
  });

  it('renders maps as a vertical list and delegates callbacks', () => {
    const defaultMap = mockMap({ id: 'default', name: 'Default', is_default: true });
    const savedMap = mockMap({ id: 'map-2', name: 'Branch' });
    const onCreateEmptyMap = vi.fn();
    const onSelectMap = vi.fn();
    const onOpenMap = vi.fn();
    const onRenameMap = vi.fn();
    const onDuplicateMap = vi.fn();
    const onDeleteMap = vi.fn();

    render(
      <SavedMapsSection
        maps={[defaultMap, savedMap]}
        selectedMapId="map-2"
        loading={false}
        error={null}
        onCreateEmptyMap={onCreateEmptyMap}
        onSelectMap={onSelectMap}
        onOpenMap={onOpenMap}
        onRenameMap={onRenameMap}
        onDuplicateMap={onDuplicateMap}
        onDeleteMap={onDeleteMap}
      />,
    );

    expect(screen.getByText('Default')).toBeInTheDocument();
    expect(screen.getByText('Branch')).toBeInTheDocument();
    const list = screen.getByRole('list', { name: 'Saved maps list' });
    expect(list.className).toContain('flex');
    expect(list.className).toContain('divide-y');
    expect(list.className).not.toContain('grid');
    expect(within(list).getAllByRole('listitem')).toHaveLength(2);

    fireEvent.click(screen.getByRole('button', { name: 'Create empty map' }));
    fireEvent.click(screen.getByRole('button', { name: 'Select map Default' }));
    fireEvent.click(screen.getByRole('button', { name: 'Open map Branch' }));
    fireEvent.click(screen.getByRole('button', { name: 'Rename Branch' }));
    fireEvent.click(screen.getByRole('button', { name: 'Duplicate Branch' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete Branch' }));

    expect(onCreateEmptyMap).toHaveBeenCalledOnce();
    expect(onSelectMap).toHaveBeenCalledWith(defaultMap);
    expect(onOpenMap).toHaveBeenCalledWith(savedMap);
    expect(onRenameMap).toHaveBeenCalledWith(savedMap);
    expect(onDuplicateMap).toHaveBeenCalledWith(savedMap);
    expect(onDeleteMap).toHaveBeenCalledWith(savedMap);
    expect(screen.queryByRole('button', { name: 'Delete Default' })).not.toBeInTheDocument();
    expect(screen.getByRole('listitem', { name: 'Map Branch' })).toHaveClass('border-l-primary');
  });

  it('keeps cached maps visible while a background refresh is loading', () => {
    const savedMap = mockMap({ id: 'map-2', name: 'Branch' });

    render(
      <SavedMapsSection
        maps={[savedMap]}
        selectedMapId="map-2"
        loading={true}
        error={null}
        onCreateEmptyMap={vi.fn()}
        onSelectMap={vi.fn()}
        onOpenMap={vi.fn()}
        onRenameMap={vi.fn()}
        onDuplicateMap={vi.fn()}
        onDeleteMap={vi.fn()}
      />,
    );

    expect(screen.queryByText('Loading maps')).not.toBeInTheDocument();
    expect(screen.getByText('Branch')).toBeInTheDocument();
    expect(screen.getByText('Refreshing')).toBeInTheDocument();
  });
});
