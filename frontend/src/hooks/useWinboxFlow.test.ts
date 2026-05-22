import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  BRIDGE_HEALTH_TIMEOUT_MESSAGE,
  BRIDGE_LAUNCH_TIMEOUT_MESSAGE,
  BRIDGE_REQUEST_TIMEOUT_MS,
} from '../utils/bridgeRequests';
import { useWinboxFlow } from './useWinboxFlow';

const apiMocks = vi.hoisted(() => ({
  fetchUserSettings: vi.fn(),
  fetchDeviceCredentialProfiles: vi.fn(),
  createBridgeLaunchRequest: vi.fn(),
}));

vi.mock('../api/client', () => apiMocks);

describe('useWinboxFlow', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    vi.stubGlobal('fetch', vi.fn());
    apiMocks.fetchUserSettings.mockResolvedValue({
      preferences: { bridge_port: 1337 },
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('launches with a user-scoped bridge launch token without reading connector secrets from settings', async () => {
    apiMocks.fetchUserSettings.mockResolvedValue({ preferences: { bridge_port: 1337 } });
    apiMocks.createBridgeLaunchRequest.mockResolvedValue({ launch_token: 'launch-token' });
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });

    const { result } = renderHook(() => useWinboxFlow());

    await act(async () => {
      await Promise.resolve();
    });

    await act(async () => {
      await result.current.launchWinbox('dev-1');
    });

    expect(result.current.winboxError).toBeNull();
    expect(apiMocks.createBridgeLaunchRequest).toHaveBeenCalledWith('dev-1');
    expect(global.fetch).toHaveBeenCalledWith(
      'http://localhost:1337/launch',
      expect.objectContaining({ body: JSON.stringify({ launch_token: 'launch-token' }) }),
    );
  });

  it('refreshes a stale false WinBox cache when reopening the same device menu', async () => {
    apiMocks.fetchDeviceCredentialProfiles
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([{ profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: true }]);

    const { result } = renderHook(() => useWinboxFlow());

    await act(async () => {
      result.current.openDeviceMenu('dev-1');
      await Promise.resolve();
    });

    expect(result.current.deviceWinboxState['dev-1']).toBe(false);

    await act(async () => {
      result.current.openDeviceMenu('dev-1');
      await Promise.resolve();
    });

    expect(apiMocks.fetchDeviceCredentialProfiles).toHaveBeenCalledTimes(2);
    expect(result.current.deviceWinboxState['dev-1']).toBe(true);
  });

  it('maps bridge launch timeout failures into the WinBox error toast', async () => {
    apiMocks.createBridgeLaunchRequest.mockResolvedValue({ launch_token: 'launch-token' });
    (global.fetch as ReturnType<typeof vi.fn>).mockImplementation(() => new Promise(() => {}));

    const { result } = renderHook(() => useWinboxFlow());

    await act(async () => {
      await Promise.resolve();
    });

    act(() => {
      void result.current.launchWinbox('dev-1');
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(BRIDGE_REQUEST_TIMEOUT_MS);
    });

    expect(result.current.winboxError).toBe(BRIDGE_LAUNCH_TIMEOUT_MESSAGE);
  });

  it('waits for settings to load before deciding the bridge secret is missing', async () => {
    let resolveSettings: ((value: { preferences: { bridge_port: number } }) => void) | null = null;
    apiMocks.fetchUserSettings.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSettings = resolve;
        }),
    );
    apiMocks.createBridgeLaunchRequest.mockResolvedValue({ launch_token: 'launch-token' });
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });

    const { result } = renderHook(() => useWinboxFlow());

    const launchPromise = result.current.launchWinbox('dev-1');

    expect(apiMocks.createBridgeLaunchRequest).not.toHaveBeenCalled();
    expect(result.current.winboxError).toBeNull();

    await act(async () => {
      resolveSettings?.({ preferences: { bridge_port: 1337 } });
      await launchPromise;
    });

    expect(apiMocks.createBridgeLaunchRequest).toHaveBeenCalledWith('dev-1');
    expect(result.current.winboxError).toBeNull();
  });

  it('waits for settings before the first bridge health check on a non-default port', async () => {
    let resolveSettings: ((value: { preferences: { bridge_port: number } }) => void) | null = null;
    apiMocks.fetchUserSettings.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSettings = resolve;
        }),
    );
    apiMocks.fetchDeviceCredentialProfiles.mockResolvedValue([
      { profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: true },
    ]);
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });

    const { result } = renderHook(() => useWinboxFlow());

    act(() => {
      result.current.openDeviceMenu('dev-1');
    });

    expect(global.fetch).not.toHaveBeenCalled();

    await act(async () => {
      resolveSettings?.({ preferences: { bridge_port: 9000 } });
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(global.fetch).toHaveBeenCalledWith(
      'http://localhost:9000/health',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it('refreshes device WinBox availability immediately before settings resolve', async () => {
    let resolveSettings: ((value: { preferences: { bridge_port: number } }) => void) | null = null;
    apiMocks.fetchUserSettings.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSettings = resolve;
        }),
    );
    apiMocks.fetchDeviceCredentialProfiles.mockResolvedValue([]);
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true });

    const { result } = renderHook(() => useWinboxFlow());

    void result.current.openDeviceMenu('dev-1');

    await act(async () => {
      await Promise.resolve();
    });

    expect(apiMocks.fetchDeviceCredentialProfiles).toHaveBeenCalledWith('dev-1');
    expect(result.current.deviceWinboxState['dev-1']).toBe(false);
    expect(global.fetch).not.toHaveBeenCalled();

    await act(async () => {
      resolveSettings?.({ preferences: { bridge_port: 9000 } });
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(global.fetch).toHaveBeenCalledWith(
      'http://localhost:9000/health',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it('surfaces bridge health timeout errors after opening the device menu', async () => {
    apiMocks.fetchDeviceCredentialProfiles.mockResolvedValue([
      { profile_id: 'p1', name: 'Admin', role: 'Admin', is_winbox: true },
    ]);
    (global.fetch as ReturnType<typeof vi.fn>).mockImplementation(() => new Promise(() => {}));

    const { result } = renderHook(() => useWinboxFlow());

    await act(async () => {
      await Promise.resolve();
    });

    act(() => {
      result.current.openDeviceMenu('dev-1');
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(BRIDGE_REQUEST_TIMEOUT_MS);
    });

    expect(result.current.bridgeChecked).toBe(true);
    expect(result.current.bridgeRunning).toBe(false);
    expect(result.current.winboxError).toBe(BRIDGE_HEALTH_TIMEOUT_MESSAGE);
  });

  it('accepts direct availability cache updates from device configuration', async () => {
    const { result } = renderHook(() => useWinboxFlow());

    await act(async () => {
      await Promise.resolve();
    });

    act(() => {
      result.current.setDeviceWinboxAvailability('dev-1', true);
    });

    expect(result.current.deviceWinboxState['dev-1']).toBe(true);
  });
});
