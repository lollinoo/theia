/**
 * Exercises toolbar component behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen, within } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { Toolbar } from './Toolbar';

// Mock MaterialIcon to make assertions about which icons are rendered
vi.mock('./MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span data-testid={`material-icon-${name}`} className="material-symbols-rounded">
      {name}
    </span>
  ),
}));

const defaultProps = {
  onSearch: vi.fn(),
  onAddDevice: vi.fn(),
  onCreateLink: vi.fn(),
  onAlerts: vi.fn(),
  onToggleEditMode: vi.fn(),
  onToggleSnapToGrid: vi.fn(),
  editMode: false,
  snapToGrid: true,
  alertCount: 0,
};

function SnapPreferenceToolbarHarness() {
  const [snapToGrid, setSnapToGrid] = useState(true);

  return (
    <Toolbar
      {...defaultProps}
      editMode
      snapToGrid={snapToGrid}
      onToggleSnapToGrid={() => setSnapToGrid((current) => !current)}
    />
  );
}

describe('Toolbar (COMP-04)', () => {
  it('renders MaterialIcon components — no inline SVGs', () => {
    const { container } = render(<Toolbar {...defaultProps} />);
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));
    // With MaterialIcon mocked, the mocked spans appear instead of SVGs
    const materialIcons = container.querySelectorAll('[data-testid^="material-icon-"]');
    expect(materialIcons.length).toBeGreaterThanOrEqual(6);
    // No real SVG elements should remain (only the mocked spans)
    const svgElements = container.querySelectorAll('svg');
    expect(svgElements.length).toBe(0);
  });

  it('renders icons for canvas actions without a global settings shortcut', () => {
    render(<Toolbar {...defaultProps} editMode />);
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));

    expect(screen.getByTestId('material-icon-edit')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-search')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-add')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-link')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-notifications')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-grid_4x4')).toBeInTheDocument();
    expect(screen.queryByTestId('material-icon-settings')).not.toBeInTheDocument();
  });

  it('does not expose snap to grid outside edit mode', () => {
    render(<Toolbar {...defaultProps} editMode={false} />);
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));

    expect(screen.queryByRole('button', { name: /Snap to grid/ })).not.toBeInTheDocument();
    expect(screen.queryByTestId('material-icon-grid_4x4')).not.toBeInTheDocument();
  });

  it('exposes the enabled snap action as an active pressed toggle', () => {
    render(<Toolbar {...defaultProps} editMode snapToGrid />);
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));

    const toggle = screen.getByRole('button', { name: 'Snap to grid: On' });

    expect(toggle).toHaveAttribute('title', 'Snap to grid: On');
    expect(toggle).toHaveAttribute('aria-pressed', 'true');
    expect(toggle.className).toContain('bg-primary/12');
    expect(within(toggle).getByTestId('material-icon-grid_4x4')).toBeInTheDocument();
  });

  it('exposes the disabled snap action as an unpressed toggle', () => {
    render(<Toolbar {...defaultProps} editMode snapToGrid={false} />);
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));

    const toggle = screen.getByRole('button', { name: 'Snap to grid: Off' });

    expect(toggle).toHaveAttribute('title', 'Snap to grid: Off');
    expect(toggle).toHaveAttribute('aria-pressed', 'false');
    expect(toggle.className).not.toContain('bg-primary/12');
  });

  it('calls the snap preference callback', () => {
    const onToggleSnapToGrid = vi.fn();
    render(<Toolbar {...defaultProps} editMode onToggleSnapToGrid={onToggleSnapToGrid} />);
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));

    fireEvent.click(screen.getByRole('button', { name: 'Snap to grid: On' }));

    expect(onToggleSnapToGrid).toHaveBeenCalledOnce();
  });

  it('retains focus on the snap action when its state label changes', () => {
    render(<SnapPreferenceToolbarHarness />);
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));
    const enabledToggle = screen.getByRole('button', { name: 'Snap to grid: On' });
    enabledToggle.focus();

    fireEvent.click(enabledToggle);

    const disabledToggle = screen.getByRole('button', { name: 'Snap to grid: Off' });
    expect(disabledToggle).toBe(enabledToggle);
    expect(disabledToggle).toHaveFocus();
  });

  it('does not render border-b separators between buttons', () => {
    const { container } = render(<Toolbar {...defaultProps} />);
    const html = container.innerHTML;
    expect(html).not.toContain('border-b border-outline');
    expect(html).not.toContain('border-b');
  });

  it('renders alert count badge when alertCount > 0', () => {
    const { container } = render(<Toolbar {...defaultProps} alertCount={3} />);
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));
    // The badge span contains the count number
    const badgeEl = container.querySelector('.bg-status-down');
    expect(badgeEl).not.toBeNull();
    expect(badgeEl?.textContent).toBe('3');
  });

  it('does not render alert badge when alertCount is 0', () => {
    const { container } = render(<Toolbar {...defaultProps} alertCount={0} />);
    // No bg-status-down badge (only bg-status-down on the notifications icon area)
    const html = container.innerHTML;
    // The badge wraps the count text; if alertCount is 0 no badge span appears
    expect(html).not.toContain('bg-status-down');
  });

  it('keeps all toolbar actions as icon buttons with accessible titles', () => {
    render(<Toolbar {...defaultProps} alertCount={8} />);
    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));

    expect(screen.getByTitle(/Edit Mode/)).toBeInTheDocument();
    expect(screen.getByTitle(/Search/)).toBeInTheDocument();
    expect(screen.getByTitle(/Add Device/)).toBeInTheDocument();
    expect(screen.getByTitle(/Create Link/)).toBeInTheDocument();
    expect(screen.getByTitle('Alerts')).toBeInTheDocument();
    expect(screen.queryByTitle(/Settings/)).not.toBeInTheDocument();
    expect(screen.getByText('8')).toBeInTheDocument();
  });

  it('keeps desktop tools mounted by default while mobile tools are collapsed', () => {
    const { container } = render(<Toolbar {...defaultProps} />);

    expect(container.firstElementChild?.className).toContain('top-32');
    expect(container.firstElementChild?.className).toContain('sm:top-20');
    expect(container.firstElementChild?.className).toContain('xl:top-4');
    expect(container.firstElementChild?.className).not.toContain('lg:top-4');
    expect(container.firstElementChild?.className).not.toContain('top-16');
    const mobileToggle = screen.getByRole('button', { name: 'Show canvas tools' });

    expect(mobileToggle.className).toContain('sm:hidden');
    expect(within(mobileToggle).getByTestId('material-icon-build')).toBeInTheDocument();
    expect(within(mobileToggle).queryByTestId('material-icon-terminal')).not.toBeInTheDocument();
    expect(within(mobileToggle).queryByTestId('material-icon-hub')).not.toBeInTheDocument();
    expect(within(mobileToggle).queryByTestId('material-icon-settings')).not.toBeInTheDocument();
    expect(screen.queryByTestId('material-icon-filter_list')).not.toBeInTheDocument();
    expect(screen.queryByTestId('material-icon-widgets')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show canvas tools' })).toHaveAttribute(
      'aria-expanded',
      'false',
    );
    expect(screen.getByTitle(/Search/).className).toContain('hidden sm:flex');
    expect(screen.getByTitle(/Add Device/).className).toContain('hidden sm:flex');

    fireEvent.click(screen.getByRole('button', { name: 'Show canvas tools' }));

    expect(screen.getByRole('button', { name: 'Hide canvas tools' })).toHaveAttribute(
      'aria-expanded',
      'true',
    );
    expect(screen.getByTitle(/Search/)).toBeInTheDocument();
    expect(screen.getByTitle(/Add Device/)).toBeInTheDocument();
    expect(screen.getByTitle(/Create Link/)).toBeInTheDocument();
    expect(screen.getByTitle('Alerts')).toBeInTheDocument();
    expect(screen.queryByTitle(/Settings/)).not.toBeInTheDocument();
    expect(screen.getByTitle(/Search/).className).not.toContain('hidden sm:flex');
  });
});
