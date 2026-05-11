import type { Device, Link } from '../../types/api';
import type { AlertDTO, PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import {
  type ComposeCanvasTopologyInput,
  type ComposeCanvasTopologyResult,
  composeCanvasTopology,
} from './topologyComposer';

type PositionMap = Map<string, { x: number; y: number; pinned?: boolean }>;
type ComputedPositionMap = Map<string, { x: number; y: number }>;

export interface CanvasTopologyCompositionCacheKey {
  signature: string;
  runtimeIdentity?: string | null;
  runtimeVersion?: number | null;
  runtimeSnapshot?: SnapshotPayload | null;
  openDeviceMenu: unknown;
  openEdgeMenu: unknown;
  openSelfLinkDetails?: unknown;
}

export interface BuildCanvasTopologyCompositionCacheKeyInput {
  mapKey: string;
  topologySignature: string;
  topologyVersion?: string;
  topologyEtag?: string | null;
  schemaVersion?: number;
  devices: Device[];
  links: Link[];
  savedPositions: PositionMap;
  computedPositions: ComputedPositionMap;
  currentPositions: PositionMap;
  defaultPosition?: { x: number; y: number };
  editMode: boolean;
  placementDeviceIds: Set<string>;
  runtimeIdentity?: string;
  runtimeVersion?: number;
  runtimeSnapshot?: SnapshotPayload | null;
  alerts: AlertDTO[];
  prometheusStatus: PrometheusStatusPayload | null;
  openDeviceMenu: unknown;
  openEdgeMenu: unknown;
  openSelfLinkDetails?: unknown;
}

export type CanvasTopologyComposer = (
  input: ComposeCanvasTopologyInput,
) => ComposeCanvasTopologyResult;

interface CanvasTopologyCompositionCache {
  compose: (
    input: ComposeCanvasTopologyInput,
    key: CanvasTopologyCompositionCacheKey,
  ) => ComposeCanvasTopologyResult;
  clear: () => void;
}

function sortedRecordEntries(record: Record<string, string> | undefined): string[][] {
  return Object.entries(record ?? {}).sort(([left], [right]) => left.localeCompare(right));
}

function devicePresentationSignature(devices: Device[]): unknown[] {
  return devices
    .map((device) => ({
      id: device.id,
      hostname: device.hostname,
      ip: device.ip,
      notes: device.notes ?? null,
      device_type: device.device_type,
      poll_class: device.poll_class,
      poll_interval_override: device.poll_interval_override,
      polling_enabled: device.polling_enabled,
      status: device.status,
      sys_name: device.sys_name,
      sys_descr: device.sys_descr,
      hardware_model: device.hardware_model,
      os_version: device.os_version ?? null,
      vendor: device.vendor,
      managed: device.managed,
      tags: sortedRecordEntries(device.tags),
      interfaces: device.interfaces.map((iface) => ({
        id: iface.id,
        if_index: iface.if_index,
        if_name: iface.if_name,
        if_descr: iface.if_descr,
        speed: iface.speed,
        admin_status: iface.admin_status,
        oper_status: iface.oper_status,
      })),
      area_ids: device.area_ids,
      backup_supported: device.backup_supported,
      metrics_source: device.metrics_source,
      prometheus_label_name: device.prometheus_label_name,
      prometheus_label_value: device.prometheus_label_value,
      topology_discovery_mode: device.topology_discovery_mode ?? null,
      effective_topology_discovery_mode: device.effective_topology_discovery_mode ?? null,
      topology_bootstrap_state: device.topology_bootstrap_state ?? null,
      last_topology_discovery_at: device.last_topology_discovery_at ?? null,
      last_topology_discovery_result: device.last_topology_discovery_result ?? null,
      map_visual_color: device.map_visual_color ?? null,
    }))
    .sort((left, right) => left.id.localeCompare(right.id));
}

function linkPresentationSignature(links: Link[]): unknown[] {
  return links
    .map((link) => ({
      id: link.id,
      source_device_id: link.source_device_id,
      source_if_name: link.source_if_name,
      target_device_id: link.target_device_id,
      target_if_name: link.target_if_name,
      discovery_protocol: link.discovery_protocol,
      source_if_speed: link.source_if_speed,
      source_if_oper_status: link.source_if_oper_status,
      target_if_speed: link.target_if_speed,
      target_if_oper_status: link.target_if_oper_status,
    }))
    .sort((left, right) => left.id.localeCompare(right.id));
}

function positionMapSignature(map: PositionMap | ComputedPositionMap): unknown[] {
  return Array.from(map.entries())
    .map(([deviceId, position]) => [
      deviceId,
      position.x,
      position.y,
      'pinned' in position ? position.pinned === true : false,
    ])
    .sort(([left], [right]) => String(left).localeCompare(String(right)));
}

function placementSignature(deviceIds: Set<string>): string[] {
  return Array.from(deviceIds).sort();
}

function alertSignature(alerts: AlertDTO[]): unknown[] {
  return alerts
    .map((alert) => [alert.device_id, alert.severity, alert.alert_name, alert.state, alert.summary])
    .sort(([leftDeviceId, leftName], [rightDeviceId, rightName]) => {
      const deviceDelta = String(leftDeviceId).localeCompare(String(rightDeviceId));
      return deviceDelta === 0 ? String(leftName).localeCompare(String(rightName)) : deviceDelta;
    });
}

function prometheusStatusSignature(status: PrometheusStatusPayload | null): unknown {
  if (status === null) {
    return null;
  }

  return {
    enabled: status.enabled ?? null,
    available: status.available,
    error: status.error ?? null,
  };
}

function runtimeSignature(input: BuildCanvasTopologyCompositionCacheKeyInput): string {
  if (input.runtimeIdentity !== undefined && input.runtimeIdentity !== '') {
    return `identity:${input.runtimeIdentity}`;
  }
  if (input.runtimeVersion !== undefined) {
    return `version:${input.runtimeVersion}`;
  }
  return 'snapshot-ref';
}

export function buildCanvasTopologyCompositionCacheKey(
  input: BuildCanvasTopologyCompositionCacheKeyInput,
): CanvasTopologyCompositionCacheKey {
  return {
    signature: JSON.stringify({
      mapKey: input.mapKey,
      topologySignature: input.topologySignature,
      topologyVersion: input.topologyVersion ?? null,
      topologyEtag: input.topologyEtag ?? null,
      schemaVersion: input.schemaVersion ?? null,
      devices: devicePresentationSignature(input.devices),
      links: linkPresentationSignature(input.links),
      savedPositions: positionMapSignature(input.savedPositions),
      computedPositions: positionMapSignature(input.computedPositions),
      currentPositions: positionMapSignature(input.currentPositions),
      defaultPosition: input.defaultPosition ?? null,
      editMode: input.editMode,
      placementDeviceIds: placementSignature(input.placementDeviceIds),
      runtime: runtimeSignature(input),
      alerts: alertSignature(input.alerts),
      prometheusStatus: prometheusStatusSignature(input.prometheusStatus),
    }),
    runtimeIdentity: input.runtimeIdentity ?? null,
    runtimeVersion: input.runtimeVersion ?? null,
    runtimeSnapshot: input.runtimeSnapshot ?? null,
    openDeviceMenu: input.openDeviceMenu,
    openEdgeMenu: input.openEdgeMenu,
    openSelfLinkDetails: input.openSelfLinkDetails,
  };
}

function cacheKeysEqual(
  previous: CanvasTopologyCompositionCacheKey,
  next: CanvasTopologyCompositionCacheKey,
): boolean {
  if (
    previous.signature !== next.signature ||
    previous.openDeviceMenu !== next.openDeviceMenu ||
    previous.openEdgeMenu !== next.openEdgeMenu ||
    previous.openSelfLinkDetails !== next.openSelfLinkDetails
  ) {
    return false;
  }

  if (previous.runtimeIdentity !== null || next.runtimeIdentity !== null) {
    return previous.runtimeIdentity === next.runtimeIdentity;
  }
  if (previous.runtimeVersion !== null || next.runtimeVersion !== null) {
    return previous.runtimeVersion === next.runtimeVersion;
  }

  return previous.runtimeSnapshot === next.runtimeSnapshot;
}

export function createCanvasTopologyCompositionCache(
  composer: CanvasTopologyComposer = composeCanvasTopology,
): CanvasTopologyCompositionCache {
  let previousKey: CanvasTopologyCompositionCacheKey | null = null;
  let previousResult: ComposeCanvasTopologyResult | null = null;

  return {
    compose(input, key) {
      if (previousKey !== null && previousResult !== null && cacheKeysEqual(previousKey, key)) {
        return previousResult;
      }

      const result = composer(input);
      previousKey = key;
      previousResult = result;
      return result;
    },
    clear() {
      previousKey = null;
      previousResult = null;
    },
  };
}
