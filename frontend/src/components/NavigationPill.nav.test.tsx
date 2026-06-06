/**
 * Exercises navigation pill nav component behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen } from '@testing-library/react';
/**
 * COMP-03 NavBar (NavigationPill) behavioral tests.
 * The NavBar was implemented as NavigationPill in this project.
 * These tests verify the requirements from COMP-03.
 */
import { describe, expect, it, vi } from 'vitest';
import type { CanvasMap } from '../types/api';
import NavigationPill from './NavigationPill';

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
  selectedMapId: null as string | null,
  selectedMapName: 'Default',
  maps: [
    {
      id: 'default',
      name: 'Default',
      description: '',
      source_area_id: null,
      filter: {},
      is_default: true,
      device_count: 0,
      link_count: 0,
      position_count: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    } satisfies CanvasMap,
  ],
  areas: [],
  canViewAdmin: false,
  onViewChange: vi.fn(),
  onAreaSelect: vi.fn(),
  onMapSelect: vi.fn(),
  onManageMaps: vi.fn(),
  onLogout: vi.fn(),
};

describe('NavigationPill (COMP-03: NavBar requirements)', () => {
  it('uses MaterialIcon for user menu actions — no inline SVGs', () => {
    const { container } = render(<NavigationPill {...defaultProps} />);
    fireEvent.click(screen.getByRole('button', { name: 'User menu for User' }));
    // No raw SVG elements — MaterialIcon renders a span
    const svgElements = container.querySelectorAll('svg');
    expect(svgElements.length).toBe(0);
    // Material symbols span must exist
    const iconSpans = container.querySelectorAll('.material-symbols-rounded');
    expect(iconSpans.length).toBeGreaterThan(0);
  });

  it('user menu theme action renders light_mode icon when resolvedTheme is dark', () => {
    const { container } = render(<NavigationPill {...defaultProps} />);
    fireEvent.click(screen.getByRole('button', { name: 'User menu for User' }));
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

  it('keeps desktop area filters scrollable with a More menu shortcut', () => {
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
          {
            id: 'area-2',
            name: 'Distribution',
            description: '',
            color: '#FF5722',
            device_count: 1,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
          },
          {
            id: 'area-3',
            name: 'Access',
            description: '',
            color: '#2979FF',
            device_count: 1,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
          },
          {
            id: 'area-4',
            name: 'Wireless',
            description: '',
            color: '#FFD600',
            device_count: 1,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
          },
        ]}
      />,
    );

    expect(container.innerHTML).toContain('overflow-x-auto');
    expect(container.innerHTML).toContain('topology-scrollbar-none');
    expect(screen.getByRole('button', { name: 'More 1 area' })).toBeInTheDocument();
  });

  it('keeps map and area controls available while viewing Devices', () => {
    render(
      <NavigationPill
        {...defaultProps}
        activeView="dashboard"
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

    expect(screen.getByLabelText(/select topology map/i)).toBeInTheDocument();
    expect(screen.getByLabelText('Area selector')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'All areas' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Backbone' })).toBeInTheDocument();
  });
});
