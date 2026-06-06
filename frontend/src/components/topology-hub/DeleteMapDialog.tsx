/**
 * Defines delete map dialog behavior for the topology hub.
 * Keeps saved-map and area workflows separate from the live canvas surface.
 */
import type { CanvasMap } from '../../types/api';
import { MaterialIcon } from '../MaterialIcon';

/** Defines the props contract for DeleteMapDialog within the topology hub. */
export interface DeleteMapDialogProps {
  open: boolean;
  map: CanvasMap | null;
  deleting?: boolean;
  onDelete: (map: CanvasMap) => void;
  onClose: () => void;
}

/** Renders the DeleteMapDialog component within the topology hub. */
export function DeleteMapDialog({
  open,
  map,
  deleting = false,
  onDelete,
  onClose,
}: DeleteMapDialogProps) {
  if (!open || !map) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/40 px-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-map-title"
        className="w-full max-w-md rounded-lg border border-outline bg-surface p-5 shadow-panel"
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 id="delete-map-title" className="text-base font-semibold text-on-bg">
              Delete map
            </h2>
            <p className="mt-1 text-sm text-on-bg-secondary">{map.name}</p>
          </div>
          <button
            type="button"
            aria-label="Close delete map dialog"
            onClick={onClose}
            disabled={deleting}
            className="rounded-full p-1.5 text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-50"
          >
            <MaterialIcon name="close" size={18} />
          </button>
        </div>

        <p className="mt-5 text-sm text-on-bg-secondary">
          This removes the saved map, its local memberships, positions, and map-local areas. Global
          devices remain available in inventory and in other maps.
        </p>

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={deleting}
            className="rounded-lg border border-outline px-3 py-2 text-sm font-medium text-on-bg-secondary transition-colors hover:bg-surface-container hover:text-on-bg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onDelete(map)}
            disabled={deleting}
            className="rounded-lg bg-status-down px-3 py-2 text-sm font-semibold text-white transition-colors hover:bg-status-down/90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-bg"
          >
            {deleting ? 'Deleting...' : 'Delete map'}
          </button>
        </div>
      </div>
    </div>
  );
}

export default DeleteMapDialog;
