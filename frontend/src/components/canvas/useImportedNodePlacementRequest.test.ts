import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { ImportedNodePlacementRequest } from './importedNodePlacementRequest';
import { useImportedNodePlacementRequest } from './useImportedNodePlacementRequest';

const request: ImportedNodePlacementRequest = {
  requestId: 'request-1',
  mapId: 'map-1',
  deviceIds: ['device-a', 'device-b'],
};

async function flushEffects() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe('useImportedNodePlacementRequest', () => {
  it('waits until the destination map topology is rendered', async () => {
    const placeImportedNodes = vi.fn().mockResolvedValue('applied');
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ activeMapId, renderedMapKey }) =>
        useImportedNodePlacementRequest({
          request,
          activeMapId,
          renderedMapKey,
          topologyRevision: 'device-a|device-b',
          placeImportedNodes,
          onConsumed,
        }),
      { initialProps: { activeMapId: 'map-2', renderedMapKey: 'map:map-2' } },
    );

    await flushEffects();
    expect(placeImportedNodes).not.toHaveBeenCalled();

    rerender({ activeMapId: 'map-1', renderedMapKey: 'map:map-1' });
    await flushEffects();

    expect(placeImportedNodes).toHaveBeenCalledOnce();
    expect(placeImportedNodes).toHaveBeenCalledWith(request.deviceIds);
    expect(onConsumed).toHaveBeenCalledWith(request.requestId);
  });

  it('retries a pending request after topology changes and consumes it only after success', async () => {
    const placeImportedNodes = vi
      .fn()
      .mockResolvedValueOnce('pending')
      .mockResolvedValueOnce('applied');
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ topologyRevision }) =>
        useImportedNodePlacementRequest({
          request,
          activeMapId: 'map-1',
          renderedMapKey: 'map:map-1',
          topologyRevision,
          placeImportedNodes,
          onConsumed,
        }),
      { initialProps: { topologyRevision: 'device-a' } },
    );

    await flushEffects();
    expect(placeImportedNodes).toHaveBeenCalledTimes(1);
    expect(onConsumed).not.toHaveBeenCalled();

    rerender({ topologyRevision: 'device-a|device-b' });
    await flushEffects();

    expect(placeImportedNodes).toHaveBeenCalledTimes(2);
    expect(onConsumed).toHaveBeenCalledOnce();
    expect(onConsumed).toHaveBeenCalledWith(request.requestId);

    rerender({ topologyRevision: 'device-a|device-b|device-c' });
    await flushEffects();
    expect(placeImportedNodes).toHaveBeenCalledTimes(2);
  });

  it('queues a topology-change retry while the previous placement attempt is in flight', async () => {
    let resolveFirstPlacement!: (value: 'pending') => void;
    const firstPlacement = new Promise<'pending'>((resolve) => {
      resolveFirstPlacement = resolve;
    });
    const placeImportedNodes = vi
      .fn()
      .mockReturnValueOnce(firstPlacement)
      .mockResolvedValueOnce('applied');
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ topologyRevision }) =>
        useImportedNodePlacementRequest({
          request,
          activeMapId: 'map-1',
          renderedMapKey: 'map:map-1',
          topologyRevision,
          placeImportedNodes,
          onConsumed,
        }),
      { initialProps: { topologyRevision: 'device-a' } },
    );

    await flushEffects();
    rerender({ topologyRevision: 'device-a|device-b' });
    await act(async () => {
      resolveFirstPlacement('pending');
      await firstPlacement;
    });
    await flushEffects();

    expect(placeImportedNodes).toHaveBeenCalledTimes(2);
    expect(onConsumed).toHaveBeenCalledWith(request.requestId);
  });

  it('does not consume a stale result after the active map changes', async () => {
    let resolvePlacement!: (value: 'applied') => void;
    const placement = new Promise<'applied'>((resolve) => {
      resolvePlacement = resolve;
    });
    const placeImportedNodes = vi.fn().mockReturnValue(placement);
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ activeMapId, renderedMapKey }) =>
        useImportedNodePlacementRequest({
          request,
          activeMapId,
          renderedMapKey,
          topologyRevision: 'device-a|device-b',
          placeImportedNodes,
          onConsumed,
        }),
      { initialProps: { activeMapId: 'map-1', renderedMapKey: 'map:map-1' } },
    );

    await flushEffects();
    rerender({ activeMapId: 'map-2', renderedMapKey: 'map:map-2' });
    await act(async () => {
      resolvePlacement('applied');
      await placement;
    });

    expect(onConsumed).not.toHaveBeenCalled();

    rerender({ activeMapId: 'map-1', renderedMapKey: 'map:map-1' });
    await flushEffects();

    expect(placeImportedNodes).toHaveBeenCalledTimes(2);
    expect(onConsumed).toHaveBeenCalledWith(request.requestId);
  });
});
