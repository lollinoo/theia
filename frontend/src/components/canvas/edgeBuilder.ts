/**
 * Defines edge builder behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { Device, Link } from '../../types/api';
import type { AlertDTO, AlertStatus } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import type { LinkEdgeType } from '../LinkEdge';
import { buildLinkTelemetryBadges, type LinkEdgeData } from '../linkSemantics';
import { type HandleSide } from './canvasHelpers';

function normalizeLinkValue(value: string): string {
  return value.trim().toLowerCase();
}

function extractPhysicalInterfaceAnchor(name: string): string {
  const normalized = normalizeLinkValue(name);
  if (!normalized) return '';

  const virtualHints = [
    'vlan',
    'vrf',
    'vpn',
    'bridge',
    'br-',
    'bond',
    'loopback',
    'lo',
    'gre',
    'eoip',
    'wg',
    'wireguard',
    'pppoe',
    'ppp',
    'sstp',
    'ovpn',
    'l2tp',
    'vxlan',
    'veth',
    'tap',
    'tun',
  ];
  if (virtualHints.some((hint) => normalized.includes(hint))) {
    return '';
  }

  const physicalPatterns = [
    'ether',
    'eth',
    'sfp-sfpplus',
    'sfp',
    'qsfp',
    'ens',
    'eno',
    'enp',
    'gigabitethernet',
    'tengigabitethernet',
    'fastethernet',
    'ge-',
    'xe-',
    'et-',
  ];
  for (const pattern of physicalPatterns) {
    const idx = normalized.indexOf(pattern);
    if (idx < 0) continue;
    let anchor = normalized.slice(idx);
    const stop = anchor.search(/[^a-z0-9\-/]/);
    if (stop >= 0) anchor = anchor.slice(0, stop);
    anchor = anchor.replace(/^[-/\s]+|[-/\s]+$/g, '');
    if (/\d/.test(anchor)) return anchor;
  }

  if (/^(gi|te|fo|port)/.test(normalized) && /\d/.test(normalized)) {
    return normalized;
  }

  return '';
}

function isCompletePhysicalLink(link: Link): boolean {
  return (
    extractPhysicalInterfaceAnchor(link.source_if_name) !== '' &&
    extractPhysicalInterfaceAnchor(link.target_if_name) !== ''
  );
}

function linkPreferenceScore(link: Link): number {
  let score = 0;
  if (link.discovery_protocol === 'lldp') score += 100;
  if (extractPhysicalInterfaceAnchor(link.source_if_name)) score += 50;
  if (extractPhysicalInterfaceAnchor(link.target_if_name)) score += 40;
  if (link.source_if_name) score += 20;
  if (link.target_if_name) score += 20;
  return score;
}

function canonicalLinkGroupKey(link: Link): string {
  const endpoints = [link.source_device_id, link.target_device_id].sort().join('-');
  const sourceAnchor = extractPhysicalInterfaceAnchor(link.source_if_name);
  const targetAnchor = extractPhysicalInterfaceAnchor(link.target_if_name);

  if (sourceAnchor && targetAnchor) {
    return `${endpoints}|phy|${[sourceAnchor, targetAnchor].sort().join('|')}`;
  }
  if (sourceAnchor) return `${endpoints}|phy|${sourceAnchor}`;
  if (targetAnchor) return `${endpoints}|phy|${targetAnchor}`;
  return `${endpoints}|raw|${[
    `${link.source_device_id}:${normalizeLinkValue(link.source_if_name)}`,
    `${link.target_device_id}:${normalizeLinkValue(link.target_if_name)}`,
  ]
    .sort()
    .join('|')}`;
}

/** Prefers visible links for the topology canvas. */
export function preferVisibleLinks(links: Link[]): Link[] {
  const bestByGroup = new Map<string, Link>();

  for (const link of links) {
    const key = canonicalLinkGroupKey(link);
    const existing = bestByGroup.get(key);
    if (!existing || linkPreferenceScore(link) > linkPreferenceScore(existing)) {
      bestByGroup.set(key, link);
    }
  }

  const completePhysicalByPair = new Map<string, Link>();
  for (const link of bestByGroup.values()) {
    const pairKey = [link.source_device_id, link.target_device_id].sort().join('-');
    if (!isCompletePhysicalLink(link)) continue;
    const existing = completePhysicalByPair.get(pairKey);
    if (!existing || linkPreferenceScore(link) > linkPreferenceScore(existing)) {
      completePhysicalByPair.set(pairKey, link);
    }
  }

  const visible: Link[] = [];
  for (const link of bestByGroup.values()) {
    const pairKey = [link.source_device_id, link.target_device_id].sort().join('-');
    const preferredPairLink = completePhysicalByPair.get(pairKey);
    if (preferredPairLink && preferredPairLink.id !== link.id && !isCompletePhysicalLink(link)) {
      continue;
    }
    visible.push(link);
  }

  return visible;
}

/** Builds edge data for the topology canvas. */
export function buildEdgeData(
  link: Link,
  devicesByID: Map<string, Device>,
  existingData?: LinkEdgeData,
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void,
): LinkEdgeData {
  const sourceDevice = devicesByID.get(link.source_device_id);
  const targetDevice = devicesByID.get(link.target_device_id);

  // Use enriched interface data from the link response directly instead of
  // looking up device.interfaces (avoids per-device /interfaces API calls).
  const sourceSpeed = link.source_if_speed > 0 ? link.source_if_speed : 0;
  const targetSpeed = link.target_if_speed > 0 ? link.target_if_speed : 0;
  const sourceIfOperStatus = link.source_if_oper_status || undefined;
  const targetIfOperStatus = link.target_if_oper_status || undefined;

  // Detect virtual link: one side is a virtual device
  const sourceIsVirtual = sourceDevice?.device_type === 'virtual';
  const targetIsVirtual = targetDevice?.device_type === 'virtual';
  const isVirtualLink = sourceIsVirtual || targetIsVirtual;
  const inertVirtualLink =
    (sourceIsVirtual && !sourceDevice?.ip) || (targetIsVirtual && !targetDevice?.ip);

  // For virtual links, use only the real device's interface speed (D-10)
  // Virtual devices have no interfaces, so their speed is always 0
  const telemetryBadges = buildLinkTelemetryBadges({
    sourceSpeed,
    targetSpeed,
    isVirtualLink,
    sourceIsVirtual: !!sourceIsVirtual,
  });

  if (isVirtualLink) {
    const suppressSourceDeviceState = inertVirtualLink && sourceIsVirtual;
    const suppressTargetDeviceState = inertVirtualLink && targetIsVirtual;

    return {
      link,
      ...telemetryBadges,
      inertVirtualLink,
      sourceIsVirtual: !!sourceIsVirtual,
      targetIsVirtual: !!targetIsVirtual,
      onContextMenu,
      route: existingData?.route,
      routeEditable: existingData?.routeEditable,
      onRouteCommit: existingData?.onRouteCommit,
      metrics: existingData?.metrics,
      throughputLabel: existingData?.throughputLabel,
      utilization: existingData?.utilization,
      sourceIfStatus: sourceIsVirtual ? undefined : sourceIfOperStatus,
      targetIfStatus: targetIsVirtual ? undefined : targetIfOperStatus,
      sourceDeviceStatus: suppressSourceDeviceState
        ? undefined
        : (existingData?.sourceDeviceStatus ?? sourceDevice?.status),
      targetDeviceStatus: suppressTargetDeviceState
        ? undefined
        : (existingData?.targetDeviceStatus ?? targetDevice?.status),
      sourceDeviceAlertStatus: suppressSourceDeviceState
        ? undefined
        : existingData?.sourceDeviceAlertStatus,
      targetDeviceAlertStatus: suppressTargetDeviceState
        ? undefined
        : existingData?.targetDeviceAlertStatus,
      sourceDeviceHealth: suppressSourceDeviceState ? undefined : existingData?.sourceDeviceHealth,
      targetDeviceHealth: suppressTargetDeviceState ? undefined : existingData?.targetDeviceHealth,
      sourceDevicePrimaryHealth: suppressSourceDeviceState
        ? undefined
        : existingData?.sourceDevicePrimaryHealth,
      targetDevicePrimaryHealth: suppressTargetDeviceState
        ? undefined
        : existingData?.targetDevicePrimaryHealth,
      sourceDeviceReachability: suppressSourceDeviceState
        ? undefined
        : existingData?.sourceDeviceReachability,
      targetDeviceReachability: suppressTargetDeviceState
        ? undefined
        : existingData?.targetDeviceReachability,
      sourceDeviceNetworkReachable: suppressSourceDeviceState
        ? undefined
        : existingData?.sourceDeviceNetworkReachable,
      targetDeviceNetworkReachable: suppressTargetDeviceState
        ? undefined
        : existingData?.targetDeviceNetworkReachable,
      sourceDeviceSnmpReachable: suppressSourceDeviceState
        ? undefined
        : existingData?.sourceDeviceSnmpReachable,
      targetDeviceSnmpReachable: suppressTargetDeviceState
        ? undefined
        : existingData?.targetDeviceSnmpReachable,
    };
  }

  return {
    link,
    ...telemetryBadges,
    sourceIsVirtual: false,
    targetIsVirtual: false,
    onContextMenu,
    route: existingData?.route,
    routeEditable: existingData?.routeEditable,
    onRouteCommit: existingData?.onRouteCommit,
    metrics: existingData?.metrics,
    throughputLabel: existingData?.throughputLabel,
    utilization: existingData?.utilization,
    sourceIfStatus: sourceIfOperStatus,
    targetIfStatus: targetIfOperStatus,
    sourceDeviceStatus: existingData?.sourceDeviceStatus ?? sourceDevice?.status,
    targetDeviceStatus: existingData?.targetDeviceStatus ?? targetDevice?.status,
    sourceDeviceAlertStatus: existingData?.sourceDeviceAlertStatus,
    targetDeviceAlertStatus: existingData?.targetDeviceAlertStatus,
    sourceDeviceHealth: existingData?.sourceDeviceHealth,
    targetDeviceHealth: existingData?.targetDeviceHealth,
    sourceDevicePrimaryHealth: existingData?.sourceDevicePrimaryHealth,
    targetDevicePrimaryHealth: existingData?.targetDevicePrimaryHealth,
    sourceDeviceReachability: existingData?.sourceDeviceReachability,
    targetDeviceReachability: existingData?.targetDeviceReachability,
    sourceDeviceNetworkReachable: existingData?.sourceDeviceNetworkReachable,
    targetDeviceNetworkReachable: existingData?.targetDeviceNetworkReachable,
    sourceDeviceSnmpReachable: existingData?.sourceDeviceSnmpReachable,
    targetDeviceSnmpReachable: existingData?.targetDeviceSnmpReachable,
  };
}

/** Returns handle side for the topology canvas. */
export function getHandleSide(
  sourcePosition: { x: number; y: number },
  targetPosition: { x: number; y: number },
  isSelfLoop = false,
): { sourceHandle: HandleSide; targetHandle: HandleSide } {
  if (isSelfLoop) {
    return { sourceHandle: 'right', targetHandle: 'left' };
  }

  const dx = targetPosition.x - sourcePosition.x;
  const dy = targetPosition.y - sourcePosition.y;

  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0
      ? { sourceHandle: 'right', targetHandle: 'left' }
      : { sourceHandle: 'left', targetHandle: 'right' };
  }

  return dy >= 0
    ? { sourceHandle: 'bottom', targetHandle: 'top' }
    : { sourceHandle: 'top', targetHandle: 'bottom' };
}

/** Builds topology edges for the topology canvas. */
export function buildTopologyEdges(
  links: Link[],
  devicesByID: Map<string, Device>,
  nodes: DeviceNode[],
  existingEdgeDataByID?: Map<string, LinkEdgeData>,
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void,
  alerts: AlertDTO[] = [],
): LinkEdgeType[] {
  const nodesByID = new Map(nodes.map((node) => [node.id, node]));
  const seenPhysicalLinks = new Set<string>();
  const visibleLinks = preferVisibleLinks(links);

  const candidateEdges = visibleLinks
    .filter((link) => {
      if (link.source_device_id === link.target_device_id) {
        return false;
      }
      if (!nodesByID.has(link.source_device_id) || !nodesByID.has(link.target_device_id)) {
        return false;
      }

      // Deduplicate only the same physical cable discovered from opposite ends.
      // Distinct parallel uplinks between the same device pair must remain visible.
      const normalizedEndpoints = [
        `${link.source_device_id}:${link.source_if_name}`,
        `${link.target_device_id}:${link.target_if_name}`,
      ].sort();
      const physicalKey = normalizedEndpoints.join('-');
      if (seenPhysicalLinks.has(physicalKey)) {
        return false;
      }
      seenPhysicalLinks.add(physicalKey);
      return true;
    })
    .map((link) => {
      const sourceNode = nodesByID.get(link.source_device_id)!;
      const targetNode = nodesByID.get(link.target_device_id)!;
      const { sourceHandle, targetHandle } = getHandleSide(
        sourceNode.position,
        targetNode.position,
        link.source_device_id === link.target_device_id,
      );

      const data = buildEdgeData(
        link,
        devicesByID,
        existingEdgeDataByID?.get(link.id),
        onContextMenu,
      );
      data.alertStatus = alertStatusForLink(link, alerts);

      return {
        id: link.id,
        source: link.source_device_id,
        target: link.target_device_id,
        sourceHandle,
        targetHandle,
        type: 'link',
        selectable: true,
        data,
      };
    });

  const candidateEdgesByPair = new Map<string, LinkEdgeType[]>();
  for (const edge of candidateEdges) {
    const pairKey = [edge.source, edge.target].sort().join('-');
    const pairEdges = candidateEdgesByPair.get(pairKey) ?? [];
    pairEdges.push(edge);
    candidateEdgesByPair.set(pairKey, pairEdges);
  }

  const filteredEdges: LinkEdgeType[] = [];
  for (const pairEdges of candidateEdgesByPair.values()) {
    const hasRichPhysicalEdge = pairEdges.some((edge) => {
      const link = edge.data?.link;
      return !!link && (isCompletePhysicalLink(link) || !!edge.data?.bandwidthLabel);
    });

    const survivingEdges = hasRichPhysicalEdge
      ? pairEdges.filter((edge) => {
          const link = edge.data?.link;
          if (!link) {
            return true;
          }
          if (isCompletePhysicalLink(link) || edge.data?.bandwidthLabel) {
            return true;
          }
          return false;
        })
      : pairEdges;

    survivingEdges.forEach((edge, parallelIndex) => {
      filteredEdges.push({
        ...edge,
        data: {
          ...edge.data!,
          parallelIndex,
        },
      });
    });
  }

  return filteredEdges;
}

/** Alert status for link state in the topology canvas. */
export function alertStatusForLink(link: Link, alerts: AlertDTO[]): AlertStatus {
  const deviceIds = new Set([link.source_device_id, link.target_device_id]);
  const sourceIfName = (link.source_if_name ?? '').toLowerCase();
  const targetIfName = (link.target_if_name ?? '').toLowerCase();

  const relevantAlerts = alerts.filter((alert) => {
    if (!deviceIds.has(alert.device_id)) return false;
    if (alert.state !== 'firing') return false;
    // Interface-specific alerts: check if the summary references the interface name
    const summary = alert.summary.toLowerCase();
    const isLinkAlert =
      alert.alert_name === 'LinkDown' || alert.alert_name === 'HighLinkUtilization';
    if (!isLinkAlert) return false;
    // Best-effort interface name matching
    if (sourceIfName && summary.includes(sourceIfName)) return true;
    if (targetIfName && summary.includes(targetIfName)) return true;
    // If no interface names to match, fall back to device-level match
    if (!sourceIfName && !targetIfName) return true;
    return false;
  });

  if (relevantAlerts.some((alert) => alert.severity === 'critical')) {
    return 'down';
  }
  if (relevantAlerts.some((alert) => alert.severity === 'warning')) {
    return 'degraded';
  }
  return 'normal';
}
