import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Area, CanvasMap } from '../types/api';
import NavigationPill from './NavigationPill';

// Mock fetchHealthVersion
vi.mock('../api/client', () => ({
  fetchHealthVersion: vi
    .fn()
    .mockResolvedValue({ version: '1.3.0', git_commit: 'abc', build_date: '2026-01-01' }),
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
  maps: [
    mockMap(),
    mockMap({ id: 'map-1', name: 'Backbone Map', is_default: false }),
  ],
  areas: [mockArea(), mockArea({ id: 'area-2', name: 'Distribution', color: '#FF5722' })],
  onViewChange: vi.fn(),
  onAreaSelect: vi.fn(),
  onMapSelect: vi.fn(),
  onManageMaps: vi.fn(),
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

  it('selecting a map from the pill delegates to onMapSelect', () => {
    const onMapSelect = vi.fn();
    render(<NavigationPill {...defaultProps} onMapSelect={onMapSelect} />);

    fireEvent.click(screen.getByRole('button', { name: /select topology map/i }));
    fireEvent.click(screen.getByRole('option', { name: /Backbone Map/ }));

    expect(onMapSelect).toHaveBeenCalledWith(defaultProps.maps[1]);
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
    fireEvent.click(screen.getByTestId('desktop-area-selector').querySelector('button')!);
    expect(onAreaSelect).toHaveBeenCalledWith(null);
  });

  it('clicking an area button calls onAreaSelect with area id', () => {
    const onAreaSelect = vi.fn();
    render(<NavigationPill {...defaultProps} onAreaSelect={onAreaSelect} />);
    fireEvent.click(screen.getByTestId('desktop-area-selector').querySelectorAll('button')[1]);
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
    expect(screen.getByRole('option', { name: 'All areas' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Backbone' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Distribution' })).toBeInTheDocument();
    expect(areaSelect.value).toBe('area-1');

    fireEvent.change(areaSelect, { target: { value: 'area-2' } });
    expect(onAreaSelect).toHaveBeenCalledWith('area-2');

    fireEvent.change(areaSelect, { target: { value: '__all__' } });
    expect(onAreaSelect).toHaveBeenCalledWith(null);
  });

  it('on Devices view pill shows simplified layout with Devices label and no area buttons', () => {
    render(<NavigationPill {...defaultProps} activeView="dashboard" />);
    const devicesLabel = screen.getByText('Devices');
    expect(devicesLabel).toBeDefined();
    expect(devicesLabel.className).toContain('flex-1');
    expect(screen.queryByText('All areas')).toBeNull();
    expect(screen.queryByText('Backbone')).toBeNull();
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
    expect(screen.getByLabelText('Switch to light theme')).toBeInTheDocument();
    expect(container.firstElementChild?.className).toContain('topology-glass');
  });
});
