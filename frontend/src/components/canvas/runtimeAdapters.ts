import type { Device, DeviceStatus, Link, MetricsSource } from '../../types/api';
import {
  alertStatusForDevice,
  isPrometheusUnavailable,
  type AlertDTO,
  type AlertStatus,
  type DeviceMetricsDTO,
  type LinkMetricsDTO,
  type PrometheusStatusPayload,
  type SnapshotPayload,
} from '../../types/metrics';
import {
  resolveDeviceMonitoringState,
  sanitizeDeviceMetricsForDisplay,
  type DeviceMonitoringState,
} from '../deviceVisualState';
import { buildThroughputLabel, findLinkMetrics, normalizeInterfaceName } from './canvasHelpers';

export interface RuntimeDeviceModel {
  device: Device;
  monitoringState: DeviceMonitoringState;
  metrics: DeviceMetricsDTO | null;
  alertStatus: AlertStatus;
  prometheusOutageMode: 'none' | 'offline' | 'fallback';
}

export interface RuntimeLinkModel {
  link: Link;
  sourceDeviceStatus: DeviceStatus | 'unknown';
  targetDeviceStatus: DeviceStatus | 'unknown';
  sourceMetrics: LinkMetricsDTO | null;
  targetMetrics: LinkMetricsDTO | null;
  metrics: LinkMetricsDTO | null;
  metricsUsable: boolean;
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

function effectiveStatusForDevice(
  device: Device,
  snapshot: SnapshotPayload | null,
  prometheusDown: boolean,
): DeviceStatus | undefined {
  if (prometheusDown) {
    const source = device.metrics_source || 'prometheus';
    if (source === 'prometheus' || source === 'prometheus_snmp_fallback') {
      return 'down';
    }
  }

  return normalizeDeviceStatus(snapshot?.device_statuses[device.id]);
}

function outageModeForMetricsSource(
  metricsSource: MetricsSource,
  prometheusDown: boolean,
): RuntimeDeviceModel['prometheusOutageMode'] {
  if (!prometheusDown) {
    return 'none';
  }

  if (metricsSource === 'prometheus') {
    return 'offline';
  }

  if (metricsSource === 'prometheus_snmp_fallback') {
    return 'fallback';
  }

  return 'none';
}

function findEndpointMetrics(
  snapshotMetrics: Record<string, LinkMetricsDTO[]>,
  deviceId: string,
  ifName: string,
): LinkMetricsDTO | null {
  const deviceMetrics = snapshotMetrics[deviceId];
  if (!deviceMetrics) {
    return null;
  }

  const normalizedIfName = normalizeInterfaceName(ifName);
  return deviceMetrics.find((metric) => normalizeInterfaceName(metric.if_name) === normalizedIfName) ?? null;
}

function buildInterfaceMetricsLookup(
  snapshotMetrics: Record<string, LinkMetricsDTO[]>,
): Map<string, Map<string, LinkMetricsDTO>> {
  const metricsByDeviceId = new Map<string, Map<string, LinkMetricsDTO>>();

  for (const [deviceId, metrics] of Object.entries(snapshotMetrics)) {
    const metricsByIfName = new Map<string, LinkMetricsDTO>();
    for (const metric of metrics) {
      metricsByIfName.set(normalizeInterfaceName(metric.if_name), metric);
    }
    metricsByDeviceId.set(deviceId, metricsByIfName);
  }

  return metricsByDeviceId;
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
  const interfaceMetricsByDeviceId = buildInterfaceMetricsLookup(snapshot?.link_metrics ?? {});

  for (const device of devices) {
    const nextStatus = effectiveStatusForDevice(device, snapshot, prometheusDown);
    const runtimeDevice = nextStatus
      ? { ...device, status: nextStatus }
      : device;

    devicesById.set(device.id, {
      device: runtimeDevice,
      monitoringState: resolveDeviceMonitoringState(runtimeDevice),
      metrics: snapshot
        ? sanitizeDeviceMetricsForDisplay(runtimeDevice, snapshot.device_metrics[device.id] ?? null)
        : null,
      alertStatus: alertStatusForDevice(device.id, firingAlerts),
      prometheusOutageMode: outageModeForMetricsSource(device.metrics_source, prometheusDown),
    });
  }

  const linksById = new Map<string, RuntimeLinkModel>();

  for (const link of links) {
    const sourceDeviceStatus = devicesById.get(link.source_device_id)?.device.status ?? 'unknown';
    const targetDeviceStatus = devicesById.get(link.target_device_id)?.device.status ?? 'unknown';
    const metricsBlocked = sourceDeviceStatus === 'down' && targetDeviceStatus === 'down';
    const sourceMetrics = snapshot && !metricsBlocked
      ? findEndpointMetrics(snapshot.link_metrics, link.source_device_id, link.source_if_name)
      : null;
    const targetMetrics = snapshot && !metricsBlocked
      ? findEndpointMetrics(snapshot.link_metrics, link.target_device_id, link.target_if_name)
      : null;
    const metrics = snapshot && !metricsBlocked
      ? findLinkMetrics(snapshot.link_metrics, link)
      : null;

    linksById.set(link.id, {
      link,
      sourceDeviceStatus,
      targetDeviceStatus,
      sourceMetrics,
      targetMetrics,
      metrics,
      metricsUsable: metrics !== null,
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
