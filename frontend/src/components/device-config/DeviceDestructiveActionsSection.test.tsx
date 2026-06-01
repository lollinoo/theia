import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useLayoutEffect } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { deleteDevice } from '../../api/client';
import { DeviceDestructiveActionsSection } from './DeviceDestructiveActionsSection';

vi.mock('../../api/client', () => ({
  deleteDevice: vi.fn().mockResolvedValue(undefined),
}));

function renderSection({
  deviceId = 'dev-1',
  readOnly = false,
  mapContext,
  onRemoveFromMap,
  onDeviceDeleted = vi.fn(),
}: {
  deviceId?: string;
  readOnly?: boolean;
  mapContext?: { mapId: string; mapName: string };
  onRemoveFromMap?: (deviceId: string) => void | Promise<void>;
  onDeviceDeleted?: () => void;
} = {}) {
  return render(
    <DeviceDestructiveActionsSection
      deviceId={deviceId}
      readOnly={readOnly}
      mapContext={mapContext}
      onRemoveFromMap={onRemoveFromMap}
      onDeviceDeleted={onDeviceDeleted}
    />,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DeviceDestructiveActionsSection', () => {
  it('renders map-scoped removal with the map name and removes without global delete', async () => {
    const onRemoveFromMap = vi.fn().mockResolvedValue(undefined);

    renderSection({
      mapContext: { mapId: 'map-1', mapName: 'Backbone' },
      onRemoveFromMap,
    });

    expect(
      screen.getByText(
        'Removes this device only from Backbone. Inventory and other maps are kept.',
      ),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Remove from this map' }));

    await waitFor(() => expect(onRemoveFromMap).toHaveBeenCalledWith('dev-1'));
    expect(deleteDevice).not.toHaveBeenCalled();
  });

  it('shows remove loading state while removal is pending', async () => {
    let resolveRemove: (() => void) | undefined;
    const onRemoveFromMap = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveRemove = resolve;
        }),
    );

    renderSection({
      mapContext: { mapId: 'map-1', mapName: 'Backbone' },
      onRemoveFromMap,
    });

    fireEvent.click(screen.getByRole('button', { name: 'Remove from this map' }));

    expect(screen.getByRole('button', { name: 'Removing...' })).toBeDisabled();

    resolveRemove?.();

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Remove from this map' })).toBeEnabled(),
    );
  });

  it('confirms global delete and notifies when deletion succeeds', async () => {
    const onDeviceDeleted = vi.fn();

    renderSection({ onDeviceDeleted });

    fireEvent.click(screen.getByRole('button', { name: 'Delete device everywhere' }));
    fireEvent.click(screen.getByRole('button', { name: 'Confirm Delete' }));

    await waitFor(() => expect(deleteDevice).toHaveBeenCalledWith('dev-1'));
    await waitFor(() => expect(onDeviceDeleted).toHaveBeenCalledTimes(1));
  });

  it('closes delete confirmation when canceled', () => {
    renderSection();

    fireEvent.click(screen.getByRole('button', { name: 'Delete device everywhere' }));
    expect(
      screen.getByText('Are you sure? This deletes the device everywhere and cannot be undone.'),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByText('Confirm Delete')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete device everywhere' })).toBeInTheDocument();
  });

  it('resets delete confirmation after switching devices', () => {
    const view = renderSection({ deviceId: 'dev-1' });

    fireEvent.click(screen.getByRole('button', { name: 'Delete device everywhere' }));
    expect(screen.getByRole('button', { name: 'Confirm Delete' })).toBeInTheDocument();

    view.rerender(<DeviceDestructiveActionsSection deviceId="dev-2" onDeviceDeleted={vi.fn()} />);

    expect(screen.queryByRole('button', { name: 'Confirm Delete' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete device everywhere' })).toBeInTheDocument();
  });

  it('does not expose stale delete confirmation during the first render after switching devices', () => {
    const onDeviceDeleted = vi.fn();

    function DeleteProbe({ active }: { active: boolean }) {
      useLayoutEffect(() => {
        if (!active) return;
        screen.queryByRole('button', { name: 'Confirm Delete' })?.click();
      }, [active]);
      return null;
    }

    function Harness({ deviceId, probe }: { deviceId: string; probe: boolean }) {
      return (
        <>
          <DeviceDestructiveActionsSection deviceId={deviceId} onDeviceDeleted={onDeviceDeleted} />
          <DeleteProbe active={probe} />
        </>
      );
    }

    const view = render(<Harness deviceId="dev-1" probe={false} />);

    fireEvent.click(screen.getByRole('button', { name: 'Delete device everywhere' }));

    view.rerender(<Harness deviceId="dev-2" probe={true} />);

    expect(deleteDevice).not.toHaveBeenCalled();
    expect(onDeviceDeleted).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Confirm Delete' })).not.toBeInTheDocument();
  });

  it('disables action entry points when read-only', () => {
    const onRemoveFromMap = vi.fn().mockResolvedValue(undefined);

    renderSection({
      readOnly: true,
      mapContext: { mapId: 'map-1', mapName: 'Backbone' },
      onRemoveFromMap,
    });

    expect(screen.getByRole('button', { name: 'Remove from this map' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Delete device everywhere' })).toBeDisabled();
  });

  it('closes delete confirmation and returns to the initial button when deletion fails', async () => {
    vi.mocked(deleteDevice).mockRejectedValueOnce(new Error('delete failed'));

    renderSection();

    fireEvent.click(screen.getByRole('button', { name: 'Delete device everywhere' }));
    fireEvent.click(screen.getByRole('button', { name: 'Confirm Delete' }));

    await waitFor(() => expect(screen.queryByText('Confirm Delete')).not.toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Delete device everywhere' })).toBeEnabled();
  });
});
