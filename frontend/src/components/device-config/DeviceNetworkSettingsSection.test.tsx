/**
 * Exercises device network settings section device configuration behavior so refactors preserve the documented contract.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Device, SNMPProfile } from '../../types/api';
import { createDeviceConfigFormModel, type DeviceFormModel } from '../forms/deviceFormModels';
import { DeviceNetworkSettingsSection } from './DeviceNetworkSettingsSection';

vi.mock('../../api/client', () => ({
  checkPrometheusHealth: vi.fn().mockImplementation(() => new Promise<never>(() => {})),
  fetchSNMPProfiles: vi.fn().mockImplementation(() => new Promise<never>(() => {})),
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

function mockSNMPProfile(overrides: Partial<SNMPProfile> = {}): SNMPProfile {
  return {
    id: 'profile-1',
    name: 'Router SNMP',
    description: '',
    snmp: {
      version: '3',
      username: 'monitor',
      security_level: 'authPriv',
      auth_protocol: 'SHA',
      priv_protocol: 'AES',
    },
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

function renderSection({
  device = mockDevice(),
  initialForm = createDeviceConfigFormModel(device, false),
  readOnly = false,
  isVirtual = false,
  fieldErrors = {},
  onFieldError = vi.fn(),
  onSNMPProfileSelected = vi.fn(),
}: {
  device?: Device;
  initialForm?: DeviceFormModel;
  readOnly?: boolean;
  isVirtual?: boolean;
  fieldErrors?: Record<string, string>;
  onFieldError?: (field: string, err: string | null) => void;
  onSNMPProfileSelected?: (profileId: string) => void;
} = {}) {
  let form = initialForm;
  const onFormChange = vi.fn((update: Partial<DeviceFormModel>) => {
    form = { ...form, ...update };
  });
  const onPrometheusChange = vi.fn((update: Partial<DeviceFormModel['prometheus']>) => {
    form = { ...form, prometheus: { ...form.prometheus, ...update } };
  });
  const onSnmpChange = vi.fn((update: Partial<DeviceFormModel['snmp']>) => {
    form = { ...form, snmp: { ...form.snmp, ...update } };
  });

  const result = render(
    <DeviceNetworkSettingsSection
      device={device}
      form={form}
      readOnly={readOnly}
      isVirtual={isVirtual}
      fieldErrors={fieldErrors}
      onFormChange={onFormChange}
      onPrometheusChange={onPrometheusChange}
      onSnmpChange={onSnmpChange}
      onFieldError={onFieldError}
      onSNMPProfileSelected={onSNMPProfileSelected}
    />,
  );

  return {
    ...result,
    onFieldError,
    onFormChange,
    onPrometheusChange,
    onSnmpChange,
    onSNMPProfileSelected,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DeviceNetworkSettingsSection', () => {
  it('renders nothing for virtual devices and does not load physical-device network settings', async () => {
    const { checkPrometheusHealth, fetchSNMPProfiles } = await import('../../api/client');

    renderSection({
      isVirtual: true,
      initialForm: createDeviceConfigFormModel(
        mockDevice({ device_type: 'virtual', metrics_source: 'none' }),
        true,
      ),
    });

    expect(screen.queryByText('Vendor')).not.toBeInTheDocument();
    expect(screen.queryByText('Metrics Source')).not.toBeInTheDocument();
    expect(checkPrometheusHealth).not.toHaveBeenCalled();
    expect(fetchSNMPProfiles).not.toHaveBeenCalled();
  });

  it('keeps vendor options and reports vendor changes to the parent form', () => {
    const { onFormChange } = renderSection({
      initialForm: {
        ...createDeviceConfigFormModel(mockDevice({ vendor: '' }), false),
        vendor: '',
      },
    });

    expect(screen.getByText('Vendor')).toBeInTheDocument();
    expect(screen.getByText('— Select vendor —')).toBeInTheDocument();
    expect(screen.getByText('MikroTik')).toBeInTheDocument();
    expect(
      screen.getByText('Vendor tag determines backup commands and metric queries.'),
    ).toBeInTheDocument();

    fireEvent.change(screen.getByDisplayValue('— Select vendor —'), {
      target: { value: 'mikrotik' },
    });

    expect(onFormChange).toHaveBeenCalledWith({ vendor: 'mikrotik' });
  });

  it('renders additional address rows and reports edits to the parent form', () => {
    const { onFormChange } = renderSection({
      initialForm: {
        ...createDeviceConfigFormModel(mockDevice(), false),
        additionalAddresses: [{ address: '192.0.2.10', role: 'backup', label: 'OOB' }],
      },
    });

    expect(screen.getByText('Additional addresses')).toBeInTheDocument();
    expect(screen.getByLabelText('Additional address 1')).toHaveValue('192.0.2.10');
    expect(screen.getByLabelText('Address role 1')).toHaveValue('backup');
    expect(screen.getByLabelText('Address label 1')).toHaveValue('OOB');

    fireEvent.change(screen.getByLabelText('Additional address 1'), {
      target: { value: '192.0.2.11' },
    });

    expect(onFormChange).toHaveBeenCalledWith({
      additionalAddresses: [{ address: '192.0.2.11', role: 'backup', label: 'OOB' }],
    });

    fireEvent.click(screen.getByRole('button', { name: 'Add address' }));

    expect(onFormChange).toHaveBeenCalledWith({
      additionalAddresses: [
        { address: '192.0.2.10', role: 'backup', label: 'OOB' },
        expect.objectContaining({ address: '', role: 'management', label: '' }),
      ],
    });
  });

  it('warns and rejects Prometheus modes when Prometheus health is unavailable', async () => {
    const { checkPrometheusHealth } = await import('../../api/client');
    (checkPrometheusHealth as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      available: false,
      enabled: true,
      url: '',
    });
    const { onFormChange } = renderSection();

    await waitFor(() => {
      expect(
        screen.getByText(
          'Prometheus is not configured or unreachable. Only SNMP Direct is available.',
        ),
      ).toBeInTheDocument();
    });

    expect(screen.getByText('Prometheus (unavailable)')).toBeDisabled();
    expect(screen.getByText('Prometheus + SNMP Fallback (unavailable)')).toBeDisabled();

    fireEvent.change(screen.getByDisplayValue('SNMP Direct'), {
      target: { value: 'prometheus' },
    });

    expect(onFormChange).not.toHaveBeenCalledWith({ metricsMode: 'prometheus' });
  });

  it('renders Prometheus target controls only for Prometheus-backed modes', async () => {
    const { checkPrometheusHealth } = await import('../../api/client');
    (checkPrometheusHealth as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      available: true,
      enabled: true,
      url: '',
    });
    const device = mockDevice({ metrics_source: 'prometheus', prometheus_label_value: '' });
    const { onFormChange, onPrometheusChange, onFieldError } = renderSection({
      device,
      initialForm: createDeviceConfigFormModel(device, false),
    });

    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'Prometheus' })).not.toBeDisabled();
    });
    expect(screen.getByText('Prometheus Target')).toBeInTheDocument();
    expect(
      screen.getByText('Metrics from Prometheus only. No fallback if Prometheus is unreachable.'),
    ).toBeInTheDocument();
    expect(screen.getByDisplayValue('instance (IP address)')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('10.0.0.1')).toBeInTheDocument();

    fireEvent.change(screen.getByDisplayValue('instance (IP address)'), {
      target: { value: 'identity' },
    });
    expect(onPrometheusChange).toHaveBeenCalledWith({ labelName: 'identity' });

    fireEvent.change(screen.getByPlaceholderText('10.0.0.1'), {
      target: { value: 'router-01' },
    });
    expect(onPrometheusChange).toHaveBeenCalledWith({ labelValue: 'router-01' });
    expect(onFieldError).toHaveBeenCalledWith('prometheusLabelValue', null);

    fireEvent.change(screen.getAllByRole('combobox')[1], {
      target: { value: 'prometheus_snmp_fallback' },
    });
    expect(onFormChange).toHaveBeenCalledWith({ metricsMode: 'prometheus_snmp_fallback' });
  });

  it('renders SNMP profile loader and SNMPv3 controls for SNMP-backed modes', async () => {
    const { fetchSNMPProfiles } = await import('../../api/client');
    (fetchSNMPProfiles as ReturnType<typeof vi.fn>).mockResolvedValueOnce([mockSNMPProfile()]);
    const device = mockDevice({ metrics_source: 'prometheus_snmp_fallback' });
    const { onSNMPProfileSelected, onSnmpChange } = renderSection({
      device,
      initialForm: {
        ...createDeviceConfigFormModel(device, false),
        metricsMode: 'prometheus_snmp_fallback',
        snmp: {
          ...createDeviceConfigFormModel(device, false).snmp,
          version: '3',
          securityLevel: 'authPriv',
        },
      },
    });

    await waitFor(() => {
      expect(screen.getByText('Router SNMP (SNMP 3)')).toBeInTheDocument();
    });
    expect(
      screen.getByText(
        'Falls back to SNMP if Prometheus is unavailable or has no data for this device.',
      ),
    ).toBeInTheDocument();
    expect(screen.getByDisplayValue('SNMP v3')).toBeInTheDocument();
    expect(
      screen.getByText('SNMPv3 Credentials (leave blank to keep current)'),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Username')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Auth Key')).toHaveAttribute('autocomplete', 'new-password');
    expect(screen.getByPlaceholderText('Encryption Key')).toHaveAttribute(
      'autocomplete',
      'new-password',
    );

    fireEvent.change(screen.getByDisplayValue('Load credentials from profile...'), {
      target: { value: 'profile-1' },
    });
    expect(onSNMPProfileSelected).toHaveBeenCalledWith('profile-1');

    fireEvent.change(screen.getByDisplayValue('SNMP v3'), { target: { value: '2c' } });
    expect(onSnmpChange).toHaveBeenCalledWith({ version: '2c' });
  });

  it('disables network setting controls when read-only', () => {
    renderSection({ readOnly: true });

    expect(screen.getByDisplayValue('MikroTik')).toBeDisabled();
    expect(screen.getByDisplayValue('SNMP Direct')).toBeDisabled();
    expect(screen.getByDisplayValue('SNMP v2c')).toBeDisabled();
    expect(
      screen.getByPlaceholderText('SNMP Community (leave blank to keep current)'),
    ).toBeDisabled();
  });
});
