import type { SnapshotPayload } from '../types/metrics';

export interface CanvasRuntimeBootstrap {
  snapshot: SnapshotPayload;
  runtimeVersion?: number;
  runtimeIdentity?: string;
}

type CanvasRuntimeBootstrapListener = (bootstrap: CanvasRuntimeBootstrap) => void;

const listeners = new Set<CanvasRuntimeBootstrapListener>();
let currentBootstrap: CanvasRuntimeBootstrap | null = null;

export function getCanvasRuntimeBootstrap(): CanvasRuntimeBootstrap | null {
  return currentBootstrap;
}

export function publishCanvasRuntimeBootstrap(bootstrap: CanvasRuntimeBootstrap): void {
  currentBootstrap = bootstrap;
  for (const listener of listeners) {
    listener(bootstrap);
  }
}

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

export function resetCanvasRuntimeBootstrap(): void {
  currentBootstrap = null;
  listeners.clear();
}
