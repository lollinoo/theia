/**
 * Defines structural refresh queue behavior for the topology canvas.
 * Documents how canonical topology data is projected into the interactive view layer.
 */
import type { StructuralRefreshCause } from './topologyRecovery';

/** Describes the structural refresh queue contract used by the topology canvas. */
export interface StructuralRefreshQueue {
  queue: (cause: StructuralRefreshCause) => void;
  cancel: () => void;
}

interface StructuralRefreshQueueOptions {
  debounceMs: number;
  runRefresh: (causes: Set<StructuralRefreshCause>) => void | Promise<void>;
  setTimeoutFn: (handler: () => void, timeout: number) => number;
  clearTimeoutFn: (timerId: number) => void;
}

// createStructuralRefreshQueue coalesces structural events and serializes asynchronous refreshes.
export function createStructuralRefreshQueue({
  debounceMs,
  runRefresh,
  setTimeoutFn,
  clearTimeoutFn,
}: StructuralRefreshQueueOptions): StructuralRefreshQueue {
  const pendingCauses = new Set<StructuralRefreshCause>();
  let timer: number | null = null;
  let running = false;

  const runPendingRefresh = () => {
    if (running || pendingCauses.size === 0) {
      return;
    }

    const refreshCauses = new Set(pendingCauses);
    pendingCauses.clear();
    running = true;

    const settleRefresh = () => {
      running = false;
      runPendingRefresh();
    };

    try {
      const result = runRefresh(refreshCauses);
      if (result === undefined) {
        settleRefresh();
        return;
      }
      void result.then(settleRefresh, settleRefresh);
    } catch {
      settleRefresh();
    }
  };

  return {
    queue(cause: StructuralRefreshCause) {
      pendingCauses.add(cause);

      if (running || timer !== null) {
        return;
      }

      timer = setTimeoutFn(() => {
        timer = null;
        runPendingRefresh();
      }, debounceMs);
    },

    cancel() {
      if (timer !== null) {
        clearTimeoutFn(timer);
        timer = null;
      }
      pendingCauses.clear();
    },
  };
}
