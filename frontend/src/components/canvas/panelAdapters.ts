/**
 * Defines panel adapters behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { Device, InterfaceInfo, Link } from '../../types/api';
import {
  type AlertDTO,
  formatThroughput,
  type LinkMetricsDTO,
  type RuntimeFlag,
  type RuntimeReason,
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
  countActiveAlertsFromRuntimeState,
  type RuntimeDeviceModel,
  type RuntimeState,
} from './runtimeAdapters';

const UNKNOWN_UTILIZATION_COLOR = 'var(--color-status-unknown)';
type EndpointNegotiationTone = Extract<LinkNegotiationModel['tone'], 'up' | 'warning' | 'critical'>;
type RuntimeAlertDraft = AlertsPanelAlertModel & { priority: number };

/** Describes the adapted interface stats contract used by the topology canvas. */
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
      primaryHealth: null,
      runtimeFlags: [],
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

function linkedInterfaceNamesForDevice(deviceId: string, links: Link[]): Set<string> {
  const names = new Set<string>();

  for (const link of links) {
    if (link.source_device_id === deviceId) {
      const sourceIfName = normalizeInterfaceName(link.source_if_name);
      if (sourceIfName) {
        names.add(sourceIfName);
      }
    }

    if (link.target_device_id === deviceId) {
      const targetIfName = normalizeInterfaceName(link.target_if_name);
      if (targetIfName) {
        names.add(targetIfName);
      }
    }
  }

  return names;
}

function sortDeviceInterfaces(
  interfaces: InterfaceInfo[],
  linkedInterfaceNames: Set<string>,
): InterfaceInfo[] {
  return interfaces
    .filter((iface) => {
      const lower = iface.if_name.toLowerCase();
      return !lower.startsWith('lo') && lower !== 'null' && !lower.startsWith('null');
    })
    .sort((left, right) => {
      const leftLinked =
        left.in_use || linkedInterfaceNames.has(normalizeInterfaceName(left.if_name));
      const rightLinked =
        right.in_use || linkedInterfaceNames.has(normalizeInterfaceName(right.if_name));
      if (leftLinked !== rightLinked) {
        return leftLinked ? -1 : 1;
      }

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
  const runtimeStatusUnavailable =
    availabilityReason !== null && metricsUnavailableMessage !== null;
  const statusLabel = interfaceInfo
    ? deviceDown
      ? 'down'
      : runtimeStatusUnavailable
        ? 'unknown'
        : interfaceInfo.oper_status
    : null;
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
  endpointTone: EndpointNegotiationTone | null = null,
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
      tone: endpointTone ?? 'matched',
    };
  }

  if (sourceSpeed > 0 && targetSpeed > 0) {
    return {
      sourceLabel,
      targetLabel,
      summaryLabel: `${formatBandwidth(sourceSpeed)} vs ${formatBandwidth(targetSpeed)}`,
      detailLabel: 'The two ends report different negotiated speeds.',
      tone: endpointTone ?? 'mismatch',
    };
  }

  if (sourceSpeed > 0 || targetSpeed > 0) {
    return {
      sourceLabel,
      targetLabel,
      summaryLabel: sourceSpeed > 0 ? sourceLabel : targetLabel,
      detailLabel: 'Only one side exposed a negotiated speed.',
      tone: endpointTone ?? 'partial',
    };
  }

  return {
    sourceLabel,
    targetLabel,
    summaryLabel: 'Autonegotiation',
    detailLabel: 'Waiting for interface speed data from one or both ends.',
    tone: endpointTone ?? 'unknown',
  };
}

function endpointNegotiationTone(
  runtimeDevice: RuntimeDeviceModel,
): Exclude<EndpointNegotiationTone, 'up'> | null {
  const metrics = runtimeDevice.metrics;
  if (runtimeDevice.device.device_type === 'virtual' && !runtimeDevice.device.ip) {
    return null;
  }

  if (
    runtimeDevice.alertStatus === 'down' ||
    runtimeDevice.device.status === 'down' ||
    runtimeDevice.runtimeStatus === 'down' ||
    runtimeDevice.primaryHealth === 'unreachable' ||
    runtimeDevice.primaryHealth === 'quarantined' ||
    metrics?.health === 'critical' ||
    metrics?.reachability === 'hard_down' ||
    metrics?.network_reachable === 'false'
  ) {
    return 'critical';
  }

  if (
    runtimeDevice.alertStatus === 'degraded' ||
    runtimeDevice.device.status === 'probing' ||
    runtimeDevice.runtimeStatus === 'probing' ||
    runtimeDevice.primaryHealth === 'snmp_degraded' ||
    metrics?.health === 'warning' ||
    metrics?.reachability === 'soft_down' ||
    metrics?.snmp_reachable === 'false'
  ) {
    return 'warning';
  }

  return null;
}

function isNoIpVirtualEndpoint(runtimeDevice: RuntimeDeviceModel): boolean {
  return runtimeDevice.device.device_type === 'virtual' && !runtimeDevice.device.ip;
}

function isIpVirtualEndpoint(runtimeDevice: RuntimeDeviceModel): boolean {
  return runtimeDevice.device.device_type === 'virtual' && !!runtimeDevice.device.ip;
}

function hasHealthyRuntimeEndpoint(runtimeDevice: RuntimeDeviceModel): boolean {
  return (
    runtimeDevice.alertStatus === 'normal' &&
    runtimeDevice.device.status === 'up' &&
    (runtimeDevice.runtimeStatus === null || runtimeDevice.runtimeStatus === 'up') &&
    (runtimeDevice.primaryHealth === null ||
      runtimeDevice.primaryHealth === 'up_fresh' ||
      runtimeDevice.primaryHealth === 'up_stale') &&
    (runtimeDevice.metrics === null ||
      ((runtimeDevice.metrics.health === 'healthy' || runtimeDevice.metrics.health === 'unknown') &&
        (runtimeDevice.metrics.reachability === 'up' ||
          runtimeDevice.metrics.reachability === 'unknown') &&
        (runtimeDevice.metrics.network_reachable === 'true' ||
          runtimeDevice.metrics.network_reachable === 'unknown') &&
        (runtimeDevice.metrics.snmp_reachable === 'true' ||
          runtimeDevice.metrics.snmp_reachable === 'unknown')))
  );
}

function isHealthyPhysicalEndpoint(
  runtimeDevice: RuntimeDeviceModel,
  interfaceInfo: InterfaceInfo | null,
): boolean {
  if (runtimeDevice.device.device_type === 'virtual') {
    return false;
  }

  return hasHealthyRuntimeEndpoint(runtimeDevice) && interfaceInfo?.oper_status === 'up';
}

function isHealthyIpVirtualEndpoint(runtimeDevice: RuntimeDeviceModel): boolean {
  return isIpVirtualEndpoint(runtimeDevice) && hasHealthyRuntimeEndpoint(runtimeDevice);
}

function linkEndpointNegotiationTone(
  sourceDevice: RuntimeDeviceModel,
  targetDevice: RuntimeDeviceModel,
  sourceInterfaceInfo: InterfaceInfo | null,
  targetInterfaceInfo: InterfaceInfo | null,
): EndpointNegotiationTone | null {
  const sourceTone = endpointNegotiationTone(sourceDevice);
  const targetTone = endpointNegotiationTone(targetDevice);

  if (sourceTone === 'critical' || targetTone === 'critical') {
    return 'critical';
  }

  if (sourceTone === 'warning' || targetTone === 'warning') {
    return 'warning';
  }

  if (
    isNoIpVirtualEndpoint(sourceDevice) &&
    isHealthyPhysicalEndpoint(targetDevice, targetInterfaceInfo)
  ) {
    return 'up';
  }

  if (
    isNoIpVirtualEndpoint(targetDevice) &&
    isHealthyPhysicalEndpoint(sourceDevice, sourceInterfaceInfo)
  ) {
    return 'up';
  }

  if (
    isHealthyIpVirtualEndpoint(sourceDevice) &&
    isHealthyPhysicalEndpoint(targetDevice, targetInterfaceInfo)
  ) {
    return 'up';
  }

  if (
    isHealthyIpVirtualEndpoint(targetDevice) &&
    isHealthyPhysicalEndpoint(sourceDevice, sourceInterfaceInfo)
  ) {
    return 'up';
  }

  return null;
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

function runtimeAlert(
  runtimeDevice: RuntimeDeviceModel,
  alertName: string,
  severity: string,
  summary: string,
  priority: number,
): RuntimeAlertDraft {
  return {
    deviceId: runtimeDevice.device.id,
    deviceLabel: labelForDevice(runtimeDevice.device),
    alertName,
    severity,
    state: 'firing',
    summary,
    priority,
  };
}

function runtimeFlagAlert(runtimeDevice: RuntimeDeviceModel, flag: RuntimeFlag): RuntimeAlertDraft {
  switch (flag) {
    case 'deadline_missed':
      return runtimeAlert(
        runtimeDevice,
        'PollingDeadlineMissed',
        'warning',
        'Runtime polling missed its expected deadline.',
        40,
      );
    case 'overloaded':
      return runtimeAlert(
        runtimeDevice,
        'PollingOverloaded',
        'critical',
        'Runtime polling is overloaded.',
        41,
      );
    case 'background_pending':
      return runtimeAlert(
        runtimeDevice,
        'BackgroundPollingPending',
        'warning',
        'Background telemetry refresh is still pending.',
        42,
      );
    case 'partial_telemetry':
      return runtimeAlert(
        runtimeDevice,
        'PartialTelemetry',
        'warning',
        'Runtime telemetry is partial.',
        43,
      );
    case 'degraded_risk':
      return runtimeAlert(
        runtimeDevice,
        'PollingDegradedRisk',
        'warning',
        'Runtime polling is at risk of degraded coverage.',
        44,
      );
    case 'persistence_lagging':
      return runtimeAlert(
        runtimeDevice,
        'PersistenceLagging',
        'warning',
        'Runtime persistence is lagging behind live telemetry.',
        45,
      );
  }
}

function shouldShowWarningHealth(runtimeDevice: RuntimeDeviceModel): boolean {
  const metrics = runtimeDevice.metrics;

  if (!metrics || metrics.health !== 'warning') {
    return false;
  }

  return (
    runtimeDevice.primaryHealth === 'snmp_degraded' ||
    runtimeDevice.primaryHealth === 'up_stale' ||
    metrics.reachability === 'soft_down' ||
    metrics.freshness === 'stale' ||
    metrics.metrics_status === 'partial' ||
    runtimeDevice.runtimeFlags.length > 0
  );
}

function runtimeDeviceAlerts(runtimeDevice: RuntimeDeviceModel): RuntimeAlertDraft[] {
  const metrics = runtimeDevice.metrics;

  if (!metrics || runtimeDevice.runtimeStatus === 'unmonitored') {
    return [];
  }

  const alerts: RuntimeAlertDraft[] = [];
  const deviceUnreachable =
    runtimeDevice.primaryHealth === 'unreachable' ||
    metrics.reachability === 'hard_down' ||
    metrics.primary_reason === 'device_unreachable';

  if (runtimeDevice.primaryHealth === 'quarantined') {
    alerts.push(
      runtimeAlert(
        runtimeDevice,
        'DeviceQuarantined',
        'critical',
        'Device is quarantined from runtime polling.',
        10,
      ),
    );
  } else if (deviceUnreachable) {
    alerts.push(
      runtimeAlert(
        runtimeDevice,
        'DeviceUnreachable',
        'critical',
        'Device cannot be reached by runtime polling.',
        11,
      ),
    );
  } else if (
    runtimeDevice.primaryHealth === 'snmp_degraded' ||
    metrics.reachability === 'soft_down' ||
    metrics.snmp_reachable === 'false'
  ) {
    alerts.push(
      runtimeAlert(runtimeDevice, 'SNMPDegraded', 'warning', 'SNMP reachability is degraded.', 20),
    );
  } else if (metrics.reachability === 'unknown') {
    alerts.push(
      runtimeAlert(
        runtimeDevice,
        'ReachabilityUnknown',
        'warning',
        'Runtime reachability is unknown.',
        21,
      ),
    );
  }

  if (metrics.health === 'critical' && !deviceUnreachable) {
    alerts.push(
      runtimeAlert(
        runtimeDevice,
        'DeviceHealthCritical',
        'critical',
        'Runtime health is critical.',
        30,
      ),
    );
  } else if (shouldShowWarningHealth(runtimeDevice)) {
    alerts.push(
      runtimeAlert(
        runtimeDevice,
        'DeviceHealthWarning',
        'warning',
        'Runtime health is warning.',
        31,
      ),
    );
  }

  if (runtimeDevice.primaryHealth === 'up_stale' || metrics.freshness === 'stale') {
    alerts.push(
      runtimeAlert(runtimeDevice, 'TelemetryStale', 'warning', 'Runtime telemetry is stale.', 35),
    );
  } else if (runtimeDevice.primaryHealth === 'probing' || metrics.freshness === 'awaiting_poll') {
    alerts.push(
      runtimeAlert(
        runtimeDevice,
        'AwaitingPoll',
        'warning',
        'Runtime is awaiting a successful poll.',
        36,
      ),
    );
  }

  for (const flag of runtimeDevice.runtimeFlags) {
    alerts.push(runtimeFlagAlert(runtimeDevice, flag));
  }

  if (
    metrics.metrics_status === 'unavailable' &&
    metrics.metrics_reason !== 'ok' &&
    metrics.metrics_reason !== 'device_unreachable' &&
    metrics.metrics_reason !== 'unmonitored'
  ) {
    alerts.push(
      runtimeAlert(
        runtimeDevice,
        'TelemetryUnavailable',
        'warning',
        runtimeReasonMessage(metrics.metrics_reason),
        50,
      ),
    );
  } else if (
    metrics.metrics_status === 'partial' &&
    !runtimeDevice.runtimeFlags.includes('partial_telemetry')
  ) {
    alerts.push(
      runtimeAlert(
        runtimeDevice,
        'PartialTelemetry',
        'warning',
        'Runtime telemetry is partial.',
        51,
      ),
    );
  }

  return alerts;
}

function linkEndpointLabel(
  deviceMap: Map<string, Device>,
  deviceId: string,
  ifName: string,
): string {
  const device = deviceMap.get(deviceId);
  const deviceLabel = device ? labelForDevice(device) : deviceId.slice(0, 8);
  return ifName ? `${deviceLabel} ${ifName}` : deviceLabel;
}

function runtimeLinkAlerts(
  runtimeState: RuntimeState,
  deviceMap: Map<string, Device>,
): RuntimeAlertDraft[] {
  const alerts: RuntimeAlertDraft[] = [];

  for (const runtimeLink of runtimeState.linksById.values()) {
    if (runtimeLink.metricsStatus === null) {
      continue;
    }

    const sourceLabel = linkEndpointLabel(
      deviceMap,
      runtimeLink.link.source_device_id,
      runtimeLink.link.source_if_name,
    );
    const targetLabel = linkEndpointLabel(
      deviceMap,
      runtimeLink.link.target_device_id,
      runtimeLink.link.target_if_name,
    );
    const deviceId = `${runtimeLink.link.source_device_id}:${runtimeLink.link.id}`;
    const deviceLabel = `${sourceLabel} to ${targetLabel}`;

    if (runtimeLink.metricsStatus === 'unavailable') {
      alerts.push({
        deviceId,
        deviceLabel,
        alertName: 'LinkTelemetryUnavailable',
        severity: 'warning',
        state: 'firing',
        summary:
          runtimeLink.metricsReason && runtimeLink.metricsReason !== 'ok'
            ? runtimeReasonMessage(runtimeLink.metricsReason)
            : 'Link telemetry is unavailable.',
        priority: 70,
      });
    } else if (runtimeLink.metricsStatus === 'partial') {
      alerts.push({
        deviceId,
        deviceLabel,
        alertName: 'LinkTelemetryPartial',
        severity: 'warning',
        state: 'firing',
        summary: 'Link telemetry is partial.',
        priority: 71,
      });
    }
  }

  return alerts;
}

function appendUniqueAlert(
  alerts: RuntimeAlertDraft[],
  seen: Set<string>,
  alert: RuntimeAlertDraft,
) {
  const key = `${alert.deviceId}:${alert.alertName}`;
  if (seen.has(key)) {
    return;
  }

  seen.add(key);
  alerts.push(alert);
}

function runtimeDerivedAlerts(
  runtimeState: RuntimeState,
  deviceMap: Map<string, Device>,
  existingAlerts: AlertsPanelAlertModel[],
): AlertsPanelAlertModel[] {
  const seen = new Set(existingAlerts.map((alert) => `${alert.deviceId}:${alert.alertName}`));
  const derivedAlerts: RuntimeAlertDraft[] = [];

  for (const runtimeDevice of runtimeState.devicesById.values()) {
    for (const alert of runtimeDeviceAlerts(runtimeDevice)) {
      appendUniqueAlert(derivedAlerts, seen, alert);
    }
  }

  for (const alert of runtimeLinkAlerts(runtimeState, deviceMap)) {
    appendUniqueAlert(derivedAlerts, seen, alert);
  }

  return derivedAlerts
    .sort((a, b) => a.priority - b.priority || a.deviceLabel.localeCompare(b.deviceLabel))
    .map(({ priority: _priority, ...alert }) => alert);
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

/** Builds alerts panel model for the topology canvas. */
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
  const runtimeAlerts = runtimeDerivedAlerts(runtimeState, deviceMap, firingAlerts);
  const allFiringAlerts = [...firingAlerts, ...runtimeAlerts];
  const activeAlertCount = Math.max(
    countActiveAlertsFromRuntimeState(runtimeState, alerts),
    allFiringAlerts.length,
  );
  const resolvedAlerts = alerts
    .filter((alert) => alert.state !== 'firing')
    .map((alert) => adaptAlert(deviceMap, alert));

  return {
    activeAlertCount,
    firingAlerts: allFiringAlerts,
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

/** Builds device interface panel model for the topology canvas. */
export function buildDeviceInterfacePanelModel({
  device,
  runtimeState,
  loadingInterfaces,
  interfaces,
  links = [],
}: {
  device: Device;
  runtimeState: RuntimeState;
  loadingInterfaces: boolean;
  interfaces: InterfaceInfo[];
  links?: Link[];
}): DeviceInterfacePanelModel {
  const runtimeDevice = resolveRuntimeDevice(runtimeState, device);
  const deviceLabel = labelForDevice(runtimeDevice.device);
  const linkedInterfaceNames = linkedInterfaceNamesForDevice(runtimeDevice.device.id, links);
  const sections = sortDeviceInterfaces(interfaces, linkedInterfaceNames).map((iface) => {
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

/** Builds link interface panel model for the topology canvas. */
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
    negotiation: buildNegotiationModel(
      sourceInterfaceInfo,
      targetInterfaceInfo,
      linkEndpointNegotiationTone(
        runtimeSource,
        runtimeTarget,
        sourceInterfaceInfo,
        targetInterfaceInfo,
      ),
    ),
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
