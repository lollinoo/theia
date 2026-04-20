import type { Device, InterfaceInfo, Link } from '../../types/api';
import {
  formatThroughput,
  utilizationColor,
  type AlertDTO,
  type LinkMetricsDTO,
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
import type { RuntimeDeviceModel, RuntimeState } from './runtimeAdapters';

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
  return runtimeState.devicesById.get(device.id) ?? {
    device,
    monitoringState: 'monitorable',
    metrics: null,
    alertStatus: 'normal',
    prometheusOutageMode: 'none',
  };
}

function metricsAvailability(runtimeDevice: RuntimeDeviceModel): {
  available: boolean;
  message: string | null;
  reason: InterfaceSectionModel['availabilityReason'];
} {
  if (runtimeDevice.device.status === 'down' && runtimeDevice.prometheusOutageMode === 'none') {
    return { available: false, message: 'Device unreachable', reason: 'device-down' };
  }
  if (runtimeDevice.prometheusOutageMode === 'offline') {
    return { available: false, message: 'Prometheus unavailable', reason: 'prometheus-unavailable' };
  }
  return { available: true, message: null, reason: null };
}

function emptyStats(): AdaptedInterfaceStats {
  return {
    txLabel: '--',
    rxLabel: '--',
    utilizationPct: null,
    utilizationColor: UNKNOWN_UTILIZATION_COLOR,
  };
}

function adaptInterfaceStats(metrics: LinkMetricsDTO | null, available: boolean): AdaptedInterfaceStats {
  if (!available || !metrics) {
    return emptyStats();
  }

  return {
    txLabel: metrics.tx_bps != null ? formatThroughput(metrics.tx_bps) : '--',
    rxLabel: metrics.rx_bps != null ? formatThroughput(metrics.rx_bps) : '--',
    utilizationPct: metrics.utilization != null ? Math.round(metrics.utilization * 100) : null,
    utilizationColor: metrics.utilization != null ? utilizationColor(metrics.utilization) : UNKNOWN_UTILIZATION_COLOR,
  };
}

function findInterfaceMetrics(
  runtimeState: RuntimeState,
  deviceId: string,
  ifName: string,
): LinkMetricsDTO | null {
  const normalizedIfName = normalizeInterfaceName(ifName);
  const directMetric = runtimeState.interfaceMetricsByDeviceId.get(deviceId)?.get(normalizedIfName);

  if (directMetric) {
    return directMetric;
  }

  for (const runtimeLink of runtimeState.linksById.values()) {
    const sourceMatches = runtimeLink.link.source_device_id === deviceId
      && normalizeInterfaceName(runtimeLink.link.source_if_name) === normalizedIfName;
    const targetMatches = runtimeLink.link.target_device_id === deviceId
      && normalizeInterfaceName(runtimeLink.link.target_if_name) === normalizedIfName;

    if (sourceMatches) {
      return runtimeLink.sourceMetrics;
    }

    if (targetMatches) {
      return runtimeLink.targetMetrics;
    }
  }

  return null;
}

function matchInterfaceInfo(interfaces: InterfaceInfo[], ifName: string): InterfaceInfo | null {
  const normalizedIfName = normalizeInterfaceName(ifName);
  return interfaces.find((iface) => normalizeInterfaceName(iface.if_name) === normalizedIfName) ?? null;
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
  const normalizedOperStatus = operStatus === 'up' || operStatus === 'down'
    ? operStatus
    : 'unknown';

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
  const speedLabel = interfaceInfo && interfaceInfo.speed > 0 ? formatBandwidth(interfaceInfo.speed) : null;
  const deviceDown = availabilityReason === 'device-down';
  const statusLabel = interfaceInfo
    ? (deviceDown ? 'down' : interfaceInfo.oper_status)
    : null;
  const statusTone = deviceDown || statusLabel === 'down'
    ? 'down'
    : statusLabel === 'up'
      ? 'up'
      : 'neutral';

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

export function buildAlertsPanelModel({
  alerts,
  runtimeState,
}: {
  alerts: AlertDTO[];
  runtimeState: RuntimeState;
}): AlertsPanelModel {
  const devices = Array.from(runtimeState.devicesById.values()).map((entry) => entry.device);
  const deviceMap = new Map(devices.map((device) => [device.id, device]));
  const firingAlerts = alerts
    .filter((alert) => alert.state === 'firing')
    .map((alert) => adaptAlert(deviceMap, alert));
  const resolvedAlerts = alerts
    .filter((alert) => alert.state !== 'firing')
    .map((alert) => adaptAlert(deviceMap, alert));

  const offlineDevices = devices
    .filter((device) => runtimeState.devicesById.get(device.id)?.prometheusOutageMode === 'offline')
    .map((device) => ({ id: device.id, label: labelForDevice(device) }));
  const fallbackDevices = devices
    .filter((device) => runtimeState.devicesById.get(device.id)?.prometheusOutageMode === 'fallback')
    .map((device) => ({ id: device.id, label: labelForDevice(device) }));

  return {
    firingAlerts,
    resolvedAlerts,
    prometheusOutage: offlineDevices.length > 0 || fallbackDevices.length > 0
      ? { offlineDevices, fallbackDevices }
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
  const availability = metricsAvailability(runtimeDevice);
  const deviceLabel = labelForDevice(runtimeDevice.device);
  const sections = sortDeviceInterfaces(interfaces).map((iface) => buildInterfaceSection({
    deviceLabel,
    ifName: iface.if_name,
    availabilityReason: availability.reason,
    metricsUnavailableMessage: availability.message,
    stats: adaptInterfaceStats(
      findInterfaceMetrics(runtimeState, runtimeDevice.device.id, iface.if_name),
      availability.available,
    ),
    interfaceInfo: iface,
  }));

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
  const sourceAvailability = metricsAvailability(runtimeSource);
  const targetAvailability = metricsAvailability(runtimeTarget);
  const sourceInterfaceInfo = matchInterfaceInfo(sourceInterfaces, link.source_if_name)
    ?? fallbackInterfaceInfo({
      ifName: link.source_if_name,
      speed: link.source_if_speed,
      operStatus: link.source_if_oper_status,
    });
  const targetInterfaceInfo = matchInterfaceInfo(targetInterfaces, link.target_if_name)
    ?? fallbackInterfaceInfo({
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
      stats: adaptInterfaceStats(
        findInterfaceMetrics(runtimeState, runtimeSource.device.id, link.source_if_name),
        sourceAvailability.available,
      ),
      interfaceInfo: sourceInterfaceInfo,
    }),
    target: buildInterfaceSection({
      deviceLabel: labelForDevice(runtimeTarget.device),
      ifName: link.target_if_name,
      availabilityReason: targetAvailability.reason,
      metricsUnavailableMessage: targetAvailability.message,
      stats: adaptInterfaceStats(
        findInterfaceMetrics(runtimeState, runtimeTarget.device.id, link.target_if_name),
        targetAvailability.available,
      ),
      interfaceInfo: targetInterfaceInfo,
    }),
  };
}
