import { describe, expect, it } from 'vitest';

import { CANVAS_PERF_BENCHMARK_METRICS, runCanvasPerfBenchmark } from './canvasPerfBenchmark';
import { CANVAS_PERF_SCENARIOS, type CanvasPerfScenarioName } from './canvasPerfScenarios';

describe('canvasPerfBenchmark', () => {
  it('tracks the incremental layout path separately from full force layout', () => {
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('computeForceLayout');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('incrementalLayout');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('runtimePatch');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('composeCanvasTopologyCached');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('renderProjection');

    const result = runCanvasPerfBenchmark({
      iterations: 1,
      warmupIterations: 0,
      scenarioNames: ['small'],
    });

    expect(result.scenarios.small.metrics.incrementalLayout.count).toBe(1);
    expect(result.scenarios.small.metrics.runtimePatch.count).toBe(1);
    expect(result.scenarios.small.metrics.composeCanvasTopologyCached.count).toBe(1);
    expect(result.scenarios.small.metrics.renderProjection.count).toBe(1);
  });

  it('produces aggregate metrics for every official scenario and benchmarked function', () => {
    const result = runCanvasPerfBenchmark({
      iterations: 1,
      warmupIterations: 0,
      iterationsByScenario: {
        small: 1,
        medium: 1,
        large: 1,
        stress: 1,
      },
    });

    expect(result.version).toBe(1);
    expect(result.iterations).toBe(1);
    expect(Object.keys(result.scenarios)).toEqual(Object.keys(CANVAS_PERF_SCENARIOS));

    for (const scenarioName of Object.keys(CANVAS_PERF_SCENARIOS) as CanvasPerfScenarioName[]) {
      const scenario = result.scenarios[scenarioName];
      expect(scenario.input).toEqual(CANVAS_PERF_SCENARIOS[scenarioName]);

      for (const metricName of CANVAS_PERF_BENCHMARK_METRICS) {
        const aggregate = scenario.metrics[metricName];
        expect(aggregate).toBeDefined();
        expect(aggregate.count).toBeGreaterThan(0);
        expect(aggregate.minMs).toBeLessThanOrEqual(aggregate.avgMs);
        expect(aggregate.avgMs).toBeLessThanOrEqual(aggregate.maxMs);
        expect(aggregate.p95Ms).toBeLessThanOrEqual(aggregate.maxMs);
      }
    }
  }, 15_000);

  it('returns a JSON-serializable contract without timing thresholds', () => {
    const result = runCanvasPerfBenchmark({
      iterations: 1,
      warmupIterations: 0,
      scenarioNames: ['small'],
    });

    expect(() => JSON.stringify(result)).not.toThrow();
    expect(result.scenarios.small.metrics.composeCanvasTopology.count).toBe(1);
  });
});
