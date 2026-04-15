import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';
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

  it('accepts direct cache updates from the device config panel', () => {
    const { result } = renderHook(() => useDeviceWinboxAvailability());

    act(() => {
      result.current.setDeviceWinboxAvailability('dev-1', true);
    });

    expect(result.current.deviceWinboxState['dev-1']).toBe(true);
  });
});
