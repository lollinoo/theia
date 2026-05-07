import { fireEvent, render, screen } from '@testing-library/react';
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
      onOpenMap: vi.fn(),
      onDuplicateMap: vi.fn(),
      onDeleteMap: vi.fn(),
    };

    const { rerender } = render(
      <SavedMapsSection {...props} loading={true} error={null} />,
    );
    expect(screen.getByText('Loading maps')).toBeInTheDocument();

    rerender(<SavedMapsSection {...props} loading={false} error="Could not load maps" />);
    expect(screen.getByText('Could not load maps')).toBeInTheDocument();

    rerender(<SavedMapsSection {...props} loading={false} error={null} />);
    expect(screen.getByText('No saved maps')).toBeInTheDocument();
  });

  it('renders map cards and delegates callbacks', () => {
    const defaultMap = mockMap({ id: 'default', name: 'Default', is_default: true });
    const savedMap = mockMap({ id: 'map-2', name: 'Branch' });
    const onOpenMap = vi.fn();
    const onDuplicateMap = vi.fn();
    const onDeleteMap = vi.fn();

    render(
      <SavedMapsSection
        maps={[defaultMap, savedMap]}
        loading={false}
        error={null}
        onOpenMap={onOpenMap}
        onDuplicateMap={onDuplicateMap}
        onDeleteMap={onDeleteMap}
      />,
    );

    expect(screen.getByText('Default')).toBeInTheDocument();
    expect(screen.getByText('Branch')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Open map Branch' }));
    fireEvent.click(screen.getByRole('button', { name: 'Duplicate Branch' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete Branch' }));

    expect(onOpenMap).toHaveBeenCalledWith(savedMap);
    expect(onDuplicateMap).toHaveBeenCalledWith(savedMap);
    expect(onDeleteMap).toHaveBeenCalledWith(savedMap);
    expect(screen.queryByRole('button', { name: 'Delete Default' })).not.toBeInTheDocument();
  });
});
