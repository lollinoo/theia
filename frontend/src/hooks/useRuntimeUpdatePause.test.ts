/**
 * Exercises use runtime update pause hook lifecycle behavior so refactors preserve the documented contract.
 */
import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useRuntimeUpdatePause } from './useRuntimeUpdatePause';

describe('useRuntimeUpdatePause', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('keeps runtime updates paused until the interaction idle delay elapses', () => {
    const { result, rerender } = renderHook(
      ({ active }: { active: boolean }) => useRuntimeUpdatePause(active),
      { initialProps: { active: false } },
    );

    expect(result.current).toBe(false);

    rerender({ active: true });
    expect(result.current).toBe(true);

    rerender({ active: false });
    expect(result.current).toBe(true);

    act(() => {
      vi.advanceTimersByTime(1499);
    });
    expect(result.current).toBe(true);

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(result.current).toBe(false);
  });
});
