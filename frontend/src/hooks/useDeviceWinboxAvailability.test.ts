import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useDeviceWinboxAvailability } from './useDeviceWinboxAvailability';

vi.mock('../api/client', () => ({
  fetchDeviceCredentialProfiles: vi.fn(),
}));

describe('useDeviceWinboxAvailability', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('does not fetch until explicitly refreshed', () => {
    const { result } = renderHook(() => useDeviceWinboxAvailability());
    expect(result.current.deviceWinboxState).toEqual({});
  });

  it('stores true when the device has a WinBox profile', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValue([
      { profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: true },
    ]);

    const { result } = renderHook(() => useDeviceWinboxAvailability());
    act(() => {
      result.current.refreshDeviceWinboxAvailability('dev-1');
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.deviceWinboxState['dev-1']).toBe(true);
  });

  it('fetches again every time the menu is opened', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../api/client');
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValue([
      { profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: true },
    ]);

    const { result } = renderHook(() => useDeviceWinboxAvailability());
    act(() => {
      result.current.refreshDeviceWinboxAvailability('dev-1');
    });
    await act(async () => {
      await Promise.resolve();
    });

    act(() => {
      result.current.refreshDeviceWinboxAvailability('dev-1');
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(fetchDeviceCredentialProfiles).toHaveBeenCalledTimes(2);
  });

  it('keeps in-flight refreshes isolated per device', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../api/client');
    let resolveDeviceA: ((value: Array<{ is_winbox: boolean }>) => void) | null = null;
    let resolveDeviceB: ((value: Array<{ is_winbox: boolean }>) => void) | null = null;
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockImplementation(
      (deviceId: string) => {
        if (deviceId === 'dev-a') {
          return new Promise((resolve) => {
            resolveDeviceA = resolve;
          });
        }

        return new Promise((resolve) => {
          resolveDeviceB = resolve;
        });
      },
    );

    const { result } = renderHook(() => useDeviceWinboxAvailability());

    act(() => {
      result.current.refreshDeviceWinboxAvailability('dev-a');
      result.current.refreshDeviceWinboxAvailability('dev-b');
    });

    await act(async () => {
      resolveDeviceB?.([{ is_winbox: false }]);
      await Promise.resolve();
    });

    await act(async () => {
      resolveDeviceA?.([{ is_winbox: true }]);
      await Promise.resolve();
    });

    expect(result.current.deviceWinboxState['dev-a']).toBe(true);
    expect(result.current.deviceWinboxState['dev-b']).toBe(false);
  });

  it('accepts direct cache updates from the device config panel', () => {
    const { result } = renderHook(() => useDeviceWinboxAvailability());

    act(() => {
      result.current.setDeviceWinboxAvailability('dev-1', true);
    });

    expect(result.current.deviceWinboxState['dev-1']).toBe(true);
  });

  it('ignores an older in-flight refresh after an explicit cache update for the same device', async () => {
    const { fetchDeviceCredentialProfiles } = await import('../api/client');
    let resolveRefresh: ((value: Array<{ is_winbox: boolean }>) => void) | null = null;
    (fetchDeviceCredentialProfiles as ReturnType<typeof vi.fn>).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveRefresh = resolve;
        }),
    );

    const { result } = renderHook(() => useDeviceWinboxAvailability());

    act(() => {
      result.current.refreshDeviceWinboxAvailability('dev-1');
    });

    act(() => {
      result.current.setDeviceWinboxAvailability('dev-1', true);
    });

    await act(async () => {
      resolveRefresh?.([{ is_winbox: false }]);
      await Promise.resolve();
    });

    expect(result.current.deviceWinboxState['dev-1']).toBe(true);
  });
});
