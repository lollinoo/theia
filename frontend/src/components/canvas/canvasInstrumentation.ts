/**
 * Defines canvas instrumentation behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
export type CanvasMetricName =
  | 'topology-load'
  | 'layout'
  | 'snapshot-apply'
  | 'buildTopologyNodes'
  | 'buildTopologyEdges'
  | 'buildCanvasTopologyCompositionCacheKey'
  | 'buildCanvasTopologyCompositionCacheKeyLegacy'
  | 'composeCanvasTopology'
  | 'composeCanvasTopologyCached'
  | 'areaProjection'
  | 'renderProjection'
  | 'runtimePatch'
  | 'incrementalLayout'
  | 'newNodePlacement'
  | 'computeForceLayout'
  | 'deviceCardRender'
  | 'frameTime'
  | 'frameOverBudget16'
  | 'frameOverBudget33'
  | 'frameOverBudget50'
  | 'longTask';

/** Describes the canvas perf scenario name contract used by the topology canvas. */
export type CanvasPerfScenarioName = 'runtime' | 'small' | 'medium' | 'large' | 'stress';

/** Describes the canvas measurement name contract used by the topology canvas. */
export type CanvasMeasurementName =
  | 'theia:canvas:topology-load'
  | 'theia:canvas:layout'
  | 'theia:canvas:snapshot-apply';

/** Describes the canvas measurement trigger contract used by the topology canvas. */
export type CanvasMeasurementTrigger =
  | 'initial_load'
  | 'backend_reconnected'
  | 'topology_changed'
  | 'snapshot'
  | 'manual_refresh';

/** Describes the canvas recorded metric name contract used by the topology canvas. */
export type CanvasRecordedMetricName = CanvasMetricName | CanvasMeasurementName;

/** Describes the canvas metric sample contract used by the topology canvas. */
export interface CanvasMetricSample {
  name: CanvasRecordedMetricName;
  scenario: CanvasPerfScenarioName;
  durationMs: number;
  timestamp: number;
  metadata?: Record<string, unknown>;
  trigger?: CanvasMeasurementTrigger;
  recordedAt?: string;
}

/** Describes the canvas measurement record contract used by the topology canvas. */
export interface CanvasMeasurementRecord extends CanvasMetricSample {
  name: CanvasMeasurementName;
  scenario: 'runtime';
  trigger: CanvasMeasurementTrigger;
  recordedAt: string;
}

/** Describes the canvas metric aggregate contract used by the topology canvas. */
export interface CanvasMetricAggregate {
  count: number;
  minMs: number;
  maxMs: number;
  avgMs: number;
  p95Ms: number;
}

/** Describes the canvas metrics export contract used by the topology canvas. */
export interface CanvasMetricsExport {
  version: 1;
  generatedAt: string;
  samples: CanvasMetricSample[];
  aggregates: Record<string, CanvasMetricAggregate>;
}

const maxCanvasMetricSamples = 1000;
const maxCanvasRenderMetricDurations = 1000;
let nodeCanvasMetrics: CanvasMetricSample[] = [];
let canvasRenderMetricsEnabled = false;
let canvasRenderMetricSequence = 0;
const canvasRuntimeMetricAggregates = new Map<
  string,
  { count: number; minMs: number; maxMs: number; totalMs: number; durations: number[] }
>();

/** Describes the canvas render metric measurement contract used by the topology canvas. */
export interface CanvasRenderMetricMeasurement {
  component: string;
  startedAt: number;
  markPrefix: string;
}

function metricSamples(): CanvasMetricSample[] {
  if (typeof window === 'undefined') {
    return nodeCanvasMetrics;
  }

  return window.__THEIA_CANVAS_METRICS__ ?? [];
}

function setMetricSamples(samples: CanvasMetricSample[]): void {
  if (typeof window === 'undefined') {
    nodeCanvasMetrics = samples;
    return;
  }

  window.__THEIA_CANVAS_METRICS__ = samples;
  installCanvasMetricWindowHelpers();
}

function installCanvasMetricWindowHelpers(): void {
  if (typeof window === 'undefined') {
    return;
  }

  window.__THEIA_CANVAS_METRICS_EXPORT__ = exportCanvasMetrics;
  window.__THEIA_CANVAS_METRICS_CLEAR__ = clearCanvasMetrics;
  window.__THEIA_CANVAS_RENDER_METRICS_ENABLE__ = () => setCanvasRenderMetricsEnabled(true);
  window.__THEIA_CANVAS_RENDER_METRICS_DISABLE__ = () => setCanvasRenderMetricsEnabled(false);
}

function normalizeMetricName(name: CanvasRecordedMetricName): CanvasMetricName {
  if (name === 'theia:canvas:topology-load') return 'topology-load';
  if (name === 'theia:canvas:layout') return 'layout';
  if (name === 'theia:canvas:snapshot-apply') return 'snapshot-apply';
  return name;
}

function nowMs(): number {
  return typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : Date.now();
}

function roundMetric(value: number): number {
  return Number(value.toFixed(3));
}

/** Records canvas metric for the topology canvas. */
export function recordCanvasMetric(sample: CanvasMetricSample): void {
  const nextBuffer = [...metricSamples(), sample];
  if (nextBuffer.length > maxCanvasMetricSamples) {
    nextBuffer.splice(0, nextBuffer.length - maxCanvasMetricSamples);
  }
  setMetricSamples(nextBuffer);
}

/** Aggregates canvas metric samples for the topology canvas. */
export function aggregateCanvasMetricSamples(
  samples: CanvasMetricSample[],
): Record<string, CanvasMetricAggregate> {
  const durationsByKey = new Map<string, number[]>();

  for (const sample of samples) {
    const key = `${sample.scenario}:${normalizeMetricName(sample.name)}`;
    const durations = durationsByKey.get(key) ?? [];
    durations.push(sample.durationMs);
    durationsByKey.set(key, durations);
  }

  const aggregates: Record<string, CanvasMetricAggregate> = {};
  for (const [key, durations] of durationsByKey.entries()) {
    const sorted = [...durations].sort((a, b) => a - b);
    const sum = sorted.reduce((total, value) => total + value, 0);
    const p95Index = Math.max(0, Math.ceil(0.95 * sorted.length) - 1);

    aggregates[key] = {
      count: sorted.length,
      minMs: roundMetric(sorted[0] ?? 0),
      maxMs: roundMetric(sorted[sorted.length - 1] ?? 0),
      avgMs: roundMetric(sum / sorted.length),
      p95Ms: roundMetric(sorted[p95Index] ?? 0),
    };
  }

  return aggregates;
}

/** Exports canvas metrics for the topology canvas. */
export function exportCanvasMetrics(): CanvasMetricsExport {
  const samples = [...metricSamples()];

  return {
    version: 1,
    generatedAt: new Date().toISOString(),
    samples,
    aggregates: {
      ...aggregateCanvasMetricSamples(samples),
      ...aggregateCanvasRuntimeMetricSamples(),
    },
  };
}

/** Clears canvas metrics for the topology canvas. */
export function clearCanvasMetrics(): void {
  setMetricSamples([]);
  canvasRuntimeMetricAggregates.clear();
}

/** Sets canvas render metrics enabled for the topology canvas. */
export function setCanvasRenderMetricsEnabled(enabled: boolean): void {
  canvasRenderMetricsEnabled = enabled;
  installCanvasMetricWindowHelpers();
}

/** Identifies canvas render metrics enabled for the topology canvas. */
export function isCanvasRenderMetricsEnabled(): boolean {
  return canvasRenderMetricsEnabled;
}

function safeMark(markName: string): void {
  if (typeof performance === 'undefined' || typeof performance.mark !== 'function') {
    return;
  }
  performance.mark(markName);
}

function safeMeasure(measureName: string, startMark: string, endMark: string): void {
  if (
    typeof performance === 'undefined' ||
    typeof performance.measure !== 'function' ||
    typeof performance.clearMarks !== 'function' ||
    typeof performance.clearMeasures !== 'function'
  ) {
    return;
  }

  performance.measure(measureName, startMark, endMark);
  performance.clearMarks(startMark);
  performance.clearMarks(endMark);
  performance.clearMeasures(measureName);
}

function finalizeMeasurement(
  name: CanvasMeasurementName,
  trigger: CanvasMeasurementTrigger,
  startedAt: number,
  markPrefix: string,
): void {
  const endedAt = nowMs();
  const durationMs = Math.max(0, roundMetric(endedAt - startedAt));
  const endMark = `${markPrefix}:end`;
  const recordedAt = new Date().toISOString();

  safeMark(endMark);
  safeMeasure(markPrefix, `${markPrefix}:start`, endMark);
  recordCanvasMetric({
    name,
    scenario: 'runtime',
    trigger,
    durationMs,
    timestamp: Date.now(),
    recordedAt,
    metadata: { trigger },
  });
}

/** Measures canvas metric for the topology canvas. */
export function measureCanvasMetric<T>(
  name: CanvasMetricName,
  scenario: CanvasPerfScenarioName,
  work: () => T,
  metadata?: Record<string, unknown>,
): T {
  const startedAt = nowMs();
  const markPrefix = `theia:canvas:${scenario}:${name}:${Date.now()}`;
  safeMark(`${markPrefix}:start`);

  try {
    return work();
  } finally {
    const endedAt = nowMs();
    const durationMs = Math.max(0, roundMetric(endedAt - startedAt));
    const endMark = `${markPrefix}:end`;
    safeMark(endMark);
    safeMeasure(markPrefix, `${markPrefix}:start`, endMark);
    recordCanvasMetric({
      name,
      scenario,
      durationMs,
      timestamp: Date.now(),
      metadata,
    });
  }
}

/** Starts canvas render metric for the topology canvas. */
export function startCanvasRenderMetric(component: string): CanvasRenderMetricMeasurement | null {
  if (!isCanvasRenderMetricsEnabled()) {
    return null;
  }

  const startedAt = nowMs();
  canvasRenderMetricSequence += 1;
  const markPrefix = `theia:canvas:render:${component}:${Date.now()}:${canvasRenderMetricSequence}`;
  safeMark(`${markPrefix}:start`);
  return { component, startedAt, markPrefix };
}

/** Finishes canvas render metric for the topology canvas. */
export function finishCanvasRenderMetric(
  measurement: CanvasRenderMetricMeasurement | null,
  metadata: Record<string, unknown> = {},
): void {
  if (measurement === null) {
    return;
  }

  const endedAt = nowMs();
  const durationMs = Math.max(0, roundMetric(endedAt - measurement.startedAt));
  const endMark = `${measurement.markPrefix}:end`;
  safeMark(endMark);
  safeMeasure(measurement.markPrefix, `${measurement.markPrefix}:start`, endMark);
  recordCanvasComponentRenderMetric(measurement.component, durationMs, metadata);
}

/** Records canvas component render metric for the topology canvas. */
export function recordCanvasComponentRenderMetric(
  component: string,
  durationMs: number,
  metadata: Record<string, unknown> = {},
): void {
  if (!isCanvasRenderMetricsEnabled()) {
    return;
  }

  recordCanvasAggregateMetric({
    name: 'deviceCardRender',
    scenario: 'runtime',
    durationMs,
    timestamp: Date.now(),
    metadata: {
      component,
      ...metadata,
    },
  });
}

function recordCanvasAggregateMetric(sample: CanvasMetricSample): void {
  const key = `${sample.scenario}:${normalizeMetricName(sample.name)}`;
  const aggregate = canvasRuntimeMetricAggregates.get(key) ?? {
    count: 0,
    minMs: sample.durationMs,
    maxMs: sample.durationMs,
    totalMs: 0,
    durations: [],
  };

  aggregate.count += 1;
  aggregate.minMs = Math.min(aggregate.minMs, sample.durationMs);
  aggregate.maxMs = Math.max(aggregate.maxMs, sample.durationMs);
  aggregate.totalMs += sample.durationMs;
  aggregate.durations.push(sample.durationMs);

  if (aggregate.durations.length > maxCanvasRenderMetricDurations) {
    aggregate.durations.splice(0, aggregate.durations.length - maxCanvasRenderMetricDurations);
  }

  canvasRuntimeMetricAggregates.set(key, aggregate);
}

function aggregateCanvasRuntimeMetricSamples(): Record<string, CanvasMetricAggregate> {
  const aggregates: Record<string, CanvasMetricAggregate> = {};

  for (const [key, aggregate] of canvasRuntimeMetricAggregates.entries()) {
    const sorted = [...aggregate.durations].sort((a, b) => a - b);
    const p95Index = Math.max(0, Math.ceil(0.95 * sorted.length) - 1);
    aggregates[key] = {
      count: aggregate.count,
      minMs: roundMetric(aggregate.minMs),
      maxMs: roundMetric(aggregate.maxMs),
      avgMs: roundMetric(aggregate.totalMs / aggregate.count),
      p95Ms: roundMetric(sorted[p95Index] ?? 0),
    };
  }

  return aggregates;
}

/** Records canvas frame time for the topology canvas. */
export function recordCanvasFrameTime(
  durationMs: number,
  metadata: Record<string, unknown> = {},
): void {
  const roundedDurationMs = Math.max(0, roundMetric(durationMs));
  const timestamp = Date.now();

  recordCanvasAggregateMetric({
    name: 'frameTime',
    scenario: 'runtime',
    durationMs: roundedDurationMs,
    timestamp,
    metadata,
  });

  if (roundedDurationMs > 16.667) {
    recordCanvasAggregateMetric({
      name: 'frameOverBudget16',
      scenario: 'runtime',
      durationMs: roundedDurationMs,
      timestamp,
      metadata: { ...metadata, thresholdMs: 16.667 },
    });
  }

  if (roundedDurationMs > 33.333) {
    recordCanvasAggregateMetric({
      name: 'frameOverBudget33',
      scenario: 'runtime',
      durationMs: roundedDurationMs,
      timestamp,
      metadata: { ...metadata, thresholdMs: 33.333 },
    });
  }

  if (roundedDurationMs > 50) {
    recordCanvasAggregateMetric({
      name: 'frameOverBudget50',
      scenario: 'runtime',
      durationMs: roundedDurationMs,
      timestamp,
      metadata: { ...metadata, thresholdMs: 50 },
    });
  }
}

/** Records canvas long task for the topology canvas. */
export function recordCanvasLongTask(
  durationMs: number,
  metadata: Record<string, unknown> = {},
): void {
  recordCanvasAggregateMetric({
    name: 'longTask',
    scenario: 'runtime',
    durationMs: Math.max(0, roundMetric(durationMs)),
    timestamp: Date.now(),
    metadata,
  });
}

/** Measures canvas work for the topology canvas. */
export function measureCanvasWork<T>(
  name: CanvasMeasurementName,
  trigger: CanvasMeasurementTrigger,
  work: () => T,
): T {
  const startedAt = nowMs();
  const markPrefix = `${name}:${trigger}:${Date.now()}`;
  safeMark(`${markPrefix}:start`);

  try {
    return work();
  } finally {
    finalizeMeasurement(name, trigger, startedAt, markPrefix);
  }
}

/** Measures canvas async work for the topology canvas. */
export async function measureCanvasAsyncWork<T>(
  name: CanvasMeasurementName,
  trigger: CanvasMeasurementTrigger,
  work: () => Promise<T>,
): Promise<T> {
  const startedAt = nowMs();
  const markPrefix = `${name}:${trigger}:${Date.now()}`;
  safeMark(`${markPrefix}:start`);

  try {
    return await work();
  } finally {
    finalizeMeasurement(name, trigger, startedAt, markPrefix);
  }
}

installCanvasMetricWindowHelpers();
