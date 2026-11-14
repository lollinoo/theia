/**
 * Exercises theme context shared context behavior so refactors preserve the documented contract.
 */
import { act, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { requireThemeContext, ThemeProvider, useTheme } from './ThemeContext';

function TestConsumer() {
  const { theme, resolvedTheme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="resolved">{resolvedTheme}</span>
      <button type="button" data-testid="toggle-light" onClick={() => setTheme('light')}>
        Light
      </button>
      <button type="button" data-testid="toggle-dark" onClick={() => setTheme('dark')}>
        Dark
      </button>
      <button type="button" data-testid="toggle-system" onClick={() => setTheme('system')}>
        System
      </button>
    </div>
  );
}

function mockMatchMedia(prefersDark: boolean) {
  const listeners: Array<(e: { matches: boolean }) => void> = [];
  const mql = {
    matches: prefersDark,
    media: '(prefers-color-scheme: dark)',
    addEventListener: vi.fn((_event: string, handler: (e: { matches: boolean }) => void) => {
      listeners.push(handler);
    }),
    removeEventListener: vi.fn((_event: string, handler: (e: { matches: boolean }) => void) => {
      const idx = listeners.indexOf(handler);
      if (idx >= 0) listeners.splice(idx, 1);
    }),
    dispatchEvent: vi.fn(),
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
  };
  window.matchMedia = vi.fn().mockReturnValue(mql);
  return { mql, listeners };
}

describe('ThemeProvider', () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute('data-theme');
    document.documentElement.style.colorScheme = '';
    vi.restoreAllMocks();
    // Default: OS prefers dark
    mockMatchMedia(true);
  });

  // --- THEME-01: Toggle behavior ---

  it('defaults to system theme preference when no localStorage value', () => {
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    );

    expect(screen.getByTestId('theme').textContent).toBe('system');
  });

  it('toggles from dark to light when setTheme("light") is called', () => {
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    );

    act(() => {
      screen.getByTestId('toggle-light').click();
    });

    expect(screen.getByTestId('resolved').textContent).toBe('light');
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
  });

  it('toggles from light to dark when setTheme("dark") is called', () => {
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    );

    act(() => {
      screen.getByTestId('toggle-light').click();
    });
    expect(screen.getByTestId('resolved').textContent).toBe('light');

    act(() => {
      screen.getByTestId('toggle-dark').click();
    });
    expect(screen.getByTestId('resolved').textContent).toBe('dark');
  });

  // --- THEME-02: Persistence ---

  it('persists theme choice to localStorage under "theia-theme" key', () => {
    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    );

    act(() => {
      screen.getByTestId('toggle-light').click();
    });

    expect(localStorage.getItem('theia-theme')).toBe('light');
  });

  it('reads stored theme from localStorage on mount', () => {
    localStorage.setItem('theia-theme', 'light');

    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    );

    expect(screen.getByTestId('theme').textContent).toBe('light');
    expect(screen.getByTestId('resolved').textContent).toBe('light');
  });

  // --- THEME-03: OS preference detection ---

  it('resolves "system" to "dark" when OS prefers dark', () => {
    mockMatchMedia(true);

    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    );

    expect(screen.getByTestId('theme').textContent).toBe('system');
    expect(screen.getByTestId('resolved').textContent).toBe('dark');
  });

  it('resolves "system" to "light" when OS prefers light', () => {
    mockMatchMedia(false);

    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    );

    expect(screen.getByTestId('theme').textContent).toBe('system');
    expect(screen.getByTestId('resolved').textContent).toBe('light');
  });

  it('updates resolvedTheme and document.documentElement dynamically when prefers-color-scheme changes under system theme', () => {
    const { mql, listeners } = mockMatchMedia(true);

    render(
      <ThemeProvider>
        <TestConsumer />
      </ThemeProvider>,
    );

    expect(screen.getByTestId('theme').textContent).toBe('system');
    expect(screen.getByTestId('resolved').textContent).toBe('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    expect(document.documentElement.style.colorScheme).toBe('dark');

    // Simulate live OS color scheme change: dark -> light
    act(() => {
      mql.matches = false;
      for (const listener of listeners) {
        listener({ matches: false });
      }
    });

    expect(screen.getByTestId('theme').textContent).toBe('system');
    expect(screen.getByTestId('resolved').textContent).toBe('light');
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    expect(document.documentElement.style.colorScheme).toBe('light');

    // Simulate live OS color scheme change: light -> dark
    act(() => {
      mql.matches = true;
      for (const listener of listeners) {
        listener({ matches: true });
      }
    });

    expect(screen.getByTestId('theme').textContent).toBe('system');
    expect(screen.getByTestId('resolved').textContent).toBe('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    expect(document.documentElement.style.colorScheme).toBe('dark');
  });

  // --- THEME-04: Context value memoization ---

  it('preserves the exact same context value reference across re-renders when theme and resolvedTheme are unchanged', () => {
    let capturedContext: ReturnType<typeof useTheme> | null = null;
    let renderCount = 0;

    function ValueInspector() {
      capturedContext = useTheme();
      renderCount += 1;
      return null;
    }

    const { rerender } = render(
      <ThemeProvider>
        <ValueInspector />
      </ThemeProvider>,
    );

    const initialContext = capturedContext;
    expect(initialContext).not.toBeNull();
    expect(renderCount).toBe(1);

    // Re-render ThemeProvider without theme changes
    rerender(
      <ThemeProvider>
        <ValueInspector />
      </ThemeProvider>,
    );

    expect(renderCount).toBe(2);
    expect(Object.is(capturedContext, initialContext)).toBe(true);
    expect(capturedContext?.setTheme).toBe(initialContext?.setTheme);
  });

  it('updates context value reference only when theme changes, maintaining stable setTheme callback', () => {
    let capturedContext: ReturnType<typeof useTheme> | null = null;

    function ValueInspector() {
      capturedContext = useTheme();
      return (
        <button
          type="button"
          data-testid="set-dark"
          onClick={() => capturedContext?.setTheme('dark')}
        >
          Dark
        </button>
      );
    }

    mockMatchMedia(false);

    render(
      <ThemeProvider>
        <ValueInspector />
      </ThemeProvider>,
    );

    const initialContext = capturedContext;
    expect(initialContext?.theme).toBe('system');
    expect(initialContext?.resolvedTheme).toBe('light');

    act(() => {
      screen.getByTestId('set-dark').click();
    });

    expect(Object.is(capturedContext, initialContext)).toBe(false);
    expect(capturedContext?.theme).toBe('dark');
    expect(capturedContext?.resolvedTheme).toBe('dark');
    expect(capturedContext?.setTheme).toBe(initialContext?.setTheme);
  });

  // --- Error boundary ---

  it('throws when useTheme is called outside ThemeProvider', () => {
    expect(() => requireThemeContext(null)).toThrow('useTheme must be used within ThemeProvider');
  });
});
