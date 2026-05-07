import { useEffect, useState } from 'react';
import type { Area } from '../../types/api';
import { MaterialIcon } from '../MaterialIcon';

export interface CreateMapDialogSubmit {
  name: string;
  sourceArea: Area | null;
}

export interface CreateMapDialogProps {
  open: boolean;
  sourceArea: Area | null;
  onCreate: (payload: CreateMapDialogSubmit) => void;
  onClose: () => void;
}

export function CreateMapDialog({ open, sourceArea, onCreate, onClose }: CreateMapDialogProps) {
  const [name, setName] = useState('');
  const trimmedName = name.trim();

  useEffect(() => {
    if (open) {
      setName(sourceArea ? `${sourceArea.name} Map` : '');
    }
  }, [open, sourceArea]);

  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/40 px-4">
      <form
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-map-title"
        className="w-full max-w-md rounded-lg border border-outline bg-surface p-5 shadow-panel"
        onSubmit={(event) => {
          event.preventDefault();
          if (trimmedName.length === 0) {
            return;
          }
          onCreate({ name: trimmedName, sourceArea });
        }}
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 id="create-map-title" className="text-base font-semibold text-on-bg">
              Create map
            </h2>
            {sourceArea && (
              <p className="mt-1 text-sm text-on-bg-secondary">{sourceArea.name}</p>
            )}
          </div>
          <button
            type="button"
            aria-label="Close create map dialog"
            onClick={onClose}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            <MaterialIcon name="close" size={18} />
          </button>
        </div>

        <label className="mt-5 block text-sm font-medium text-on-bg" htmlFor="create-map-name">
          Map name
        </label>
        <input
          id="create-map-name"
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
            Create map
          </button>
        </div>
      </form>
    </div>
  );
}

export default CreateMapDialog;
