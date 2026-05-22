export type TopologyZoomBand = 'overview' | 'compact' | 'summary' | 'detail';

export function resolveTopologyZoomBand(zoom: number): TopologyZoomBand {
  if (!Number.isFinite(zoom) || zoom <= 0) {
    return 'detail';
  }

  if (zoom < 0.45) {
    return 'overview';
  }

  if (zoom < 0.75) {
    return 'compact';
  }

  if (zoom < 0.95) {
    return 'summary';
  }

  return 'detail';
}
