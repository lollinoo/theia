import type { Edge, Node } from '@xyflow/react';

import type { Device, Link } from '../../types/api';
import type { AlertDTO, AlertStatus } from '../../types/metrics';
import type { DeviceNodeData } from '../DeviceCard';
import type { LinkEdgeData } from '../LinkEdge';
import { formatBandwidth } from '../LinkEdge';
import { inferSpeedLabel, type HandleSide } from './canvasHelpers';

export function buildEdgeData(
  link: Link,
  devicesByID: Map<string, Device>,
  existingData?: LinkEdgeData,
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void,
): LinkEdgeData {
  const sourceDevice = devicesByID.get(link.source_device_id);
  const targetDevice = devicesByID.get(link.target_device_id);
  const sourceInterface = sourceDevice?.interfaces.find(
    (iface) => iface.if_name === link.source_if_name,
  );
  const targetInterface = targetDevice?.interfaces.find(
    (iface) => iface.if_name === link.target_if_name,
  );

  // Compare negotiation speeds from both sides; show minimum with warning on mismatch
  const sourceSpeed = sourceInterface?.speed && sourceInterface.speed > 0 ? sourceInterface.speed : 0;
  const targetSpeed = targetInterface?.speed && targetInterface.speed > 0 ? targetInterface.speed : 0;

  let bandwidthLabel: string | undefined;
  let speedMismatch = false;

  if (sourceSpeed > 0 && targetSpeed > 0) {
    if (sourceSpeed !== targetSpeed) {
      // Mismatch: show minimum speed with warning indicator
      const minSpeed = Math.min(sourceSpeed, targetSpeed);
      bandwidthLabel = `${formatBandwidth(minSpeed)} (!)`;
      speedMismatch = true;
    } else {
      bandwidthLabel = formatBandwidth(sourceSpeed);
    }
  } else if (sourceSpeed > 0) {
    bandwidthLabel = formatBandwidth(sourceSpeed);
  } else if (targetSpeed > 0) {
    bandwidthLabel = formatBandwidth(targetSpeed);
  } else {
    bandwidthLabel = inferSpeedLabel(sourceDevice, targetDevice);
  }

  return {
    link,
    bandwidthLabel,
    speedMismatch,
    onContextMenu,
    metrics: existingData?.metrics,
    throughputLabel: existingData?.throughputLabel,
    utilization: existingData?.utilization,
    sourceIfStatus: sourceInterface?.oper_status,
    targetIfStatus: targetInterface?.oper_status,
    sourceDeviceStatus: existingData?.sourceDeviceStatus ?? sourceDevice?.status,
    targetDeviceStatus: existingData?.targetDeviceStatus ?? targetDevice?.status,
  };
}

export function getHandleSide(
  sourcePosition: { x: number; y: number },
  targetPosition: { x: number; y: number },
): { sourceHandle: HandleSide; targetHandle: HandleSide } {
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

export function buildTopologyEdges(
  links: Link[],
  devicesByID: Map<string, Device>,
  nodes: Node<DeviceNodeData>[],
  existingEdgeDataByID?: Map<string, LinkEdgeData>,
  onContextMenu?: (event: MouseEvent | React.MouseEvent<SVGPathElement>, edgeID: string) => void,
): Edge<LinkEdgeData>[] {
  const nodesByID = new Map(nodes.map((node) => [node.id, node]));
  const pairCounts = new Map<string, number>();
  const seenPairs = new Set<string>();

  return links
    .filter((link) => {
      if (!nodesByID.has(link.source_device_id) || !nodesByID.has(link.target_device_id)) {
        return false;
      }
      // Deduplicate bidirectional links: A->B and B->A for the same port pair are the same physical link
      const pairKey = [link.source_device_id, link.target_device_id].sort().join('-');
      if (seenPairs.has(pairKey)) {
        return false;
      }
      seenPairs.add(pairKey);
      return true;
    })
    .map((link) => {
      const sourceNode = nodesByID.get(link.source_device_id)!;
      const targetNode = nodesByID.get(link.target_device_id)!;
      const { sourceHandle, targetHandle } = getHandleSide(
        sourceNode.position,
        targetNode.position,
      );

      const pairKey = [link.source_device_id, link.target_device_id].sort().join('-');
      const parallelIndex = pairCounts.get(pairKey) || 0;
      pairCounts.set(pairKey, parallelIndex + 1);

      const data = buildEdgeData(
        link,
        devicesByID,
        existingEdgeDataByID?.get(link.id),
        onContextMenu,
      );
      data.parallelIndex = parallelIndex;

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
}

export function alertStatusForLink(link: Link, alerts: AlertDTO[]): AlertStatus {
  const deviceIds = new Set([link.source_device_id, link.target_device_id]);
  const sourceIfName = (link.source_if_name ?? '').toLowerCase();
  const targetIfName = (link.target_if_name ?? '').toLowerCase();

  const relevantAlerts = alerts.filter((alert) => {
    if (!deviceIds.has(alert.device_id)) return false;
    if (alert.state !== 'firing') return false;
    // Interface-specific alerts: check if the summary references the interface name
    const summary = alert.summary.toLowerCase();
    const isLinkAlert = alert.alert_name === 'LinkDown' || alert.alert_name === 'HighLinkUtilization';
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
