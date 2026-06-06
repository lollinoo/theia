/**
 * Exercises create map dialog topology hub behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Area } from '../../types/api';
import { CreateMapDialog } from './CreateMapDialog';

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: '',
    color: '#2979FF',
    device_count: 4,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    ...overrides,
  };
}

describe('CreateMapDialog', () => {
  it('renders the map name field when open', () => {
    render(
      <CreateMapDialog open={true} sourceArea={mockArea()} onCreate={vi.fn()} onClose={vi.fn()} />,
    );

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByLabelText('Map name')).toBeInTheDocument();
    expect(screen.getByText('Backbone')).toBeInTheDocument();
  });

  it('does not render when closed', () => {
    render(
      <CreateMapDialog open={false} sourceArea={mockArea()} onCreate={vi.fn()} onClose={vi.fn()} />,
    );

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('submits name and source area context', () => {
    const area = mockArea();
    const onCreate = vi.fn();

    render(<CreateMapDialog open={true} sourceArea={area} onCreate={onCreate} onClose={vi.fn()} />);

    fireEvent.change(screen.getByLabelText('Map name'), {
      target: { value: 'Backbone Saved Map' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Create map' }));

    expect(onCreate).toHaveBeenCalledWith({
      name: 'Backbone Saved Map',
      sourceArea: area,
    });
  });

  it('cancel and close controls call onClose', () => {
    const onClose = vi.fn();

    render(
      <CreateMapDialog open={true} sourceArea={mockArea()} onCreate={vi.fn()} onClose={onClose} />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    fireEvent.click(screen.getByRole('button', { name: 'Close create map dialog' }));

    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
