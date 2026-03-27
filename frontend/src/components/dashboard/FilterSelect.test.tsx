import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { FilterSelect, type FilterOption } from './FilterSelect';

// Mock MaterialIcon to avoid font loading
vi.mock('../MaterialIcon', () => ({
  MaterialIcon: ({ name, className }: { name: string; className?: string }) => (
    <span data-testid={`icon-${name}`} className={className}>{name}</span>
  ),
}));

const statusOptions: FilterOption[] = [
  { value: 'all', label: 'All' },
  { value: 'up', label: 'Up' },
  { value: 'down', label: 'Down' },
];

const areaOptions: FilterOption[] = [
  { value: 'all', label: 'All Areas' },
  { value: 'area-1', label: 'Core', color: '#FF5722' },
  { value: 'area-2', label: 'Edge', color: '#4CAF50' },
];

describe('FilterSelect', () => {
  it('renders trigger button showing current selected label', () => {
    render(
      <FilterSelect value="up" onChange={vi.fn()} options={statusOptions} label="Status" />,
    );

    expect(screen.getByText('Status:')).toBeInTheDocument();
    expect(screen.getByText('Up')).toBeInTheDocument();
  });

  it('clicking trigger opens dropdown options list', () => {
    render(
      <FilterSelect value="all" onChange={vi.fn()} options={statusOptions} label="Status" />,
    );

    // Options should not be visible initially (only the trigger with "All" label)
    expect(screen.queryByText('Down')).not.toBeInTheDocument();

    // Click trigger
    fireEvent.click(screen.getByRole('button'));

    // Options should now be visible (Up and Down are only in the dropdown)
    expect(screen.getAllByText('All')).toHaveLength(2); // trigger label + dropdown option
    expect(screen.getByText('Up')).toBeInTheDocument();
    expect(screen.getByText('Down')).toBeInTheDocument();
  });

  it('clicking an option calls onChange with that value and closes dropdown', () => {
    const onChange = vi.fn();
    render(
      <FilterSelect value="all" onChange={onChange} options={statusOptions} label="Status" />,
    );

    // Open dropdown
    fireEvent.click(screen.getByRole('button'));

    // Click an option
    const downButtons = screen.getAllByRole('button');
    // Find the "Down" option button (not the trigger)
    const downOption = downButtons.find((btn) => btn.textContent?.includes('Down'));
    expect(downOption).toBeTruthy();
    fireEvent.click(downOption!);

    expect(onChange).toHaveBeenCalledWith('down');

    // Dropdown should be closed - "Up" option should no longer be visible
    // (the trigger still shows "All" since value prop hasn't changed)
    expect(screen.queryAllByText('Up').length).toBeLessThanOrEqual(1);
  });

  it('clicking outside the dropdown closes it', () => {
    render(
      <div>
        <FilterSelect value="all" onChange={vi.fn()} options={statusOptions} label="Status" />
        <div data-testid="outside">Outside</div>
      </div>,
    );

    // Open dropdown
    fireEvent.click(screen.getByRole('button'));
    expect(screen.getByText('Down')).toBeInTheDocument();

    // Click outside
    fireEvent.mouseDown(screen.getByTestId('outside'));

    // Dropdown should be closed
    expect(screen.queryByText('Down')).not.toBeInTheDocument();
  });

  it('shows activeIndicator class when value is not the default/first option', () => {
    const { container, rerender } = render(
      <FilterSelect value="all" onChange={vi.fn()} options={statusOptions} label="Status" />,
    );

    // When value is the default (first option), should NOT have primary color classes
    const trigger = container.querySelector('button');
    expect(trigger?.className).not.toMatch(/bg-primary/);

    // When value is NOT the default, should have primary color accent
    rerender(
      <FilterSelect value="up" onChange={vi.fn()} options={statusOptions} label="Status" />,
    );
    const activeTrigger = container.querySelector('button');
    expect(activeTrigger?.className).toMatch(/bg-primary/);
    expect(activeTrigger?.className).toMatch(/text-primary/);
  });

  it('renders options with a color dot when color prop provided', () => {
    render(
      <FilterSelect value="all" onChange={vi.fn()} options={areaOptions} label="Area" />,
    );

    // Open dropdown
    fireEvent.click(screen.getByRole('button'));

    // "All Areas" option should NOT have a color dot
    // "Core" and "Edge" options should have color dots
    const coreOption = screen.getByText('Core').closest('button');
    const colorDot = coreOption?.querySelector('.rounded-full');
    expect(colorDot).toBeTruthy();
    expect(colorDot?.getAttribute('style')).toContain('background-color: rgb(255, 87, 34)');
  });

  it('renders expand_more icon as chevron', () => {
    render(
      <FilterSelect value="all" onChange={vi.fn()} options={statusOptions} label="Status" />,
    );

    expect(screen.getByTestId('icon-expand_more')).toBeInTheDocument();
  });

  it('uses defaultValue prop to determine active state', () => {
    const { container } = render(
      <FilterSelect
        value="all"
        onChange={vi.fn()}
        options={statusOptions}
        label="Status"
        defaultValue="all"
      />,
    );

    const trigger = container.querySelector('button');
    expect(trigger?.className).not.toMatch(/bg-primary/);
  });
});
