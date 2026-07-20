/**
 * Exercises the persisted canvas snap preference so reloads retain the user's movement mode.
 */
import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { canvasSnapPreferenceStorageKey, useCanvasSnapPreference } from './useCanvasSnapPreference';

afterEach(() => {
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
});
