import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

type ThemePreference = 'dark' | 'light' | 'system';
export type ResolvedTheme = 'dark' | 'light';

interface ThemeContextValue {
  theme: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: ThemePreference) => void;
}

const STORAGE_KEY = 'theia-theme';

const ThemeContext = createContext<ThemeContextValue | null>(null);

function getSystemTheme(): ResolvedTheme {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemePreference>(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'dark' || stored === 'light' || stored === 'system') return stored;
    return 'system';
  });

  const resolvedTheme = useMemo<ResolvedTheme>(() => {
    return theme === 'system' ? getSystemTheme() : theme;
  }, [theme]);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', resolvedTheme);
    document.documentElement.style.colorScheme = resolvedTheme;
  }, [resolvedTheme]);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, theme);
  }, [theme]);

  useEffect(() => {
    if (theme !== 'system') return;
    const mql = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = () => {
      setThemeState('system');
    };
    mql.addEventListener('change', handler);
    return () => mql.removeEventListener('change', handler);
  }, [theme]);

  const setTheme = (newTheme: ThemePreference) => setThemeState(newTheme);

  return (
    <ThemeContext.Provider value={{ theme, resolvedTheme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider');
  return ctx;
}

// Light-mode equivalents for the neon area color palette.
// Darkened for legibility against light surfaces.
const LIGHT_COLOR_MAP: Record<string, string> = {
  '#00E676': '#00804A',
  '#00e676': '#00804A',
  '#2979FF': '#1565C0',
  '#2979ff': '#1565C0',
  '#E040FB': '#9C27B0',
  '#e040fb': '#9C27B0',
  '#FFEA00': '#B8860B',
  '#ffea00': '#B8860B',
  '#FF6D00': '#D84315',
  '#ff6d00': '#D84315',
  '#00BCD4': '#00838F',
  '#00bcd4': '#00838F',
  '#FF1744': '#C62828',
  '#ff1744': '#C62828',
};

export function adaptAreaColor(hex: string, resolvedTheme: ResolvedTheme): string {
  if (resolvedTheme === 'dark') return hex;
  return LIGHT_COLOR_MAP[hex] ?? hex;
}
