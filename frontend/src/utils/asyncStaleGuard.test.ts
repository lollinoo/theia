/**
 * Exercises async stale guard utility behavior so refactors preserve the documented contract.
 */
import { describe, expect, it } from 'vitest';

import { createAsyncStaleGuard } from './asyncStaleGuard';

describe('createAsyncStaleGuard', () => {
  it('starts active and runs callbacks until cancelled', () => {
    const guard = createAsyncStaleGuard();

    expect(guard.isActive()).toBe(true);
    expect(guard.run(() => 'ready')).toBe('ready');

    guard.cancel();

    expect(guard.isActive()).toBe(false);
    expect(guard.run(() => 'stale')).toBeUndefined();
  });

  it('allows cancel to be called repeatedly', () => {
    const guard = createAsyncStaleGuard();

    guard.cancel();
    guard.cancel();

    expect(guard.isActive()).toBe(false);
  });
});
