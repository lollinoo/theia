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

function encodeSignaturePart(value: unknown): string {
  if (value === undefined) {
    return 'u:';
  }
  if (value === null) {
    return 'n:';
  }

  const encoded = String(value);
  return `${encoded.length}:${encoded}`;
}

function encodeSignatureParts(values: unknown[]): string {
  return values.map(encodeSignaturePart).join('');
}

function positionMapSignature(map: PositionMap | ComputedPositionMap): string {
  const parts = [`size:${map.size}`];
  const entries = Array.from(map.entries()).sort(([left], [right]) => left.localeCompare(right));
  for (const [deviceId, position] of entries) {
    parts.push(
      encodeSignatureParts([
        deviceId,
        position.x,
        position.y,
        'pinned' in position ? position.pinned === true : false,
      ]),
    );
  }
  return parts.join('|');
}

function placementSignature(deviceIds: Set<string>): string {
  const parts = [`size:${deviceIds.size}`];
  const sortedDeviceIds = Array.from(deviceIds.values()).sort((left, right) =>
    left.localeCompare(right),
  );
  for (const deviceId of sortedDeviceIds) {
    parts.push(encodeSignaturePart(deviceId));
  }
  return parts.join('|');
}

function alertSignature(alerts: AlertDTO[]): string {
  const parts = [`size:${alerts.length}`];
  const sortedAlertSignatures = alerts
    .map((alert) =>
      encodeSignatureParts([
        alert.device_id,
        alert.severity,
        alert.alert_name,
        alert.state,
        alert.summary,
      ]),
    )
    .sort((left, right) => left.localeCompare(right));
  for (const alertSignatureValue of sortedAlertSignatures) {
    parts.push(alertSignatureValue);
  }
  return parts.join('|');
}

function prometheusStatusSignature(status: PrometheusStatusPayload | null): string {
  if (status === null) {
    return 'null';
  }

  return encodeSignatureParts([status.enabled ?? null, status.available, status.error ?? null]);
}

function normalizedRuntimeIdentity(
  input: BuildCanvasTopologyCompositionCacheKeyInput,
): string | null {
  if (input.runtimeIdentity !== undefined && input.runtimeIdentity !== '') {
    return input.runtimeIdentity;
  }
  return null;
}

function normalizedRuntimeVersion(
  input: BuildCanvasTopologyCompositionCacheKeyInput,
): number | null {
  return input.runtimeVersion ?? null;
}

function runtimeSignature(input: BuildCanvasTopologyCompositionCacheKeyInput): string {
  const runtimeIdentity = normalizedRuntimeIdentity(input);
  const runtimeVersion = normalizedRuntimeVersion(input);

  if (runtimeIdentity !== null || runtimeVersion !== null) {
    return encodeSignatureParts(['runtime', runtimeIdentity, runtimeVersion]);
  }

  return 'snapshot-ref';
}

function hasServerTopologyIdentifier(input: BuildCanvasTopologyCompositionCacheKeyInput): boolean {
  return (
    (input.topologyEtag !== undefined &&
      input.topologyEtag !== null &&
      input.topologyEtag !== '') ||
    (input.topologyVersion !== undefined && input.topologyVersion !== '')
  );
}

function topologySignatureLayer(input: BuildCanvasTopologyCompositionCacheKeyInput): unknown {
  if (hasServerTopologyIdentifier(input)) {
    return {
      etag: input.topologyEtag ?? null,
      version: input.topologyVersion ?? null,
    };
  }

  return {
    signature: input.topologySignature,
    devices: devicePresentationSignature(input.devices),
    links: linkPresentationSignature(input.links),
  };
}

export function buildCanvasTopologyCompositionCacheKey(
  input: BuildCanvasTopologyCompositionCacheKeyInput,
): CanvasTopologyCompositionCacheKey {
  const runtimeIdentity = normalizedRuntimeIdentity(input);
  const runtimeVersion = normalizedRuntimeVersion(input);

  return {
    signature: JSON.stringify({
      mapKey: input.mapKey,
      schemaVersion: input.schemaVersion ?? null,
      topology: topologySignatureLayer(input),
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
    runtimeIdentity,
    runtimeVersion,
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
    return (
      previous.runtimeIdentity === next.runtimeIdentity &&
      previous.runtimeVersion === next.runtimeVersion
    );
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
