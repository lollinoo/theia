export type TopologyZoomBand = 'overview' | 'compact' | 'summary' | 'detail';

export function resolveTopologyZoomBand(zoom: number): TopologyZoomBand {
  if (!Number.isFinite(zoom) || zoom <= 0) {
    return 'detail';
  }

  if (zoom < 0.25) {
    return 'overview';
  }

  if (zoom < 0.35) {
    return 'compact';
  }

  if (zoom < 0.55) {
    return 'summary';
  }

  return 'detail';
}
