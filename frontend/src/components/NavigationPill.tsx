import { useCallback, useEffect, useRef, useState } from 'react';
import type { ActiveView } from '../App';
import { fetchHealthVersion } from '../api/client';
import { adaptAreaColor, useTheme } from '../contexts/ThemeContext';
import type { Area, CanvasMap } from '../types/api';
import { MapSelector } from './MapSelector';
import { MaterialIcon } from './MaterialIcon';

interface NavigationPillProps {
  activeView: ActiveView;
  selectedAreaId: string | null;
  selectedMapId: string | null;
  selectedMapName: string;
  maps: CanvasMap[];
  areas: Area[];
  onViewChange: (view: ActiveView) => void;
  onAreaSelect: (areaId: string | null) => void;
  onMapSelect: (map: CanvasMap) => void;
  onManageMaps: () => void;
}

function NavigationPill({
  activeView,
  selectedAreaId,
  selectedMapId,
  selectedMapName,
  maps,
  areas,
  onViewChange,
  onAreaSelect,
  onMapSelect,
  onManageMaps,
}: NavigationPillProps) {
  const [version, setVersion] = useState('');
  const areaScrollerRef = useRef<HTMLDivElement>(null);
  const [areaScrollState, setAreaScrollState] = useState({
    hasOverflow: false,
    canScrollLeft: false,
    canScrollRight: false,
  });
  const { resolvedTheme, setTheme } = useTheme();

  useEffect(() => {
    fetchHealthVersion().then((v) => setVersion(v.version));
  }, []);

  const toggleTheme = () => {
    setTheme(resolvedTheme === 'dark' ? 'light' : 'dark');
  };

  const isHub = activeView === 'hub';
  const isAllAreas = activeView === 'canvas' && selectedAreaId === null;
  const isDashboard = activeView === 'dashboard';

  const updateAreaScrollState = useCallback(() => {
    const scroller = areaScrollerRef.current;
    if (!scroller) {
      setAreaScrollState({ hasOverflow: false, canScrollLeft: false, canScrollRight: false });
      return;
    }

    const maxScrollLeft = Math.max(0, scroller.scrollWidth - scroller.clientWidth);
    const nextState = {
      hasOverflow: maxScrollLeft > 1,
      canScrollLeft: scroller.scrollLeft > 1,
      canScrollRight: scroller.scrollLeft < maxScrollLeft - 1,
    };
    setAreaScrollState((current) => {
      if (
        current.hasOverflow === nextState.hasOverflow &&
        current.canScrollLeft === nextState.canScrollLeft &&
        current.canScrollRight === nextState.canScrollRight
      ) {
        return current;
      }
      return nextState;
    });
  }, []);

  useEffect(() => {
    updateAreaScrollState();
    const scroller = areaScrollerRef.current;
    if (!scroller) return;

    scroller.addEventListener('scroll', updateAreaScrollState, { passive: true });
    window.addEventListener('resize', updateAreaScrollState);
    const resizeObserver =
      typeof ResizeObserver !== 'undefined' ? new ResizeObserver(updateAreaScrollState) : null;
    resizeObserver?.observe(scroller);

    return () => {
      scroller.removeEventListener('scroll', updateAreaScrollState);
      window.removeEventListener('resize', updateAreaScrollState);
      resizeObserver?.disconnect();
    };
  }, [areas.length, updateAreaScrollState]);

  const scrollAreaSelector = useCallback(
    (direction: -1 | 1) => {
      const scroller = areaScrollerRef.current;
      if (!scroller) return;
      scroller.scrollBy({
        left: direction * Math.round(Math.max(144, scroller.clientWidth * 0.65)),
        behavior: 'smooth',
      });
      window.setTimeout(updateAreaScrollState, 180);
    },
    [updateAreaScrollState],
  );

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
        aria-label="Topology Hub"
        title="Topology Hub"
      >
        <MaterialIcon name="hub" size={20} />
      </button>

      {/* MAP + AREA BUTTONS */}
      {isDashboard ? (
        <span className="flex-1 px-3 py-2 text-sm font-semibold text-on-bg whitespace-nowrap">
          Devices
        </span>
      ) : (
        <>
          <MapSelector
            maps={maps}
            selectedMapId={selectedMapId}
            selectedMapName={selectedMapName}
            onSelectMap={onMapSelect}
            onManageMaps={onManageMaps}
            placement="inline"
          />
          <select
            aria-label="Area selector"
            value={selectedAreaId ?? '__all__'}
            onChange={(event) =>
              onAreaSelect(event.target.value === '__all__' ? null : event.target.value)
            }
            className="h-10 min-w-[7rem] flex-1 rounded-full border border-outline-subtle bg-surface-container-high px-3 text-sm text-on-bg shadow-pill outline-none transition-colors focus:ring-2 focus:ring-focus-ring sm:hidden"
          >
            <option value="__all__">All areas</option>
            {areas.map((area) => (
              <option key={area.id} value={area.id}>
                {area.name}
              </option>
            ))}
          </select>
          <div
            data-testid="desktop-area-selector"
            className="hidden max-w-[56vw] min-w-0 items-center gap-1 sm:flex"
          >
            {areaScrollState.hasOverflow && (
              <button
                type="button"
                aria-label="Scroll areas left"
                disabled={!areaScrollState.canScrollLeft}
                onClick={() => scrollAreaSelector(-1)}
                className="inline-flex h-8 w-8 flex-none items-center justify-center rounded-full border border-outline-subtle bg-surface-container-high text-on-bg-secondary shadow-pill transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-35"
              >
                <span
                  aria-hidden="true"
                  className="h-2 w-2 rotate-45 border-b-2 border-l-2 border-current"
                />
              </button>
            )}
            <div
              ref={areaScrollerRef}
              data-testid="desktop-area-selector-scroll"
              className="topology-scrollbar-none flex min-w-0 flex-1 items-center gap-1 overflow-x-auto scroll-smooth"
            >
              <button
                type="button"
                onClick={() => onAreaSelect(null)}
                className={`rounded-full border px-3 py-2 text-sm whitespace-nowrap transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg ${
                  isAllAreas
                    ? 'border-outline-strong bg-surface-container-high font-semibold text-on-bg shadow-pill'
                    : 'border-transparent text-on-bg-secondary hover:bg-surface-container hover:text-on-bg'
                }`}
              >
                All areas
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
            {areaScrollState.hasOverflow && (
              <button
                type="button"
                aria-label="Scroll areas right"
                disabled={!areaScrollState.canScrollRight}
                onClick={() => scrollAreaSelector(1)}
                className="inline-flex h-8 w-8 flex-none items-center justify-center rounded-full border border-outline-subtle bg-surface-container-high text-on-bg-secondary shadow-pill transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-35"
              >
                <span
                  aria-hidden="true"
                  className="h-2 w-2 rotate-45 border-t-2 border-r-2 border-current"
                />
              </button>
            )}
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
