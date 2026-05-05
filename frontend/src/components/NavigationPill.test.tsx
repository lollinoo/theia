import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Area } from '../types/api';
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

const defaultProps = {
  activeView: 'hub' as const,
  selectedAreaId: null as string | null,
  areas: [mockArea(), mockArea({ id: 'area-2', name: 'Distribution', color: '#FF5722' })],
  onViewChange: vi.fn(),
  onAreaSelect: vi.fn(),
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

  it('renders Global button and area buttons for each area', () => {
    render(<NavigationPill {...defaultProps} />);
    const desktopAreaSelector = screen.getByTestId('desktop-area-selector');
    expect(desktopAreaSelector.textContent).toContain('Global');
    expect(desktopAreaSelector.textContent).toContain('Backbone');
    expect(desktopAreaSelector.textContent).toContain('Distribution');
    expect(desktopAreaSelector.className).toContain('hidden');
    expect(desktopAreaSelector.className).toContain('sm:flex');
  });

  it('clicking Global calls onAreaSelect with null (shows full canvas)', () => {
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

  it('renders a mobile area select that switches between Global and areas', () => {
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
    expect(screen.getByRole('option', { name: 'Global' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Backbone' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Distribution' })).toBeInTheDocument();
    expect(areaSelect.value).toBe('area-1');

    fireEvent.change(areaSelect, { target: { value: 'area-2' } });
    expect(onAreaSelect).toHaveBeenCalledWith('area-2');

    fireEvent.change(areaSelect, { target: { value: '__global__' } });
    expect(onAreaSelect).toHaveBeenCalledWith(null);
  });

  it('on Devices view pill shows simplified layout with Devices label and no area buttons', () => {
    render(<NavigationPill {...defaultProps} activeView="dashboard" />);
    const devicesLabel = screen.getByText('Devices');
    expect(devicesLabel).toBeDefined();
    expect(devicesLabel.className).toContain('flex-1');
    expect(screen.queryByText('Global')).toBeNull();
    expect(screen.queryByText('Backbone')).toBeNull();
  });

  it('clicking Hub icon calls onViewChange hub', () => {
    const onViewChange = vi.fn();
    render(<NavigationPill {...defaultProps} activeView="dashboard" onViewChange={onViewChange} />);
    const hubButton = screen.getByLabelText('Area Hub');
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
    expect(screen.getByLabelText('Area Hub')).toBeInTheDocument();
    expect(screen.getByTestId('desktop-area-selector').textContent).toContain('Global');
    expect(screen.getByTestId('desktop-area-selector').textContent).toContain('Backbone');
    expect(screen.getByTestId('desktop-area-selector').textContent).toContain('Distribution');
    expect(screen.getByLabelText('Devices Dashboard')).toBeInTheDocument();
    expect(screen.getByLabelText('Switch to light theme')).toBeInTheDocument();
    expect(container.firstElementChild?.className).toContain('topology-glass');
  });
});
