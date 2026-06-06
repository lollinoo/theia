/**
 * Provides async stale guard utility behavior shared by frontend workflows.
 * Keeps non-UI policy and formatting rules reusable across components.
 */
export interface AsyncStaleGuard {
  isActive: () => boolean;
  cancel: () => void;
  run: <T>(callback: () => T) => T | undefined;
}

/** Creates async stale guard for the shared frontend utility layer. */
export function createAsyncStaleGuard(): AsyncStaleGuard {
  let active = true;

  return {
    isActive: () => active,
    cancel: () => {
      active = false;
    },
    run: <T>(callback: () => T) => {
      if (!active) return undefined;
      return callback();
    },
  };
}
