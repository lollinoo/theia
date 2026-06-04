import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Area, Device } from '../../types/api';
import { type DeviceFormModel, createDeviceConfigFormModel } from '../forms/deviceFormModels';
import { DeviceAreasSection } from './DeviceAreasSection';

vi.mock('../../api/client', () => ({
  fetchAreas: vi.fn().mockImplementation(() => new Promise<never>(() => {})),
}));

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    notes: null,
    device_type: 'router',
    poll_class: 'core',
    poll_interval_override: null,
    polling_enabled: true,
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    backup_supported: true,
    metrics_source: 'snmp',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    topology_discovery_mode: 'inherit',
    effective_topology_discovery_mode: 'off',
    topology_bootstrap_state: 'idle',
    last_topology_discovery_at: null,
    last_topology_discovery_result: '',
    area_ids: [],
    ...overrides,
  };
}

function mockArea(overrides: Partial<Area> = {}): Area {
  return {
    id: 'area-1',
    name: 'Backbone',
    description: '',
    color: '#00E676',
    device_count: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function renderSection({
  device = mockDevice(),
  initialForm = createDeviceConfigFormModel(device, false),
  areas,
  readOnly = false,
  isVirtual = false,
  mapContext,
}: {
  device?: Device;
  initialForm?: DeviceFormModel;
  areas?: Area[];
  readOnly?: boolean;
  isVirtual?: boolean;
  mapContext?: { mapId: string; mapName: string };
} = {}) {
  let form = initialForm;
  const onFormChange = vi.fn((update: Partial<DeviceFormModel>) => {
    form = { ...form, ...update };
    result.rerender(
      <DeviceAreasSection
        form={form}
        areas={areas}
        readOnly={readOnly}
        isVirtual={isVirtual}
        mapContext={mapContext}
        onFormChange={onFormChange}
        onVirtualChange={onVirtualChange}
      />,
    );
  });
  const onVirtualChange = vi.fn((update: Partial<DeviceFormModel['virtual']>) => {
    form = { ...form, virtual: { ...form.virtual, ...update } };
    result.rerender(
      <DeviceAreasSection
        form={form}
        areas={areas}
        readOnly={readOnly}
        isVirtual={isVirtual}
        mapContext={mapContext}
        onFormChange={onFormChange}
        onVirtualChange={onVirtualChange}
      />,
    );
  });

  const result = render(
    <DeviceAreasSection
      form={form}
      areas={areas}
      readOnly={readOnly}
      isVirtual={isVirtual}
      mapContext={mapContext}
      onFormChange={onFormChange}
      onVirtualChange={onVirtualChange}
    />,
  );

  return { ...result, onFormChange, onVirtualChange };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DeviceAreasSection', () => {
  it('uses provided areas without fetching and updates areaIds when areas are added and removed', async () => {
    const { fetchAreas } = await import('../../api/client');
    const initialForm = createDeviceConfigFormModel(mockDevice({ area_ids: ['area-1'] }), false);
    const backbone = mockArea();
    const access = mockArea({ id: 'area-2', name: 'Access', color: '#2979FF' });
    const { onFormChange } = renderSection({ initialForm, areas: [backbone, access] });

    expect(fetchAreas).not.toHaveBeenCalled();
    expect(screen.getByText('Backbone')).toBeInTheDocument();
    expect(screen.getByText('Add another area...')).toBeInTheDocument();

    fireEvent.change(screen.getByDisplayValue('Add another area...'), {
      target: { value: 'area-2' },
    });

    expect(onFormChange).toHaveBeenLastCalledWith({ areaIds: ['area-1', 'area-2'] });
    expect(screen.getByText('All areas assigned')).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole('button')[0]);

    expect(onFormChange).toHaveBeenLastCalledWith({ areaIds: ['area-2'] });
  });

  it('fetches areas when none are provided and ignores failures as non-fatal', async () => {
    const { fetchAreas } = await import('../../api/client');
    (fetchAreas as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockArea({ id: 'area-1', name: 'Backbone' }),
    ]);

    renderSection();

    await waitFor(() =>
      expect(screen.getByText('Unassigned - select area...')).toBeInTheDocument(),
    );
    expect(fetchAreas).toHaveBeenCalledTimes(1);

    (fetchAreas as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('offline'));

    renderSection();

    await waitFor(() => expect(fetchAreas).toHaveBeenCalledTimes(2));
    expect(screen.getAllByText('No areas created')).toHaveLength(1);
  });

  it('renders virtual color controls only for virtual devices in a map context', () => {
    const virtualForm = createDeviceConfigFormModel(
      mockDevice({
        device_type: 'virtual',
        ip: '',
        metrics_source: 'none',
        tags: { display_name: 'Virtual cloud', virtual_subtype: 'cloud' },
        map_visual_color: '#123ABC',
      }),
      true,
    );
    const { onVirtualChange, rerender } = renderSection({
      initialForm: virtualForm,
      isVirtual: true,
      mapContext: { mapId: 'map-1', mapName: 'Backbone' },
    });

    expect(screen.getByLabelText('Virtual node color')).toHaveValue('#123abc');
    fireEvent.change(screen.getByLabelText('Virtual node color'), { target: { value: '#ff00aa' } });
    expect(onVirtualChange).toHaveBeenLastCalledWith({ visualColor: '#FF00AA' });

    fireEvent.click(screen.getByRole('button', { name: 'Use area/default color' }));
    expect(onVirtualChange).toHaveBeenLastCalledWith({ visualColor: null });

    rerender(
      <DeviceAreasSection
        form={virtualForm}
        isVirtual={false}
        onFormChange={vi.fn()}
        onVirtualChange={vi.fn()}
      />,
    );
    expect(screen.queryByLabelText('Virtual node color')).not.toBeInTheDocument();
  });
});
