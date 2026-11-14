/**
 * Defines node builder behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { SnapGrid } from '@xyflow/react';

import type { Device, Link } from '../../types/api';
import { type AlertDTO, alertStatusForDevice, type SnapshotPayload } from '../../types/metrics';
import type { DeviceNode, DeviceNodeRuntimeData } from '../DeviceCard';
import {
  resolveDeviceMonitoringState,
  sanitizeDeviceMetricsForDisplay,
} from '../deviceVisualState';
import { snapPositionToGrid } from './canvasGrid';
import { preferVisibleLinks } from './edgeBuilder';
import { buildRuntimeState, type RuntimeState } from './runtimeAdapters';

function hasUsablePosition(
  position: { x: number; y: number; pinned?: boolean } | undefined,
): boolean {
  return position !== undefined && Number.isFinite(position.x) && Number.isFinite(position.y);
}
function selfLinkScore(link: Link): number {
  let score = 0;
  if (link.discovery_protocol === 'lldp') score += 4;
  if (link.source_if_name) score += 2;
  if (link.target_if_name) score += 2;
  return score;
}

export interface BuildTopologyNodesInput {
  devices: Device[];
  savedPositions?: Map<string, { x: number; y: number; pinned?: boolean }>;
  computedPositions?: Map<string, { x: number; y: number }>;
  explicitPositions?: Map<string, { x: number; y: number }>;
  editMode?: boolean;
  openDeviceMenu?: (event: React.MouseEvent, deviceId: string) => void;
  pendingSnapshot?: SnapshotPayload | null;
  snapshot?: SnapshotPayload | null;
  alerts?: AlertDTO[];
  links?: Link[];
  onSelfLinkClick?: (link: Link) => void;
  currentPositions?: Map<string, { x: number; y: number; pinned?: boolean }>;
  placementDeviceIds?: Set<string>;
  snapGrid?: SnapGrid | null;
  runtimeState?: RuntimeState;
}

/** Builds topology nodes for the topology canvas. */
export function buildTopologyNodes(
  devices: Device[],
  savedPositions?: Map<string, { x: number; y: number; pinned?: boolean }>,
  computedPositions?: Map<string, { x: number; y: number }>,
  explicitPositions?: Map<string, { x: number; y: number }>,
  editMode?: boolean,
  openDeviceMenu?: (event: React.MouseEvent, deviceId: string) => void,
  pendingSnapshot?: SnapshotPayload | null,
  alerts?: AlertDTO[],
  links?: Link[],
  onSelfLinkClick?: (link: Link) => void,
  currentPositions?: Map<string, { x: number; y: number; pinned?: boolean }>,
  placementDeviceIds?: Set<string>,
  snapGrid?: SnapGrid | null,
  runtimeState?: RuntimeState,
): DeviceNode[];
export function buildTopologyNodes(input: BuildTopologyNodesInput): DeviceNode[];
export function buildTopologyNodes(
  devicesOrInput: Device[] | BuildTopologyNodesInput,
  savedPositions: Map<string, { x: number; y: number; pinned?: boolean }> = new Map(),
  computedPositions: Map<string, { x: number; y: number }> = new Map(),
  explicitPositions: Map<string, { x: number; y: number }> = new Map(),
  editMode = false,
  openDeviceMenu: (event: React.MouseEvent, deviceId: string) => void = () => undefined,
  pendingSnapshot: SnapshotPayload | null = null,
  alerts: AlertDTO[] = [],
  links: Link[] = [],
  onSelfLinkClick?: (link: Link) => void,
  currentPositions: Map<string, { x: number; y: number; pinned?: boolean }> = new Map(),
  placementDeviceIds?: Set<string>,
  snapGrid: SnapGrid | null = null,
  runtimeState?: RuntimeState,
): DeviceNode[] {
  const input: BuildTopologyNodesInput = Array.isArray(devicesOrInput)
    ? {
        devices: devicesOrInput,
        savedPositions,
        computedPositions,
        explicitPositions,
        editMode,
        openDeviceMenu,
        pendingSnapshot,
        alerts,
        links,
        onSelfLinkClick,
        currentPositions,
        placementDeviceIds:
          placementDeviceIds ?? new Set(devicesOrInput.map((device) => device.id)),
        snapGrid,
        runtimeState,
      }
    : devicesOrInput;

  const devices = input.devices;
  const resolvedSavedPositions = input.savedPositions ?? new Map();
  const resolvedComputedPositions = input.computedPositions ?? new Map();
  const resolvedExplicitPositions = input.explicitPositions ?? new Map();
  const resolvedCurrentPositions = input.currentPositions ?? new Map();
  const resolvedPlacementDeviceIds =
    input.placementDeviceIds ?? new Set(devices.map((device) => device.id));
  const resolvedEditMode = input.editMode ?? false;
  const resolvedOpenDeviceMenu = input.openDeviceMenu ?? (() => undefined);
  const resolvedAlerts = input.alerts ?? [];
  const resolvedLinks = input.links ?? [];
  const resolvedSnapGrid = input.snapGrid ?? null;
  const snapshot = input.snapshot ?? input.pendingSnapshot ?? null;

  const resolvedRuntimeState =
    input.runtimeState ??
    buildRuntimeState({
      devices,
      links: resolvedLinks,
      snapshot,
      alerts: resolvedAlerts,
      prometheusStatus: null,
    });

  const selfLinksByDeviceId = new Map<string, Link[]>();
  for (const link of preferVisibleLinks(resolvedLinks)) {
    if (link.source_device_id !== link.target_device_id) continue;
    const deviceLinks = selfLinksByDeviceId.get(link.source_device_id) ?? [];
    deviceLinks.push(link);
    selfLinksByDeviceId.set(link.source_device_id, deviceLinks);
  }

  for (const deviceLinks of selfLinksByDeviceId.values()) {
    deviceLinks.sort((left, right) => {
      const scoreDelta = selfLinkScore(right) - selfLinkScore(left);
      if (scoreDelta !== 0) return scoreDelta;
      return left.id.localeCompare(right.id);
    });
  }

  return devices.map((device) => {
    const current = resolvedCurrentPositions.get(device.id);
    const saved = resolvedSavedPositions.get(device.id);
    const explicit = resolvedExplicitPositions.get(device.id);
    const computed = resolvedPlacementDeviceIds.has(device.id)
      ? resolvedComputedPositions.get(device.id)
      : undefined;
    const position = hasUsablePosition(explicit)
      ? explicit
      : hasUsablePosition(current)
        ? current
        : hasUsablePosition(saved)
          ? saved
          : computed;
    const explicitlyPlaced = hasUsablePosition(explicit);
    const computedPlacement =
      !hasUsablePosition(current) && !hasUsablePosition(saved) && hasUsablePosition(computed);
    const unresolvedPosition = position ?? { x: 0, y: 0 };
    const resolvedPosition =
      resolvedSnapGrid && (explicitlyPlaced || computedPlacement)
        ? snapPositionToGrid(unresolvedPosition, resolvedSnapGrid)
        : unresolvedPosition;
    const selfLinks = selfLinksByDeviceId.get(device.id);

    const runtimeDevice = resolvedRuntimeState.devicesById.get(device.id);
    const monitoringState = runtimeDevice?.monitoringState ?? resolveDeviceMonitoringState(device);
    const runtime: DeviceNodeRuntimeData = runtimeDevice
      ? {
          status: runtimeDevice.device.status,
          metrics: runtimeDevice.metrics,
          alertStatus: runtimeDevice.alertStatus,
          monitoringState: runtimeDevice.monitoringState,
        }
      : {
          status: device.status,
          metrics: sanitizeDeviceMetricsForDisplay(device, null, monitoringState),
          alertStatus: alertStatusForDevice(device.id, resolvedAlerts),
          monitoringState,
        };

    // Virtual devices have no SNMP metrics; detect and propagate flags
    const isVirtual = device.device_type === 'virtual';
    return {
      id: device.id,
      type: 'device',
      position: {
        x: resolvedPosition.x,
        y: resolvedPosition.y,
      },
      data: {
        kind: 'device',
        device,
        runtime,
        pinned: explicitlyPlaced ? false : (current?.pinned ?? saved?.pinned ?? false),
        highlighted: false,
        editMode: resolvedEditMode,
        onContextMenu: resolvedOpenDeviceMenu,
        isVirtual,
        subtype: isVirtual ? (device.tags?.virtual_subtype ?? 'generic') : undefined,
        selfLinks,
        onSelfLinkClick: input.onSelfLinkClick,
      },
    };
  });
}
