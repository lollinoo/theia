/**
 * Owns debounced setting persistence, per-key request sequencing, and transient save status.
 */
import { useCallback, useEffect, useRef, useState } from 'react';

const DEFAULT_DEBOUNCE_MS = 500;
const DEFAULT_SAVED_DURATION_MS = 2000;
const SAVE_ERROR_MESSAGE = 'Failed to save setting. Please try again.';

/** Current persistence state exposed for one setting key. */
export interface SettingSaveState {
  status: 'idle' | 'saving' | 'saved' | 'error';
  error: string | null;
}

/** Per-save timing and success behavior for immediate or debounced controls. */
export interface SaveSettingOptions {
  delayMs?: number;
  onSuccess?: () => void;
}

interface SettingRequest {
  version: number;
  debounceTimer: number | null;
  savedTimer: number | null;
  inFlight: boolean;
  pending: (() => void) | null;
}

interface UseSettingAutosaveResult {
  states: Readonly<Record<string, SettingSaveState>>;
  save: (key: string, value: string, options?: SaveSettingOptions) => void;
  cancel: (key: string) => void;
}

/**
 * Persists settings with one independent debounce and request sequence per key.
 * Superseded and unmounted requests are still allowed to settle, but cannot mutate UI state.
 */
export function useSettingAutosave(
  persist: (key: string, value: string) => Promise<unknown>,
): UseSettingAutosaveResult {
  const [states, setStates] = useState<Record<string, SettingSaveState>>({});
  const requestsRef = useRef(new Map<string, SettingRequest>());
  const mountedRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;
    const requests = requestsRef.current;
    return () => {
      mountedRef.current = false;
      for (const request of requests.values()) {
        if (request.debounceTimer !== null) window.clearTimeout(request.debounceTimer);
        if (request.savedTimer !== null) window.clearTimeout(request.savedTimer);
        request.pending = null;
      }
      requests.clear();
    };
  }, []);

  const getRequest = useCallback((key: string): SettingRequest => {
    const current = requestsRef.current.get(key);
    if (current) return current;
    const request: SettingRequest = {
      version: 0,
      debounceTimer: null,
      savedTimer: null,
      inFlight: false,
      pending: null,
    };
    requestsRef.current.set(key, request);
    return request;
  }, []);

  const cancel = useCallback(
    (key: string) => {
      const request = getRequest(key);
      request.version += 1;
      if (request.debounceTimer !== null) window.clearTimeout(request.debounceTimer);
      if (request.savedTimer !== null) window.clearTimeout(request.savedTimer);
      request.debounceTimer = null;
      request.savedTimer = null;
      request.pending = null;
      if (mountedRef.current) {
        setStates((current) => ({
          ...current,
          [key]: { status: 'idle', error: null },
        }));
      }
    },
    [getRequest],
  );

  const save = useCallback(
    (key: string, value: string, options: SaveSettingOptions = {}) => {
      const request = getRequest(key);
      request.version += 1;
      const version = request.version;
      if (request.debounceTimer !== null) window.clearTimeout(request.debounceTimer);
      if (request.savedTimer !== null) window.clearTimeout(request.savedTimer);
      request.debounceTimer = null;
      request.savedTimer = null;
      request.pending = null;
      setStates((current) => ({
        ...current,
        [key]: { status: 'saving', error: null },
      }));

      const persistLatest = () => {
        request.debounceTimer = null;
        if (request.inFlight) {
          request.pending = persistLatest;
          return;
        }
        request.pending = null;
        request.inFlight = true;

        const settle = (succeeded: boolean) => {
          request.inFlight = false;
          if (mountedRef.current && request.version === version) {
            if (succeeded) {
              setStates((current) => ({
                ...current,
                [key]: { status: 'saved', error: null },
              }));
              options.onSuccess?.();
              request.savedTimer = window.setTimeout(() => {
                if (!mountedRef.current || request.version !== version) return;
                request.savedTimer = null;
                setStates((current) => ({
                  ...current,
                  [key]: { status: 'idle', error: null },
                }));
              }, DEFAULT_SAVED_DURATION_MS);
            } else {
              setStates((current) => ({
                ...current,
                [key]: { status: 'error', error: SAVE_ERROR_MESSAGE },
              }));
            }
          }

          const pending = request.pending;
          request.pending = null;
          pending?.();
        };

        void Promise.resolve()
          .then(() => persist(key, value))
          .then(
            () => settle(true),
            () => settle(false),
          );
      };

      const delayMs = options.delayMs ?? DEFAULT_DEBOUNCE_MS;
      if (delayMs === 0) {
        persistLatest();
      } else {
        request.debounceTimer = window.setTimeout(persistLatest, delayMs);
      }
    },
    [getRequest, persist],
  );

  return { states, save, cancel };
}
