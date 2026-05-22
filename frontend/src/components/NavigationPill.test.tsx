import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Area, CanvasMap } from '../types/api';
import NavigationPill from './NavigationPill';

// Mock fetchHealthVersion
vi.mock('../api/client', () => ({
  fetchHealthVersion: vi.fn().mockResolvedValue({
    version: '1.3.0',
    git_commit: 'abc',
    build_date: '2026-01-01',
  }),
}));

// Mock useTheme
const mockSetTheme = vi.fn();
vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({
    theme: 'dark' as const,
    resolvedTheme: 'dark' as const,
    setTheme: mockSetTheme,
  }),
  adaptAreaColor: (hex: string) => hex,
}));

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: 'Core backbone area',
    color: '#00E676',
    device_count: 10,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function mockMap(overrides: Partial<CanvasMap> = {}): CanvasMap {
  return {
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
    ...overrides,
  };
}

const defaultProps = {
  activeView: 'hub' as const,
  selectedAreaId: null as string | null,
  selectedMapId: null as string | null,
  selectedMapName: 'Default',
  maps: [mockMap(), mockMap({ id: 'map-1', name: 'Backbone Map', is_default: false })],
  areas: [mockArea(), mockArea({ id: 'area-2', name: 'Distribution', color: '#FF5722' })],
  canViewAdmin: false,
  onViewChange: vi.fn(),
  onAreaSelect: vi.fn(),
  onMapSelect: vi.fn(),
  onManageMaps: vi.fn(),
  onLogout: vi.fn(),
};

describe('NavigationPill', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders THEIA branding text and version', async () => {
    render(<NavigationPill {...defaultProps} />);
    expect(screen.getByText('THEIA')).toBeDefined();
    const version = await screen.findByText('v1.3.0');
    expect(version).toBeDefined();
  });

  it('renders the map selector and area buttons for each area', () => {
    render(<NavigationPill {...defaultProps} />);
    const desktopAreaSelector = screen.getByTestId('desktop-area-selector');
    expect(screen.getByRole('button', { name: /select topology map/i })).toHaveTextContent(
      'Default',
    );
    expect(desktopAreaSelector.textContent).toContain('All areas');
    expect(desktopAreaSelector.textContent).toContain('Backbone');
    expect(desktopAreaSelector.textContent).toContain('Distribution');
    expect(desktopAreaSelector.className).toContain('hidden');
    expect(desktopAreaSelector.className).toContain('sm:flex');
  });

  it('keeps the area strip scrollable and exposes a More menu shortcut', async () => {
    const scrollWidthDescriptor = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      'scrollWidth',
    );
    const clientWidthDescriptor = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      'clientWidth',
    );
    const scrollByDescriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollBy');
    const scrollBy = vi.fn();
    const onAreaSelect = vi.fn();

    Object.defineProperty(HTMLElement.prototype, 'scrollWidth', {
      configurable: true,
      get() {
        return (this as HTMLElement).dataset.testid === 'desktop-area-selector-scroll' ? 900 : 0;
      },
    });
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true,
      get() {
        return (this as HTMLElement).dataset.testid === 'desktop-area-selector-scroll' ? 320 : 0;
      },
    });
    Object.defineProperty(HTMLElement.prototype, 'scrollBy', {
      configurable: true,
      writable: true,
      value: scrollBy,
    });

    try {
      render(
        <NavigationPill
          {...defaultProps}
          activeView="canvas"
          onAreaSelect={onAreaSelect}
          areas={[
            mockArea({ id: 'area-1', name: 'Backbone' }),
            mockArea({ id: 'area-2', name: 'Distribution' }),
            mockArea({ id: 'area-3', name: 'Access' }),
            mockArea({ id: 'area-4', name: 'Datacenter' }),
            mockArea({ id: 'area-5', name: 'Wireless' }),
          ]}
        />,
      );

      const desktopAreaSelector = screen.getByTestId('desktop-area-selector');
      const scroller = screen.getByTestId('desktop-area-selector-scroll');

      expect(scroller.className).toContain('topology-scrollbar-none');
      expect(scroller.className).toContain('overflow-x-auto');
      expect(within(scroller).getByRole('button', { name: 'Backbone' })).toBeDefined();
      expect(within(scroller).getByRole('button', { name: 'Distribution' })).toBeDefined();
      expect(within(scroller).getByRole('button', { name: 'Access' })).toBeDefined();
      expect(within(scroller).queryByRole('button', { name: 'Datacenter' })).toBeNull();
      expect(within(scroller).queryByRole('button', { name: 'Wireless' })).toBeNull();
      expect(within(scroller).getByRole('button', { name: 'More 2 areas' })).toBeDefined();

      await waitFor(() => {
        expect(screen.getByLabelText('Scroll areas right')).toBeInTheDocument();
      });
      fireEvent.click(screen.getByLabelText('Scroll areas right'));
      expect(scrollBy).toHaveBeenCalledWith({ left: 208, behavior: 'smooth' });

      fireEvent.click(within(scroller).getByRole('button', { name: 'More 2 areas' }));
      fireEvent.click(
        within(screen.getByRole('listbox', { name: 'More map areas' })).getByRole('option', {
          name: 'Datacenter',
        }),
      );
      expect(onAreaSelect).toHaveBeenCalledWith('area-4');
    } finally {
      if (scrollWidthDescriptor) {
        Object.defineProperty(HTMLElement.prototype, 'scrollWidth', scrollWidthDescriptor);
      }
      if (clientWidthDescriptor) {
        Object.defineProperty(HTMLElement.prototype, 'clientWidth', clientWidthDescriptor);
      }
      if (scrollByDescriptor) {
        Object.defineProperty(HTMLElement.prototype, 'scrollBy', scrollByDescriptor);
      } else {
        (HTMLElement.prototype as { scrollBy?: unknown }).scrollBy = undefined;
      }
    }
  });

  it('selecting a map from the pill delegates to onMapSelect', () => {
    const onMapSelect = vi.fn();
    render(<NavigationPill {...defaultProps} onMapSelect={onMapSelect} />);

    fireEvent.click(screen.getByRole('button', { name: /select topology map/i }));
    fireEvent.click(screen.getByRole('option', { name: /Backbone Map/ }));

    expect(onMapSelect).toHaveBeenCalledWith(defaultProps.maps[1]);
  });

  it('renders logout inside the user menu for every authenticated user', () => {
    const onLogout = vi.fn();

    render(<NavigationPill {...defaultProps} onLogout={onLogout} />);

    expect(screen.queryByRole('button', { name: 'Logout' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'User menu for User' }));
    const logoutItem = screen.getByRole('menuitem', { name: 'Logout' });
    expect(logoutItem).toHaveClass('text-critical');
    fireEvent.click(logoutItem);

    expect(onLogout).toHaveBeenCalledTimes(1);
  });

  it('groups Admin Area, theme toggle, and logout inside the user menu', () => {
    const onViewChange = vi.fn();
    const onLogout = vi.fn();

    render(
      <NavigationPill
        {...defaultProps}
        canViewAdmin={true}
        userLabel="Alice Admin"
        onViewChange={onViewChange}
        onLogout={onLogout}
      />,
    );

    expect(screen.queryByLabelText('Admin Dashboard')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Logout' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Switch to light theme')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'User menu for Alice Admin' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Admin Area' }));

    expect(onViewChange).toHaveBeenCalledWith('admin');

    fireEvent.click(screen.getByRole('button', { name: 'User menu for Alice Admin' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Light mode' }));

    expect(mockSetTheme).toHaveBeenCalledWith('light');

    fireEvent.click(screen.getByRole('button', { name: 'User menu for Alice Admin' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Logout' }));

    expect(onLogout).toHaveBeenCalledTimes(1);
  });

  it('clicking All areas calls onAreaSelect with null for the current map', () => {
    const onAreaSelect = vi.fn();
    render(
      <NavigationPill
        {...defaultProps}
        activeView="canvas"
        selectedAreaId="area-1"
        onAreaSelect={onAreaSelect}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'All areas' }));
    expect(onAreaSelect).toHaveBeenCalledWith(null);
  });

  it('clicking an area button calls onAreaSelect with area id', () => {
    const onAreaSelect = vi.fn();
    render(<NavigationPill {...defaultProps} onAreaSelect={onAreaSelect} />);
    fireEvent.click(screen.getByRole('button', { name: /Backbone/ }));
    expect(onAreaSelect).toHaveBeenCalledWith('area-1');
  });

  it('renders a mobile area select that switches between All areas and map-local areas', () => {
    const onAreaSelect = vi.fn();
    render(
      <NavigationPill
        {...defaultProps}
        activeView="canvas"
        selectedAreaId="area-1"
        onAreaSelect={onAreaSelect}
      />,
    );

    const areaSelect = screen.getByLabelText('Area selector') as HTMLSelectElement;
    expect(areaSelect.className).toContain('sm:hidden');
    expect(areaSelect.className).toContain('min-w-0');
    expect(areaSelect.className).not.toContain('min-w-[7rem]');
    expect(screen.getByRole('option', { name: 'All areas' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Backbone' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Distribution' })).toBeInTheDocument();
    expect(areaSelect.value).toBe('area-1');

    fireEvent.change(areaSelect, { target: { value: 'area-2' } });
    expect(onAreaSelect).toHaveBeenCalledWith('area-2');

    fireEvent.change(areaSelect, { target: { value: '__all__' } });
    expect(onAreaSelect).toHaveBeenCalledWith(null);
  });

  it('wraps map and area controls onto their own mobile row', () => {
    const { container } = render(
      <NavigationPill
        {...defaultProps}
        activeView="canvas"
        selectedMapName="Very Long Map Name That Must Not Overlap Areas"
      />,
    );

    const root = container.firstElementChild;
    const mobileControls = screen.getByTestId('mobile-map-area-controls');

    expect(root?.className).toContain('flex-wrap');
    expect(root?.className).toContain('justify-center');
    expect(root?.className).toContain('sm:flex-nowrap');
    expect(root?.className).toContain('sm:justify-start');
    expect(mobileControls.className).toContain('order-last');
    expect(mobileControls.className).toContain('w-full');
    expect(mobileControls.className).toContain('min-w-0');
    expect(mobileControls.className).toContain('justify-center');
    expect(mobileControls.className).toContain('sm:w-auto');
    expect(mobileControls.className).toContain('sm:justify-start');
  });

  it('on Devices view pill keeps map and area context controls available', () => {
    render(<NavigationPill {...defaultProps} activeView="dashboard" />);

    expect(screen.getByRole('button', { name: /select topology map/i })).toHaveTextContent(
      'Default',
    );
    expect(screen.getByTestId('desktop-area-selector').textContent).toContain('All areas');
    expect(screen.getByTestId('desktop-area-selector').textContent).toContain('Backbone');
    expect(screen.getByLabelText('Devices Dashboard').className).toContain('border-outline-strong');
  });

  it('clicking Hub icon calls onViewChange hub', () => {
    const onViewChange = vi.fn();
    render(<NavigationPill {...defaultProps} activeView="dashboard" onViewChange={onViewChange} />);
    const hubButton = screen.getByLabelText('Topology Hub');
    fireEvent.click(hubButton);
    expect(onViewChange).toHaveBeenCalledWith('hub');
  });

  it('clicking Devices icon calls onViewChange dashboard', () => {
    const onViewChange = vi.fn();
    render(<NavigationPill {...defaultProps} onViewChange={onViewChange} />);
    const devicesButton = screen.getByLabelText('Devices Dashboard');
    fireEvent.click(devicesButton);
    expect(onViewChange).toHaveBeenCalledWith('dashboard');
  });

  it('hides Admin Area without permission and shows it in the user menu when allowed', () => {
    const onViewChange = vi.fn();
    const { rerender } = render(<NavigationPill {...defaultProps} onViewChange={onViewChange} />);

    expect(screen.queryByLabelText('Admin Dashboard')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'User menu for User' }));
    expect(screen.queryByRole('menuitem', { name: 'Admin Area' })).not.toBeInTheDocument();
    fireEvent.keyDown(document, { key: 'Escape' });

    rerender(<NavigationPill {...defaultProps} canViewAdmin={true} onViewChange={onViewChange} />);

    fireEvent.click(screen.getByRole('button', { name: 'User menu for User' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Admin Area' }));

    expect(onViewChange).toHaveBeenCalledWith('admin');
  });

  it('preserves the enterprise navigation model with area filters and fixed action buttons', () => {
    const { container } = render(
      <NavigationPill {...defaultProps} activeView="canvas" selectedAreaId={null} />,
    );

    expect(screen.getByText('THEIA')).toBeInTheDocument();
    expect(screen.getByLabelText('Topology Hub')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /select topology map/i })).toBeInTheDocument();
    expect(screen.getByTestId('desktop-area-selector').textContent).toContain('All areas');
    expect(screen.getByTestId('desktop-area-selector').textContent).toContain('Backbone');
    expect(screen.getByTestId('desktop-area-selector').textContent).toContain('Distribution');
    expect(screen.getByLabelText('Devices Dashboard')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'User menu for User' }));
    expect(screen.getByRole('menuitem', { name: 'Light mode' })).toBeInTheDocument();
    expect(container.firstElementChild?.className).toContain('topology-glass');
  });
});
