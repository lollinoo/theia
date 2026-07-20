/**
 * Exercises canvas perf benchmark topology canvas behavior so refactors preserve the documented contract.
 */
import { afterEach, describe, expect, it, vi } from 'vitest';

import { CANVAS_PERF_BENCHMARK_METRICS, runCanvasPerfBenchmark } from './canvasPerfBenchmark';
import { CANVAS_PERF_SCENARIOS, type CanvasPerfScenarioName } from './canvasPerfScenarios';

describe('canvasPerfBenchmark', () => {
  afterEach(() => {
    vi.doUnmock('./runtimePatches');
  });

  it('tracks the incremental layout path separately from full force layout', () => {
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('computeForceLayout');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('incrementalLayout');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('runtimePatch');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('buildCanvasTopologyCompositionCacheKey');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('buildCanvasTopologyCompositionCacheKeyLegacy');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('composeCanvasTopologyCached');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('renderProjection');
    expect(CANVAS_PERF_BENCHMARK_METRICS).toContain('newNodePlacement');

    const result = runCanvasPerfBenchmark({
      iterations: 1,
      warmupIterations: 0,
      scenarioNames: ['small'],
    });

    expect(result.scenarios.small.metrics.incrementalLayout.count).toBe(1);
    expect(result.scenarios.small.metrics.runtimePatch.count).toBe(1);
    expect(result.scenarios.small.metrics.buildCanvasTopologyCompositionCacheKey.count).toBe(1);
    expect(result.scenarios.small.metrics.buildCanvasTopologyCompositionCacheKeyLegacy.count).toBe(
      1,
    );
    expect(result.scenarios.small.metrics.composeCanvasTopologyCached.count).toBe(1);
    expect(result.scenarios.small.metrics.renderProjection.count).toBe(1);
    expect(result.scenarios.small.metrics.newNodePlacement.count).toBe(1);
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
    expect(result.iterationsByScenario).toEqual({
      small: 1,
      medium: 1,
      large: 1,
      stress: 1,
    });
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
  }, 60_000);

  it('reports effective sample counts when scenario defaults or overrides are used', () => {
    const defaultResult = runCanvasPerfBenchmark({
      warmupIterations: 0,
      scenarioNames: ['small'],
    });

    expect(defaultResult.iterations).toBe(50);
    expect(defaultResult.iterationsByScenario).toEqual({ small: 50 });
    expect(defaultResult.scenarios.small.metrics.computeForceLayout.count).toBe(50);

    const mixedResult = runCanvasPerfBenchmark({
      iterations: 2,
      warmupIterations: 0,
      iterationsByScenario: {
        small: 1,
      },
      scenarioNames: ['small', 'medium'],
    });

    expect(mixedResult.iterations).toBe(2);
    expect(mixedResult.iterationsByScenario).toEqual({ small: 1, medium: 2 });
    expect(mixedResult.scenarios.small.metrics.computeForceLayout.count).toBe(1);
    expect(mixedResult.scenarios.medium.metrics.computeForceLayout.count).toBe(2);
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

  it('measures runtime patching through indexed node and edge paths', async () => {
    vi.resetModules();
    const nodePatchCalls: unknown[] = [];
    const edgePatchCalls: unknown[] = [];

    vi.doMock('./runtimePatches', async (importOriginal) => {
      const actual = await importOriginal<typeof import('./runtimePatches')>();
      return {
        ...actual,
        patchRuntimeNodes: (input: Parameters<typeof actual.patchRuntimeNodes>[0]) => {
          nodePatchCalls.push(input);
          return actual.patchRuntimeNodes(input);
        },
        patchRuntimeEdges: (input: Parameters<typeof actual.patchRuntimeEdges>[0]) => {
          edgePatchCalls.push(input);
          return actual.patchRuntimeEdges(input);
        },
      };
    });

    const { runCanvasPerfBenchmark: runBenchmarkWithMock } = await import('./canvasPerfBenchmark');

    runBenchmarkWithMock({
      iterations: 1,
      warmupIterations: 0,
      scenarioNames: ['small'],
    });

    expect(nodePatchCalls).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          nodeIndexById: expect.any(Map),
        }),
      ]),
    );
    expect(edgePatchCalls).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          edgeIndexById: expect.any(Map),
        }),
      ]),
    );
  });
});
