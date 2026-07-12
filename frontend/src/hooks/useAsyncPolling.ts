/**
 * Coordinates a single non-overlapping asynchronous polling loop.
 */
import { useCallback, useEffect, useRef } from 'react';

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
  inFlight: boolean;
  mounted: boolean;
  timer: ReturnType<typeof setTimeout> | null;
  version: number;
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
    inFlight: false,
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
      state.timer = setTimeout(() => {
        state.timer = null;
        void runRef.current(version);
      }, intervalMsRef.current);
    },
    [clearTimer],
  );

  runRef.current = async (version: number) => {
    const state = stateRef.current;
    if (!state.mounted || !state.active || state.version !== version || state.inFlight) return;

    state.inFlight = true;
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
      state.inFlight = false;
      if (state.mounted && state.active) {
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
    if (!state.inFlight) schedule(state.version);
  }, [clearTimer, schedule]);

  useEffect(() => {
    const state = stateRef.current;
    state.mounted = true;
    return () => {
      state.mounted = false;
      stop();
    };
  }, [stop]);

  return { start, stop };
}
