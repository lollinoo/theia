import type { Device, DeviceStatus, Link } from '../../types/api';
import {
  type AlertDTO,
  type AlertStatus,
  type DeviceMetricsDTO,
  type LinkMetricsDTO,
  type OperationalStatus,
  type PrimaryHealth,
  type PrometheusStatusPayload,
  type RuntimeFlag,
  type RuntimeReason,
  type SnapshotPayload,
  alertStatusForDevice,
  isPrometheusUnavailable,
} from '../../types/metrics';
import {
  type DeviceMonitoringState,
  resolveDeviceMonitoringState,
  sanitizeDeviceMetricsForDisplay,
} from '../deviceVisualState';
import { buildThroughputLabel, normalizeInterfaceName } from './canvasHelpers';

export interface RuntimeDeviceModel {
  device: Device;
  monitoringState: DeviceMonitoringState;
  metrics: DeviceMetricsDTO | null;
  alertStatus: AlertStatus;
  runtimeStatus: OperationalStatus | null;
  primaryHealth: PrimaryHealth | null;
  runtimeFlags: RuntimeFlag[];
}

export interface RuntimeLinkModel {
  link: Link;
  sourceDeviceStatus: DeviceStatus | 'unknown';
  targetDeviceStatus: DeviceStatus | 'unknown';
  metrics: LinkMetricsDTO | null;
  metricsUsable: boolean;
  metricsStatus: LinkMetricsDTO['metrics_status'] | null;
  metricsReason: RuntimeReason | null;
  throughputLabel: string | undefined;
  utilization: number | null;
}

export interface RuntimeState {
  prometheusDown: boolean;
  firingAlerts: AlertDTO[];
  devicesById: Map<string, RuntimeDeviceModel>;
  linksById: Map<string, RuntimeLinkModel>;
  interfaceMetricsByDeviceId: Map<string, Map<string, LinkMetricsDTO>>;
}

interface BuildRuntimeStateParams {
  devices: Device[];
  links: Link[];
  snapshot: SnapshotPayload | null;
  alerts: AlertDTO[];
  prometheusStatus: PrometheusStatusPayload | null;
}

export function countActiveAlertsFromRuntimeState(
  runtimeState: RuntimeState,
  alerts: AlertDTO[],
): number {
  const runtimeDevicesById = new Map(
    Array.from(runtimeState.devicesById.values())
      .filter((entry) => entry.runtimeStatus !== null)
      .map((entry) => [entry.device.id, entry] as const),
  );

  if (runtimeDevicesById.size === 0) {
    return alerts.filter((alert) => alert.state === 'firing').length;
  }

  return (
    Array.from(runtimeDevicesById.values()).reduce(
      (count, entry) => count + (entry.metrics?.firing_alert_count ?? 0),
      0,
    ) +
    alerts.filter((alert) => alert.state === 'firing' && !runtimeDevicesById.has(alert.device_id))
      .length
  );
}

function normalizeDeviceStatus(status: string | undefined): DeviceStatus | undefined {
  switch (status) {
    case 'up':
    case 'down':
    case 'probing':
    case 'unknown':
      return status;
    default:
      return undefined;
  }
}

function effectiveStatusForRuntimeDevice(
  runtimeDevice: DeviceMetricsDTO | undefined,
): DeviceStatus | undefined {
  return normalizeDeviceStatus(runtimeDevice?.operational_status);
}

function runtimeMonitoringState(
  device: Device,
  runtimeDevice: DeviceMetricsDTO | undefined,
): DeviceMonitoringState {
  return runtimeDevice?.operational_status === 'unmonitored'
    ? 'unmonitored'
    : resolveDeviceMonitoringState(device);
}

function buildInterfaceMetricsLookup(
  runtimeLinks: Record<string, LinkMetricsDTO>,
): Map<string, Map<string, LinkMetricsDTO>> {
  const metricsByDeviceId = new Map<string, Map<string, LinkMetricsDTO>>();

  function setInterfaceMetric(deviceId: string, ifName: string, metric: LinkMetricsDTO) {
    const metricsByIfName = metricsByDeviceId.get(deviceId) ?? new Map<string, LinkMetricsDTO>();
    metricsByIfName.set(normalizeInterfaceName(ifName), metric);
    metricsByDeviceId.set(deviceId, metricsByIfName);
  }

  for (const metric of Object.values(runtimeLinks)) {
    if (!metricsUsable(metric)) {
      continue;
    }

    setInterfaceMetric(metric.source_device_id, metric.source_if_name, metric);
    setInterfaceMetric(metric.target_device_id, metric.target_if_name, metric);
  }

  return metricsByDeviceId;
}

function metricsUsable(runtimeLink: LinkMetricsDTO | null): boolean {
  if (!runtimeLink) {
    return false;
  }

  return runtimeLink.metrics_status === 'available' || runtimeLink.metrics_status === 'partial';
}

export function buildRuntimeState({
  devices,
  links,
  snapshot,
  alerts,
  prometheusStatus,
}: BuildRuntimeStateParams): RuntimeState {
  const prometheusDown = isPrometheusUnavailable(prometheusStatus);
  const firingAlerts = alerts.filter((alert) => alert.state === 'firing');
  const devicesById = new Map<string, RuntimeDeviceModel>();
  const interfaceMetricsByDeviceId = buildInterfaceMetricsLookup(snapshot?.links ?? {});

  for (const device of devices) {
    const runtimeMetrics = snapshot?.devices[device.id];
    const monitoringState = runtimeMonitoringState(device, runtimeMetrics);
    const nextStatus = effectiveStatusForRuntimeDevice(runtimeMetrics);
    const runtimeDevice = nextStatus ? { ...device, status: nextStatus } : device;

    devicesById.set(device.id, {
      device: runtimeDevice,
      monitoringState,
      metrics: sanitizeDeviceMetricsForDisplay(
        runtimeDevice,
        runtimeMetrics ?? null,
        monitoringState,
      ),
      alertStatus: runtimeMetrics?.alert_status ?? alertStatusForDevice(device.id, firingAlerts),
      runtimeStatus: runtimeMetrics?.operational_status ?? null,
      primaryHealth: runtimeMetrics?.primary_health ?? null,
      runtimeFlags: runtimeMetrics?.runtime_flags ?? [],
    });
  }

  const linksById = new Map<string, RuntimeLinkModel>();

  for (const link of links) {
    const sourceDeviceStatus = devicesById.get(link.source_device_id)?.device.status ?? 'unknown';
    const targetDeviceStatus = devicesById.get(link.target_device_id)?.device.status ?? 'unknown';
    const runtimeMetrics = snapshot?.links[link.id] ?? null;
    const usableMetrics = metricsUsable(runtimeMetrics);
    const metrics = usableMetrics ? runtimeMetrics : null;

    linksById.set(link.id, {
      link,
      sourceDeviceStatus,
      targetDeviceStatus,
      metrics,
      metricsUsable: usableMetrics,
      metricsStatus: runtimeMetrics?.metrics_status ?? null,
      metricsReason: runtimeMetrics?.metrics_reason ?? null,
      throughputLabel: metrics ? buildThroughputLabel(metrics) : undefined,
      utilization: metrics?.utilization ?? null,
    });
  }

  return {
    prometheusDown,
    firingAlerts,
    devicesById,
    linksById,
    interfaceMetricsByDeviceId,
  };
}

export function primaryHealthPriority(value: PrimaryHealth | null): number {
  switch (value) {
    case 'quarantined':
      return 0;
    case 'unreachable':
      return 1;
    case 'snmp_degraded':
      return 2;
    case 'up_stale':
      return 3;
    case 'up_fresh':
      return 4;
    case 'probing':
      return 5;
    default:
      return 6;
  }
}
