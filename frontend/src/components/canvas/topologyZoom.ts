/**
 * Defines topology zoom behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
export type TopologyZoomBand = 'overview' | 'compact' | 'summary' | 'detail';

/** Resolves topology zoom band for the topology canvas. */
export function resolveTopologyZoomBand(zoom: number): TopologyZoomBand {
  if (!Number.isFinite(zoom) || zoom <= 0) {
    return 'detail';
  }

  if (zoom < 0.32) {
    return 'overview';
  }

  if (zoom < 0.45) {
    return 'compact';
  }

  if (zoom < 0.55) {
    return 'summary';
  }

  return 'detail';
}
