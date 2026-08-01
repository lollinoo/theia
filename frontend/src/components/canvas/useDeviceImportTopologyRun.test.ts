import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchCanvasMapTopology } from '../../api/client';
import {
  applyDeviceImportTopologyLayout,
  continueDeviceImportTopologyRun,
  fetchActiveDeviceImportTopologyRun,
  fetchDeviceImportTopologyRun,
  markDeviceImportTopologyManualEdit,
  retryDeviceImportTopologyRun,
} from '../../api/deviceImport';
import type { ImportedNodePlacementRequest } from './importedNodePlacementRequest';
import { useDeviceImportTopologyRun } from './useDeviceImportTopologyRun';

vi.mock('../../api/deviceImport', async (loadOriginal) => ({
  ...(await loadOriginal<typeof import('../../api/deviceImport')>()),
  applyDeviceImportTopologyLayout: vi.fn(),
  continueDeviceImportTopologyRun: vi.fn(),
  fetchActiveDeviceImportTopologyRun: vi.fn(),
  fetchDeviceImportTopologyRun: vi.fn(),
  markDeviceImportTopologyManualEdit: vi.fn(),
  retryDeviceImportTopologyRun: vi.fn(),
}));

vi.mock('../../api/client', async (loadOriginal) => ({
  ...(await loadOriginal<typeof import('../../api/client')>()),
  fetchCanvasMapTopology: vi.fn(),
}));

const now = '2026-07-31T12:00:00Z';

function snapshot(
  state: 'discovering' | 'ready_for_layout' | 'completed' = 'discovering',
  withWarning = true,
) {
  return {
    run: {
      id: 'run-1',
      map_id: 'map-1',
      file_digest: 'sha256:file',
      layout_scope: 'preserve' as const,
      state,
      auto_layout_allowed: true,
      backgrounded: false,
      layout_input_token: 'sha256:topology',
      reconcile_attempts: 0,
      created_at: now,
      updated_at: now,
    },
    items: [
      {
        device_id: 'imported',
        state:
          state === 'discovering'
            ? ('running' as const)
            : withWarning
              ? ('warning' as const)
              : ('succeeded' as const),
        attempt: 1,
        result_code:
          state === 'discovering' ? undefined : withWarning ? 'unresolved_neighbors' : 'discovered',
        message:
          state === 'discovering'
            ? undefined
            : withWarning
              ? 'One neighbor needs attention.'
              : 'Topology discovery completed.',
        neighbor_count: state === 'discovering' ? 0 : 2,
        links_created: state === 'discovering' ? 0 : 1,
        unresolved_neighbors: state === 'discovering' || !withWarning ? 0 : 1,
        updated_at: now,
      },
    ],
  };
}

const request: ImportedNodePlacementRequest = {
  requestId: 'request-1',
  mapId: 'map-1',
  deviceIds: ['imported'],
  topologyRunId: 'run-1',
  topologyLayoutScope: 'preserve',
};

async function flushEffects() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe('useDeviceImportTopologyRun', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(fetchActiveDeviceImportTopologyRun).mockResolvedValue(null);
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue(snapshot());
    vi.mocked(applyDeviceImportTopologyLayout).mockResolvedValue();
    vi.mocked(continueDeviceImportTopologyRun).mockResolvedValue();
    vi.mocked(markDeviceImportTopologyManualEdit).mockResolvedValue();
    vi.mocked(retryDeviceImportTopologyRun).mockResolvedValue();
  });

  it('resumes an active durable run after a page refresh', async () => {
    vi.mocked(fetchActiveDeviceImportTopologyRun).mockResolvedValue(snapshot());

    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request: null,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: vi.fn(),
      }),
    );

    await waitFor(() => expect(result.current.snapshot?.run.id).toBe('run-1'));
    expect(fetchActiveDeviceImportTopologyRun).toHaveBeenCalledWith('map-1');
    expect(result.current.phase).toBe('discovery');
    expect(result.current.progress).toMatchObject({ total: 1, completed: 0, running: 1 });

    act(() => {
      window.dispatchEvent(
        new CustomEvent('device-import-topology-run-changed', {
          detail: { run_id: 'run-1', map_id: 'map-1' },
        }),
      );
    });
    await waitFor(() => expect(fetchDeviceImportTopologyRun).toHaveBeenCalledWith('run-1'));
  });

  it('blocks mutations until the initial active-run lookup is authoritative', async () => {
    const lookup = deferred<ReturnType<typeof snapshot> | null>();
    vi.mocked(fetchActiveDeviceImportTopologyRun).mockReturnValue(lookup.promise);
    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request: null,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: vi.fn(),
      }),
    );

    expect(result.current.mutationBlocked).toBe(true);
    await act(async () => {
      await expect(result.current.markManualEdit()).rejects.toThrow(
        'Topology import status is still loading',
      );
    });
    expect(markDeviceImportTopologyManualEdit).not.toHaveBeenCalled();

    await act(async () => lookup.resolve(null));
    await waitFor(() => expect(result.current.mutationBlocked).toBe(false));
    expect(result.current.error).toBeNull();
  });

  it('keeps a failed active-run lookup blocked and retryable', async () => {
    vi.mocked(fetchActiveDeviceImportTopologyRun)
      .mockRejectedValueOnce(new Error('initial lookup failed'))
      .mockRejectedValueOnce(new Error('retry lookup failed'));
    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request: null,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: vi.fn(),
      }),
    );

    await waitFor(() => expect(result.current.error).toBe('initial lookup failed'));
    expect(result.current.mutationBlocked).toBe(true);

    await act(async () => result.current.refresh());
    expect(result.current.error).toBe('retry lookup failed');
    expect(result.current.mutationBlocked).toBe(true);
  });

  it('atomically applies the deterministic layout when discovery is ready', async () => {
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue(snapshot('ready_for_layout', false));
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue({
      status: 'ok',
      topology: {
        devices: [{ id: 'existing' }, { id: 'imported' }],
        links: [
          {
            id: 'link-1',
            source_device_id: 'existing',
            target_device_id: 'imported',
          },
        ],
        positions: {
          existing: { device_id: 'existing', x: 0, y: 0, pinned: true },
        },
      },
    } as never);
    const reloadTopology = vi.fn().mockResolvedValue('applied');
    const onConsumed = vi.fn();

    renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map([['existing', { x: 0, y: 0, pinned: true }]]),
        reloadTopology,
        onConsumed,
      }),
    );

    await waitFor(() => expect(applyDeviceImportTopologyLayout).toHaveBeenCalledOnce());
    expect(applyDeviceImportTopologyLayout).toHaveBeenCalledWith(
      'run-1',
      expect.objectContaining({
        input_token: 'sha256:topology',
        positions: [expect.objectContaining({ device_id: 'imported' })],
        reset_link_route_ids: ['link-1'],
      }),
    );
    expect(reloadTopology).toHaveBeenCalledWith(true, 'topology_import_layout');
    expect(onConsumed).toHaveBeenCalledWith('request-1');
  });

  it('keeps the layout phase visible until authoritative canvas reload finishes', async () => {
    const readySnapshot = snapshot('ready_for_layout', false);
    const completedSnapshot = snapshot('completed', false);
    vi.mocked(fetchDeviceImportTopologyRun)
      .mockResolvedValueOnce(readySnapshot)
      .mockResolvedValue(completedSnapshot);
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue({
      status: 'ok',
      topology: {
        devices: [{ id: 'imported' }],
        links: [],
        positions: {},
      },
    } as never);
    const reloadTopology = deferred<'applied'>();
    const onConsumed = vi.fn();
    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: () => reloadTopology.promise,
        onConsumed,
      }),
    );

    await waitFor(() => expect(applyDeviceImportTopologyLayout).toHaveBeenCalledOnce());
    await waitFor(() => expect(result.current.applyingLayout).toBe(true));

    act(() => {
      window.dispatchEvent(
        new CustomEvent('device-import-topology-run-changed', {
          detail: { run_id: 'run-1', map_id: 'map-1' },
        }),
      );
    });
    await waitFor(() => expect(result.current.snapshot?.run.state).toBe('completed'));
    expect(result.current.phase).toBe('layout');
    expect(onConsumed).not.toHaveBeenCalled();

    await act(async () => reloadTopology.resolve('applied'));
    await waitFor(() => expect(result.current.applyingLayout).toBe(false));
    expect(result.current.phase).toBe('complete');
    expect(onConsumed).toHaveBeenCalledWith('request-1');
  });

  it('holds the canvas position-save fence through layout apply and authoritative reload', async () => {
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue(snapshot('ready_for_layout', false));
    const order: string[] = [];
    vi.mocked(fetchCanvasMapTopology).mockImplementation(async () => {
      order.push('fetch-topology');
      return {
        status: 'ok',
        topology: {
          devices: [{ id: 'imported' }],
          links: [],
          positions: {},
        },
      } as never;
    });
    vi.mocked(applyDeviceImportTopologyLayout).mockImplementation(async () => {
      order.push('apply-layout');
    });
    const reloadTopology = vi.fn(async () => {
      order.push('reload-topology');
      return 'applied' as const;
    });
    const withPositionSaveFence = vi.fn(async (operation: () => Promise<void>) => {
      order.push('fence-start');
      await operation();
      order.push('fence-end');
    });
    const onConsumed = vi.fn();

    renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology,
        withPositionSaveFence,
        onConsumed,
      }),
    );

    await waitFor(() => expect(onConsumed).toHaveBeenCalledWith('request-1'));
    expect(withPositionSaveFence).toHaveBeenCalledOnce();
    expect(order).toEqual([
      'fence-start',
      'fetch-topology',
      'apply-layout',
      'reload-topology',
      'fence-end',
    ]);
    expect(reloadTopology).toHaveBeenCalledWith(true, 'topology_import_layout');
  });

  it('retries a stale authoritative canvas reload before consuming the import request', async () => {
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue(snapshot('ready_for_layout', false));
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue({
      status: 'ok',
      topology: {
        devices: [{ id: 'imported' }],
        links: [],
        positions: {},
      },
    } as never);
    const reloadTopology = vi.fn().mockResolvedValueOnce('stale').mockResolvedValueOnce('applied');
    const onConsumed = vi.fn();

    renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology,
        onConsumed,
      }),
    );

    await waitFor(() => expect(onConsumed).toHaveBeenCalledWith('request-1'));
    expect(reloadTopology).toHaveBeenCalledTimes(2);
    expect(reloadTopology).toHaveBeenNthCalledWith(1, true, 'topology_import_layout');
    expect(reloadTopology).toHaveBeenNthCalledWith(2, true, 'topology_import_layout');
  });

  it('keeps a completed run in layout phase when reload fails and retries that reload', async () => {
    const readySnapshot = snapshot('ready_for_layout', false);
    const completedSnapshot = snapshot('completed', false);
    vi.mocked(fetchDeviceImportTopologyRun)
      .mockResolvedValueOnce(readySnapshot)
      .mockResolvedValue(completedSnapshot);
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue({
      status: 'ok',
      topology: {
        devices: [{ id: 'imported' }],
        links: [],
        positions: {},
      },
    } as never);
    const firstReload = deferred<'applied'>();
    const reloadTopology = vi
      .fn()
      .mockReturnValueOnce(firstReload.promise)
      .mockResolvedValueOnce('applied');
    const onConsumed = vi.fn();
    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology,
        onConsumed,
      }),
    );

    await waitFor(() => expect(result.current.applyingLayout).toBe(true));
    act(() => {
      window.dispatchEvent(
        new CustomEvent('device-import-topology-run-changed', {
          detail: { run_id: 'run-1', map_id: 'map-1' },
        }),
      );
    });
    await waitFor(() => expect(result.current.snapshot?.run.state).toBe('completed'));

    await act(async () => firstReload.reject(new Error('authoritative reload failed')));
    await waitFor(() => expect(result.current.error).toBe('authoritative reload failed'));
    expect(result.current.phase).toBe('layout');
    expect(result.current.applyingLayout).toBe(true);
    expect(onConsumed).not.toHaveBeenCalled();

    await act(async () => result.current.refresh());

    expect(reloadTopology).toHaveBeenCalledTimes(2);
    expect(result.current.error).toBeNull();
    expect(result.current.applyingLayout).toBe(false);
    expect(result.current.phase).toBe('complete');
    expect(onConsumed).toHaveBeenCalledWith('request-1');
  });

  it('preserves a reload error when an in-flight run poll settles afterward', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    const readySnapshot = snapshot('ready_for_layout', false);
    const completedSnapshot = snapshot('completed', false);
    const poll = deferred<ReturnType<typeof snapshot>>();
    const layoutApplication = deferred<void>();
    vi.mocked(fetchDeviceImportTopologyRun)
      .mockResolvedValueOnce(readySnapshot)
      .mockReturnValueOnce(poll.promise);
    vi.mocked(applyDeviceImportTopologyLayout).mockReturnValue(layoutApplication.promise);
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue({
      status: 'ok',
      topology: {
        devices: [{ id: 'imported' }],
        links: [],
        positions: {},
      },
    } as never);
    const rendered = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: vi.fn().mockRejectedValue(new Error('authoritative reload failed')),
      }),
    );
    const { result } = rendered;

    try {
      await flushEffects();
      expect(applyDeviceImportTopologyLayout).toHaveBeenCalledOnce();
      await act(async () => vi.advanceTimersByTimeAsync(1000));
      expect(fetchDeviceImportTopologyRun).toHaveBeenCalledTimes(2);

      await act(async () => layoutApplication.resolve());
      await flushEffects();
      expect(result.current.error).toBe('authoritative reload failed');
      await act(async () => poll.resolve(completedSnapshot));

      expect(result.current.error).toBe('authoritative reload failed');
      expect(result.current.phase).toBe('layout');
      expect(result.current.applyingLayout).toBe(true);
    } finally {
      rendered.unmount();
      vi.useRealTimers();
    }
  });

  it('waits for operator confirmation before laying out warning results', async () => {
    const warningSnapshot = snapshot('ready_for_layout');
    vi.mocked(fetchDeviceImportTopologyRun)
      .mockResolvedValueOnce(warningSnapshot)
      .mockResolvedValue({
        ...warningSnapshot,
        run: { ...warningSnapshot.run, backgrounded: true },
      });
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue({
      status: 'ok',
      topology: {
        devices: [{ id: 'imported' }],
        links: [],
        positions: {},
      },
    } as never);
    const reloadTopology = vi.fn().mockResolvedValue('applied');
    const onConsumed = vi.fn();
    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology,
        onConsumed,
      }),
    );
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());
    expect(applyDeviceImportTopologyLayout).not.toHaveBeenCalled();

    await act(async () => result.current.continuePartial());

    await waitFor(() => expect(applyDeviceImportTopologyLayout).toHaveBeenCalledOnce());
    await waitFor(() => expect(onConsumed).toHaveBeenCalledWith('request-1'));
    expect(reloadTopology).toHaveBeenCalledWith(true, 'topology_import_layout');
    expect(result.current.phase).toBe('complete');
  });

  it('automatically lays out imported nodes when every device is SNMP unreachable', async () => {
    const allOfflineSnapshot = {
      ...snapshot('ready_for_layout', false),
      items: [
        {
          ...snapshot('ready_for_layout', false).items[0],
          state: 'failed' as const,
          result_code: 'snmp_unreachable' as const,
          message: 'SNMP topology discovery did not complete.',
          neighbor_count: 0,
          links_created: 0,
          unresolved_neighbors: 0,
        },
      ],
    };
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue(allOfflineSnapshot);
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue({
      status: 'ok',
      topology: {
        devices: [{ id: 'imported' }],
        links: [],
        positions: {},
      },
    } as never);

    renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: vi.fn().mockResolvedValue('applied'),
      }),
    );

    await waitFor(() => expect(applyDeviceImportTopologyLayout).toHaveBeenCalledOnce());
    expect(continueDeviceImportTopologyRun).not.toHaveBeenCalled();
    expect(applyDeviceImportTopologyLayout).toHaveBeenCalledWith(
      'run-1',
      expect.objectContaining({
        input_token: 'sha256:topology',
        positions: [expect.objectContaining({ device_id: 'imported' })],
        reset_link_route_ids: [],
      }),
    );
  });

  it('automatically lays out imported nodes when discovery finds no neighbors or links', async () => {
    const noTopologySnapshot = {
      ...snapshot('ready_for_layout', false),
      items: [
        {
          ...snapshot('ready_for_layout', false).items[0],
          state: 'warning' as const,
          result_code: 'no_neighbors' as const,
          message: 'No LLDP or CDP neighbors were discovered.',
          neighbor_count: 0,
          links_created: 0,
          unresolved_neighbors: 0,
        },
      ],
    };
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue(noTopologySnapshot);
    vi.mocked(fetchCanvasMapTopology).mockResolvedValue({
      status: 'ok',
      topology: {
        devices: [{ id: 'imported' }],
        links: [],
        positions: {},
      },
    } as never);

    renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: vi.fn().mockResolvedValue('applied'),
      }),
    );

    await waitFor(() => expect(applyDeviceImportTopologyLayout).toHaveBeenCalledOnce());
    expect(continueDeviceImportTopologyRun).not.toHaveBeenCalled();
  });

  it('still requires confirmation when an offline run contains another issue', async () => {
    const baseItem = snapshot('ready_for_layout', false).items[0];
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue({
      ...snapshot('ready_for_layout', false),
      items: [
        {
          ...baseItem,
          state: 'failed' as const,
          result_code: 'snmp_unreachable' as const,
          message: 'SNMP topology discovery did not complete.',
          neighbor_count: 0,
          links_created: 0,
          unresolved_neighbors: 0,
        },
        {
          ...baseItem,
          device_id: 'needs-attention',
          state: 'warning' as const,
          result_code: 'unresolved_neighbors' as const,
          message: 'One neighbor needs attention.',
          unresolved_neighbors: 1,
        },
      ],
    });

    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: vi.fn().mockResolvedValue('applied'),
      }),
    );

    await waitFor(() => expect(result.current.snapshot?.run.state).toBe('ready_for_layout'));
    await flushEffects();
    expect(applyDeviceImportTopologyLayout).not.toHaveBeenCalled();
  });

  it('marks only the first manual canvas mutation and disables late auto-layout locally', async () => {
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue(snapshot());
    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: vi.fn(),
      }),
    );
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());

    await act(async () => {
      await result.current.markManualEdit();
      await result.current.markManualEdit();
    });

    expect(markDeviceImportTopologyManualEdit).toHaveBeenCalledOnce();
    expect(result.current.snapshot?.run.auto_layout_allowed).toBe(false);
  });

  it('keeps mutations blocked and retries cancellation after a manual-edit request fails', async () => {
    vi.mocked(markDeviceImportTopologyManualEdit)
      .mockRejectedValueOnce(new Error('manual edit unavailable'))
      .mockResolvedValueOnce();
    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: 'map:map-1',
        nodePositions: new Map(),
        reloadTopology: vi.fn(),
      }),
    );
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());
    expect(result.current.mutationBlocked).toBe(true);

    await act(async () => {
      await expect(result.current.markManualEdit()).rejects.toThrow('manual edit unavailable');
    });
    expect(result.current.snapshot?.run.auto_layout_allowed).toBe(true);
    expect(result.current.mutationBlocked).toBe(true);

    await act(async () => result.current.markManualEdit());
    expect(markDeviceImportTopologyManualEdit).toHaveBeenCalledTimes(2);
    expect(result.current.snapshot?.run.auto_layout_allowed).toBe(false);
    expect(result.current.mutationBlocked).toBe(false);
  });

  it('discards an awaited refresh when the selected map changes', async () => {
    let resolveOldRefresh: ((value: ReturnType<typeof snapshot>) => void) | undefined;
    const oldRefresh = new Promise<ReturnType<typeof snapshot>>((resolve) => {
      resolveOldRefresh = resolve;
    });
    const mapTwoSnapshot = {
      ...snapshot(),
      run: { ...snapshot().run, id: 'run-2', map_id: 'map-2' },
    };
    vi.mocked(fetchActiveDeviceImportTopologyRun).mockImplementation(async (mapID) => {
      if (mapID === 'map-2') return mapTwoSnapshot;
      return snapshot();
    });
    vi.mocked(fetchDeviceImportTopologyRun).mockImplementation(async (runID) => {
      if (runID === 'run-1') return oldRefresh;
      return mapTwoSnapshot;
    });
    const { result, rerender } = renderHook(
      ({ mapId }) =>
        useDeviceImportTopologyRun({
          mapId,
          request: null,
          renderedMapKey: mapId ? `map:${mapId}` : null,
          nodePositions: new Map(),
          reloadTopology: vi.fn(),
        }),
      { initialProps: { mapId: 'map-1' as string | null } },
    );
    await waitFor(() => expect(result.current.snapshot?.run.id).toBe('run-1'));

    let refreshPromise: Promise<void> | undefined;
    act(() => {
      refreshPromise = result.current.refresh();
    });
    rerender({ mapId: 'map-2' });
    await waitFor(() => expect(result.current.snapshot?.run.id).toBe('run-2'));

    await act(async () => {
      resolveOldRefresh?.(snapshot());
      await refreshPromise;
    });
    expect(result.current.snapshot?.run.id).toBe('run-2');
    expect(result.current.snapshot?.run.map_id).toBe('map-2');
  });

  it('does not clear the current map error when an old WebSocket refresh settles', async () => {
    const oldRefresh = deferred<ReturnType<typeof snapshot>>();
    vi.mocked(fetchActiveDeviceImportTopologyRun).mockImplementation(async (mapID) => {
      if (mapID === 'map-2') throw new Error('map two lookup failed');
      return snapshot();
    });
    vi.mocked(fetchDeviceImportTopologyRun).mockReturnValue(oldRefresh.promise);
    const { result, rerender } = renderHook(
      ({ mapId }) =>
        useDeviceImportTopologyRun({
          mapId,
          request: null,
          renderedMapKey: mapId ? `map:${mapId}` : null,
          nodePositions: new Map(),
          reloadTopology: vi.fn(),
        }),
      { initialProps: { mapId: 'map-1' as string | null } },
    );
    await waitFor(() => expect(result.current.snapshot?.run.id).toBe('run-1'));

    act(() => {
      window.dispatchEvent(
        new CustomEvent('device-import-topology-run-changed', {
          detail: { run_id: 'run-1', map_id: 'map-1' },
        }),
      );
    });
    await waitFor(() => expect(fetchDeviceImportTopologyRun).toHaveBeenCalledWith('run-1'));
    rerender({ mapId: 'map-2' });
    await waitFor(() => expect(result.current.error).toBe('map two lookup failed'));

    await act(async () => oldRefresh.resolve(snapshot()));
    expect(result.current.error).toBe('map two lookup failed');
    expect(result.current.mutationBlocked).toBe(true);
  });

  it('discards a late continue failure after the selected map changes', async () => {
    const pendingContinue = deferred<void>();
    const mapTwoSnapshot = {
      ...snapshot(),
      run: { ...snapshot().run, id: 'run-2', map_id: 'map-2' },
    };
    vi.mocked(fetchActiveDeviceImportTopologyRun).mockImplementation(async (mapID) =>
      mapID === 'map-2' ? mapTwoSnapshot : snapshot(),
    );
    vi.mocked(continueDeviceImportTopologyRun).mockReturnValue(pendingContinue.promise);
    const { result, rerender } = renderHook(
      ({ mapId }) =>
        useDeviceImportTopologyRun({
          mapId,
          request: null,
          renderedMapKey: mapId ? `map:${mapId}` : null,
          nodePositions: new Map(),
          reloadTopology: vi.fn(),
        }),
      { initialProps: { mapId: 'map-1' as string | null } },
    );
    await waitFor(() => expect(result.current.snapshot?.run.id).toBe('run-1'));

    let actionPromise: Promise<void> | undefined;
    act(() => {
      actionPromise = result.current.continuePartial();
    });
    await waitFor(() => expect(continueDeviceImportTopologyRun).toHaveBeenCalledWith('run-1'));
    rerender({ mapId: 'map-2' });
    await waitFor(() => expect(result.current.snapshot?.run.id).toBe('run-2'));

    await act(async () => {
      pendingContinue.reject(new Error('old map continue failed'));
      await actionPromise;
    });
    expect(result.current.snapshot?.run.id).toBe('run-2');
    expect(result.current.error).toBeNull();
  });

  it('discards a late retry failure after the selected map changes', async () => {
    const pendingRetry = deferred<void>();
    const mapTwoSnapshot = {
      ...snapshot(),
      run: { ...snapshot().run, id: 'run-2', map_id: 'map-2' },
    };
    vi.mocked(fetchActiveDeviceImportTopologyRun).mockImplementation(async (mapID) =>
      mapID === 'map-2' ? mapTwoSnapshot : snapshot(),
    );
    vi.mocked(retryDeviceImportTopologyRun).mockReturnValue(pendingRetry.promise);
    const { result, rerender } = renderHook(
      ({ mapId }) =>
        useDeviceImportTopologyRun({
          mapId,
          request: null,
          renderedMapKey: mapId ? `map:${mapId}` : null,
          nodePositions: new Map(),
          reloadTopology: vi.fn(),
        }),
      { initialProps: { mapId: 'map-1' as string | null } },
    );
    await waitFor(() => expect(result.current.snapshot?.run.id).toBe('run-1'));

    let actionPromise: Promise<void> | undefined;
    act(() => {
      actionPromise = result.current.retry(['imported']);
    });
    await waitFor(() =>
      expect(retryDeviceImportTopologyRun).toHaveBeenCalledWith('run-1', ['imported']),
    );
    rerender({ mapId: 'map-2' });
    await waitFor(() => expect(result.current.snapshot?.run.id).toBe('run-2'));

    await act(async () => {
      pendingRetry.reject(new Error('old map retry failed'));
      await actionPromise;
    });
    expect(result.current.snapshot?.run.id).toBe('run-2');
    expect(result.current.error).toBeNull();
  });

  it('discards a late layout failure after the selected map changes', async () => {
    const pendingTopology = deferred<Awaited<ReturnType<typeof fetchCanvasMapTopology>>>();
    const mapTwoSnapshot = {
      ...snapshot(),
      run: { ...snapshot().run, id: 'run-2', map_id: 'map-2' },
    };
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue(snapshot('ready_for_layout', false));
    vi.mocked(fetchActiveDeviceImportTopologyRun).mockImplementation(async (mapID) =>
      mapID === 'map-2' ? mapTwoSnapshot : null,
    );
    vi.mocked(fetchCanvasMapTopology).mockReturnValue(pendingTopology.promise);
    const { result, rerender } = renderHook(
      ({ mapId, importRequest }) =>
        useDeviceImportTopologyRun({
          mapId,
          request: importRequest,
          renderedMapKey: mapId ? `map:${mapId}` : null,
          nodePositions: new Map(),
          reloadTopology: vi.fn(),
        }),
      {
        initialProps: {
          mapId: 'map-1' as string | null,
          importRequest: request as ImportedNodePlacementRequest | null,
        },
      },
    );
    await waitFor(() => expect(fetchCanvasMapTopology).toHaveBeenCalledWith('map-1'));

    rerender({ mapId: 'map-2', importRequest: null });
    await waitFor(() => expect(result.current.snapshot?.run.id).toBe('run-2'));
    await act(async () => pendingTopology.reject(new Error('old map layout failed')));

    expect(result.current.snapshot?.run.id).toBe('run-2');
    expect(result.current.error).toBeNull();
  });

  it('coalesces repeated retry requests while the first retry is pending', async () => {
    const pendingRetry = deferred<void>();
    vi.mocked(fetchDeviceImportTopologyRun).mockResolvedValue(snapshot('ready_for_layout'));
    vi.mocked(retryDeviceImportTopologyRun).mockReturnValue(pendingRetry.promise);
    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: null,
        nodePositions: new Map(),
        reloadTopology: vi.fn(),
      }),
    );
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());

    let firstRetry: Promise<void> | undefined;
    let secondRetry: Promise<void> | undefined;
    act(() => {
      firstRetry = result.current.retry(['imported']);
      secondRetry = result.current.retry(['imported']);
    });

    expect(result.current.retryPending).toBe(true);
    expect(retryDeviceImportTopologyRun).toHaveBeenCalledOnce();

    await act(async () => {
      pendingRetry.resolve();
      await Promise.all([firstRetry, secondRetry]);
    });
    expect(result.current.retryPending).toBe(false);
  });

  it('exposes retry and continue actions with refreshed state', async () => {
    vi.mocked(fetchDeviceImportTopologyRun)
      .mockResolvedValueOnce(snapshot())
      .mockResolvedValue(snapshot('ready_for_layout'));
    const { result } = renderHook(() =>
      useDeviceImportTopologyRun({
        mapId: 'map-1',
        request,
        renderedMapKey: null,
        nodePositions: new Map(),
        reloadTopology: vi.fn(),
      }),
    );
    await waitFor(() => expect(result.current.snapshot).not.toBeNull());

    await act(async () => {
      await result.current.continuePartial();
      await result.current.retry(['imported']);
    });
    await flushEffects();

    expect(continueDeviceImportTopologyRun).toHaveBeenCalledWith('run-1');
    expect(retryDeviceImportTopologyRun).toHaveBeenCalledWith('run-1', ['imported']);
    expect(fetchDeviceImportTopologyRun).toHaveBeenCalledTimes(3);
  });
});
