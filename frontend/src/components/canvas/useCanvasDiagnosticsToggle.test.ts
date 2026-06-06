import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import {
  canvasDiagnosticsStorageKey,
  initialCanvasDiagnosticsVisible,
  isCanvasDiagnosticsShortcut,
  useCanvasDiagnosticsToggle,
} from './useCanvasDiagnosticsToggle';

afterEach(() => {
  window.localStorage.clear();
  window.history.replaceState(null, '', '/');
});

describe('canvas diagnostics toggle', () => {
  it('persists query-param activation for subsequent loads', () => {
    window.history.replaceState(null, '', '/?canvasDiagnostics=1');

    expect(initialCanvasDiagnosticsVisible()).toBe(true);
    expect(window.localStorage.getItem(canvasDiagnosticsStorageKey)).toBe('true');
  });

  it('toggles diagnostics from the keyboard shortcut and persists the state', () => {
    const { result } = renderHook(() => useCanvasDiagnosticsToggle());

    expect(result.current.diagnosticsVisible).toBe(false);

    act(() => {
      window.dispatchEvent(
        new KeyboardEvent('keydown', {
          altKey: true,
          ctrlKey: true,
          code: 'KeyD',
          key: 'd',
        }),
      );
    });

    expect(result.current.diagnosticsVisible).toBe(true);
    expect(window.localStorage.getItem(canvasDiagnosticsStorageKey)).toBe('true');

    act(() => {
      result.current.closeDiagnostics();
    });

    expect(result.current.diagnosticsVisible).toBe(false);
    expect(window.localStorage.getItem(canvasDiagnosticsStorageKey)).toBe('false');
  });

  it('matches ctrl/meta alt D shortcuts only', () => {
    expect(
      isCanvasDiagnosticsShortcut(
        new KeyboardEvent('keydown', { altKey: true, metaKey: true, code: 'KeyD', key: 'd' }),
      ),
    ).toBe(true);
    expect(
      isCanvasDiagnosticsShortcut(
        new KeyboardEvent('keydown', { altKey: true, code: 'KeyD', key: 'd' }),
      ),
    ).toBe(false);
  });
});
