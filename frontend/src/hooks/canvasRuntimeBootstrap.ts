/**
 * Coordinates canvas runtime bootstrap state and side effects for consuming components.
 * Owns cleanup-sensitive lifecycle work so callers receive stable state and actions.
 */
import type { SnapshotPayload } from '../types/metrics';

/** Describes the canvas runtime bootstrap contract used by the React hook lifecycle. */
export interface CanvasRuntimeBootstrap {
  snapshot: SnapshotPayload;
  runtimeStreamId?: string;
  runtimeVersion?: number;
  runtimeIdentity?: string;
}

type CanvasRuntimeBootstrapListener = (bootstrap: CanvasRuntimeBootstrap) => void;

const listeners = new Set<CanvasRuntimeBootstrapListener>();
let currentBootstrap: CanvasRuntimeBootstrap | null = null;

/** Returns canvas runtime bootstrap for the React hook lifecycle. */
export function getCanvasRuntimeBootstrap(): CanvasRuntimeBootstrap | null {
  return currentBootstrap;
}

/** Publish canvas runtime bootstrap for the React hook lifecycle. */
export function publishCanvasRuntimeBootstrap(bootstrap: CanvasRuntimeBootstrap): void {
  currentBootstrap = bootstrap;
  for (const listener of listeners) {
    listener(bootstrap);
  }
}

/** Subscribes to canvas runtime bootstrap for the React hook lifecycle. */
export function subscribeCanvasRuntimeBootstrap(
  listener: CanvasRuntimeBootstrapListener,
): () => void {
  listeners.add(listener);
  if (currentBootstrap !== null) {
    listener(currentBootstrap);
  }
  return () => {
    listeners.delete(listener);
  };
}

/** Resets canvas runtime bootstrap for the React hook lifecycle. */
export function resetCanvasRuntimeBootstrap(): void {
  currentBootstrap = null;
  listeners.clear();
}
