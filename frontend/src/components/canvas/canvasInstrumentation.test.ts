import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  clearCanvasMetrics,
  exportCanvasMetrics,
  measureCanvasMetric,
  recordCanvasMetric,
} from './canvasInstrumentation';

describe('canvasInstrumentation', () => {
  beforeEach(() => {
    clearCanvasMetrics();
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
});
