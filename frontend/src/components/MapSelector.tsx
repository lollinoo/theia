import { useEffect, useMemo, useRef, useState } from 'react';
import type { CanvasMap } from '../types/api';
import { MaterialIcon } from './MaterialIcon';

interface MapSelectorProps {
  maps: CanvasMap[];
  selectedMapId: string | null;
  selectedMapName: string;
  onSelectMap: (map: CanvasMap) => void;
  onManageMaps: () => void;
  placement?: 'floating' | 'inline';
}

const fallbackDefaultMap: CanvasMap = {
  id: 'default',
  name: 'Default',
  description: '',
  source_area_id: null,
  filter: {},
  is_default: true,
  device_count: 0,
  link_count: 0,
  position_count: 0,
  created_at: '',
  updated_at: '',
};

function isSelectedMap(map: CanvasMap, selectedMapId: string | null) {
  return (map.is_default && selectedMapId === null) || map.id === selectedMapId;
}

export function MapSelector({
  maps,
  selectedMapId,
  selectedMapName,
  onSelectMap,
  onManageMaps,
  placement = 'floating',
}: MapSelectorProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const menuMaps = useMemo(() => {
    const defaultMap = maps.find((map) => map.is_default) ?? fallbackDefaultMap;
    return [defaultMap, ...maps.filter((map) => !map.is_default)];
  }, [maps]);
  const selectedOptionIndex = useMemo(() => {
    const index = menuMaps.findIndex((map) => isSelectedMap(map, selectedMapId));
    return index >= 0 ? index : 0;
  }, [menuMaps, selectedMapId]);

  const closeMenu = () => {
    setOpen(false);
    triggerRef.current?.focus();
  };

  const selectMap = (map: CanvasMap) => {
    closeMenu();
    onSelectMap(map);
  };

  const focusOption = (index: number) => {
    optionRefs.current[index]?.focus();
  };

  useEffect(() => {
    if (!open) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        closeMenu();
      }
    };
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Node) || !rootRef.current?.contains(target)) {
        closeMenu();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    document.addEventListener('pointerdown', handlePointerDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('pointerdown', handlePointerDown);
    };
  }, [open]);

  useEffect(() => {
    if (open) {
      focusOption(selectedOptionIndex);
    }
  }, [open, selectedOptionIndex]);

  const rootClassName =
    placement === 'floating'
      ? 'absolute right-20 top-20 z-10'
      : 'relative min-w-0 flex-1 sm:min-w-[8rem] sm:flex-none';
  const triggerClassName =
    placement === 'floating'
      ? 'topology-glass topology-floating-shadow flex h-11 max-w-[min(15rem,calc(100vw-6rem))] items-center gap-2 rounded-[16px] px-3 text-sm font-medium text-on-bg transition-[background-color,color,border-color,transform] duration-150 hover:-translate-y-0.5 hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg'
      : 'flex h-10 w-full max-w-full items-center gap-2 rounded-full border border-outline-subtle bg-surface-container-high px-3 text-sm font-medium text-on-bg shadow-pill outline-none transition-colors hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg sm:max-w-[min(14rem,42vw)]';
  const menuClassName =
    placement === 'floating'
      ? 'topology-glass topology-floating-shadow absolute right-0 mt-2 w-[min(16rem,calc(100vw-6rem))] overflow-hidden rounded-[16px] p-1.5'
      : 'topology-glass topology-floating-shadow absolute left-0 mt-2 w-[min(16rem,calc(100vw-2rem))] overflow-hidden rounded-[16px] p-1.5 sm:left-auto sm:right-0';

  return (
    <div ref={rootRef} className={rootClassName}>
      <button
        ref={triggerRef}
        type="button"
        aria-label={`Select topology map, current map ${selectedMapName}`}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className={triggerClassName}
        title="Select topology map"
      >
        <MaterialIcon name="map" className="text-[20px]" />
        <span className="min-w-0 truncate">{selectedMapName}</span>
        <MaterialIcon name={open ? 'expand_less' : 'expand_more'} className="text-[20px]" />
      </button>

      {open && (
        <div className={menuClassName}>
          <div role="listbox" aria-label="Topology maps" tabIndex={-1}>
            {menuMaps.map((map, index) => {
              const selected = isSelectedMap(map, selectedMapId);
              return (
                <button
                  key={map.id}
                  ref={(element) => {
                    optionRefs.current[index] = element;
                  }}
                  type="button"
                  role="option"
                  aria-selected={selected}
                  tabIndex={-1}
                  onClick={() => selectMap(map)}
                  onKeyDown={(event) => {
                    switch (event.key) {
                      case 'ArrowDown':
                        event.preventDefault();
                        focusOption(Math.min(index + 1, menuMaps.length - 1));
                        break;
                      case 'ArrowUp':
                        event.preventDefault();
                        focusOption(Math.max(index - 1, 0));
                        break;
                      case 'Home':
                        event.preventDefault();
                        focusOption(0);
                        break;
                      case 'End':
                        event.preventDefault();
                        focusOption(menuMaps.length - 1);
                        break;
                      case 'Enter':
                      case ' ':
                        event.preventDefault();
                        selectMap(map);
                        break;
                      case 'Escape':
                        event.preventDefault();
                        closeMenu();
                        break;
                    }
                  }}
                  className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition-colors duration-150 hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring ${
                    selected ? 'text-primary' : 'text-on-bg'
                  }`}
                >
                  <span aria-hidden="true" className="flex h-5 w-5 items-center justify-center">
                    {selected && <MaterialIcon name="check" className="text-[18px]" />}
                  </span>
                  <span className="min-w-0 flex-1 truncate">{map.name}</span>
                </button>
              );
            })}
          </div>
          <div className="my-1 h-px bg-outline-subtle" />
          <button
            type="button"
            onClick={() => {
              closeMenu();
              onManageMaps();
            }}
            className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm text-on-bg-secondary transition-colors duration-150 hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
          >
            <MaterialIcon name="settings" className="text-[18px]" />
            <span className="min-w-0 flex-1 truncate">Manage maps</span>
          </button>
        </div>
      )}
    </div>
  );
}
