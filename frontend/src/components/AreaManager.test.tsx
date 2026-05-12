import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AreaManager } from './AreaManager';

vi.mock('../contexts/ThemeContext', () => ({
  useTheme: () => ({ theme: 'dark' as const, resolvedTheme: 'dark' as const, setTheme: vi.fn() }),
  adaptAreaColor: (hex: string) => hex,
}));

// Mock API calls
const mockFetchAreas = vi.fn();
const mockCreateArea = vi.fn();
const mockUpdateArea = vi.fn();
const mockDeleteArea = vi.fn();
const mockFetchDevices = vi.fn();
const mockUpdateDevice = vi.fn();
const mockCreateCanvasMapArea = vi.fn();
const mockUpdateCanvasMapArea = vi.fn();
const mockDeleteCanvasMapArea = vi.fn();
const mockUpdateCanvasMapDeviceAreas = vi.fn();

vi.mock('../api/client', () => ({
  fetchAreas: (...args: unknown[]) => mockFetchAreas(...args),
  createArea: (...args: unknown[]) => mockCreateArea(...args),
  updateArea: (...args: unknown[]) => mockUpdateArea(...args),
  deleteArea: (...args: unknown[]) => mockDeleteArea(...args),
  fetchDevices: (...args: unknown[]) => mockFetchDevices(...args),
  updateDevice: (...args: unknown[]) => mockUpdateDevice(...args),
  createCanvasMapArea: (...args: unknown[]) => mockCreateCanvasMapArea(...args),
  updateCanvasMapArea: (...args: unknown[]) => mockUpdateCanvasMapArea(...args),
  deleteCanvasMapArea: (...args: unknown[]) => mockDeleteCanvasMapArea(...args),
  updateCanvasMapDeviceAreas: (...args: unknown[]) => mockUpdateCanvasMapDeviceAreas(...args),
}));

beforeEach(() => {
  vi.clearAllMocks();
  mockFetchAreas.mockResolvedValue([]);
  mockFetchDevices.mockResolvedValue([]);
  mockCreateCanvasMapArea.mockResolvedValue({
    id: 'map-area-new',
    name: 'NewArea',
    description: '',
    color: '#00E676',
    device_count: 0,
    created_at: '',
    updated_at: '',
  });
  mockUpdateCanvasMapArea.mockResolvedValue({
    id: 'map-area-1',
    name: 'Backbone',
    description: '',
    color: '#00E676',
    device_count: 0,
    created_at: '',
    updated_at: '',
  });
  mockDeleteCanvasMapArea.mockResolvedValue(undefined);
  mockUpdateCanvasMapDeviceAreas.mockResolvedValue({
    id: 'map-1',
    name: 'Backbone',
    description: '',
    source_area_id: null,
    filter: {},
    is_default: false,
    device_count: 1,
    link_count: 0,
    position_count: 0,
    created_at: '',
    updated_at: '',
  });
});

describe('AreaManager', () => {
  it('renders empty state when no areas exist', async () => {
    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText(/no areas yet/i)).toBeInTheDocument();
    });
  });

  it('renders area list with names and device counts', async () => {
    mockFetchAreas.mockResolvedValue([
      {
        id: 'a1',
        name: 'Backbone',
        description: 'Core',
        color: '#2979FF',
        device_count: 3,
        created_at: '',
        updated_at: '',
      },
      {
        id: 'a2',
        name: 'Edge',
        description: '',
        color: '#00E676',
        device_count: 0,
        created_at: '',
        updated_at: '',
      },
    ]);

    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText('Backbone')).toBeInTheDocument();
    });
    expect(screen.getByText('Edge')).toBeInTheDocument();
    // Device count badge should be visible
    expect(screen.getByText('3 devices')).toBeInTheDocument();
  });

  it('switches to create mode when New is clicked', async () => {
    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText(/no areas yet/i)).toBeInTheDocument();
    });

    const newBtn = screen.getByRole('button', { name: /new/i });
    await userEvent.click(newBtn);

    // Should show create form with "Create Area" button
    await waitFor(() => {
      expect(screen.getByText(/create area/i)).toBeInTheDocument();
    });
  });

  it('calls createArea with form data on submit', async () => {
    mockCreateArea.mockResolvedValue({
      id: 'new-1',
      name: 'NewArea',
      description: '',
      color: '#00E676',
      device_count: 0,
      created_at: '',
      updated_at: '',
    });

    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText(/no areas yet/i)).toBeInTheDocument();
    });

    await userEvent.click(screen.getByRole('button', { name: /new/i }));

    await waitFor(() => {
      expect(screen.getByPlaceholderText(/backbone/i)).toBeInTheDocument();
    });

    await userEvent.type(screen.getByPlaceholderText(/backbone/i), 'NewArea');
    await userEvent.click(screen.getByText(/create area/i));

    await waitFor(() => {
      expect(mockCreateArea).toHaveBeenCalledWith(expect.objectContaining({ name: 'NewArea' }));
    });
  });

  it('shows delete confirmation with device count', async () => {
    mockFetchAreas.mockResolvedValue([
      {
        id: 'a1',
        name: 'ToDelete',
        description: '',
        color: '#FF1744',
        device_count: 5,
        created_at: '',
        updated_at: '',
      },
    ]);

    render(<AreaManager />);
    await waitFor(() => {
      expect(screen.getByText('ToDelete')).toBeInTheDocument();
    });

    // Click the delete button via its aria-label
    const deleteButton = screen.getByRole('button', { name: /delete area/i });
    await userEvent.click(deleteButton);

    // Should show confirmation with device count
    await waitFor(() => {
      expect(screen.getByText(/5 devices will be unassigned/i)).toBeInTheDocument();
    });
  });

  it('creates areas through the selected saved map instead of global areas', async () => {
    const onAreasChange = vi.fn();

    render(
      <AreaManager
        mapContext={{ mapId: 'map-1', mapName: 'Backbone' }}
        areas={[]}
        devices={[]}
        onAreasChange={onAreasChange}
      />,
    );

    expect(screen.getByText(/no areas yet/i)).toBeInTheDocument();
    expect(mockFetchAreas).not.toHaveBeenCalled();
    expect(mockFetchDevices).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', { name: /new/i }));
    await userEvent.type(screen.getByPlaceholderText(/backbone/i), 'NewArea');
    await userEvent.click(screen.getByText(/create area/i));

    await waitFor(() => {
      expect(mockCreateCanvasMapArea).toHaveBeenCalledWith('map-1', {
        name: 'NewArea',
        description: '',
        color: '#00E676',
      });
    });
    expect(mockCreateArea).not.toHaveBeenCalled();
    expect(onAreasChange).toHaveBeenCalled();
  });

  it('creates map-local areas with a custom color from the color picker', async () => {
    const onAreasChange = vi.fn();

    render(
      <AreaManager
        mapContext={{ mapId: 'map-1', mapName: 'Backbone' }}
        areas={[]}
        devices={[]}
        onAreasChange={onAreasChange}
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: /new/i }));
    fireEvent.change(screen.getByLabelText('Custom color'), { target: { value: '#123abc' } });
    await userEvent.type(screen.getByPlaceholderText(/backbone/i), 'CustomArea');
    await userEvent.click(screen.getByText(/create area/i));

    await waitFor(() => {
      expect(mockCreateCanvasMapArea).toHaveBeenCalledWith('map-1', {
        name: 'CustomArea',
        description: '',
        color: '#123ABC',
      });
    });
    expect(onAreasChange).toHaveBeenCalled();
  });

  it('does not render preset color swatches in the area form', async () => {
    render(
      <AreaManager mapContext={{ mapId: 'map-1', mapName: 'Backbone' }} areas={[]} devices={[]} />,
    );

    await userEvent.click(screen.getByRole('button', { name: /new/i }));

    expect(screen.getByLabelText('Custom color')).toBeInTheDocument();
    expect(screen.queryByTitle('#00E676')).not.toBeInTheDocument();
    expect(screen.queryByTitle('#2979FF')).not.toBeInTheDocument();
    expect(screen.queryByTitle('#FF1744')).not.toBeInTheDocument();
  });

  it('updates map-local area colors from the color picker', async () => {
    const onAreasChange = vi.fn();

    render(
      <AreaManager
        mapContext={{ mapId: 'map-1', mapName: 'Backbone' }}
        areas={[
          {
            id: 'map-area-1',
            name: 'Backbone',
            description: '',
            color: '#00E676',
            device_count: 1,
            created_at: '',
            updated_at: '',
          },
        ]}
        devices={[]}
        onAreasChange={onAreasChange}
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: /edit area/i }));
    fireEvent.change(screen.getByLabelText('Custom color'), { target: { value: '#abcdef' } });
    await userEvent.click(screen.getByText(/save changes/i));

    await waitFor(() => {
      expect(mockUpdateCanvasMapArea).toHaveBeenCalledWith('map-1', 'map-area-1', {
        name: 'Backbone',
        description: '',
        color: '#ABCDEF',
      });
    });
    expect(onAreasChange).toHaveBeenCalled();
  });

  it('uses uniform icon button sizing for every area row action', () => {
    render(
      <AreaManager
        mapContext={{ mapId: 'map-1', mapName: 'Backbone' }}
        areas={[
          {
            id: 'map-area-1',
            name: 'Backbone',
            description: '',
            color: '#00E676',
            device_count: 1,
            created_at: '',
            updated_at: '',
          },
        ]}
        devices={[]}
        onOpenArea={vi.fn()}
        onCreateMapFromArea={vi.fn()}
      />,
    );

    const actionButtons = [
      screen.getByRole('button', { name: 'Open area Backbone' }),
      screen.getByRole('button', { name: 'Create map from area Backbone' }),
      screen.getByRole('button', { name: /edit area/i }),
      screen.getByRole('button', { name: /delete area/i }),
    ];

    for (const button of actionButtons) {
      expect(button.className).toContain('h-8');
      expect(button.className).toContain('w-8');
      expect(button.className).toContain('items-center');
      expect(button.className).toContain('justify-center');
    }
  });

  it('updates map-local device area assignments without mutating global devices', async () => {
    const onAreasChange = vi.fn();

    render(
      <AreaManager
        mapContext={{ mapId: 'map-1', mapName: 'Backbone' }}
        areas={[
          {
            id: 'map-area-1',
            name: 'Backbone',
            description: '',
            color: '#00E676',
            device_count: 1,
            created_at: '',
            updated_at: '',
          },
        ]}
        devices={[
          {
            id: 'dev-1',
            hostname: 'router-01',
            ip: '10.0.0.1',
            notes: '',
            device_type: 'router',
            status: 'up',
            sys_name: 'router-01',
            sys_descr: '',
            hardware_model: '',
            vendor: 'mikrotik',
            managed: true,
            backup_supported: true,
            poll_class: 'standard',
            poll_interval_override: null,
            polling_enabled: true,
            metrics_source: 'none',
            prometheus_label_name: '',
            prometheus_label_value: '',
            topology_discovery_mode: 'inherit',
            effective_topology_discovery_mode: 'off',
            topology_bootstrap_state: 'idle',
            last_topology_discovery_at: null,
            last_topology_discovery_result: '',
            area_ids: ['map-area-1'],
            interfaces: [],
            tags: {},
            created_at: '',
            updated_at: '',
          },
        ]}
        onAreasChange={onAreasChange}
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: /edit area/i }));
    await userEvent.click(screen.getByRole('button', { name: /remove device/i }));

    await waitFor(() => {
      expect(mockUpdateCanvasMapDeviceAreas).toHaveBeenCalledWith('map-1', {
        device_ids: ['dev-1'],
        area_ids: [],
      });
    });
    expect(mockUpdateDevice).not.toHaveBeenCalled();
    expect(onAreasChange).toHaveBeenCalled();
  });
});
