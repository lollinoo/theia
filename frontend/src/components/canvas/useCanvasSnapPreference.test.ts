/**
 * Exercises the persisted canvas snap preference so reloads retain the user's movement mode.
 */
import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { canvasSnapPreferenceStorageKey, useCanvasSnapPreference } from './useCanvasSnapPreference';

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
});

describe('useCanvasSnapPreference', () => {
  it('defaults snapping on when no preference has been stored', () => {
    const { result } = renderHook(() => useCanvasSnapPreference());

    expect(result.current.snapToGrid).toBe(true);
  });

  it('loads a stored false preference', () => {
    window.localStorage.setItem(canvasSnapPreferenceStorageKey, 'false');

    const { result } = renderHook(() => useCanvasSnapPreference());

    expect(result.current.snapToGrid).toBe(false);
  });

  it('defaults snapping on when reading storage throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage unavailable');
    });

    const { result } = renderHook(() => useCanvasSnapPreference());

    expect(result.current.snapToGrid).toBe(true);
  });

  it('toggles and persists the preference', () => {
    const { result } = renderHook(() => useCanvasSnapPreference());

    act(() => {
      result.current.toggleSnapToGrid();
    });

    expect(result.current.snapToGrid).toBe(false);
    expect(window.localStorage.getItem(canvasSnapPreferenceStorageKey)).toBe('false');

    act(() => {
      result.current.toggleSnapToGrid();
    });

    expect(result.current.snapToGrid).toBe(true);
    expect(window.localStorage.getItem(canvasSnapPreferenceStorageKey)).toBe('true');
  });

  it('keeps toggled state when writing storage throws', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage unavailable');
    });
    const { result } = renderHook(() => useCanvasSnapPreference());

    expect(() => {
      act(() => {
        result.current.toggleSnapToGrid();
      });
    }).not.toThrow();

    expect(result.current.snapToGrid).toBe(false);
  });
});
