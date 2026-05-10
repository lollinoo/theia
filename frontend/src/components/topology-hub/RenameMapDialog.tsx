import { useEffect, useState } from 'react';
import type { CanvasMap } from '../../types/api';
import { MaterialIcon } from '../MaterialIcon';

export interface RenameMapDialogSubmit {
  name: string;
  map: CanvasMap;
}

export interface RenameMapDialogProps {
  open: boolean;
  map: CanvasMap | null;
  renaming?: boolean;
  onRename: (payload: RenameMapDialogSubmit) => void;
  onClose: () => void;
}

export function RenameMapDialog({
  open,
  map,
  renaming = false,
  onRename,
  onClose,
}: RenameMapDialogProps) {
  const [name, setName] = useState('');
  const trimmedName = name.trim();

  useEffect(() => {
    if (open && map) {
      setName(map.name);
    }
  }, [open, map]);

  if (!open || !map) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/40 px-4">
      <form
        role="dialog"
        aria-modal="true"
        aria-labelledby="rename-map-title"
        className="w-full max-w-md rounded-lg border border-outline bg-surface p-5 shadow-panel"
        onSubmit={(event) => {
          event.preventDefault();
          if (trimmedName.length === 0 || renaming) {
            return;
          }
          onRename({ name: trimmedName, map });
        }}
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 id="rename-map-title" className="text-base font-semibold text-on-bg">
              Rename map
            </h2>
            <p className="mt-1 text-sm text-on-bg-secondary">{map.name}</p>
          </div>
          <button
            type="button"
            aria-label="Close rename map dialog"
            onClick={onClose}
            disabled={renaming}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-50"
          >
            <MaterialIcon name="close" size={18} />
          </button>
        </div>

        <label className="mt-5 block text-sm font-medium text-on-bg" htmlFor="rename-map-name">
          Map name
        </label>
        <input
          id="rename-map-name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          disabled={renaming}
          className="mt-2 h-10 w-full rounded-lg border border-outline-subtle bg-surface-container px-3 text-sm text-on-bg outline-none transition-colors focus:ring-2 focus:ring-focus-ring disabled:cursor-not-allowed disabled:opacity-60"
        />

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={renaming}
            className="rounded-lg border border-outline px-3 py-2 text-sm font-medium text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={trimmedName.length === 0 || renaming}
            className="rounded-lg bg-primary px-3 py-2 text-sm font-semibold text-on-primary transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            {renaming ? 'Renaming...' : 'Rename map'}
          </button>
        </div>
      </form>
    </div>
  );
}

export default RenameMapDialog;
