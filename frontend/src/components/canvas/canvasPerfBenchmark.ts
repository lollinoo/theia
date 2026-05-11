import type { AutoLayoutEdge, AutoLayoutNode } from '../../hooks/useAutoLayout';
import { computeForceLayout } from '../../hooks/useAutoLayout';
import type { PrometheusStatusPayload, SnapshotPayload } from '../../types/metrics';
import type { DeviceNode } from '../DeviceCard';
import { projectAreaTopology } from './areaProjection';
import {
  type CanvasMetricAggregate,
  type CanvasMetricName,
  type CanvasMetricSample,
  aggregateCanvasMetricSamples,
} from './canvasInstrumentation';
import {
  CANVAS_PERF_SCENARIOS,
  type CanvasPerfScenario,
  type CanvasPerfScenarioName,
  generateCanvasPerfScenario,
} from './canvasPerfScenarios';
import { buildTopologyEdges } from './edgeBuilder';
import {
  buildIncrementalLayoutInputs,
  computeIncrementalLayoutPositions,
} from './incrementalLayout';
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
  buildCanvasTopologyCompositionCacheKey,
  createCanvasTopologyCompositionCache,
} from './topologyCompositionCache';
import { buildTopologyIdentity } from './topologyIdentity';

export const CANVAS_PERF_BENCHMARK_METRICS = [
  'buildTopologyNodes',
  'buildTopologyEdges',
  'composeCanvasTopology',
  'composeCanvasTopologyCached',
  'areaProjection',
  'runtimePatch',
  'incrementalLayout',
  'computeForceLayout',
] as const satisfies CanvasMetricName[];

export interface CanvasPerfBenchmarkScenarioResult {
  input: {
    deviceCount: number;
    linkCount: number;
  };
  metrics: Record<string, CanvasMetricAggregate>;
}

export interface CanvasPerfBenchmarkResult {
  version: 1;
  generatedAt: string;
  iterations: number;
  scenarios: Record<CanvasPerfScenarioName, CanvasPerfBenchmarkScenarioResult>;
}

export interface RunCanvasPerfBenchmarkOptions {
  iterations?: number;
  warmupIterations?: number;
  iterationsByScenario?: Partial<Record<CanvasPerfScenarioName, number>>;
  scenarioNames?: CanvasPerfScenarioName[];
}

const defaultIterationsByScenario: Record<CanvasPerfScenarioName, number> = {
  small: 50,
  medium: 30,
  large: 15,
  stress: 5,
};

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

  const nodes = measureLocalMetric(samples, scenarioName, 'buildTopologyNodes', () =>
    buildTopologyNodes(
      runtimeDevices,
      scenario.positions,
      new Map(),
      { x: 120, y: 120 },
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

  const compositionInput = {
    devices: scenario.devices,
    links: scenario.links,
    runtimeState,
    savedPositions: scenario.positions,
    computedPositions: new Map<string, { x: number; y: number }>(),
    currentPositions: buildCurrentPositions(nodes),
    defaultPosition: { x: 120, y: 120 },
    editMode: false,
    openDeviceMenu: noopDeviceMenu,
    openEdgeMenu: noopEdgeMenu,
    placementDeviceIds,
    alerts: scenario.alerts,
  };

  measureLocalMetric(samples, scenarioName, 'composeCanvasTopology', () =>
    composeCanvasTopology(compositionInput),
  );

  const compositionCache = createCanvasTopologyCompositionCache();
  const compositionCacheKey = buildCanvasTopologyCompositionCacheKey({
    mapKey: `benchmark:${scenarioName}`,
    topologySignature: buildTopologyIdentity(scenario.devices, scenario.links).signature,
    schemaVersion: 1,
    devices: scenario.devices,
    links: scenario.links,
    savedPositions: scenario.positions,
    computedPositions: compositionInput.computedPositions,
    currentPositions: compositionInput.currentPositions,
    defaultPosition: compositionInput.defaultPosition,
    editMode: compositionInput.editMode,
    placementDeviceIds,
    runtimeIdentity: `benchmark:${scenarioName}`,
    runtimeSnapshot: scenario.runtimeSnapshot,
    alerts: scenario.alerts,
    prometheusStatus,
    openDeviceMenu: noopDeviceMenu,
    openEdgeMenu: noopEdgeMenu,
  });
  compositionCache.compose(compositionInput, compositionCacheKey);
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
      nodes: patchRuntimeNodes({ nodes, runtimeState: nextRuntimeState, plan }),
      edges: patchRuntimeEdges({
        edges,
        links: scenario.links,
        runtimeState: nextRuntimeState,
        alerts: scenario.alerts,
        onEdgeContextMenu: noopEdgeMenu,
        plan,
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

export function runCanvasPerfBenchmark(
  options: RunCanvasPerfBenchmarkOptions = {},
): CanvasPerfBenchmarkResult {
  const scenarioNames =
    options.scenarioNames ?? (Object.keys(CANVAS_PERF_SCENARIOS) as CanvasPerfScenarioName[]);
  const warmupIterations = options.warmupIterations ?? 3;
  const benchmarkIterations = options.iterations ?? 20;
  const scenarios = {} as Record<CanvasPerfScenarioName, CanvasPerfBenchmarkScenarioResult>;

  for (const scenarioName of scenarioNames) {
    const scenario = generateCanvasPerfScenario(scenarioName);
    const measuredSamples: CanvasMetricSample[] = [];
    const warmupSamples: CanvasMetricSample[] = [];
    const iterations =
      options.iterationsByScenario?.[scenarioName] ??
      options.iterations ??
      defaultIterationsByScenario[scenarioName];

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
    scenarios,
  };
}
