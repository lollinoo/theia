/**
 * Coalesces application-level runtime acknowledgements behind one bounded timer.
 */
import type { RuntimeCursor } from './runtimeRecovery';

/** Delay used to collapse clustered cursor advances into one runtime ACK. */
export const RUNTIME_ACK_COALESCE_MS = 250;

/** Describes the runtime ACK client control sent over the WebSocket. */
export interface RuntimeAckControl {
  type: 'runtime_ack';
  payload: {
    runtime_stream_id: string;
    runtime_version: number;
  };
}

/** Controls the bounded runtime ACK scheduler lifecycle. */
export interface RuntimeAckScheduler {
  schedule: (cursor: RuntimeCursor | null) => void;
  flush: () => void;
  cancel: () => void;
  reset: () => void;
}

type TimerHandle = ReturnType<typeof setTimeout>;

interface RuntimeAckSchedulerOptions {
  send: (control: RuntimeAckControl) => void;
  setTimeoutFn?: (handler: () => void, timeout: number) => TimerHandle;
  clearTimeoutFn?: (timer: TimerHandle) => void;
}

/** Creates a scheduler that retains only the newest same-stream pending cursor. */
export function createRuntimeAckScheduler({
  send,
  setTimeoutFn = globalThis.setTimeout,
  clearTimeoutFn = globalThis.clearTimeout,
}: RuntimeAckSchedulerOptions): RuntimeAckScheduler {
  let streamId: string | null = null;
  let lastSentVersion = -1;
  let pendingCursor: RuntimeCursor | null = null;
  let timer: TimerHandle | null = null;
  let timerSequence = 0;
  let activeTimerToken: number | null = null;
  let isSending = false;
  let flushRequested = false;

  // Callback reentrancy can mutate the cursor even when local control-flow analysis cannot see it.
  const getPendingCursor = (): RuntimeCursor | null => pendingCursor;

  const emitPending = (): void => {
    if (isSending) {
      flushRequested = true;
      return;
    }

    do {
      flushRequested = false;
      const cursor = pendingCursor;
      if (cursor === null) {
        return;
      }

      // Detach the in-flight cursor so reentrant scheduling cannot emit it twice.
      pendingCursor = null;
      isSending = true;
      try {
        send({
          type: 'runtime_ack',
          payload: {
            runtime_stream_id: cursor.streamId,
            runtime_version: cursor.version,
          },
        });
      } catch (error) {
        const scheduledCursor = getPendingCursor();
        if (
          streamId === cursor.streamId &&
          (scheduledCursor === null ||
            (scheduledCursor.streamId === cursor.streamId &&
              scheduledCursor.version <= cursor.version))
        ) {
          pendingCursor = cursor;
        }
        throw error;
      } finally {
        isSending = false;
      }

      if (streamId === cursor.streamId) {
        lastSentVersion = Math.max(lastSentVersion, cursor.version);
        const scheduledCursor = getPendingCursor();
        if (
          scheduledCursor?.streamId === cursor.streamId &&
          scheduledCursor.version <= lastSentVersion
        ) {
          pendingCursor = null;
          clearActiveTimer();
        }
      }
    } while (flushRequested);
  };

  const clearActiveTimer = (): void => {
    activeTimerToken = null;
    if (timer !== null) {
      clearTimeoutFn(timer);
      timer = null;
    }
  };

  const flush = (): void => {
    clearActiveTimer();
    emitPending();
  };

  const cancel = (): void => {
    clearActiveTimer();
    pendingCursor = null;
    flushRequested = false;
  };

  const scheduleTimer = (): void => {
    timerSequence += 1;
    const token = timerSequence;
    activeTimerToken = token;

    let handle: TimerHandle;
    try {
      handle = setTimeoutFn(() => {
        if (activeTimerToken !== token) {
          return;
        }
        activeTimerToken = null;
        timer = null;
        emitPending();
      }, RUNTIME_ACK_COALESCE_MS);
    } catch (error) {
      if (activeTimerToken === token) {
        activeTimerToken = null;
      }
      throw error;
    }

    if (activeTimerToken === token) {
      timer = handle;
    } else {
      clearTimeoutFn(handle);
    }
  };

  return {
    schedule(cursor: RuntimeCursor | null) {
      if (
        cursor === null ||
        cursor.streamId.trim().length === 0 ||
        !Number.isSafeInteger(cursor.version) ||
        cursor.version < 0
      ) {
        return;
      }

      if (streamId === null) {
        streamId = cursor.streamId;
      }
      if (cursor.streamId !== streamId) {
        return;
      }

      if (pendingCursor !== null) {
        if (cursor.version < pendingCursor.version) {
          return;
        }
        if (cursor.version > pendingCursor.version) {
          pendingCursor = { ...cursor };
        }
      } else {
        if (cursor.version <= lastSentVersion) {
          return;
        }
        pendingCursor = { ...cursor };
      }

      if (activeTimerToken !== null) {
        return;
      }
      scheduleTimer();
    },
    flush,
    cancel,
    reset() {
      cancel();
      streamId = null;
      lastSentVersion = -1;
    },
  };
}
