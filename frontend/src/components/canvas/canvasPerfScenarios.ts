/**
 * Defines canvas perf scenarios behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { Device, DeviceInterface, DeviceType, Link } from '../../types/api';
import type {
  AlertDTO,
  DeviceRuntimeDTO,
  LinkRuntimeDTO,
  SnapshotPayload,
} from '../../types/metrics';

/** Defines canvas perf scenarios constants and helper contracts for the topology canvas. */
export const CANVAS_PERF_SCENARIOS = {
  small: { deviceCount: 25, linkCount: 40 },
  medium: { deviceCount: 100, linkCount: 180 },
  large: { deviceCount: 300, linkCount: 600 },
  stress: { deviceCount: 700, linkCount: 1500 },
} as const;

/** Describes the canvas perf scenario name contract used by the topology canvas. */
export type CanvasPerfScenarioName = keyof typeof CANVAS_PERF_SCENARIOS;

/** Describes the canvas perf scenario contract used by the topology canvas. */
export interface CanvasPerfScenario {
  name: CanvasPerfScenarioName;
  devices: Device[];
  links: Link[];
  positions: Map<string, { x: number; y: number; pinned?: boolean }>;
  runtimeSnapshot: SnapshotPayload;
  alerts: AlertDTO[];
  selectedAreaId: string;
}

interface GenerateCanvasPerfScenarioOptions {
  seed?: number;
}

const deviceTypes: DeviceType[] = ['router', 'switch', 'ap', 'firewall', 'virtual'];
const linkSpeeds = [100_000_000, 1_000_000_000, 10_000_000_000, 25_000_000_000];
const fixedTimestamp = '2026-04-30T00:00:00.000Z';

function mulberry32(seed: number): () => number {
  let state = seed;

  return function random() {
    state += 0x6d2b79f5;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function pick<T>(values: T[], random: () => number): T {
  return values[Math.floor(random() * values.length)] ?? values[0];
}

function buildInterface(
  deviceIndex: number,
  interfaceIndex: number,
  speed: number,
): DeviceInterface {
  return {
    id: `if-${deviceIndex}-${interfaceIndex}`,
    if_index: interfaceIndex,
    if_name: `ether${interfaceIndex}`,
    if_descr: `ether${interfaceIndex}`,
    speed,
    admin_status: 'up',
    oper_status: interfaceIndex % 11 === 0 ? 'down' : 'up',
  };
}

function buildDevice(index: number, areaCount: number, random: () => number): Device {
  const deviceType = index % 17 === 0 ? 'virtual' : pick(deviceTypes, random);
  const primaryArea = `area-${(index % areaCount) + 1}`;
  const secondaryArea = index % 13 === 0 ? `area-${((index + 3) % areaCount) + 1}` : null;
  const interfaceBaseSpeed = pick(linkSpeeds, random);
  const ip = deviceType === 'virtual' ? '' : `10.${Math.floor(index / 250)}.${index % 250}.1`;
  const id = `dev-${String(index + 1).padStart(4, '0')}`;

  return {
    id,
    hostname: `device-${String(index + 1).padStart(4, '0')}`,
    ip,
    addresses:
      ip === ''
        ? []
        : [
            {
              id: `${id}-primary-address`,
              device_id: id,
              address: ip,
              label: '',
              role: 'primary',
              is_primary: true,
              priority: 0,
              probe_ports: null,
            },
          ],
    probe_ports: null,
    device_type: deviceType,
    poll_class: index % 10 === 0 ? 'core' : index % 5 === 0 ? 'low' : 'standard',
    poll_interval_override: index % 19 === 0 ? 120 : null,
    polling_enabled: index % 23 !== 0,
    status: index % 29 === 0 ? 'down' : index % 31 === 0 ? 'probing' : 'up',
    sys_name: `device-${String(index + 1).padStart(4, '0')}`,
    sys_descr: `${deviceType} synthetic fixture`,
    hardware_model: deviceType === 'virtual' ? 'Synthetic Virtual' : 'Synthetic Appliance',
    os_version: `v${1 + (index % 4)}.${index % 10}`,
    vendor: index % 4 === 0 ? 'juniper' : index % 3 === 0 ? 'cisco' : 'mikrotik',
    managed: index % 37 !== 0,
    tags:
      deviceType === 'virtual'
        ? { virtual_subtype: index % 2 === 0 ? 'cloud' : 'gateway' }
        : undefined,
    interfaces: [
      buildInterface(index, 1, interfaceBaseSpeed),
      buildInterface(index, 2, pick(linkSpeeds, random)),
      buildInterface(index, 3, pick(linkSpeeds, random)),
    ],
    area_ids: secondaryArea ? [primaryArea, secondaryArea] : [primaryArea],
    backup_supported: deviceType !== 'virtual' && index % 7 !== 0,
    metrics_source: deviceType === 'virtual' ? 'none' : 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: `device-${String(index + 1).padStart(4, '0')}:9100`,
    topology_discovery_mode: index % 9 === 0 ? 'lldp_cdp' : 'inherit',
    effective_topology_discovery_mode: index % 9 === 0 ? 'lldp_cdp' : 'lldp',
    topology_bootstrap_state: index % 41 === 0 ? 'followup_scheduled' : 'completed',
    last_topology_discovery_at: fixedTimestamp,
    last_topology_discovery_result: 'ok',
  };
}

function buildLink(index: number, devices: Device[], random: () => number): Link {
  const sourceIndex = index % devices.length;
  const selfLink = index > 0 && index % 37 === 0;
  const targetOffset = 1 + Math.floor(random() * Math.max(1, devices.length / 3));
  const targetIndex = selfLink ? sourceIndex : (sourceIndex + targetOffset) % devices.length;
  const sourceSpeed = pick(linkSpeeds, random);
  const targetSpeed = index % 11 === 0 ? pick(linkSpeeds, random) : sourceSpeed;

  return {
    id: `link-${String(index + 1).padStart(5, '0')}`,
    source_device_id: devices[sourceIndex].id,
    source_if_name: `ether${1 + (index % 3)}`,
    target_device_id: devices[targetIndex].id,
    target_if_name: `ether${1 + ((index + 1) % 3)}`,
    discovery_protocol: index % 5 === 0 ? 'cdp' : 'lldp',
    source_if_speed: sourceSpeed,
    source_if_oper_status: index % 17 === 0 ? 'down' : 'up',
    target_if_speed: targetSpeed,
    target_if_oper_status: index % 19 === 0 ? 'down' : 'up',
  };
}

function buildDeviceRuntime(device: Device, index: number): DeviceRuntimeDTO {
  const down = device.status === 'down' || index % 29 === 0;
  const unmonitored = !device.polling_enabled || device.device_type === 'virtual';

  return {
    device_id: device.id,
    operational_status: unmonitored ? 'unmonitored' : down ? 'down' : 'up',
    primary_health: down ? 'unreachable' : index % 13 === 0 ? 'snmp_degraded' : 'up_fresh',
    runtime_flags: index % 11 === 0 ? ['partial_telemetry'] : [],
    field_states: {
      uptime: unmonitored ? 'missing' : 'ok',
      cpu: index % 13 === 0 ? 'stale' : 'ok',
      memory: 'ok',
    },
    network_reachable: down ? 'false' : unmonitored ? 'unknown' : 'true',
    snmp_reachable: index % 13 === 0 ? 'false' : unmonitored ? 'unknown' : 'true',
    reachability: down ? 'hard_down' : unmonitored ? 'unmonitored' : 'up',
    health: down ? 'critical' : index % 13 === 0 ? 'warning' : 'healthy',
    freshness: unmonitored ? 'unmonitored' : index % 17 === 0 ? 'stale' : 'fresh',
    primary_reason: unmonitored ? 'unmonitored' : down ? 'device_unreachable' : 'ok',
    metrics_status: unmonitored ? 'unmonitored' : index % 13 === 0 ? 'partial' : 'available',
    metrics_reason: unmonitored ? 'unmonitored' : index % 13 === 0 ? 'stale' : 'ok',
    alert_status: down ? 'down' : index % 13 === 0 ? 'degraded' : 'normal',
    firing_alert_count: down ? 2 : index % 13 === 0 ? 1 : 0,
    last_collected_at: unmonitored ? null : fixedTimestamp,
    last_polled_at: unmonitored ? null : fixedTimestamp,
    expected_poll_interval_seconds: unmonitored ? null : 60,
    cpu_percent: unmonitored ? null : (index * 7) % 100,
    mem_percent: unmonitored ? null : (index * 11) % 100,
    temp_celsius: unmonitored ? null : 35 + (index % 20),
    uptime_secs: unmonitored ? null : 10_000 + index * 60,
  };
}

function buildLinkRuntime(link: Link, index: number): LinkRuntimeDTO {
  const partial = index % 17 === 0;

  return {
    link_id: link.id,
    source_device_id: link.source_device_id,
    target_device_id: link.target_device_id,
    source_if_name: link.source_if_name,
    target_if_name: link.target_if_name,
    metrics_status: partial ? 'partial' : 'available',
    metrics_reason: partial ? 'stale' : 'ok',
    last_collected_at: fixedTimestamp,
    tx_bps: partial ? null : (index + 1) * 125_000,
    rx_bps: partial ? null : (index + 1) * 95_000,
    utilization: partial ? null : Math.min(0.98, ((index % 70) + 5) / 100),
  };
}

function buildPositions(
  devices: Device[],
  random: () => number,
): Map<string, { x: number; y: number; pinned?: boolean }> {
  const positions = new Map<string, { x: number; y: number; pinned?: boolean }>();

  devices.forEach((device, index) => {
    if (index % 3 !== 0) return;
    positions.set(device.id, {
      x: Math.round(120 + random() * 3200),
      y: Math.round(120 + random() * 2200),
      pinned: index % 2 === 0,
    });
  });

  return positions;
}

function buildAlerts(devices: Device[]): AlertDTO[] {
  return devices
    .filter((_, index) => index % 29 === 0 || index % 13 === 0)
    .slice(0, Math.max(1, Math.floor(devices.length / 12)))
    .map((device, index) => ({
      device_id: device.id,
      severity: index % 2 === 0 ? 'critical' : 'warning',
      alert_name: index % 2 === 0 ? 'DeviceDown' : 'DeviceDegraded',
      state: 'firing',
      summary: `${device.hostname} synthetic alert`,
    }));
}

function buildRuntimeSnapshot(devices: Device[], links: Link[]): SnapshotPayload {
  return {
    devices: Object.fromEntries(
      devices
        .filter((_, index) => index % 4 !== 3)
        .map((device, index) => [device.id, buildDeviceRuntime(device, index)]),
    ),
    links: Object.fromEntries(
      links
        .filter((link, index) => index % 5 !== 4 && link.source_device_id !== link.target_device_id)
        .map((link, index) => [link.id, buildLinkRuntime(link, index)]),
    ),
  };
}

/** Generates canvas perf scenario for the topology canvas. */
export function generateCanvasPerfScenario(
  name: CanvasPerfScenarioName,
  options: GenerateCanvasPerfScenarioOptions = {},
): CanvasPerfScenario {
  const definition = CANVAS_PERF_SCENARIOS[name];
  const random = mulberry32(options.seed ?? definition.deviceCount * 1009 + definition.linkCount);
  const areaCount = Math.max(3, Math.ceil(definition.deviceCount / 35));
  const devices = Array.from({ length: definition.deviceCount }, (_, index) =>
    buildDevice(index, areaCount, random),
  );
  const links = Array.from({ length: definition.linkCount }, (_, index) =>
    buildLink(index, devices, random),
  );

  return {
    name,
    devices,
    links,
    positions: buildPositions(devices, random),
    runtimeSnapshot: buildRuntimeSnapshot(devices, links),
    alerts: buildAlerts(devices),
    selectedAreaId: 'area-1',
  };
}
