import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { CanvasMap } from '../../types/api';
import { DuplicateMapDialog } from './DuplicateMapDialog';

function mockMap(overrides: Partial<CanvasMap> = {}): CanvasMap {
  return {
    id: 'map-1',
    name: 'Backbone',
    description: '',
    source_area_id: 'area-1',
    filter: { area_id: 'area-1' },
    is_default: false,
    device_count: 6,
    link_count: 3,
    position_count: 6,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    ...overrides,
  };
}

describe('DuplicateMapDialog', () => {
  it('renders source map name and map name field when open', () => {
    render(
      <DuplicateMapDialog
        open={true}
        sourceMap={mockMap()}
        onDuplicate={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByLabelText('Map name')).toBeInTheDocument();
    expect(screen.getByText('Backbone')).toBeInTheDocument();
  });

  it('does not render when closed', () => {
    render(
      <DuplicateMapDialog
        open={false}
        sourceMap={mockMap()}
        onDuplicate={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('submits name and source map context', () => {
    const sourceMap = mockMap();
    const onDuplicate = vi.fn();

    render(
      <DuplicateMapDialog
        open={true}
        sourceMap={sourceMap}
        onDuplicate={onDuplicate}
        onClose={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByLabelText('Map name'), {
      target: { value: 'Backbone Copy' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Duplicate map' }));

    expect(onDuplicate).toHaveBeenCalledWith({
      name: 'Backbone Copy',
      sourceMap,
    });
  });

  it('cancel and close controls call onClose', () => {
    const onClose = vi.fn();

    render(
      <DuplicateMapDialog
        open={true}
        sourceMap={mockMap()}
        onDuplicate={vi.fn()}
        onClose={onClose}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    fireEvent.click(screen.getByRole('button', { name: 'Close duplicate map dialog' }));

    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
