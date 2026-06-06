/**
 * Exercises map summary card topology hub behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { CanvasMap } from '../../types/api';
import { MapSummaryCard } from './MapSummaryCard';

function mockMap(overrides: Partial<CanvasMap> = {}): CanvasMap {
  return {
    id: 'map-1',
    name: 'Backbone',
    description: 'Core map',
    source_area_id: 'area-1',
    filter: { area_id: 'area-1' },
    is_default: false,
    device_count: 12,
    link_count: 5,
    position_count: 9,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    ...overrides,
  };
}

describe('MapSummaryCard', () => {
  it('renders map identity and counts', () => {
    render(
      <MapSummaryCard
        map={mockMap()}
        selected={false}
        onSelect={vi.fn()}
        onOpen={vi.fn()}
        onRename={vi.fn()}
        onDuplicate={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(screen.getByText('Backbone')).toBeInTheDocument();
    expect(screen.getByText('12')).toBeInTheDocument();
    expect(screen.getByText('5')).toBeInTheDocument();
    expect(screen.getByText('9')).toBeInTheDocument();
  });

  it('selects from the full row body and keeps action buttons separate', () => {
    const map = mockMap();
    const onSelect = vi.fn();
    const onOpen = vi.fn();
    const onRename = vi.fn();
    const onDuplicate = vi.fn();
    const onDelete = vi.fn();

    render(
      <MapSummaryCard
        map={map}
        selected={true}
        onSelect={onSelect}
        onOpen={onOpen}
        onRename={onRename}
        onDuplicate={onDuplicate}
        onDelete={onDelete}
      />,
    );

    const selectButton = screen.getByRole('button', { name: 'Select map Backbone' });
    expect(selectButton.className).toContain('absolute');
    expect(selectButton.className).toContain('inset-0');

    fireEvent.click(selectButton);
    expect(onSelect).toHaveBeenCalledWith(map);

    fireEvent.click(screen.getByRole('button', { name: 'Open map Backbone' }));
    fireEvent.click(screen.getByRole('button', { name: 'Rename Backbone' }));
    fireEvent.click(screen.getByRole('button', { name: 'Duplicate Backbone' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete Backbone' }));

    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onOpen).toHaveBeenCalledWith(map);
    expect(onRename).toHaveBeenCalledWith(map);
    expect(onDuplicate).toHaveBeenCalledWith(map);
    expect(onDelete).toHaveBeenCalledWith(map);
    expect(screen.getByRole('listitem').className).toContain('border-l-primary');
  });

  it('does not offer delete for default maps', () => {
    render(
      <MapSummaryCard
        map={mockMap({ id: 'default', name: 'Default', is_default: true })}
        selected={false}
        onSelect={vi.fn()}
        onOpen={vi.fn()}
        onRename={vi.fn()}
        onDuplicate={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Open map Default' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Rename Default' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Duplicate Default' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete Default' })).not.toBeInTheDocument();
  });
});
