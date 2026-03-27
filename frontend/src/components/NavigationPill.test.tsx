import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import NavigationPill from './NavigationPill';
import type { Area } from '../types/api';

// Mock fetchHealthVersion
vi.mock('../api/client', () => ({
  fetchHealthVersion: vi.fn().mockResolvedValue({ version: '1.3.0', git_commit: 'abc', build_date: '2026-01-01' }),
}));

// Mock useTheme
const mockSetTheme = vi.fn();
vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({ theme: 'dark' as const, resolvedTheme: 'dark' as const, setTheme: mockSetTheme }),
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
  areas: [
    mockArea(),
    mockArea({ id: 'area-2', name: 'Distribution', color: '#FF5722' }),
  ],
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
    expect(screen.getByText('Global')).toBeDefined();
    expect(screen.getByText('Backbone')).toBeDefined();
    expect(screen.getByText('Distribution')).toBeDefined();
  });

  it('clicking Global calls onAreaSelect with null (shows full canvas)', () => {
    const onAreaSelect = vi.fn();
    render(<NavigationPill {...defaultProps} activeView="canvas" selectedAreaId="area-1" onAreaSelect={onAreaSelect} />);
    fireEvent.click(screen.getByText('Global'));
    expect(onAreaSelect).toHaveBeenCalledWith(null);
  });

  it('clicking an area button calls onAreaSelect with area id', () => {
    const onAreaSelect = vi.fn();
    render(<NavigationPill {...defaultProps} onAreaSelect={onAreaSelect} />);
    fireEvent.click(screen.getByText('Backbone'));
    expect(onAreaSelect).toHaveBeenCalledWith('area-1');
  });

  it('on Devices view pill shows simplified layout with Devices label and no area buttons', () => {
    render(<NavigationPill {...defaultProps} activeView="dashboard" />);
    expect(screen.getByText('Devices')).toBeDefined();
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
});
