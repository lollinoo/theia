import { useEffect, useState } from 'react';
import { fetchAreas } from '../../api/client';
import type { Area } from '../../types/api';
import { createAsyncStaleGuard } from '../../utils/asyncStaleGuard';
import {
  type DeviceFormModel,
  defaultVirtualNodeColor,
  normalizeVirtualNodeColor,
} from '../forms/deviceFormModels';

interface DeviceAreasSectionProps {
  form: DeviceFormModel;
  areas?: Area[];
  readOnly?: boolean;
  isVirtual?: boolean;
  mapContext?: {
    mapId: string;
    mapName: string;
  };
  onFormChange: (update: Partial<DeviceFormModel>) => void;
  onVirtualChange: (update: Partial<DeviceFormModel['virtual']>) => void;
}

export function DeviceAreasSection({
  form,
  areas: providedAreas,
  readOnly = false,
  isVirtual,
  mapContext,
  onFormChange,
  onVirtualChange,
}: DeviceAreasSectionProps) {
  const [loadedAreas, setLoadedAreas] = useState<Area[]>([]);
  const areas = providedAreas ?? loadedAreas;
  const unassignedAreas = areas.filter((area) => !form.areaIds.includes(area.id));

  useEffect(() => {
    if (providedAreas) {
      setLoadedAreas([]);
      return;
    }

    const staleGuard = createAsyncStaleGuard();
    fetchAreas()
      .then((nextAreas) => {
        staleGuard.run(() => setLoadedAreas(nextAreas));
      })
      .catch(() => {
        /* non-fatal */
      });

    return () => {
      staleGuard.cancel();
    };
  }, [providedAreas]);

  return (
    <>
      <div className="space-y-1">
        <label className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary">
          Areas
        </label>
        {form.areaIds.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {form.areaIds.map((id) => {
              const area = areas.find((a) => a.id === id);
              if (!area) return null;
              return (
                <span
                  key={id}
                  className="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium text-on-bg"
                  style={{
                    backgroundColor: `${area.color}25`,
                    border: `1px solid ${area.color}60`,
                  }}
                >
                  <span
                    className="inline-block h-2 w-2 rounded-full"
                    style={{ backgroundColor: area.color }}
                  />
                  {area.name}
                  <button
                    type="button"
                    disabled={readOnly}
                    onClick={() =>
                      onFormChange({ areaIds: form.areaIds.filter((areaId) => areaId !== id) })
                    }
                    className="ml-0.5 text-on-bg-secondary hover:text-on-bg disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M6 18L18 6M6 6l12 12"
                      />
                    </svg>
                  </button>
                </span>
              );
            })}
          </div>
        )}
        <select
          value=""
          disabled={readOnly || unassignedAreas.length === 0}
          onChange={(e) => {
            if (e.target.value) {
              onFormChange({ areaIds: [...form.areaIds, e.target.value] });
            }
          }}
          className="w-full rounded-lg border border-outline-subtle bg-elevated px-3 py-2 text-sm text-on-bg focus:border-primary focus:ring-1 focus:ring-primary/30 focus:outline-none disabled:opacity-50"
        >
          <option value="">
            {areas.length === 0
              ? 'No areas created'
              : unassignedAreas.length === 0
                ? 'All areas assigned'
                : form.areaIds.length === 0
                  ? 'Unassigned - select area...'
                  : 'Add another area...'}
          </option>
          {unassignedAreas.map((area) => (
            <option key={area.id} value={area.id}>
              {area.name}
            </option>
          ))}
        </select>
      </div>

      {isVirtual && mapContext && (
        <div className="space-y-1">
          <label
            htmlFor="device-virtual-node-color"
            className="text-xs font-medium uppercase tracking-widest text-on-bg-secondary"
          >
            Virtual node color
          </label>
          <div className="flex items-center gap-2">
            <input
              id="device-virtual-node-color"
              aria-label="Virtual node color"
              type="color"
              value={form.virtual.visualColor ?? defaultVirtualNodeColor}
              disabled={readOnly}
              onChange={(e) =>
                onVirtualChange({ visualColor: normalizeVirtualNodeColor(e.target.value) })
              }
              className="h-10 w-12 shrink-0 cursor-pointer rounded-lg border border-outline-subtle bg-elevated p-1 disabled:cursor-not-allowed disabled:opacity-60"
            />
            <button
              type="button"
              disabled={readOnly}
              onClick={() => onVirtualChange({ visualColor: null })}
              className="rounded-lg bg-surface-high px-3 py-2 text-xs font-medium text-on-bg-secondary transition-colors hover:text-on-bg disabled:cursor-not-allowed disabled:opacity-60"
            >
              Use area/default color
            </button>
          </div>
        </div>
      )}
    </>
  );
}
