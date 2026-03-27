import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Toolbar } from './Toolbar';

// Mock MaterialIcon to make assertions about which icons are rendered
vi.mock('./MaterialIcon', () => ({
  MaterialIcon: ({ name }: { name: string }) => (
    <span data-testid={`material-icon-${name}`} className="material-symbols-rounded">{name}</span>
  ),
}));

const defaultProps = {
  onSearch: vi.fn(),
  onAddDevice: vi.fn(),
  onCreateLink: vi.fn(),
  onAlerts: vi.fn(),
  onSettings: vi.fn(),
  onToggleEditMode: vi.fn(),
  editMode: false,
  alertCount: 0,
};

describe('Toolbar (COMP-04)', () => {
  it('renders MaterialIcon components — no inline SVGs', () => {
    const { container } = render(<Toolbar {...defaultProps} />);
    // With MaterialIcon mocked, the mocked spans appear instead of SVGs
    const materialIcons = container.querySelectorAll('[data-testid^="material-icon-"]');
    expect(materialIcons.length).toBeGreaterThanOrEqual(6);
    // No real SVG elements should remain (only the mocked spans)
    const svgElements = container.querySelectorAll('svg');
    expect(svgElements.length).toBe(0);
  });

  it('renders icons for all 6 toolbar actions', () => {
    render(<Toolbar {...defaultProps} />);

    expect(screen.getByTestId('material-icon-edit')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-search')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-add')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-link')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-notifications')).toBeInTheDocument();
    expect(screen.getByTestId('material-icon-settings')).toBeInTheDocument();
  });

  it('does not render border-b separators between buttons', () => {
    const { container } = render(<Toolbar {...defaultProps} />);
    const html = container.innerHTML;
    expect(html).not.toContain('border-b border-outline');
    expect(html).not.toContain('border-b');
  });

  it('renders alert count badge when alertCount > 0', () => {
    const { container } = render(<Toolbar {...defaultProps} alertCount={3} />);
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
});
