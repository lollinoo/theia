import { useEffect, useMemo, useRef, useState } from 'react';
import type { CanvasMap } from '../types/api';
import { MaterialIcon } from './MaterialIcon';

interface MapSelectorProps {
  maps: CanvasMap[];
  selectedMapId: string | null;
  selectedMapName: string;
  onSelectMap: (map: CanvasMap) => void;
  onManageMaps: () => void;
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

export function MapSelector({
  maps,
  selectedMapId,
  selectedMapName,
  onSelectMap,
  onManageMaps,
}: MapSelectorProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const menuMaps = useMemo(() => {
    const defaultMap = maps.find((map) => map.is_default) ?? fallbackDefaultMap;
    return [defaultMap, ...maps.filter((map) => !map.is_default)];
  }, [maps]);

  useEffect(() => {
    if (!open) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false);
      }
    };
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Node) || !rootRef.current?.contains(target)) {
        setOpen(false);
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    document.addEventListener('pointerdown', handlePointerDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('pointerdown', handlePointerDown);
    };
  }, [open]);

  return (
    <div ref={rootRef} className="absolute right-20 top-20 z-10">
      <button
        type="button"
        aria-label="Select topology map"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="topology-glass topology-floating-shadow flex h-11 max-w-[15rem] items-center gap-2 rounded-[16px] px-3 text-sm font-medium text-on-bg transition-[background-color,color,border-color,transform] duration-150 hover:-translate-y-0.5 hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
        title="Select topology map"
      >
        <MaterialIcon name="map" className="text-[20px]" />
        <span className="min-w-0 truncate">{selectedMapName}</span>
        <MaterialIcon name={open ? 'expand_less' : 'expand_more'} className="text-[20px]" />
      </button>

      {open && (
        <div
          role="menu"
          className="topology-glass topology-floating-shadow absolute right-0 mt-2 w-64 overflow-hidden rounded-[16px] p-1.5"
        >
          {menuMaps.map((map) => {
            const selected =
              (map.is_default && selectedMapId === null) || map.id === selectedMapId;
            return (
              <button
                key={map.id}
                type="button"
                role="menuitem"
                aria-current={selected ? 'true' : undefined}
                onClick={() => {
                  setOpen(false);
                  onSelectMap(map);
                }}
                className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition-colors duration-150 hover:bg-surface-container focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring ${
                  selected ? 'text-primary' : 'text-on-bg'
                }`}
              >
                <span className="flex h-5 w-5 items-center justify-center">
                  {selected && <MaterialIcon name="check" className="text-[18px]" />}
                </span>
                <span className="min-w-0 flex-1 truncate">{map.name}</span>
              </button>
            );
          })}
          <div className="my-1 h-px bg-outline-subtle" />
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setOpen(false);
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
