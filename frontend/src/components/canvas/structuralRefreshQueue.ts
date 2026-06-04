import type { StructuralRefreshCause } from './topologyRecovery';

export interface StructuralRefreshQueue {
  queue: (cause: StructuralRefreshCause) => void;
  cancel: () => void;
}

interface StructuralRefreshQueueOptions {
  debounceMs: number;
  runRefresh: (causes: Set<StructuralRefreshCause>) => void;
  setTimeoutFn: (handler: () => void, timeout: number) => number;
  clearTimeoutFn: (timerId: number) => void;
}

// createStructuralRefreshQueue coalesces backend structural events into one debounced refresh pass.
export function createStructuralRefreshQueue({
  debounceMs,
  runRefresh,
  setTimeoutFn,
  clearTimeoutFn,
}: StructuralRefreshQueueOptions): StructuralRefreshQueue {
  const pendingCauses = new Set<StructuralRefreshCause>();
  let timer: number | null = null;

  return {
    queue(cause: StructuralRefreshCause) {
      pendingCauses.add(cause);

      if (timer !== null) {
        return;
      }

      timer = setTimeoutFn(() => {
        timer = null;
        const refreshCauses = new Set(pendingCauses);
        pendingCauses.clear();
        runRefresh(refreshCauses);
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
