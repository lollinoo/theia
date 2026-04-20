import { describe, expect, it } from 'vitest';

import type { Device, InterfaceInfo, Link } from '../../types/api';
import type { AlertDTO, SnapshotPayload } from '../../types/metrics';
import { buildRuntimeState } from './runtimeAdapters';
import {
  buildAlertsPanelModel,
  buildDeviceInterfacePanelModel,
  buildLinkInterfacePanelModel,
} from './panelAdapters';

function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    poll_class: 'standard',
    poll_interval_override: null,
    status: 'up',
    sys_name: 'router-01',
    sys_descr: 'RouterOS',
    hardware_model: 'RB4011',
    vendor: 'mikrotik',
    managed: true,
    interfaces: [],
    area_ids: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    ...overrides,
  };
}

function mockLink(overrides: Partial<Link> = {}): Link {
  return {
    id: 'link-1',
    source_device_id: 'dev-1',
    source_if_name: 'ether1',
    target_device_id: 'dev-2',
    target_if_name: 'ether2',
    discovery_protocol: 'lldp',
    source_if_speed: 1_000_000_000,
    source_if_oper_status: 'up',
    target_if_speed: 1_000_000_000,
    target_if_oper_status: 'up',
    ...overrides,
  };
}

function mockAlert(overrides: Partial<AlertDTO> = {}): AlertDTO {
  return {
    device_id: 'dev-1',
    severity: 'critical',
    alert_name: 'DeviceDown',
    state: 'firing',
    summary: 'router unreachable',
    ...overrides,
  };
}

function mockInterface(overrides: Partial<InterfaceInfo> = {}): InterfaceInfo {
  return {
    if_name: 'ether1',
    if_descr: 'Uplink',
    speed: 1_000_000_000,
    oper_status: 'up',
    admin_status: 'up',
    in_use: true,
    ...overrides,
  };
}

function mockSnapshot(overrides: Partial<SnapshotPayload> = {}): SnapshotPayload {
  return {
    device_metrics: {},
    link_metrics: {
      'dev-1': [{
        device_id: 'dev-1',
        if_name: 'ether1',
        tx_bps: 1_500,
        rx_bps: 2_500,
        utilization: 0.42,
        collected_at: '2026-04-20T12:00:00Z',
      }],
      'dev-2': [{
        device_id: 'dev-2',
        if_name: 'ether2',
        tx_bps: 3_500,
        rx_bps: 4_500,
        utilization: 0.91,
        collected_at: '2026-04-20T12:00:00Z',
      }],
    },
    device_statuses: {
      'dev-1': 'up',
      'dev-2': 'up',
    },
    ...overrides,
  };
}

describe('panelAdapters', () => {
  it('builds explicit alert view models without leaking raw alert dto fields', () => {
    const devices = [
      mockDevice({ tags: { display_name: 'Core Router' } }),
      mockDevice({
        id: 'dev-2',
        ip: '10.0.0.2',
        sys_name: 'switch-01',
        metrics_source: 'prometheus_snmp_fallback',
        tags: { display_name: 'Edge Switch' },
      }),
    ];
    const runtimeState = buildRuntimeState({
      devices,
      links: [],
      snapshot: mockSnapshot(),
      alerts: [mockAlert()],
      prometheusStatus: { enabled: true, available: false },
    });

    const model = buildAlertsPanelModel({
      alerts: [mockAlert()],
      runtimeState,
    });

    expect(model.firingAlerts[0]).toEqual({
      deviceId: 'dev-1',
      deviceLabel: 'Core Router',
      alertName: 'DeviceDown',
      severity: 'critical',
      state: 'firing',
      summary: 'router unreachable',
    });
    expect(model.prometheusOutage).toEqual({
      offlineDevices: [{ id: 'dev-1', label: 'Core Router' }],
      fallbackDevices: [{ id: 'dev-2', label: 'Edge Switch' }],
    });
    expect(model.firingAlerts[0]).not.toHaveProperty('device_id');
    expect(model.firingAlerts[0]).not.toHaveProperty('alert_name');
  });

  it('builds ordered device interface sections from runtime state and fetched inventory', () => {
    const device = mockDevice();
    const runtimeState = buildRuntimeState({
      devices: [device],
      links: [mockLink({ target_device_id: 'dev-3', target_if_name: 'ether9' })],
      snapshot: mockSnapshot({
        link_metrics: {
          'dev-1': [{
            device_id: 'dev-1',
            if_name: 'ether1',
            tx_bps: 1_500,
            rx_bps: 2_500,
            utilization: 0.42,
            collected_at: '2026-04-20T12:00:00Z',
          }],
        },
        device_statuses: { 'dev-1': 'up' },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    const model = buildDeviceInterfacePanelModel({
      device,
      runtimeState,
      loadingInterfaces: false,
      interfaces: [
        mockInterface({ if_name: 'ether2', oper_status: 'down', if_descr: 'Secondary' }),
        mockInterface({ if_name: 'lo', if_descr: 'Loopback' }),
        mockInterface({ if_name: 'ether1', oper_status: 'up' }),
      ],
    });

    expect(model.sections.map((section) => section.ifName)).toEqual(['ether1', 'ether2']);
    expect(model.sections[0]).toMatchObject({
      interfaceDescription: 'Uplink',
      speedLabel: '1 Gbps',
      statusLabel: 'up',
      txLabel: '2 Kbps',
      rxLabel: '3 Kbps',
      utilizationPct: 42,
    });
  });

  it('precomputes device interface unavailable state in the final section model', () => {
    const device = mockDevice({ status: 'up' });
    const runtimeState = buildRuntimeState({
      devices: [device],
      links: [mockLink({ target_device_id: 'dev-3', target_if_name: 'ether9' })],
      snapshot: mockSnapshot({
        link_metrics: {
          'dev-1': [{
            device_id: 'dev-1',
            if_name: 'ether1',
            tx_bps: 1_500,
            rx_bps: 2_500,
            utilization: 0.42,
            collected_at: '2026-04-20T12:00:00Z',
          }],
        },
        device_statuses: { 'dev-1': 'down' },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    const model = buildDeviceInterfacePanelModel({
      device,
      runtimeState,
      loadingInterfaces: false,
      interfaces: [mockInterface()],
    });

    expect(model.sections[0]).toMatchObject({
      availabilityReason: 'device-down',
      metricsUnavailableMessage: 'Device unreachable',
      statusLabel: 'down',
      statusTone: 'down',
      txLabel: '--',
      rxLabel: '--',
      utilizationPct: null,
    });
  });

  it('builds link interface sections and negotiation summary from fetched inventory', () => {
    const source = mockDevice();
    const target = mockDevice({
      id: 'dev-2',
      ip: '10.0.0.2',
      sys_name: 'switch-01',
    });
    const link = mockLink();
    const runtimeState = buildRuntimeState({
      devices: [source, target],
      links: [link],
      snapshot: mockSnapshot(),
      alerts: [],
      prometheusStatus: null,
    });

    const model = buildLinkInterfacePanelModel({
      link,
      sourceDevice: source,
      targetDevice: target,
      sourceInterfaces: [mockInterface()],
      targetInterfaces: [mockInterface({ if_name: 'ether2', if_descr: 'Downlink' })],
      runtimeState,
    });

    expect(model.negotiation).toMatchObject({
      summaryLabel: 'Matched at 1 Gbps',
      sourceLabel: '1 Gbps',
      targetLabel: '1 Gbps',
      tone: 'matched',
    });
    expect(model.source).toMatchObject({
      interfaceDescription: 'Uplink',
      txLabel: '2 Kbps',
      rxLabel: '3 Kbps',
      utilizationPct: 42,
    });
    expect(model.target).toMatchObject({
      interfaceDescription: 'Downlink',
      txLabel: '4 Kbps',
      rxLabel: '5 Kbps',
      utilizationPct: 91,
    });
  });

  it('builds a mismatch negotiation summary when endpoint speeds differ', () => {
    const source = mockDevice();
    const target = mockDevice({ id: 'dev-2', ip: '10.0.0.2', sys_name: 'switch-01' });
    const link = mockLink();
    const runtimeState = buildRuntimeState({
      devices: [source, target],
      links: [link],
      snapshot: mockSnapshot(),
      alerts: [],
      prometheusStatus: null,
    });

    const model = buildLinkInterfacePanelModel({
      link,
      sourceDevice: source,
      targetDevice: target,
      sourceInterfaces: [mockInterface({ speed: 1_000_000_000 })],
      targetInterfaces: [mockInterface({ if_name: 'ether2', speed: 100_000_000 })],
      runtimeState,
    });

    expect(model.negotiation).toMatchObject({
      summaryLabel: '1 Gbps vs 100 Mbps',
      detailLabel: 'The two ends report different negotiated speeds.',
      tone: 'mismatch',
    });
  });

  it('builds a partial negotiation summary when only one endpoint exposes speed', () => {
    const source = mockDevice();
    const target = mockDevice({ id: 'dev-2', ip: '10.0.0.2', sys_name: 'switch-01' });
    const link = mockLink();
    const runtimeState = buildRuntimeState({
      devices: [source, target],
      links: [link],
      snapshot: mockSnapshot(),
      alerts: [],
      prometheusStatus: null,
    });

    const model = buildLinkInterfacePanelModel({
      link,
      sourceDevice: source,
      targetDevice: target,
      sourceInterfaces: [mockInterface({ speed: 1_000_000_000 })],
      targetInterfaces: [mockInterface({ if_name: 'ether2', speed: 0 })],
      runtimeState,
    });

    expect(model.negotiation).toMatchObject({
      summaryLabel: '1 Gbps',
      detailLabel: 'Only one side exposed a negotiated speed.',
      tone: 'partial',
    });
  });

  it('builds an unknown negotiation summary when neither endpoint exposes speed', () => {
    const source = mockDevice();
    const target = mockDevice({ id: 'dev-2', ip: '10.0.0.2', sys_name: 'switch-01' });
    const link = mockLink();
    const runtimeState = buildRuntimeState({
      devices: [source, target],
      links: [link],
      snapshot: mockSnapshot(),
      alerts: [],
      prometheusStatus: null,
    });

    const model = buildLinkInterfacePanelModel({
      link,
      sourceDevice: source,
      targetDevice: target,
      sourceInterfaces: [mockInterface({ speed: 0 })],
      targetInterfaces: [mockInterface({ if_name: 'ether2', speed: 0 })],
      runtimeState,
    });

    expect(model.negotiation).toMatchObject({
      summaryLabel: 'Autonegotiation',
      detailLabel: 'Waiting for interface speed data from one or both ends.',
      tone: 'unknown',
    });
  });

  it('keeps fallback metrics visible during a Prometheus outage', () => {
    const source = mockDevice();
    const target = mockDevice({
      id: 'dev-2',
      ip: '10.0.0.2',
      sys_name: 'switch-01',
      metrics_source: 'prometheus_snmp_fallback',
    });
    const link = mockLink();
    const runtimeState = buildRuntimeState({
      devices: [source, target],
      links: [link],
      snapshot: mockSnapshot(),
      alerts: [],
      prometheusStatus: { enabled: true, available: false },
    });

    const model = buildLinkInterfacePanelModel({
      link,
      sourceDevice: source,
      targetDevice: target,
      sourceInterfaces: [mockInterface()],
      targetInterfaces: [mockInterface({ if_name: 'ether2', if_descr: 'Downlink' })],
      runtimeState,
    });

    expect(model.source).toMatchObject({
      metricsUnavailableMessage: 'Prometheus unavailable',
      txLabel: '--',
      rxLabel: '--',
    });
    expect(model.target).toMatchObject({
      metricsUnavailableMessage: null,
      txLabel: '4 Kbps',
      rxLabel: '5 Kbps',
      utilizationPct: 91,
    });
  });

  it('uses snapshot interface metrics even when no topology link exists', () => {
    const device = mockDevice();
    const runtimeState = buildRuntimeState({
      devices: [device],
      links: [],
      snapshot: mockSnapshot({
        link_metrics: {
          'dev-1': [{
            device_id: 'dev-1',
            if_name: 'ether7',
            tx_bps: 7_500,
            rx_bps: 8_500,
            utilization: 0.11,
            collected_at: '2026-04-20T12:00:00Z',
          }],
        },
        device_statuses: { 'dev-1': 'up' },
      }),
      alerts: [],
      prometheusStatus: null,
    });

    const model = buildDeviceInterfacePanelModel({
      device,
      runtimeState,
      loadingInterfaces: false,
      interfaces: [mockInterface({ if_name: 'ether7', if_descr: 'Standalone uplink' })],
    });

    expect(model.sections[0]).toMatchObject({
      ifName: 'ether7',
      txLabel: '8 Kbps',
      rxLabel: '9 Kbps',
      utilizationPct: 11,
    });
  });

  it('keeps unknown interface status in a neutral tone', () => {
    const device = mockDevice();
    const runtimeState = buildRuntimeState({
      devices: [device],
      links: [],
      snapshot: mockSnapshot({ device_statuses: { 'dev-1': 'up' } }),
      alerts: [],
      prometheusStatus: null,
    });

    const model = buildDeviceInterfacePanelModel({
      device,
      runtimeState,
      loadingInterfaces: false,
      interfaces: [mockInterface({ if_name: 'ether7', oper_status: 'unknown' })],
    });

    expect(model.sections[0]).toMatchObject({
      statusLabel: 'unknown',
      statusTone: 'neutral',
    });
  });

  it('falls back to link endpoint speed and status when interface inventory is missing', () => {
    const source = mockDevice();
    const target = mockDevice({ id: 'dev-2', ip: '10.0.0.2', sys_name: 'switch-01' });
    const link = mockLink({
      source_if_speed: 1_000_000_000,
      source_if_oper_status: 'up',
      target_if_speed: 100_000_000,
      target_if_oper_status: 'unknown',
    });
    const runtimeState = buildRuntimeState({
      devices: [source, target],
      links: [link],
      snapshot: mockSnapshot(),
      alerts: [],
      prometheusStatus: null,
    });

    const model = buildLinkInterfacePanelModel({
      link,
      sourceDevice: source,
      targetDevice: target,
      sourceInterfaces: [],
      targetInterfaces: [],
      runtimeState,
    });

    expect(model.negotiation).toMatchObject({
      summaryLabel: '1 Gbps vs 100 Mbps',
      tone: 'mismatch',
    });
    expect(model.source).toMatchObject({
      speedLabel: '1 Gbps',
      statusLabel: 'up',
      statusTone: 'up',
    });
    expect(model.target).toMatchObject({
      speedLabel: '100 Mbps',
      statusLabel: 'unknown',
      statusTone: 'neutral',
    });
  });
});
