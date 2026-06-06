/**
 * Exercises canvas instrumentation topology canvas behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  clearCanvasMetrics,
  exportCanvasMetrics,
  finishCanvasRenderMetric,
  measureCanvasMetric,
  recordCanvasFrameTime,
  recordCanvasLongTask,
  recordCanvasMetric,
  setCanvasRenderMetricsEnabled,
  startCanvasRenderMetric,
} from './canvasInstrumentation';

describe('canvasInstrumentation', () => {
  beforeEach(() => {
    clearCanvasMetrics();
    setCanvasRenderMetricsEnabled(false);
  });

  it('records raw samples in the browser buffer', () => {
    recordCanvasMetric({
      name: 'composeCanvasTopology',
      scenario: 'small',
      durationMs: 12,
      timestamp: 1000,
      metadata: { deviceCount: 25 },
    });

    expect(window.__THEIA_CANVAS_METRICS__).toEqual([
      expect.objectContaining({
        name: 'composeCanvasTopology',
        scenario: 'small',
        durationMs: 12,
        timestamp: 1000,
        metadata: { deviceCount: 25 },
      }),
    ]);
  });

  it('keeps only the newest 1000 raw samples', () => {
    for (let index = 0; index < 1005; index += 1) {
      recordCanvasMetric({
        name: 'buildTopologyNodes',
        scenario: 'small',
        durationMs: index,
        timestamp: index,
      });
    }

    expect(window.__THEIA_CANVAS_METRICS__).toHaveLength(1000);
    expect(window.__THEIA_CANVAS_METRICS__?.[0]).toMatchObject({
      durationMs: 5,
      timestamp: 5,
    });
  });

  it('exports nearest-rank p95 aggregates by scenario and metric name', () => {
    [1, 2, 3, 4, 100].forEach((durationMs, index) => {
      recordCanvasMetric({
        name: 'composeCanvasTopology',
        scenario: 'medium',
        durationMs,
        timestamp: index,
      });
    });

    const exported = exportCanvasMetrics();

    expect(exported.version).toBe(1);
    expect(exported.aggregates['medium:composeCanvasTopology']).toEqual({
      count: 5,
      minMs: 1,
      maxMs: 100,
      avgMs: 22,
      p95Ms: 100,
    });
  });

  it('measures synchronous work and installs browser export helpers', () => {
    vi.spyOn(performance, 'now').mockReturnValueOnce(10).mockReturnValueOnce(16.25);

    const result = measureCanvasMetric('buildTopologyEdges', 'large', () => 'done', {
      linkCount: 600,
    });

    expect(result).toBe('done');
    expect(window.__THEIA_CANVAS_METRICS__).toEqual([
      expect.objectContaining({
        name: 'buildTopologyEdges',
        scenario: 'large',
        durationMs: 6.25,
        metadata: { linkCount: 600 },
      }),
    ]);
    expect(window.__THEIA_CANVAS_METRICS_EXPORT__?.()).toMatchObject({
      version: 1,
      aggregates: {
        'large:buildTopologyEdges': expect.objectContaining({ count: 1 }),
      },
    });
  });

  it('clears samples and returns a JSON-stringify-safe export', () => {
    recordCanvasMetric({
      name: 'areaProjection',
      scenario: 'small',
      durationMs: 3,
      timestamp: 1,
    });

    clearCanvasMetrics();
    const exported = exportCanvasMetrics();

    expect(exported.samples).toEqual([]);
    expect(exported.aggregates).toEqual({});
    expect(() => JSON.stringify(exported)).not.toThrow();
  });

  it('records device card render samples only when render metrics are enabled', () => {
    expect(startCanvasRenderMetric('DeviceCard')).toBeNull();
    expect(exportCanvasMetrics().samples).toEqual([]);

    setCanvasRenderMetricsEnabled(true);
    vi.spyOn(performance, 'now').mockReturnValueOnce(21).mockReturnValueOnce(25.5);

    const measurement = startCanvasRenderMetric('DeviceCard');
    finishCanvasRenderMetric(measurement, { deviceId: 'dev-1', kind: 'device' });

    expect(exportCanvasMetrics().samples).toEqual([]);
    expect(exportCanvasMetrics().aggregates['runtime:deviceCardRender']).toEqual({
      count: 1,
      minMs: 4.5,
      maxMs: 4.5,
      avgMs: 4.5,
      p95Ms: 4.5,
    });
  });

  it('clears aggregate render metrics together with raw samples', () => {
    setCanvasRenderMetricsEnabled(true);
    vi.spyOn(performance, 'now').mockReturnValueOnce(1).mockReturnValueOnce(3);

    finishCanvasRenderMetric(startCanvasRenderMetric('DeviceCard'), { deviceId: 'dev-1' });
    expect(exportCanvasMetrics().aggregates['runtime:deviceCardRender']?.count).toBe(1);

    clearCanvasMetrics();

    expect(exportCanvasMetrics().aggregates['runtime:deviceCardRender']).toBeUndefined();
  });

  it('records frame timing and frame budget aggregates without raw sample growth', () => {
    recordCanvasFrameTime(12);
    recordCanvasFrameTime(20);
    recordCanvasFrameTime(40);
    recordCanvasFrameTime(55);

    const exported = exportCanvasMetrics();

    expect(exported.samples).toEqual([]);
    expect(exported.aggregates['runtime:frameTime']).toEqual({
      count: 4,
      minMs: 12,
      maxMs: 55,
      avgMs: 31.75,
      p95Ms: 55,
    });
    expect(exported.aggregates['runtime:frameOverBudget16']).toEqual({
      count: 3,
      minMs: 20,
      maxMs: 55,
      avgMs: 38.333,
      p95Ms: 55,
    });
    expect(exported.aggregates['runtime:frameOverBudget33']).toEqual({
      count: 2,
      minMs: 40,
      maxMs: 55,
      avgMs: 47.5,
      p95Ms: 55,
    });
    expect(exported.aggregates['runtime:frameOverBudget50']).toEqual({
      count: 1,
      minMs: 55,
      maxMs: 55,
      avgMs: 55,
      p95Ms: 55,
    });
  });

  it('records browser long task aggregates without adding raw samples', () => {
    recordCanvasLongTask(82.4, { attributionCount: 1 });

    const exported = exportCanvasMetrics();

    expect(exported.samples).toEqual([]);
    expect(exported.aggregates['runtime:longTask']).toEqual({
      count: 1,
      minMs: 82.4,
      maxMs: 82.4,
      avgMs: 82.4,
      p95Ms: 82.4,
    });
  });
});
