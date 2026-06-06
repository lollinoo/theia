/**
 * Defines duplicate map dialog behavior for the topology hub.
 * Keeps saved-map and area workflows separate from the live canvas surface.
 */
import { useEffect, useState } from 'react';
import type { CanvasMap } from '../../types/api';
import { MaterialIcon } from '../MaterialIcon';

/** Describes the duplicate map dialog submit contract used by the topology hub. */
export interface DuplicateMapDialogSubmit {
  name: string;
  sourceMap: CanvasMap;
}

/** Defines the props contract for DuplicateMapDialog within the topology hub. */
export interface DuplicateMapDialogProps {
  open: boolean;
  sourceMap: CanvasMap | null;
  onDuplicate: (payload: DuplicateMapDialogSubmit) => void;
  onClose: () => void;
}

/** Renders the DuplicateMapDialog component within the topology hub. */
export function DuplicateMapDialog({
  open,
  sourceMap,
  onDuplicate,
  onClose,
}: DuplicateMapDialogProps) {
  const [name, setName] = useState('');
  const trimmedName = name.trim();

  useEffect(() => {
    if (open && sourceMap) {
      setName(`${sourceMap.name} Copy`);
    }
  }, [open, sourceMap]);

  if (!open || !sourceMap) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/40 px-4">
      <form
        role="dialog"
        aria-modal="true"
        aria-labelledby="duplicate-map-title"
        className="w-full max-w-md rounded-lg border border-outline bg-surface p-5 shadow-panel"
        onSubmit={(event) => {
          event.preventDefault();
          if (trimmedName.length === 0) {
            return;
          }
          onDuplicate({ name: trimmedName, sourceMap });
        }}
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 id="duplicate-map-title" className="text-base font-semibold text-on-bg">
              Duplicate map
            </h2>
            <p className="mt-1 text-sm text-on-bg-secondary">{sourceMap.name}</p>
          </div>
          <button
            type="button"
            aria-label="Close duplicate map dialog"
            onClick={onClose}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            <MaterialIcon name="close" size={18} />
          </button>
        </div>

        <label className="mt-5 block text-sm font-medium text-on-bg" htmlFor="duplicate-map-name">
          Map name
        </label>
        <input
          id="duplicate-map-name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          className="mt-2 h-10 w-full rounded-lg border border-outline-subtle bg-surface-container px-3 text-sm text-on-bg outline-none transition-colors focus:ring-2 focus:ring-focus-ring"
        />

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-outline px-3 py-2 text-sm font-medium text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={trimmedName.length === 0}
            className="rounded-lg bg-primary px-3 py-2 text-sm font-semibold text-on-primary transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            Duplicate map
          </button>
        </div>
      </form>
    </div>
  );
}

export default DuplicateMapDialog;
