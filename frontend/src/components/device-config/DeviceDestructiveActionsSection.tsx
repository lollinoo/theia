import { useEffect, useState } from 'react';
import { deleteDevice } from '../../api/client';

interface DeviceDestructiveActionsSectionProps {
  deviceId: string;
  readOnly?: boolean;
  mapContext?: {
    mapId: string;
    mapName: string;
  };
  onDeviceDeleted: () => void;
  onRemoveFromMap?: (deviceId: string) => void | Promise<void>;
}

export function DeviceDestructiveActionsSection({
  deviceId,
  readOnly = false,
  mapContext,
  onDeviceDeleted,
  onRemoveFromMap,
}: DeviceDestructiveActionsSectionProps) {
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [removeFromMapLoading, setRemoveFromMapLoading] = useState(false);

  useEffect(() => {
    setConfirmDelete(false);
    setDeleteLoading(false);
    setRemoveFromMapLoading(false);
  }, [deviceId]);

  async function handleDelete() {
    if (readOnly) return;
    setDeleteLoading(true);
    try {
      await deleteDevice(deviceId);
      onDeviceDeleted();
    } catch {
      setDeleteLoading(false);
      setConfirmDelete(false);
    }
  }

  async function handleRemoveFromMap() {
    if (readOnly || !mapContext || !onRemoveFromMap) return;
    setRemoveFromMapLoading(true);
    try {
      await onRemoveFromMap(deviceId);
    } finally {
      setRemoveFromMapLoading(false);
    }
  }

  return (
    <>
      {mapContext && onRemoveFromMap && (
        <div className="mt-6 space-y-2 rounded-lg border border-outline-subtle bg-surface-container px-3 py-3">
          <p className="text-xs text-on-bg-secondary">
            Removes this device only from {mapContext.mapName}. Inventory and other maps are kept.
          </p>
          <button
            type="button"
            disabled={readOnly || removeFromMapLoading}
            onClick={() => {
              void handleRemoveFromMap();
            }}
            className="w-full rounded-lg bg-surface-high px-4 py-2 text-sm font-medium text-on-bg transition-colors hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
          >
            {removeFromMapLoading ? 'Removing...' : 'Remove from this map'}
          </button>
        </div>
      )}

      {/* Delete Device Everywhere */}
      <div className="mt-6 space-y-3">
        {!confirmDelete ? (
          <button
            type="button"
            disabled={readOnly}
            onClick={() => setConfirmDelete(true)}
            className="w-full rounded-lg border border-status-down/30 bg-status-down/10 px-4 py-2 text-sm font-medium text-status-down transition-colors hover:bg-status-down/20 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Delete device everywhere
          </button>
        ) : (
          <div className="space-y-2 rounded-lg border border-status-down/30 bg-status-down/10 p-3">
            <p className="text-sm text-status-down">
              Are you sure? This deletes the device everywhere and cannot be undone.
            </p>
            <div className="flex gap-2">
              <button
                type="button"
                disabled={readOnly}
                onClick={() => setConfirmDelete(false)}
                className="flex-1 rounded-lg bg-surface-high px-3 py-1.5 text-xs text-on-bg hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={readOnly || deleteLoading}
                onClick={() => {
                  void handleDelete();
                }}
                className="flex-1 rounded-lg bg-status-down px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {deleteLoading ? 'Deleting...' : 'Confirm Delete'}
              </button>
            </div>
          </div>
        )}
      </div>
    </>
  );
}
