import { useEffect, useState } from 'react';
import { fetchHealthVersion } from '../api/client';
import { useTheme, adaptAreaColor } from '../contexts/ThemeContext';
import { MaterialIcon } from './MaterialIcon';
import type { ActiveView } from '../App';
import type { Area } from '../types/api';

interface NavigationPillProps {
  activeView: ActiveView;
  selectedAreaId: string | null;
  areas: Area[];
  onViewChange: (view: ActiveView) => void;
  onAreaSelect: (areaId: string | null) => void;
}

/** Floating navigation pill replacing NavBar. Glassmorphism dark, solid tinted light. */
function NavigationPill({ activeView, selectedAreaId, areas, onViewChange, onAreaSelect }: NavigationPillProps) {
  const [version, setVersion] = useState('');
  const { resolvedTheme, setTheme } = useTheme();

  useEffect(() => {
    fetchHealthVersion().then((v) => setVersion(v.version));
  }, []);

  const toggleTheme = () => {
    setTheme(resolvedTheme === 'dark' ? 'light' : 'dark');
  };

  const isHub = activeView === 'hub';
  const isGlobal = activeView === 'canvas' && selectedAreaId === null;
  const isDashboard = activeView === 'dashboard';

  return (
    <div className="fixed top-4 left-1/2 -translate-x-1/2 z-30 flex items-center gap-1 rounded-full border border-glass-border bg-glass-bg px-3 py-1.5 shadow-lg dark:backdrop-blur-[16px] transition-colors">
      {/* BRANDING */}
      <span className="px-2 text-sm font-semibold text-on-bg tracking-wide">THEIA</span>
      {version && version !== 'unknown' && (
        <span className="text-[11px] text-on-bg-secondary/50">{`v${version}`}</span>
      )}

      <div className="h-5 w-px bg-outline/40 mx-1" />

      {/* HUB ICON */}
      <button
        onClick={() => onViewChange('hub')}
        className={`rounded-full p-2 flex items-center transition-colors ${
          isHub
            ? 'bg-surface-high text-on-bg'
            : 'text-on-bg-secondary hover:text-on-bg hover:bg-surface-high'
        }`}
        aria-label="Area Hub"
        title="Area Hub"
      >
        <MaterialIcon name="hub" size={20} />
      </button>

      {/* GLOBAL + AREA BUTTONS */}
      {isDashboard ? (
        <span className="px-3 py-2 text-sm font-semibold text-on-bg whitespace-nowrap">
          Devices
        </span>
      ) : (
        <div className="flex items-center gap-1">
          <button
            onClick={() => onAreaSelect(null)}
            className={`px-3 py-2 rounded-full text-sm whitespace-nowrap transition-colors ${
              isGlobal
                ? 'bg-surface-high text-on-bg font-semibold'
                : 'text-on-bg-secondary hover:text-on-bg hover:bg-surface-high'
            }`}
          >
            Global
          </button>

          {areas.map((area) => {
            const isActive = activeView === 'canvas' && selectedAreaId === area.id;
            return (
              <button
                key={area.id}
                onClick={() => onAreaSelect(area.id)}
                className={`px-3 py-2 rounded-full flex items-center gap-1.5 text-sm whitespace-nowrap transition-colors ${
                  isActive
                    ? 'bg-surface-high text-on-bg font-semibold'
                    : 'text-on-bg-secondary hover:text-on-bg hover:bg-surface-high'
                }`}
              >
                <span
                  className="w-2 h-2 rounded-full flex-shrink-0"
                  style={{
                    backgroundColor: adaptAreaColor(area.color, resolvedTheme),
                    boxShadow: isActive ? `0 0 8px ${adaptAreaColor(area.color, resolvedTheme)}` : undefined,
                  }}
                />
                {area.name}
              </button>
            );
          })}
        </div>
      )}

      <div className="h-5 w-px bg-outline/40 mx-1" />

      {/* DEVICES ICON */}
      <button
        onClick={() => onViewChange('dashboard')}
        className={`rounded-full p-2 flex items-center transition-colors ${
          isDashboard
            ? 'bg-surface-high text-on-bg'
            : 'text-on-bg-secondary hover:text-on-bg hover:bg-surface-high'
        }`}
        aria-label="Devices Dashboard"
        title="Devices"
      >
        <MaterialIcon name="devices" size={20} />
      </button>

      {/* THEME TOGGLE */}
      <button
        onClick={toggleTheme}
        className="rounded-full p-2 flex items-center text-on-bg-secondary hover:text-on-bg hover:bg-surface-high transition-colors"
        aria-label={resolvedTheme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
        title={resolvedTheme === 'dark' ? 'Light mode' : 'Dark mode'}
      >
        <MaterialIcon
          name={resolvedTheme === 'dark' ? 'light_mode' : 'dark_mode'}
          size={20}
        />
      </button>
    </div>
  );
}

export default NavigationPill;
