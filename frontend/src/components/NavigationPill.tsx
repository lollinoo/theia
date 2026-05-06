import { useEffect, useState } from 'react';
import type { ActiveView } from '../App';
import { fetchHealthVersion } from '../api/client';
import { adaptAreaColor, useTheme } from '../contexts/ThemeContext';
import type { Area } from '../types/api';
import { MaterialIcon } from './MaterialIcon';

interface NavigationPillProps {
  activeView: ActiveView;
  selectedAreaId: string | null;
  areas: Area[];
  onViewChange: (view: ActiveView) => void;
  onAreaSelect: (areaId: string | null) => void;
}

function NavigationPill({
  activeView,
  selectedAreaId,
  areas,
  onViewChange,
  onAreaSelect,
}: NavigationPillProps) {
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
    <div className="topology-glass topology-floating-shadow fixed left-1/2 top-4 z-30 flex w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] -translate-x-1/2 items-center gap-1 rounded-2xl px-2 py-2 transition-colors dark:backdrop-blur-[16px] sm:w-auto sm:max-w-[calc(100vw-1.5rem)] sm:rounded-full sm:px-3">
      {/* BRANDING */}
      <span className="px-1 text-sm font-semibold tracking-[0.14em] text-on-bg sm:px-2">THEIA</span>
      {version && version !== 'unknown' && (
        <span className="hidden text-[11px] font-medium text-on-bg-secondary sm:inline">
          {`v${version}`}
        </span>
      )}

      <div className="mx-1 hidden h-5 w-px bg-outline/40 sm:block" />

      {/* HUB ICON */}
      <button
        type="button"
        onClick={() => onViewChange('hub')}
        className={`flex items-center rounded-full border px-2 py-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg sm:px-3 ${
          isHub
            ? 'border-outline-strong bg-surface-container-high font-semibold text-on-bg shadow-pill'
            : 'border-transparent text-on-bg-secondary hover:bg-surface-container hover:text-on-bg'
        }`}
        aria-label="Area Hub"
        title="Area Hub"
      >
        <MaterialIcon name="hub" size={20} />
      </button>

      {/* GLOBAL + AREA BUTTONS */}
      {isDashboard ? (
        <span className="flex-1 px-3 py-2 text-sm font-semibold text-on-bg whitespace-nowrap">
          Devices
        </span>
      ) : (
        <>
          <select
            aria-label="Area selector"
            value={selectedAreaId ?? '__global__'}
            onChange={(event) =>
              onAreaSelect(event.target.value === '__global__' ? null : event.target.value)
            }
            className="h-10 min-w-0 flex-1 rounded-full border border-outline-subtle bg-surface-container-high px-3 text-sm text-on-bg shadow-pill outline-none transition-colors focus:ring-2 focus:ring-focus-ring sm:hidden"
          >
            <option value="__global__">Global</option>
            {areas.map((area) => (
              <option key={area.id} value={area.id}>
                {area.name}
              </option>
            ))}
          </select>
          <div
            data-testid="desktop-area-selector"
            className="hidden max-w-[56vw] items-center gap-1 overflow-x-auto sm:flex"
          >
            <button
              type="button"
              onClick={() => onAreaSelect(null)}
              className={`rounded-full border px-3 py-2 text-sm whitespace-nowrap transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg ${
                isGlobal
                  ? 'border-outline-strong bg-surface-container-high font-semibold text-on-bg shadow-pill'
                  : 'border-transparent text-on-bg-secondary hover:bg-surface-container hover:text-on-bg'
              }`}
            >
              Global
            </button>

            {areas.map((area) => {
              const isActive = activeView === 'canvas' && selectedAreaId === area.id;
              return (
                <button
                  key={area.id}
                  type="button"
                  onClick={() => onAreaSelect(area.id)}
                  className={`flex items-center gap-1.5 rounded-full border px-3 py-2 text-sm whitespace-nowrap transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg ${
                    isActive
                      ? 'border-outline-strong bg-surface-container-high font-semibold text-on-bg shadow-pill'
                      : 'border-transparent text-on-bg-secondary hover:bg-surface-container hover:text-on-bg'
                  }`}
                >
                  <span
                    className="h-2 w-2 flex-shrink-0 rounded-full"
                    style={{
                      backgroundColor: adaptAreaColor(area.color, resolvedTheme),
                      boxShadow: isActive
                        ? `0 0 8px ${adaptAreaColor(area.color, resolvedTheme)}`
                        : undefined,
                    }}
                  />
                  {area.name}
                </button>
              );
            })}
          </div>
        </>
      )}

      <div className="mx-1 hidden h-5 w-px bg-outline/40 sm:block" />

      {/* DEVICES ICON */}
      <button
        type="button"
        onClick={() => onViewChange('dashboard')}
        className={`flex items-center rounded-full border px-2 py-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg sm:px-3 ${
          isDashboard
            ? 'border-outline-strong bg-surface-container-high font-semibold text-on-bg shadow-pill'
            : 'border-transparent text-on-bg-secondary hover:bg-surface-container hover:text-on-bg'
        }`}
        aria-label="Devices Dashboard"
        title="Devices"
      >
        <MaterialIcon name="devices" size={20} />
      </button>

      {/* THEME TOGGLE */}
      <button
        type="button"
        onClick={toggleTheme}
        className="flex items-center rounded-full border border-transparent px-2 py-2 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg sm:px-3"
        aria-label={resolvedTheme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
        title={resolvedTheme === 'dark' ? 'Light mode' : 'Dark mode'}
      >
        <MaterialIcon name={resolvedTheme === 'dark' ? 'light_mode' : 'dark_mode'} size={20} />
      </button>
    </div>
  );
}

export default NavigationPill;
