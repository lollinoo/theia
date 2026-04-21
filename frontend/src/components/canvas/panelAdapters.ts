import type { Device, InterfaceInfo, Link } from '../../types/api';
import {
  type AlertDTO,
  type LinkMetricsDTO,
  type RuntimeReason,
  formatThroughput,
  utilizationColor,
} from '../../types/metrics';
import { formatBandwidth } from '../linkSemantics';
import type {
  AlertsPanelAlertModel,
  AlertsPanelModel,
  DeviceInterfacePanelModel,
  InterfaceSectionModel,
  LinkInterfacePanelModel,
  LinkNegotiationModel,
} from '../panelModels';
import { normalizeInterfaceName } from './canvasHelpers';
import {
  type RuntimeDeviceModel,
  type RuntimeState,
  countActiveAlertsFromRuntimeState,
} from './runtimeAdapters';

const UNKNOWN_UTILIZATION_COLOR = 'var(--color-status-unknown)';

export interface AdaptedInterfaceStats {
  txLabel: string;
  rxLabel: string;
  utilizationPct: number | null;
  utilizationColor: string;
}

function labelForDevice(device: Device): string {
  return device.tags?.display_name || device.sys_name || device.ip || device.id.slice(0, 8);
}

function resolveRuntimeDevice(runtimeState: RuntimeState, device: Device): RuntimeDeviceModel {
  return (
    runtimeState.devicesById.get(device.id) ?? {
      device,
      monitoringState: 'monitorable',
      metrics: null,
      alertStatus: 'normal',
      runtimeStatus: null,
    }
  );
}

function runtimeReasonMessage(reason: Exclude<RuntimeReason, 'ok'>): string {
  switch (reason) {
    case 'awaiting_poll':
      return 'Awaiting first poll';
    case 'stale':
      return 'Telemetry stale';
    case 'device_unreachable':
      return 'Device unreachable';
    case 'upstream_unavailable':
      return 'Runtime upstream unavailable';
    case 'no_data':
      return 'No runtime telemetry';
    case 'unmonitored':
      return 'Device unmonitored';
    case 'unsupported':
      return 'Telemetry unsupported';
  }
}

function metricsAvailability(runtimeDevice: RuntimeDeviceModel): {
  available: boolean;
  message: string | null;
  reason: InterfaceSectionModel['availabilityReason'];
} {
  if (runtimeDevice.metrics?.primary_reason === 'device_unreachable') {
    return {
      available: false,
      message: runtimeReasonMessage('device_unreachable'),
      reason: 'device_unreachable',
    };
  }

  const metricsReason = runtimeDevice.metrics?.metrics_reason;
  if (
    runtimeDevice.metrics &&
    runtimeDevice.metrics.metrics_status !== 'available' &&
    runtimeDevice.metrics.metrics_status !== 'partial' &&
    metricsReason &&
    metricsReason !== 'ok'
  ) {
    return {
      available: false,
      message: runtimeReasonMessage(metricsReason),
      reason: metricsReason,
    };
  }

  return { available: true, message: null, reason: null };
}

function linkMetricsAvailability(
  runtimeState: RuntimeState,
  linkId: string,
): {
  available: boolean;
  message: string | null;
  reason: InterfaceSectionModel['availabilityReason'];
} {
  const runtimeLink = runtimeState.linksById.get(linkId);

  if (
    runtimeLink &&
    !runtimeLink.metricsUsable &&
    runtimeLink.metricsReason &&
    runtimeLink.metricsReason !== 'ok'
  ) {
    return {
      available: false,
      message: runtimeReasonMessage(runtimeLink.metricsReason),
      reason: runtimeLink.metricsReason,
    };
  }

  return { available: true, message: null, reason: null };
}

function linkPanelAvailability(
  runtimeDevice: RuntimeDeviceModel,
  linkAvailability: ReturnType<typeof linkMetricsAvailability>,
  hasLinkRuntime: boolean,
): ReturnType<typeof metricsAvailability> {
  const deviceAvailability = metricsAvailability(runtimeDevice);

  if (deviceAvailability.reason === 'device_unreachable') {
    return deviceAvailability;
  }

  if (!linkAvailability.available) {
    return linkAvailability;
  }

  if (hasLinkRuntime) {
    return { available: true, message: null, reason: null };
  }

  return deviceAvailability;
}

function interfaceMetricsAvailability(
  runtimeState: RuntimeState,
  runtimeDevice: RuntimeDeviceModel,
  ifName: string,
): ReturnType<typeof metricsAvailability> {
  const deviceAvailability = metricsAvailability(runtimeDevice);

  if (deviceAvailability.available || deviceAvailability.reason === 'device_unreachable') {
    return deviceAvailability;
  }

  return findIndexedInterfaceMetrics(runtimeState, runtimeDevice.device.id, ifName)
    ? { available: true, message: null, reason: null }
    : deviceAvailability;
}

function emptyStats(): AdaptedInterfaceStats {
  return {
    txLabel: '--',
    rxLabel: '--',
    utilizationPct: null,
    utilizationColor: UNKNOWN_UTILIZATION_COLOR,
  };
}

function adaptInterfaceStats(
  metrics: LinkMetricsDTO | null,
  available: boolean,
): AdaptedInterfaceStats {
  if (!available || !metrics) {
    return emptyStats();
  }

  return {
    txLabel: metrics.tx_bps != null ? formatThroughput(metrics.tx_bps) : '--',
    rxLabel: metrics.rx_bps != null ? formatThroughput(metrics.rx_bps) : '--',
    utilizationPct: metrics.utilization != null ? Math.round(metrics.utilization * 100) : null,
    utilizationColor:
      metrics.utilization != null
        ? utilizationColor(metrics.utilization)
        : UNKNOWN_UTILIZATION_COLOR,
  };
}

function findIndexedInterfaceMetrics(
  runtimeState: RuntimeState,
  deviceId: string,
  ifName: string,
): LinkMetricsDTO | null {
  const normalizedIfName = normalizeInterfaceName(ifName);
  return runtimeState.interfaceMetricsByDeviceId.get(deviceId)?.get(normalizedIfName) ?? null;
}

function matchInterfaceInfo(interfaces: InterfaceInfo[], ifName: string): InterfaceInfo | null {
  const normalizedIfName = normalizeInterfaceName(ifName);
  return (
    interfaces.find((iface) => normalizeInterfaceName(iface.if_name) === normalizedIfName) ?? null
  );
}

function fallbackInterfaceInfo({
  ifName,
  speed,
  operStatus,
}: {
  ifName: string;
  speed: number | null | undefined;
  operStatus: string | null | undefined;
}): InterfaceInfo {
  const normalizedOperStatus =
    operStatus === 'up' || operStatus === 'down' ? operStatus : 'unknown';

  return {
    if_name: ifName,
    if_descr: '',
    speed: speed ?? 0,
    oper_status: normalizedOperStatus,
    admin_status: normalizedOperStatus,
    in_use: true,
  };
}

function sortDeviceInterfaces(interfaces: InterfaceInfo[]): InterfaceInfo[] {
  return interfaces
    .filter((iface) => {
      const lower = iface.if_name.toLowerCase();
      return !lower.startsWith('lo') && lower !== 'null' && !lower.startsWith('null');
    })
    .sort((left, right) => {
      const leftUp = left.oper_status === 'up';
      const rightUp = right.oper_status === 'up';
      if (leftUp !== rightUp) {
        return leftUp ? -1 : 1;
      }
      return left.if_name.localeCompare(right.if_name);
    });
}

function buildInterfaceSection({
  deviceLabel,
  ifName,
  availabilityReason,
  metricsUnavailableMessage,
  stats,
  interfaceInfo,
}: {
  deviceLabel: string;
  ifName: string;
  availabilityReason: InterfaceSectionModel['availabilityReason'];
  metricsUnavailableMessage: string | null;
  stats: AdaptedInterfaceStats;
  interfaceInfo: InterfaceInfo | null;
}): InterfaceSectionModel {
  const speedLabel =
    interfaceInfo && interfaceInfo.speed > 0 ? formatBandwidth(interfaceInfo.speed) : null;
  const deviceDown = availabilityReason === 'device_unreachable';
  const statusLabel = interfaceInfo ? (deviceDown ? 'down' : interfaceInfo.oper_status) : null;
  const statusTone =
    deviceDown || statusLabel === 'down' ? 'down' : statusLabel === 'up' ? 'up' : 'neutral';

  return {
    deviceLabel,
    ifName,
    interfaceDescription: interfaceInfo?.if_descr ?? null,
    speedLabel,
    statusLabel,
    statusTone,
    availabilityReason,
    metricsUnavailableMessage,
    txLabel: stats.txLabel,
    rxLabel: stats.rxLabel,
    utilizationPct: stats.utilizationPct,
    utilizationColor: stats.utilizationColor,
  };
}

function buildNegotiationModel(
  sourceInterfaceInfo: InterfaceInfo | null,
  targetInterfaceInfo: InterfaceInfo | null,
): LinkNegotiationModel {
  const sourceSpeed = sourceInterfaceInfo?.speed ?? 0;
  const targetSpeed = targetInterfaceInfo?.speed ?? 0;
  const sourceLabel = sourceSpeed > 0 ? formatBandwidth(sourceSpeed) : 'Unknown';
  const targetLabel = targetSpeed > 0 ? formatBandwidth(targetSpeed) : 'Unknown';

  if (sourceSpeed > 0 && targetSpeed > 0 && sourceSpeed === targetSpeed) {
    return {
      sourceLabel,
      targetLabel,
      summaryLabel: `Matched at ${formatBandwidth(sourceSpeed)}`,
      detailLabel: 'Both interfaces report the same negotiated speed.',
      tone: 'matched',
    };
  }

  if (sourceSpeed > 0 && targetSpeed > 0) {
    return {
      sourceLabel,
      targetLabel,
      summaryLabel: `${formatBandwidth(sourceSpeed)} vs ${formatBandwidth(targetSpeed)}`,
      detailLabel: 'The two ends report different negotiated speeds.',
      tone: 'mismatch',
    };
  }

  if (sourceSpeed > 0 || targetSpeed > 0) {
    return {
      sourceLabel,
      targetLabel,
      summaryLabel: sourceSpeed > 0 ? sourceLabel : targetLabel,
      detailLabel: 'Only one side exposed a negotiated speed.',
      tone: 'partial',
    };
  }

  return {
    sourceLabel,
    targetLabel,
    summaryLabel: 'Autonegotiation',
    detailLabel: 'Waiting for interface speed data from one or both ends.',
    tone: 'unknown',
  };
}

function adaptAlert(deviceMap: Map<string, Device>, alert: AlertDTO): AlertsPanelAlertModel {
  const device = deviceMap.get(alert.device_id);
  return {
    deviceId: alert.device_id,
    deviceLabel: device ? labelForDevice(device) : alert.device_id.slice(0, 8),
    alertName: alert.alert_name,
    severity: alert.severity,
    state: alert.state,
    summary: alert.summary,
  };
}

function syntheticRuntimeAlert(runtimeDevice: RuntimeDeviceModel): AlertsPanelAlertModel {
  const count = runtimeDevice.metrics?.firing_alert_count ?? 0;
  const severity = runtimeDevice.alertStatus === 'down' ? 'critical' : 'warning';

  return {
    deviceId: runtimeDevice.device.id,
    deviceLabel: labelForDevice(runtimeDevice.device),
    alertName: runtimeDevice.alertStatus === 'down' ? 'RuntimeDown' : 'RuntimeDegraded',
    severity,
    state: 'firing',
    summary:
      count > 0
        ? `${count} active runtime alert${count === 1 ? '' : 's'}`
        : 'Normalized runtime reports an active alert state.',
  };
}

function runtimeBackedFiringAlerts(
  deviceMap: Map<string, Device>,
  alerts: AlertDTO[],
  runtimeDevice: RuntimeDeviceModel,
): AlertsPanelAlertModel[] {
  const normalizedCount = runtimeDevice.metrics?.firing_alert_count ?? 0;

  if (normalizedCount <= 0) {
    return [];
  }

  const rawAlerts = alerts
    .filter((alert) => alert.device_id === runtimeDevice.device.id && alert.state === 'firing')
    .map((alert) => adaptAlert(deviceMap, alert));

  if (rawAlerts.length === 0) {
    return [syntheticRuntimeAlert(runtimeDevice)];
  }

  return rawAlerts.slice(0, normalizedCount);
}

export function buildAlertsPanelModel({
  alerts,
  runtimeState,
}: {
  alerts: AlertDTO[];
  runtimeState: RuntimeState;
}): AlertsPanelModel {
  const devices = Array.from(runtimeState.devicesById.values()).map((entry) => entry.device);
  const deviceMap = new Map(devices.map((device) => [device.id, device]));
  const runtimeDevicesById = new Map(
    Array.from(runtimeState.devicesById.values())
      .filter((entry) => entry.runtimeStatus !== null)
      .map((entry) => [entry.device.id, entry] as const),
  );
  const runtimeAlertsAvailable = runtimeDevicesById.size > 0;
  const runtimeDrivenFiringAlerts = Array.from(runtimeDevicesById.values())
    .filter((entry) => entry.alertStatus !== 'normal')
    .flatMap((entry) => runtimeBackedFiringAlerts(deviceMap, alerts, entry));
  const fallbackRawFiringAlerts = alerts
    .filter((alert) => alert.state === 'firing' && !runtimeDevicesById.has(alert.device_id))
    .map((alert) => adaptAlert(deviceMap, alert));
  const firingAlerts = runtimeAlertsAvailable
    ? [...runtimeDrivenFiringAlerts, ...fallbackRawFiringAlerts]
    : alerts
        .filter((alert) => alert.state === 'firing')
        .map((alert) => adaptAlert(deviceMap, alert));
  const activeAlertCount = countActiveAlertsFromRuntimeState(runtimeState, alerts);
  const resolvedAlerts = alerts
    .filter((alert) => alert.state !== 'firing')
    .map((alert) => adaptAlert(deviceMap, alert));

  return {
    activeAlertCount,
    firingAlerts,
    resolvedAlerts,
    prometheusDiagnostics: runtimeState.prometheusDown
      ? {
          title: 'Prometheus diagnostics unavailable',
          detail:
            'Runtime status and alerts use normalized telemetry. Prometheus health is shown here for operator diagnostics only.',
        }
      : null,
  };
}

export function buildDeviceInterfacePanelModel({
  device,
  runtimeState,
  loadingInterfaces,
  interfaces,
}: {
  device: Device;
  runtimeState: RuntimeState;
  loadingInterfaces: boolean;
  interfaces: InterfaceInfo[];
}): DeviceInterfacePanelModel {
  const runtimeDevice = resolveRuntimeDevice(runtimeState, device);
  const deviceLabel = labelForDevice(runtimeDevice.device);
  const sections = sortDeviceInterfaces(interfaces).map((iface) => {
    const availability = interfaceMetricsAvailability(runtimeState, runtimeDevice, iface.if_name);

    return buildInterfaceSection({
      deviceLabel,
      ifName: iface.if_name,
      availabilityReason: availability.reason,
      metricsUnavailableMessage: availability.message,
      stats: adaptInterfaceStats(
        findIndexedInterfaceMetrics(runtimeState, runtimeDevice.device.id, iface.if_name),
        availability.available,
      ),
      interfaceInfo: iface,
    });
  });

  return {
    deviceId: runtimeDevice.device.id,
    deviceLabel,
    loadingInterfaces,
    sections,
  };
}

export function buildLinkInterfacePanelModel({
  link,
  sourceDevice,
  targetDevice,
  sourceInterfaces,
  targetInterfaces,
  runtimeState,
}: {
  link: Link;
  sourceDevice: Device;
  targetDevice: Device;
  sourceInterfaces: InterfaceInfo[];
  targetInterfaces: InterfaceInfo[];
  runtimeState: RuntimeState;
}): LinkInterfacePanelModel {
  const runtimeSource = resolveRuntimeDevice(runtimeState, sourceDevice);
  const runtimeTarget = resolveRuntimeDevice(runtimeState, targetDevice);
  const runtimeLink = runtimeState.linksById.get(link.id) ?? null;
  const linkAvailability = linkMetricsAvailability(runtimeState, link.id);
  const sourceAvailability = linkPanelAvailability(
    runtimeSource,
    linkAvailability,
    runtimeLink !== null,
  );
  const targetAvailability = linkPanelAvailability(
    runtimeTarget,
    linkAvailability,
    runtimeLink !== null,
  );
  const sourceInterfaceInfo =
    matchInterfaceInfo(sourceInterfaces, link.source_if_name) ??
    fallbackInterfaceInfo({
      ifName: link.source_if_name,
      speed: link.source_if_speed,
      operStatus: link.source_if_oper_status,
    });
  const targetInterfaceInfo =
    matchInterfaceInfo(targetInterfaces, link.target_if_name) ??
    fallbackInterfaceInfo({
      ifName: link.target_if_name,
      speed: link.target_if_speed,
      operStatus: link.target_if_oper_status,
    });

  return {
    linkId: link.id,
    negotiation: buildNegotiationModel(sourceInterfaceInfo, targetInterfaceInfo),
    source: buildInterfaceSection({
      deviceLabel: labelForDevice(runtimeSource.device),
      ifName: link.source_if_name,
      availabilityReason: sourceAvailability.reason,
      metricsUnavailableMessage: sourceAvailability.message,
      stats: adaptInterfaceStats(runtimeLink?.metrics ?? null, sourceAvailability.available),
      interfaceInfo: sourceInterfaceInfo,
    }),
    target: buildInterfaceSection({
      deviceLabel: labelForDevice(runtimeTarget.device),
      ifName: link.target_if_name,
      availabilityReason: targetAvailability.reason,
      metricsUnavailableMessage: targetAvailability.message,
      stats: adaptInterfaceStats(runtimeLink?.metrics ?? null, targetAvailability.available),
      interfaceInfo: targetInterfaceInfo,
    }),
  };
}
