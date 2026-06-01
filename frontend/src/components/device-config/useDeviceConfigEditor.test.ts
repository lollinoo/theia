import { act, renderHook, waitFor } from '@testing-library/react';
import type { FormEvent } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ServerError } from '../../api/errors';
import type { Device, SNMPProfile } from '../../types/api';
import { useDeviceConfigEditor } from './useDeviceConfigEditor';

const apiMocks = vi.hoisted(() => ({
  revealSNMPProfile: vi.fn(),
  updateCanvasMapDeviceAreas: vi.fn(),
  updateCanvasMapDeviceVisualColor: vi.fn(),
  updateDevice: vi.fn(),
}));

vi.mock('../../api/client', () => apiMocks);

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

function renderEditor(
  options: {
    device?: Device;
    readOnly?: boolean;
    isVirtual?: boolean;
    mapContext?: { mapId: string; mapName: string };
    onDeviceUpdated?: (updated: Device) => void;
  } = {},
) {
  return renderHook(
    (props) =>
      useDeviceConfigEditor({
        device: props.device,
        readOnly: props.readOnly ?? false,
        isVirtual: props.isVirtual,
        mapContext: props.mapContext,
        onDeviceUpdated: props.onDeviceUpdated,
      }),
    {
      initialProps: {
        device: options.device ?? mockDevice(),
        readOnly: options.readOnly,
        isVirtual: options.isVirtual,
        mapContext: options.mapContext,
        onDeviceUpdated: options.onDeviceUpdated ?? vi.fn(),
      },
    },
  );
}

function submitEvent() {
  return { preventDefault: vi.fn() } as unknown as FormEvent;
}

describe('useDeviceConfigEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMocks.updateDevice.mockResolvedValue(mockDevice());
    apiMocks.updateCanvasMapDeviceAreas.mockResolvedValue({});
    apiMocks.updateCanvasMapDeviceVisualColor.mockResolvedValue({});
  });

  it('preserves in-progress edits when area IDs are returned in a different order', () => {
    const { result, rerender } = renderEditor({
      device: mockDevice({
        area_ids: ['area-1', 'area-2'],
        tags: { display_name: 'Core router' },
      }),
    });

    act(() => {
      result.current.updateForm({ displayName: 'Unsaved local edit' });
      result.current.setFieldError('displayName', 'Display name is too long');
    });

    rerender({
      device: mockDevice({
        area_ids: ['area-2', 'area-1'],
        tags: { display_name: 'Core router' },
      }),
      readOnly: false,
      isVirtual: undefined,
      mapContext: undefined,
      onDeviceUpdated: vi.fn(),
    });

    expect(result.current.form.displayName).toBe('Unsaved local edit');
    expect(result.current.fieldErrors['displayName']).toBe('Display name is too long');
  });

  it('preserves in-progress form edits for runtime-only changes and resets for config changes', () => {
    const { result, rerender } = renderEditor({
      device: mockDevice({ tags: { display_name: 'Core router' } }),
    });

    act(() => {
      result.current.updateForm({ displayName: 'Unsaved local edit' });
      result.current.setFieldError('displayName', 'Display name is too long');
    });

    rerender({
      device: mockDevice({ status: 'down', tags: { display_name: 'Core router' } }),
      readOnly: false,
      isVirtual: undefined,
      mapContext: undefined,
      onDeviceUpdated: vi.fn(),
    });

    expect(result.current.form.displayName).toBe('Unsaved local edit');
    expect(result.current.fieldErrors['displayName']).toBe('Display name is too long');

    rerender({
      device: mockDevice({ tags: { display_name: 'Distribution router' } }),
      readOnly: false,
      isVirtual: undefined,
      mapContext: undefined,
      onDeviceUpdated: vi.fn(),
    });

    expect(result.current.form.displayName).toBe('Distribution router');
    expect(result.current.fieldErrors).toEqual({});
  });

  it('blocks save on validation errors and does not call updateDevice', async () => {
    const { result } = renderEditor();

    act(() => {
      result.current.updateForm({ ip: '' });
    });

    await act(async () => {
      await result.current.handleEditSave(submitEvent());
    });

    expect(result.current.fieldErrors['ip']).toBe('IP address or hostname is required');
    expect(apiMocks.updateDevice).not.toHaveBeenCalled();
  });

  it('saves physical map-scoped areas without sending global area_ids', async () => {
    const onDeviceUpdated = vi.fn();
    const updatedDevice = mockDevice({ notes: 'Updated globally' });
    apiMocks.updateDevice.mockResolvedValueOnce(updatedDevice);
    const { result } = renderEditor({
      mapContext: { mapId: 'map-1', mapName: 'Backbone' },
      onDeviceUpdated,
    });

    act(() => {
      result.current.updateForm({ areaIds: ['area-1'] });
    });

    await act(async () => {
      await result.current.handleEditSave(submitEvent());
    });

    expect(apiMocks.updateDevice).toHaveBeenCalledWith(
      'dev-1',
      expect.not.objectContaining({ area_ids: expect.anything() }),
    );
    expect(apiMocks.updateCanvasMapDeviceAreas).toHaveBeenCalledWith('map-1', {
      device_ids: ['dev-1'],
      area_ids: ['area-1'],
    });
    expect(onDeviceUpdated).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'dev-1', area_ids: ['area-1'] }),
    );
  });

  it('saves virtual map-scoped visual color without a global update when only color changed', async () => {
    const onDeviceUpdated = vi.fn();
    const virtualDevice = mockDevice({
      device_type: 'virtual',
      ip: '',
      metrics_source: 'none',
      tags: { display_name: 'Virtual cloud', virtual_subtype: 'cloud' },
      map_visual_color: '#123ABC',
    });
    const { result } = renderEditor({
      device: virtualDevice,
      isVirtual: true,
      mapContext: { mapId: 'map-1', mapName: 'Backbone' },
      onDeviceUpdated,
    });

    act(() => {
      result.current.updateVirtual({ visualColor: null });
    });

    await act(async () => {
      await result.current.handleEditSave(submitEvent());
    });

    expect(apiMocks.updateDevice).not.toHaveBeenCalled();
    expect(apiMocks.updateCanvasMapDeviceVisualColor).toHaveBeenCalledWith('map-1', 'dev-1', {
      visual_color: null,
    });
    expect(onDeviceUpdated).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'dev-1', map_visual_color: null }),
    );
  });

  it('maps ServerError to sanitized editError with correlation ref', async () => {
    apiMocks.updateDevice.mockRejectedValueOnce(new ServerError('raw internal failure', 'dc001'));
    const { result } = renderEditor();

    await act(async () => {
      await result.current.handleEditSave(submitEvent());
    });

    expect(result.current.editError).toBe('Something went wrong (ref: dc001)');
  });

  it('applies a revealed SNMP profile with the device config purpose string', async () => {
    const profile: SNMPProfile = {
      id: 'profile-1',
      name: 'Core v3',
      description: '',
      snmp: {
        version: '3',
        username: 'snmp-user',
        security_level: 'authPriv',
        auth_protocol: 'SHA',
        auth_password: 'auth-secret',
        priv_protocol: 'AES',
        priv_password: 'priv-secret',
      },
      created_at: '',
      updated_at: '',
    };
    apiMocks.revealSNMPProfile.mockResolvedValueOnce(profile);
    const { result } = renderEditor();

    await act(async () => {
      await result.current.applyProfile('profile-1');
    });

    expect(apiMocks.revealSNMPProfile).toHaveBeenCalledWith(
      'profile-1',
      'apply SNMP profile to device config',
    );
    await waitFor(() => {
      expect(result.current.form.snmp).toMatchObject({
        version: '3',
        username: 'snmp-user',
        authPassword: 'auth-secret',
        privPassword: 'priv-secret',
      });
    });
  });

  it('ignores a revealed SNMP profile when the editor switches devices before reveal completes', async () => {
    let resolveProfile: (profile: SNMPProfile) => void = () => {};
    const profile: SNMPProfile = {
      id: 'profile-1',
      name: 'Core v3',
      description: '',
      snmp: {
        version: '3',
        username: 'snmp-user',
        security_level: 'authPriv',
        auth_protocol: 'SHA',
        auth_password: 'auth-secret',
        priv_protocol: 'AES',
        priv_password: 'priv-secret',
      },
      created_at: '',
      updated_at: '',
    };
    apiMocks.revealSNMPProfile.mockReturnValueOnce(
      new Promise<SNMPProfile>((resolve) => {
        resolveProfile = resolve;
      }),
    );
    const { result, rerender } = renderEditor({
      device: mockDevice({ id: 'dev-1', hostname: 'router-01' }),
    });

    let applyPromise: Promise<void> | undefined;
    act(() => {
      applyPromise = result.current.applyProfile('profile-1');
    });

    rerender({
      device: mockDevice({ id: 'dev-2', hostname: 'router-02' }),
      readOnly: false,
      isVirtual: undefined,
      mapContext: undefined,
      onDeviceUpdated: vi.fn(),
    });

    await act(async () => {
      resolveProfile(profile);
      await applyPromise;
    });

    expect(result.current.form.hostname).toBe('router-02');
    expect(result.current.form.snmp.username).toBe('');
    expect(result.current.form.snmp.authPassword).toBe('');
    expect(result.current.form.snmp.privPassword).toBe('');
  });

  it('keeps the second SNMP profile when the first reveal resolves last', async () => {
    let resolveFirstProfile: (profile: SNMPProfile) => void = () => {};
    let resolveSecondProfile: (profile: SNMPProfile) => void = () => {};
    const firstProfile: SNMPProfile = {
      id: 'profile-1',
      name: 'Core v3',
      description: '',
      snmp: {
        version: '3',
        username: 'first-user',
        security_level: 'authPriv',
        auth_protocol: 'SHA',
        auth_password: 'first-auth-secret',
        priv_protocol: 'AES',
        priv_password: 'first-priv-secret',
      },
      created_at: '',
      updated_at: '',
    };
    const secondProfile: SNMPProfile = {
      id: 'profile-2',
      name: 'Distribution v3',
      description: '',
      snmp: {
        version: '3',
        username: 'second-user',
        security_level: 'authPriv',
        auth_protocol: 'SHA',
        auth_password: 'second-auth-secret',
        priv_protocol: 'AES',
        priv_password: 'second-priv-secret',
      },
      created_at: '',
      updated_at: '',
    };
    apiMocks.revealSNMPProfile
      .mockReturnValueOnce(
        new Promise<SNMPProfile>((resolve) => {
          resolveFirstProfile = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise<SNMPProfile>((resolve) => {
          resolveSecondProfile = resolve;
        }),
      );
    const { result } = renderEditor();

    let firstApplyPromise: Promise<void> | undefined;
    let secondApplyPromise: Promise<void> | undefined;
    act(() => {
      firstApplyPromise = result.current.applyProfile('profile-1');
      secondApplyPromise = result.current.applyProfile('profile-2');
    });

    await act(async () => {
      resolveSecondProfile(secondProfile);
      await secondApplyPromise;
    });
    await act(async () => {
      resolveFirstProfile(firstProfile);
      await firstApplyPromise;
    });

    expect(result.current.form.snmp).toMatchObject({
      version: '3',
      username: 'second-user',
      authPassword: 'second-auth-secret',
      privPassword: 'second-priv-secret',
    });
  });

  it('clears the saved indicator timeout on unmount after a successful save', async () => {
    vi.useFakeTimers();
    const clearTimeoutSpy = vi.spyOn(window, 'clearTimeout');
    try {
      const { result, unmount } = renderEditor();

      await act(async () => {
        await result.current.handleEditSave(submitEvent());
      });

      expect(result.current.editSaved).toBe(true);

      unmount();

      expect(clearTimeoutSpy).toHaveBeenCalled();
    } finally {
      clearTimeoutSpy.mockRestore();
      vi.useRealTimers();
    }
  });
});
