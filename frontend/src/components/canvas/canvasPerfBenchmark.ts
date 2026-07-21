/**
 * Defines canvas perf benchmark behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { AutoLayoutEdge, AutoLayoutNode } from '../../hooks/useAutoLayout';
import { computeForceLayout } from '../../hooks/useAutoLayout';
import type { Device } from '../../types/api';
import type { PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import { buildEditableLinkPath } from '../editableLinkGeometry';
import { projectAreaTopology } from './areaProjection';
import {
  aggregateCanvasMetricSamples,
  type CanvasMetricAggregate,
  type CanvasMetricName,
  type CanvasMetricSample,
} from './canvasInstrumentation';
import {
  CANVAS_PERF_SCENARIOS,
  type CanvasPerfScenario,
  type CanvasPerfScenarioName,
  generateCanvasPerfScenario,
} from './canvasPerfScenarios';
import { projectCanvasRenderGraph } from './canvasRenderProjection';
import { buildTopologyEdges } from './edgeBuilder';
import {
  buildIncrementalLayoutInputs,
  computeIncrementalLayoutPositions,
} from './incrementalLayout';
import { findNewNodePlacement } from './newNodePlacement';
import { buildTopologyNodes } from './nodeBuilder';
import { buildRuntimeState } from './runtimeAdapters';
import {
  buildRuntimePatchPlan,
  patchRuntimeDevices,
  patchRuntimeEdges,
  patchRuntimeNodes,
} from './runtimePatches';
import { composeCanvasTopology } from './topologyComposer';
import {
  type BuildCanvasTopologyCompositionCacheKeyInput,
  buildCanvasTopologyCompositionCacheKey,
  createCanvasTopologyCompositionCache,
} from './topologyCompositionCache';
import { buildTopologyIdentity } from './topologyIdentity';

/** Defines canvas perf benchmark metrics constants and helper contracts for the topology canvas. */
export const CANVAS_PERF_BENCHMARK_METRICS = [
  'buildTopologyNodes',
  'buildTopologyEdges',
  'buildCanvasTopologyCompositionCacheKey',
  'buildCanvasTopologyCompositionCacheKeyLegacy',
  'composeCanvasTopology',
  'composeCanvasTopologyCached',
  'areaProjection',
  'renderProjection',
  'runtimePatch',
  'incrementalLayout',
  'newNodePlacement',
  'editableLinkGeometry',
  'computeForceLayout',
] as const satisfies CanvasMetricName[];

/** Describes the canvas perf benchmark scenario result contract used by the topology canvas. */
export interface CanvasPerfBenchmarkScenarioResult {
  input: {
    deviceCount: number;
    linkCount: number;
  };
  metrics: Record<string, CanvasMetricAggregate>;
}

/** Describes the canvas perf benchmark result contract used by the topology canvas. */
export interface CanvasPerfBenchmarkResult {
  version: 1;
  generatedAt: string;
  iterations: number;
  iterationsByScenario?: Partial<Record<CanvasPerfScenarioName, number>>;
  scenarios: Record<CanvasPerfScenarioName, CanvasPerfBenchmarkScenarioResult>;
}

/** Describes the run canvas perf benchmark options contract used by the topology canvas. */
export interface RunCanvasPerfBenchmarkOptions {
  iterations?: number;
  warmupIterations?: number;
  iterationsByScenario?: Partial<Record<CanvasPerfScenarioName, number>>;
  scenarioNames?: CanvasPerfScenarioName[];
}

/** Configures the isolated worst-case editable link geometry benchmark. */
export interface RunEditableLinkGeometryBenchmarkOptions {
  iterations?: number;
  warmupIterations?: number;
}

const defaultIterationsByScenario: Record<CanvasPerfScenarioName, number> = {
  small: 50,
  medium: 30,
  large: 15,
  stress: 5,
};

function iterationsForScenario(
  options: RunCanvasPerfBenchmarkOptions,
  scenarioName: CanvasPerfScenarioName,
): number {
  return (
    options.iterationsByScenario?.[scenarioName] ??
    options.iterations ??
    defaultIterationsByScenario[scenarioName]
  );
}

const prometheusStatus: PrometheusStatusPayload = {
  enabled: true,
  available: true,
};

const noopDeviceMenu = (() => undefined) as unknown as (
  event: React.MouseEvent,
  deviceId: string,
) => void;
const noopEdgeMenu = (() => undefined) as unknown as (
  event: MouseEvent | React.MouseEvent<SVGPathElement>,
  edgeId: string,
) => void;
const noopGhostClick = () => undefined;

const editableLinkBenchmarkRoute = {
  version: 1 as const,
  waypoints: Array.from({ length: 16 }, (_, index) => ({
    x: 320 + index * 88,
    y: index % 2 === 0 ? 96 : 544,
  })),
};
const editableLinkBenchmarkSourceRect = { x: 32, y: 260, width: 240, height: 120 };
const editableLinkBenchmarkTargetRect = { x: 1_720, y: 260, width: 240, height: 120 };

function nowMs(): number {
  return typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : Date.now();
}

function measureLocalMetric<T>(
  samples: CanvasMetricSample[],
  scenario: CanvasPerfScenarioName,
  name: (typeof CANVAS_PERF_BENCHMARK_METRICS)[number],
  work: () => T,
): T {
  const startedAt = nowMs();
  try {
    return work();
  } finally {
    const durationMs = Number(Math.max(0, nowMs() - startedAt).toFixed(3));
    samples.push({
      name,
      scenario,
      durationMs,
      timestamp: Date.now(),
    });
  }
}

function benchmarkEditableLinkGeometry(
  samples: CanvasMetricSample[],
  scenarioName: CanvasPerfScenarioName,
): void {
  measureLocalMetric(samples, scenarioName, 'editableLinkGeometry', () =>
    buildEditableLinkPath({
      sourceRect: editableLinkBenchmarkSourceRect,
      targetRect: editableLinkBenchmarkTargetRect,
      fallbackSource: { x: 272, y: 320 },
      fallbackTarget: { x: 1_720, y: 320 },
      route: editableLinkBenchmarkRoute,
      parallelIndex: 0,
    }),
  );
}

/** Measures maximum-size editable link geometry without running unrelated graph benchmarks. */
export function runEditableLinkGeometryBenchmark(
  options: RunEditableLinkGeometryBenchmarkOptions = {},
): CanvasMetricAggregate {
  const scenarioName = 'stress';
  const measuredSamples: CanvasMetricSample[] = [];
  const warmupSamples: CanvasMetricSample[] = [];
  const warmupIterations = options.warmupIterations ?? 3;
  const iterations = options.iterations ?? defaultIterationsByScenario.stress;

  for (let index = 0; index < warmupIterations; index += 1) {
    benchmarkEditableLinkGeometry(warmupSamples, scenarioName);
  }

  for (let index = 0; index < iterations; index += 1) {
    benchmarkEditableLinkGeometry(measuredSamples, scenarioName);
  }

  const metric =
    aggregateCanvasMetricSamples(measuredSamples)[`${scenarioName}:editableLinkGeometry`];
  if (!metric) {
    throw new Error('Editable link geometry benchmark requires at least one measured iteration.');
  }
  return metric;
}

function legacyPositionEntries(
  positions: Map<string, { x: number; y: number; pinned?: boolean }>,
): unknown[] {
  return [...positions.entries()]
    .map(([deviceId, position]) => ({
      deviceId,
      x: position.x,
      y: position.y,
      pinned: position.pinned === true,
    }))
    .sort((left, right) => left.deviceId.localeCompare(right.deviceId));
}

function buildLegacyStructuralCompositionCacheSignature(
  input: BuildCanvasTopologyCompositionCacheKeyInput,
): string {
  return JSON.stringify({
    mapKey: input.mapKey,
    topologySignature: input.topologySignature,
    topologyVersion: input.topologyVersion ?? null,
    topologyEtag: input.topologyEtag ?? null,
    schemaVersion: input.schemaVersion ?? null,
    devices: input.devices
      .map((device) => ({
        ...device,
        tags: Object.entries(device.tags ?? {}).sort(([left], [right]) =>
          left.localeCompare(right),
        ),
        interfaces: device.interfaces.map((iface) => ({ ...iface })),
      }))
      .sort((left, right) => left.id.localeCompare(right.id)),
    links: input.links
      .map((link) => ({ ...link }))
      .sort((left, right) => left.id.localeCompare(right.id)),
    savedPositions: legacyPositionEntries(input.savedPositions),
    computedPositions: legacyPositionEntries(input.computedPositions),
    currentPositions: legacyPositionEntries(input.currentPositions),
    explicitPositions: legacyPositionEntries(input.explicitPositions),
    editMode: input.editMode,
    placementDeviceIds: [...input.placementDeviceIds].sort((left, right) =>
      left.localeCompare(right),
    ),
    runtimeIdentity: input.runtimeIdentity ?? null,
    runtimeVersion: input.runtimeVersion ?? null,
    runtimeSnapshot: input.runtimeSnapshot ?? null,
    alerts: input.alerts
      .map((alert) => ({ ...alert }))
      .sort((left, right) =>
        `${left.device_id}:${left.severity}:${left.alert_name}:${left.state}:${left.summary}`.localeCompare(
          `${right.device_id}:${right.severity}:${right.alert_name}:${right.state}:${right.summary}`,
        ),
      ),
    prometheusStatus: input.prometheusStatus,
  });
}

function buildCurrentPositions(
  nodes: DeviceNode[],
): Map<string, { x: number; y: number; pinned?: boolean }> {
  return new Map(
    nodes.map((node) => [
      node.id,
      {
        x: node.position.x,
        y: node.position.y,
        pinned: node.data.pinned,
      },
    ]),
  );
}

function buildLayoutInputs(scenario: CanvasPerfScenario): {
  layoutNodes: AutoLayoutNode[];
  layoutEdges: AutoLayoutEdge[];
} {
  return {
    layoutNodes: scenario.devices.map((device) => {
      const position = scenario.positions.get(device.id);
      return {
        id: device.id,
        x: position?.x,
        y: position?.y,
        pinned: position?.pinned,
      };
    }),
    layoutEdges: scenario.links
      .filter((link) => link.source_device_id !== link.target_device_id)
      .map((link) => ({
        source: link.source_device_id,
        target: link.target_device_id,
      })),
  };
}

const benchmarkAreaColors = ['#2563eb', '#16a34a', '#dc2626', '#9333ea', '#d97706', '#0891b2'];

function buildAreaColorMap(devices: Device[]): Map<string, string> {
  const areaColorMap = new Map<string, string>();

  for (const device of devices) {
    for (const areaId of device.area_ids ?? []) {
      if (areaColorMap.has(areaId)) continue;
      areaColorMap.set(
        areaId,
        benchmarkAreaColors[areaColorMap.size % benchmarkAreaColors.length]!,
      );
    }
  }

  return areaColorMap;
}

function selectedDeviceIds(devices: Device[]): Set<string> {
  return new Set(devices[0] ? [devices[0].id] : []);
}

function buildIncrementalBenchmarkState(scenario: CanvasPerfScenario): {
  placementDeviceIds: Set<string>;
  effectivePositions: Map<string, { x: number; y: number; pinned?: boolean }>;
} {
  const placementDeviceIds = new Set<string>();
  const placementInterval = Math.max(5, Math.floor(scenario.devices.length / 20));

  scenario.devices.forEach((device, index) => {
    if (index > 0 && index % placementInterval === 0) {
      placementDeviceIds.add(device.id);
    }
  });

  if (placementDeviceIds.size === 0 && scenario.devices.length > 1) {
    placementDeviceIds.add(scenario.devices[1]!.id);
  }

  const effectivePositions = new Map<string, { x: number; y: number; pinned?: boolean }>();
  scenario.devices.forEach((device, index) => {
    if (placementDeviceIds.has(device.id)) return;

    effectivePositions.set(device.id, {
      x: 120 + (index % 30) * 180,
      y: 120 + Math.floor(index / 30) * 140,
      pinned: true,
    });
  });

  return { placementDeviceIds, effectivePositions };
}

function buildRuntimePatchSnapshot(scenario: CanvasPerfScenario): SnapshotPayload {
  const nextDevices = { ...scenario.runtimeSnapshot.devices };
  const firstRuntimeDevice = scenario.devices
    .map((device) => scenario.runtimeSnapshot.devices[device.id])
    .find((runtimeDevice) => runtimeDevice !== undefined);

  if (firstRuntimeDevice) {
    nextDevices[firstRuntimeDevice.device_id] = {
      ...firstRuntimeDevice,
      cpu_percent:
        typeof firstRuntimeDevice.cpu_percent === 'number'
          ? (firstRuntimeDevice.cpu_percent + 7) % 100
          : 7,
      health: firstRuntimeDevice.health === 'critical' ? 'warning' : 'critical',
    };
  }

  const nextLinks = { ...scenario.runtimeSnapshot.links };
  const firstRuntimeLink = scenario.links
    .map((link) => scenario.runtimeSnapshot.links[link.id])
    .find((runtimeLink) => runtimeLink !== undefined);

  if (firstRuntimeLink) {
    nextLinks[firstRuntimeLink.link_id] = {
      ...firstRuntimeLink,
      metrics_status: 'available',
      metrics_reason: 'ok',
      utilization:
        typeof firstRuntimeLink.utilization === 'number'
          ? Math.min(0.99, firstRuntimeLink.utilization + 0.08)
          : 0.42,
    };
  }

  return {
    devices: nextDevices,
    links: nextLinks,
  };
}

function benchmarkOperations(
  samples: CanvasMetricSample[],
  scenarioName: CanvasPerfScenarioName,
  scenario: CanvasPerfScenario,
): void {
  const runtimeState = buildRuntimeState({
    devices: scenario.devices,
    links: scenario.links,
    snapshot: scenario.runtimeSnapshot,
    alerts: scenario.alerts,
    prometheusStatus,
  });
  const runtimeDevices = scenario.devices.map(
    (device) => runtimeState.devicesById.get(device.id)?.device ?? device,
  );
  const devicesById = new Map(runtimeDevices.map((device) => [device.id, device]));
  const currentPositions = new Map(scenario.positions);
  const placementDeviceIds = new Set<string>();
  const explicitPositions =
    runtimeDevices[0] === undefined
      ? new Map<string, { x: number; y: number }>()
      : new Map([[runtimeDevices[0].id, { x: 120, y: 120 }]]);

  const nodes = measureLocalMetric(samples, scenarioName, 'buildTopologyNodes', () =>
    buildTopologyNodes(
      runtimeDevices,
      scenario.positions,
      new Map(),
      explicitPositions,
      false,
      noopDeviceMenu,
      scenario.runtimeSnapshot,
      scenario.alerts,
      scenario.links,
      undefined,
      currentPositions,
      placementDeviceIds,
    ),
  );

  const edges = measureLocalMetric(samples, scenarioName, 'buildTopologyEdges', () =>
    buildTopologyEdges(
      scenario.links,
      devicesById,
      nodes,
      undefined,
      noopEdgeMenu,
      scenario.alerts,
    ),
  );
  const nodeIndexById = new Map(nodes.map((node, index) => [node.id, index]));
  const edgeIndexById = new Map(edges.map((edge, index) => [edge.id, index]));

  const compositionInput = {
    devices: scenario.devices,
    links: scenario.links,
    runtimeState,
    savedPositions: scenario.positions,
    computedPositions: new Map<string, { x: number; y: number }>(),
    currentPositions: buildCurrentPositions(nodes),
    explicitPositions,
    editMode: false,
    openDeviceMenu: noopDeviceMenu,
    openEdgeMenu: noopEdgeMenu,
    placementDeviceIds,
    alerts: scenario.alerts,
    snapGrid: null,
  };

  measureLocalMetric(samples, scenarioName, 'composeCanvasTopology', () =>
    composeCanvasTopology(compositionInput),
  );

  const compositionCache = createCanvasTopologyCompositionCache();
  const topologySignature = buildTopologyIdentity(scenario.devices, scenario.links).signature;
  const buildCompositionCacheKeyInput = (): BuildCanvasTopologyCompositionCacheKeyInput => ({
    mapKey: `benchmark:${scenarioName}`,
    topologySignature,
    topologyVersion: `benchmark-topology:${scenarioName}`,
    topologyEtag: `"benchmark-${scenarioName}"`,
    schemaVersion: 1,
    devices: scenario.devices,
    links: scenario.links,
    savedPositions: scenario.positions,
    computedPositions: compositionInput.computedPositions,
    currentPositions: compositionInput.currentPositions,
    explicitPositions: compositionInput.explicitPositions,
    editMode: compositionInput.editMode,
    snapGrid: compositionInput.snapGrid,
    placementDeviceIds,
    runtimeIdentity: `benchmark:${scenarioName}`,
    runtimeVersion: 1,
    runtimeSnapshot: scenario.runtimeSnapshot,
    alerts: scenario.alerts,
    prometheusStatus,
    openDeviceMenu: noopDeviceMenu,
    openEdgeMenu: noopEdgeMenu,
  });
  const buildCompositionCacheKey = () =>
    buildCanvasTopologyCompositionCacheKey(buildCompositionCacheKeyInput());
  const buildLegacyCompositionCacheSignature = () =>
    buildLegacyStructuralCompositionCacheSignature(buildCompositionCacheKeyInput());
  const compositionCacheKey = buildCompositionCacheKey();
  compositionCache.compose(compositionInput, compositionCacheKey);
  measureLocalMetric(samples, scenarioName, 'buildCanvasTopologyCompositionCacheKey', () =>
    buildCompositionCacheKey(),
  );
  measureLocalMetric(samples, scenarioName, 'buildCanvasTopologyCompositionCacheKeyLegacy', () =>
    buildLegacyCompositionCacheSignature(),
  );
  measureLocalMetric(samples, scenarioName, 'composeCanvasTopologyCached', () =>
    compositionCache.compose(compositionInput, compositionCacheKey),
  );

  measureLocalMetric(samples, scenarioName, 'areaProjection', () =>
    projectAreaTopology({
      devices: scenario.devices,
      links: scenario.links,
      selectedAreaId: scenario.selectedAreaId,
    }),
  );

  const globalProjectionInput = projectAreaTopology({
    devices: scenario.devices,
    links: scenario.links,
    selectedAreaId: null,
  });
  const selectedAreaProjectionInput = projectAreaTopology({
    devices: scenario.devices,
    links: scenario.links,
    selectedAreaId: scenario.selectedAreaId,
  });
  const areaColorMap = buildAreaColorMap(scenario.devices);
  measureLocalMetric(samples, scenarioName, 'renderProjection', () => {
    const globalProjection = projectCanvasRenderGraph({
      nodes,
      edges,
      devices: scenario.devices,
      filteredDevices: globalProjectionInput.filteredDevices,
      filteredLinks: globalProjectionInput.filteredLinks,
      ghostDevices: globalProjectionInput.ghostDevices,
      runtimeState,
      areaColorMap,
      effectiveAreaId: null,
      selectedRealNodeIds: selectedDeviceIds(globalProjectionInput.filteredDevices),
      ghostMeasurements: new Map(),
      areaColorNodeCache: new Map(),
      onGhostClick: noopGhostClick,
    });

    return {
      global: globalProjection,
      selectedArea: projectCanvasRenderGraph({
        nodes,
        edges,
        devices: scenario.devices,
        filteredDevices: selectedAreaProjectionInput.filteredDevices,
        filteredLinks: selectedAreaProjectionInput.filteredLinks,
        ghostDevices: selectedAreaProjectionInput.ghostDevices,
        runtimeState,
        areaColorMap,
        effectiveAreaId: scenario.selectedAreaId,
        selectedRealNodeIds: selectedDeviceIds(selectedAreaProjectionInput.filteredDevices),
        ghostMeasurements: new Map(),
        areaColorNodeCache: globalProjection.areaColorNodeCache,
        onGhostClick: noopGhostClick,
      }),
    };
  });

  measureLocalMetric(samples, scenarioName, 'runtimePatch', () => {
    const nextSnapshot = buildRuntimePatchSnapshot(scenario);
    const nextRuntimeState = buildRuntimeState({
      devices: scenario.devices,
      links: scenario.links,
      snapshot: nextSnapshot,
      alerts: scenario.alerts,
      prometheusStatus,
    });
    const plan = buildRuntimePatchPlan({
      previousSnapshot: scenario.runtimeSnapshot,
      nextSnapshot,
      links: scenario.links,
    });

    return {
      devices: patchRuntimeDevices({
        devices: runtimeDevices,
        runtimeState: nextRuntimeState,
        plan,
      }),
      nodes: patchRuntimeNodes({ nodes, runtimeState: nextRuntimeState, plan, nodeIndexById }),
      edges: patchRuntimeEdges({
        edges,
        links: scenario.links,
        runtimeState: nextRuntimeState,
        alerts: scenario.alerts,
        onEdgeContextMenu: noopEdgeMenu,
        plan,
        edgeIndexById,
      }),
    };
  });

  measureLocalMetric(samples, scenarioName, 'incrementalLayout', () => {
    const { placementDeviceIds: incrementalPlacementDeviceIds, effectivePositions } =
      buildIncrementalBenchmarkState(scenario);
    const { layoutNodes, layoutEdges } = buildIncrementalLayoutInputs({
      devices: scenario.devices,
      links: scenario.links,
      placementDeviceIds: incrementalPlacementDeviceIds,
      effectivePositions,
    });

    return computeIncrementalLayoutPositions({
      layoutNodes,
      layoutEdges,
      placementDeviceIds: incrementalPlacementDeviceIds,
      width: 2400,
      height: 1600,
    });
  });

  const placementObstacles = scenario.devices.map((device) => {
    const position = scenario.positions.get(device.id) ?? { x: 0, y: 0 };
    return { x: position.x, y: position.y, width: 370, height: 140 };
  });
  measureLocalMetric(samples, scenarioName, 'newNodePlacement', () =>
    findNewNodePlacement({
      viewport: { x: 0, y: 0, width: 1440, height: 900 },
      nodeSize: { width: 370, height: 140 },
      obstacles: placementObstacles,
    }),
  );

  benchmarkEditableLinkGeometry(samples, scenarioName);

  const { layoutNodes, layoutEdges } = buildLayoutInputs(scenario);
  measureLocalMetric(samples, scenarioName, 'computeForceLayout', () =>
    computeForceLayout(layoutNodes, layoutEdges, 2400, 1600),
  );

  void edges;
}

function metricsForScenario(
  samples: CanvasMetricSample[],
  scenarioName: CanvasPerfScenarioName,
): Record<string, CanvasMetricAggregate> {
  const aggregateByKey = aggregateCanvasMetricSamples(samples);
  return Object.fromEntries(
    CANVAS_PERF_BENCHMARK_METRICS.map((metricName) => [
      metricName,
      aggregateByKey[`${scenarioName}:${metricName}`],
    ]),
  );
}

/** Runs canvas perf benchmark for the topology canvas. */
export function runCanvasPerfBenchmark(
  options: RunCanvasPerfBenchmarkOptions = {},
): CanvasPerfBenchmarkResult {
  const scenarioNames =
    options.scenarioNames ?? (Object.keys(CANVAS_PERF_SCENARIOS) as CanvasPerfScenarioName[]);
  const warmupIterations = options.warmupIterations ?? 3;
  let benchmarkIterations = 0;
  const iterationsByScenario: Partial<Record<CanvasPerfScenarioName, number>> = {};
  const scenarios = {} as Record<CanvasPerfScenarioName, CanvasPerfBenchmarkScenarioResult>;

  for (const scenarioName of scenarioNames) {
    const scenario = generateCanvasPerfScenario(scenarioName);
    const measuredSamples: CanvasMetricSample[] = [];
    const warmupSamples: CanvasMetricSample[] = [];
    const iterations = iterationsForScenario(options, scenarioName);
    iterationsByScenario[scenarioName] = iterations;
    benchmarkIterations = Math.max(benchmarkIterations, iterations);

    for (let index = 0; index < warmupIterations; index += 1) {
      benchmarkOperations(warmupSamples, scenarioName, scenario);
    }

    for (let index = 0; index < iterations; index += 1) {
      benchmarkOperations(measuredSamples, scenarioName, scenario);
    }

    scenarios[scenarioName] = {
      input: CANVAS_PERF_SCENARIOS[scenarioName],
      metrics: metricsForScenario(measuredSamples, scenarioName),
    };
  }

  return {
    version: 1,
    generatedAt: new Date().toISOString(),
    iterations: benchmarkIterations,
    iterationsByScenario,
    scenarios,
  };
}
