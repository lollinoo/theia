/**
 * Exercises the runtime bootstrap cursor carrier independently from the WebSocket lifecycle.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  getCanvasRuntimeBootstrap,
  publishCanvasRuntimeBootstrap,
  resetCanvasRuntimeBootstrap,
  subscribeCanvasRuntimeBootstrap,
} from './canvasRuntimeBootstrap';

const bootstrap = {
  snapshot: { devices: {}, links: {} },
  runtimeStreamId: 'runtime-stream-7',
  runtimeVersion: 7,
  runtimeIdentity: 'rt-sha256:7',
};

describe('canvasRuntimeBootstrap', () => {
  beforeEach(() => {
    resetCanvasRuntimeBootstrap();
  });

  it('stores and publishes the full runtime cursor', () => {
    const listener = vi.fn();
    const unsubscribe = subscribeCanvasRuntimeBootstrap(listener);

    publishCanvasRuntimeBootstrap(bootstrap);

    expect(getCanvasRuntimeBootstrap()).toEqual(bootstrap);
    expect(listener).toHaveBeenCalledWith(bootstrap);
    unsubscribe();
  });

  it('replays the current bootstrap to a late subscriber and clears it on reset', () => {
    publishCanvasRuntimeBootstrap(bootstrap);
    const listener = vi.fn();

    subscribeCanvasRuntimeBootstrap(listener);
    expect(listener).toHaveBeenCalledWith(bootstrap);

    resetCanvasRuntimeBootstrap();
    expect(getCanvasRuntimeBootstrap()).toBeNull();

    publishCanvasRuntimeBootstrap({ ...bootstrap, runtimeVersion: 8 });
    expect(listener).toHaveBeenCalledTimes(1);
  });
});
