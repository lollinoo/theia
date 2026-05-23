import { describe, expect, it } from 'vitest';
import { resolveTopologyZoomBand } from './topologyZoom';

describe('topologyZoom', () => {
  it('resolves semantic zoom bands at the configured thresholds', () => {
    expect(resolveTopologyZoomBand(0.1)).toBe('overview');
    expect(resolveTopologyZoomBand(0.3)).toBe('overview');
    expect(resolveTopologyZoomBand(0.31)).toBe('overview');
    expect(resolveTopologyZoomBand(0.32)).toBe('compact');
    expect(resolveTopologyZoomBand(0.44)).toBe('compact');
    expect(resolveTopologyZoomBand(0.45)).toBe('summary');
    expect(resolveTopologyZoomBand(0.54)).toBe('summary');
    expect(resolveTopologyZoomBand(0.55)).toBe('detail');
    expect(resolveTopologyZoomBand(1.5)).toBe('detail');
  });

  it('falls back to detail for invalid zoom values', () => {
    expect(resolveTopologyZoomBand(Number.NaN)).toBe('detail');
    expect(resolveTopologyZoomBand(0)).toBe('detail');
    expect(resolveTopologyZoomBand(-1)).toBe('detail');
  });
});
