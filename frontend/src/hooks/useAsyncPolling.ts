/**
 * Coordinates a single non-overlapping asynchronous polling loop.
 */
import { useCallback, useEffect, useMemo, useRef } from 'react';

interface AsyncPollingOptions<T> {
  intervalMs: number;
  poll: () => Promise<T>;
  onResult: (result: T) => false | undefined;
  onError?: (error: unknown) => void;
}

interface AsyncPollingControls {
  start: () => void;
  stop: () => void;
}

interface PollingState {
  active: boolean;
  inFlightVersion: number | null;
  mounted: boolean;
  timer: ReturnType<typeof setTimeout> | null;
  version: number;
}

function isDocumentHidden(): boolean {
  return typeof document !== 'undefined' && document.visibilityState === 'hidden';
}

/**
 * Returns stable controls for a delayed polling loop that never overlaps requests.
 * Returning `false` from `onResult` stops the loop after that result is applied.
 */
export function useAsyncPolling<T>({
  intervalMs,
  poll,
  onResult,
  onError,
}: AsyncPollingOptions<T>): AsyncPollingControls {
  const stateRef = useRef<PollingState>({
    active: false,
    inFlightVersion: null,
    mounted: false,
    timer: null,
    version: 0,
  });
  const intervalMsRef = useRef(intervalMs);
  const pollRef = useRef(poll);
  const onResultRef = useRef(onResult);
  const onErrorRef = useRef(onError);
  const runRef = useRef<(version: number) => Promise<void>>(async () => undefined);

  intervalMsRef.current = intervalMs;
  pollRef.current = poll;
  onResultRef.current = onResult;
  onErrorRef.current = onError;

  const clearTimer = useCallback(() => {
    const state = stateRef.current;
    if (state.timer !== null) {
      clearTimeout(state.timer);
      state.timer = null;
    }
  }, []);

  const schedule = useCallback(
    (version: number) => {
      const state = stateRef.current;
      if (!state.mounted || !state.active || state.version !== version) return;
      clearTimer();
      if (isDocumentHidden()) return;
      state.timer = setTimeout(() => {
        state.timer = null;
        void runRef.current(version);
      }, intervalMsRef.current);
    },
    [clearTimer],
  );

  runRef.current = async (version: number) => {
    const state = stateRef.current;
    if (
      !state.mounted ||
      !state.active ||
      state.version !== version ||
      state.inFlightVersion === version ||
      isDocumentHidden()
    ) {
      return;
    }

    state.inFlightVersion = version;
    try {
      const result = await pollRef.current();
      if (!state.mounted || !state.active || state.version !== version) return;
      if (onResultRef.current(result) === false) {
        state.active = false;
        state.version += 1;
      }
    } catch (error) {
      if (state.mounted && state.active && state.version === version) {
        onErrorRef.current?.(error);
      }
    } finally {
      if (state.inFlightVersion === version) {
        state.inFlightVersion = null;
      }
      if (state.mounted && state.active && state.version === version && !isDocumentHidden()) {
        schedule(state.version);
      }
    }
  };

  const stop = useCallback(() => {
    const state = stateRef.current;
    state.active = false;
    state.version += 1;
    clearTimer();
  }, [clearTimer]);

  const start = useCallback(() => {
    const state = stateRef.current;
    state.active = true;
    state.version += 1;
    clearTimer();
    if (state.inFlightVersion !== state.version) {
      schedule(state.version);
    }
  }, [clearTimer, schedule]);

  useEffect(() => {
    const state = stateRef.current;
    state.mounted = true;

    const handleVisibilityChange = () => {
      const currentState = stateRef.current;
      if (isDocumentHidden()) {
        clearTimer();
      } else if (
        currentState.mounted &&
        currentState.active &&
        currentState.inFlightVersion !== currentState.version &&
        currentState.timer === null
      ) {
        schedule(currentState.version);
      }
    };

    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', handleVisibilityChange);
    }

    return () => {
      state.mounted = false;
      if (typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', handleVisibilityChange);
      }
      stop();
    };
  }, [clearTimer, schedule, stop]);

  return useMemo(() => ({ start, stop }), [start, stop]);
}
