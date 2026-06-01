export interface AsyncStaleGuard {
  isActive: () => boolean;
  cancel: () => void;
  run: <T>(callback: () => T) => T | undefined;
}

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
