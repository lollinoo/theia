import { render } from '@testing-library/react';
/**
 * COMP-03 NavBar (NavigationPill) behavioral tests.
 * The NavBar was implemented as NavigationPill in this project.
 * These tests verify the requirements from COMP-03.
 */
import { describe, expect, it, vi } from 'vitest';
import NavigationPill from './NavigationPill';

// Mock API client
vi.mock('../api/client', () => ({
  fetchHealthVersion: vi
    .fn()
    .mockResolvedValue({ version: '1.3.0', git_commit: 'abc', build_date: '2026-01-01' }),
}));

// Mock ThemeContext - resolvedTheme=dark to check light_mode icon
const mockSetTheme = vi.fn();
vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({
    theme: 'dark' as const,
    resolvedTheme: 'dark' as const,
    setTheme: mockSetTheme,
  }),
  adaptAreaColor: (hex: string) => hex,
}));

const defaultProps = {
  activeView: 'hub' as const,
  selectedAreaId: null as string | null,
  areas: [],
  onViewChange: vi.fn(),
  onAreaSelect: vi.fn(),
};

describe('NavigationPill (COMP-03: NavBar requirements)', () => {
  it('uses MaterialIcon for theme toggle — no inline SVGs', () => {
    const { container } = render(<NavigationPill {...defaultProps} />);
    // No raw SVG elements — MaterialIcon renders a span
    const svgElements = container.querySelectorAll('svg');
    expect(svgElements.length).toBe(0);
    // Material symbols span must exist
    const iconSpans = container.querySelectorAll('.material-symbols-rounded');
    expect(iconSpans.length).toBeGreaterThan(0);
  });

  it('theme toggle renders light_mode icon when resolvedTheme is dark', () => {
    const { container } = render(<NavigationPill {...defaultProps} />);
    // When dark, shows light_mode icon to switch to light
    // Find icon with text content "light_mode"
    const allIcons = Array.from(container.querySelectorAll('.material-symbols-rounded'));
    const lightModeIcons = allIcons.filter((el) => el.textContent === 'light_mode');
    expect(lightModeIcons.length).toBe(1);
  });

  it('container has no border-b class (no-line rule, COMP-12)', () => {
    const { container } = render(<NavigationPill {...defaultProps} />);
    const rootDiv = container.firstChild as HTMLElement;
    expect(rootDiv).not.toBeNull();
    expect(rootDiv.className).not.toContain('border-b');
  });

  it('container has dark:backdrop-blur (dark-only blur per D-06)', () => {
    const { container } = render(<NavigationPill {...defaultProps} />);
    const rootDiv = container.firstChild as HTMLElement;
    // Should have dark:backdrop-blur not unconditional backdrop-blur
    expect(rootDiv.className).toContain('dark:backdrop-blur');
  });

  it('keeps area filters horizontally constrained instead of replacing them with a new navigation layout', () => {
    const { container } = render(
      <NavigationPill
        {...defaultProps}
        areas={[
          {
            id: 'area-1',
            name: 'Backbone',
            description: '',
            color: '#00E676',
            device_count: 1,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
          },
        ]}
      />,
    );

    expect(container.innerHTML).toContain('overflow-x-auto');
    expect(container.innerHTML).toContain('max-w-[56vw]');
  });
});
