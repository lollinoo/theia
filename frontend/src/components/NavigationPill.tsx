import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
  canViewAdmin?: boolean;
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
  canViewAdmin = false,
  onViewChange,
  onAreaSelect,
  onMapSelect,
  onManageMaps,
}: NavigationPillProps) {
  const [version, setVersion] = useState('');
  const areaScrollerRef = useRef<HTMLDivElement>(null);
  const areaScrollUpdateTimerRef = useRef<number | null>(null);
  const [areaScrollState, setAreaScrollState] = useState({
    hasOverflow: false,
    canScrollLeft: false,
    canScrollRight: false,
  });
  const [areaOverflowOpen, setAreaOverflowOpen] = useState(false);
  const areaOverflowRef = useRef<HTMLDivElement | null>(null);
  const { resolvedTheme, setTheme } = useTheme();

  useEffect(() => {
    fetchHealthVersion().then((v) => setVersion(v.version));
  }, []);

  const toggleTheme = () => {
    setTheme(resolvedTheme === 'dark' ? 'light' : 'dark');
  };

  const isHub = activeView === 'hub';
  const isTopologyContextView = activeView === 'canvas' || activeView === 'dashboard';
  const isAllAreas = isTopologyContextView && selectedAreaId === null;
  const isDashboard = activeView === 'dashboard';
  const isAdmin = activeView === 'admin';
  const maxInlineAreas = 3;
  const { inlineAreas, overflowAreas } = useMemo(
    () => ({
      inlineAreas: areas.slice(0, maxInlineAreas),
      overflowAreas: areas.slice(maxInlineAreas),
    }),
    [areas],
  );

  const updateAreaScrollState = useCallback(() => {
    const scroller = areaScrollerRef.current;
    if (!scroller) {
      setAreaScrollState({
        hasOverflow: false,
        canScrollLeft: false,
        canScrollRight: false,
      });
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

    scroller.addEventListener('scroll', updateAreaScrollState, {
      passive: true,
    });
    window.addEventListener('resize', updateAreaScrollState);
    const resizeObserver =
      typeof ResizeObserver !== 'undefined' ? new ResizeObserver(updateAreaScrollState) : null;
    resizeObserver?.observe(scroller);

    return () => {
      scroller.removeEventListener('scroll', updateAreaScrollState);
      window.removeEventListener('resize', updateAreaScrollState);
      resizeObserver?.disconnect();
      if (areaScrollUpdateTimerRef.current !== null) {
        window.clearTimeout(areaScrollUpdateTimerRef.current);
        areaScrollUpdateTimerRef.current = null;
      }
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
      if (areaScrollUpdateTimerRef.current !== null) {
        window.clearTimeout(areaScrollUpdateTimerRef.current);
      }
      areaScrollUpdateTimerRef.current = window.setTimeout(() => {
        areaScrollUpdateTimerRef.current = null;
        updateAreaScrollState();
      }, 180);
    },
    [updateAreaScrollState],
  );

  useEffect(() => {
    if (!areaOverflowOpen) return;

    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Node) || !areaOverflowRef.current?.contains(target)) {
        setAreaOverflowOpen(false);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setAreaOverflowOpen(false);
      }
    };

    document.addEventListener('pointerdown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [areaOverflowOpen]);

  return (
    <div className="topology-glass topology-floating-shadow fixed left-1/2 top-4 z-30 flex w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] -translate-x-1/2 flex-wrap items-center justify-center gap-1 rounded-2xl px-2 py-2 transition-colors dark:backdrop-blur-[16px] sm:w-auto sm:max-w-[calc(100vw-1.5rem)] sm:flex-nowrap sm:justify-start sm:rounded-full sm:px-3">
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
      <div
        data-testid="mobile-map-area-controls"
        className="order-last flex w-full min-w-0 items-center justify-center gap-1 sm:order-none sm:w-auto sm:justify-start"
      >
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
          className="h-10 min-w-0 flex-1 rounded-full border border-outline-subtle bg-surface-container-high px-3 text-sm text-on-bg shadow-pill outline-none transition-colors focus:ring-2 focus:ring-focus-ring sm:hidden"
        >
          <option value="__all__">All areas</option>
          {areas.map((area) => (
            <option key={area.id} value={area.id}>
              {area.name}
            </option>
          ))}
        </select>
        <div
          ref={areaOverflowRef}
          data-testid="desktop-area-selector"
          className="relative hidden max-w-[min(44rem,56vw)] min-w-0 items-center gap-1 sm:flex"
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

            {inlineAreas.map((area) => {
              const isActive = isTopologyContextView && selectedAreaId === area.id;
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
            {overflowAreas.length > 0 && (
              <button
                type="button"
                aria-label={`More ${overflowAreas.length} ${
                  overflowAreas.length === 1 ? 'area' : 'areas'
                }`}
                aria-haspopup="listbox"
                aria-expanded={areaOverflowOpen}
                onClick={() => setAreaOverflowOpen((current) => !current)}
                className="flex items-center gap-1 rounded-full border border-outline-subtle bg-surface-container-high px-3 py-2 text-sm font-medium whitespace-nowrap text-on-bg-secondary shadow-pill transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
              >
                More {overflowAreas.length}
                <MaterialIcon
                  name={areaOverflowOpen ? 'expand_less' : 'expand_more'}
                  className="text-[18px]"
                />
              </button>
            )}
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
          {areaOverflowOpen && (
            <div className="topology-glass topology-floating-shadow absolute right-0 top-full mt-2 w-[min(14rem,calc(100vw-2rem))] overflow-hidden rounded-[16px] p-1.5">
              <div role="listbox" aria-label="More map areas" tabIndex={-1}>
                {overflowAreas.map((area) => {
                  const isActive = isTopologyContextView && selectedAreaId === area.id;
                  return (
                    <button
                      key={area.id}
                      type="button"
                      role="option"
                      aria-selected={isActive}
                      onClick={() => {
                        setAreaOverflowOpen(false);
                        onAreaSelect(area.id);
                      }}
                      className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition-colors hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring ${
                        isActive ? 'font-semibold text-primary' : 'text-on-bg'
                      }`}
                    >
                      <span
                        className="h-2 w-2 flex-none rounded-full"
                        style={{
                          backgroundColor: adaptAreaColor(area.color, resolvedTheme),
                          boxShadow: isActive
                            ? `0 0 8px ${adaptAreaColor(area.color, resolvedTheme)}`
                            : undefined,
                        }}
                      />
                      <span className="min-w-0 flex-1 truncate">{area.name}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      </div>

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

      {canViewAdmin && (
        <button
          type="button"
          onClick={() => onViewChange('admin')}
          className={`flex items-center rounded-full border px-2 py-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg sm:px-3 ${
            isAdmin
              ? 'border-outline-strong bg-surface-container-high font-semibold text-on-bg shadow-pill'
              : 'border-transparent text-on-bg-secondary hover:bg-surface-container hover:text-on-bg'
          }`}
          aria-label="Admin Dashboard"
          title="Admin"
        >
          <MaterialIcon name="admin_panel_settings" size={20} />
        </button>
      )}

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
