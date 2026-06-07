/**
 * Exercises user settings page settings behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  fetchBridgeConnectorConfig,
  fetchUserSettings,
  generateBridgeSecret,
  type UserSettingsResponse,
  updateUserSettings,
} from '../../api/client';
import { UserSettingsPage } from './UserSettingsPage';

vi.mock('../../api/client', () => ({
  changePassword: vi.fn(),
  fetchBridgeConnectorConfig: vi.fn(),
  fetchUserSettings: vi.fn(),
  generateBridgeSecret: vi.fn(),
  revokeBridgeSecret: vi.fn(),
  rotateBridgeSecret: vi.fn(),
  updateUserSettings: vi.fn(),
}));

const activeSettings: UserSettingsResponse = {
  user: {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    display_name: 'Alice',
  },
  preferences: {
    timezone: 'UTC',
    locale: 'en-US',
    bridge_port: 1337,
    global_bridge_port: 1337,
    bridge_port_override: null,
  },
  bridge: {
    configured: true,
    credential: {
      id: 'cred-1',
      secret_prefix: 'theia_bridge_public',
      status: 'active',
      created_at: '2026-05-21T10:00:00Z',
    },
  },
};

function restoreOwnProperty(
  object: object,
  property: PropertyKey,
  descriptor?: PropertyDescriptor,
) {
  if (descriptor) {
    Object.defineProperty(object, property, descriptor);
    return;
  }
  Reflect.deleteProperty(object, property);
}

describe('UserSettingsPage', () => {
  let openSpy: ReturnType<typeof vi.spyOn>;
  let clipboardDescriptor: PropertyDescriptor | undefined;
  let execCommandDescriptor: PropertyDescriptor | undefined;

  beforeEach(() => {
    clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    execCommandDescriptor = Object.getOwnPropertyDescriptor(document, 'execCommand');
    openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
    vi.mocked(fetchUserSettings).mockResolvedValue(activeSettings);
    vi.mocked(updateUserSettings).mockResolvedValue(activeSettings);
    vi.mocked(fetchBridgeConnectorConfig).mockResolvedValue({
      config: {
        theia_base_url: 'http://localhost:3000',
        theia_origin: 'http://localhost:3000',
      },
      downloads: [
        {
          label: 'Linux x64',
          os: 'linux',
          arch: 'amd64',
          url: '/api/v1/settings/bridge/connector/download/linux/amd64',
          available: false,
        },
        {
          label: 'Windows x64',
          os: 'windows',
          arch: 'amd64',
          url: '/api/v1/settings/bridge/connector/download/windows/amd64',
          available: true,
        },
        {
          label: 'macOS Intel',
          os: 'darwin',
          arch: 'amd64',
          url: '/api/v1/settings/bridge/connector/download/darwin/amd64',
          available: false,
        },
      ],
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    openSpy.mockRestore();
    restoreOwnProperty(navigator, 'clipboard', clipboardDescriptor);
    restoreOwnProperty(document, 'execCommand', execCommandDescriptor);
  });

  it('renders unavailable connector binaries as disabled buttons and includes macOS targets', async () => {
    render(<UserSettingsPage />);

    await screen.findByRole('heading', { name: 'User Settings' });

    fireEvent.click(screen.getByRole('button', { name: 'Bridge connector actions' }));

    await waitFor(() => {
      expect(screen.getByRole('menuitem', { name: 'Download Linux x64' })).toBeDisabled();
    });
    expect(screen.getByRole('menuitem', { name: 'Download Windows x64' })).toHaveAttribute(
      'href',
      '/api/v1/settings/bridge/connector/download/windows/amd64',
    );
    expect(screen.getByRole('menuitem', { name: 'Download macOS Intel' })).toBeDisabled();
  });

  it('uses comboboxes for timezone and locale selections', async () => {
    render(<UserSettingsPage />);

    const timezone = await screen.findByRole('combobox', { name: 'Timezone' });
    const locale = screen.getByRole('combobox', { name: 'Locale' });

    expect(timezone).toHaveValue('UTC');
    expect(locale).toHaveValue('en-US');

    fireEvent.change(timezone, { target: { value: 'Europe/Rome' } });
    fireEvent.change(locale, { target: { value: 'it-IT' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save Profile' }));

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith(
        expect.objectContaining({
          timezone: 'Europe/Rome',
          locale: 'it-IT',
        }),
      );
    });
    const payload = vi.mocked(updateUserSettings).mock.calls[0]?.[0] ?? {};
    expect(payload).not.toHaveProperty('bridge_port');
    expect(payload).not.toHaveProperty('bridge_port_override');
  });

  it('moves bridge port editing into the bridge connector section as an optional user override', async () => {
    render(<UserSettingsPage />);

    await screen.findByRole('heading', { name: 'User Settings' });

    const accountProfile = screen.getByRole('region', { name: 'Account Profile' });
    const bridgeConnector = screen.getByRole('region', { name: 'Bridge Connector' });

    expect(within(accountProfile).queryByRole('spinbutton', { name: 'Bridge port' })).toBeNull();
    expect(within(bridgeConnector).getByRole('spinbutton', { name: 'Bridge port' })).toHaveValue(
      1337,
    );
    expect(within(bridgeConnector).getByLabelText('Use global bridge port')).toBeChecked();
    expect(within(bridgeConnector).getByRole('spinbutton', { name: 'Bridge port' })).toBeDisabled();

    fireEvent.click(within(bridgeConnector).getByLabelText('Use global bridge port'));
    fireEvent.change(within(bridgeConnector).getByRole('spinbutton', { name: 'Bridge port' }), {
      target: { value: '1444' },
    });
    fireEvent.click(within(bridgeConnector).getByRole('button', { name: 'Save Bridge Port' }));

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({ bridge_port_override: 1444 });
    });
  });

  it('clears the user bridge port override when the global bridge port is selected', async () => {
    vi.mocked(fetchUserSettings).mockResolvedValue({
      ...activeSettings,
      preferences: {
        ...activeSettings.preferences,
        bridge_port: 1444,
        global_bridge_port: 1337,
        bridge_port_override: 1444,
      },
    });

    render(<UserSettingsPage />);

    await screen.findByRole('heading', { name: 'User Settings' });

    const bridgeConnector = screen.getByRole('region', { name: 'Bridge Connector' });
    expect(within(bridgeConnector).getByLabelText('Use global bridge port')).not.toBeChecked();

    fireEvent.click(within(bridgeConnector).getByLabelText('Use global bridge port'));
    fireEvent.click(within(bridgeConnector).getByRole('button', { name: 'Save Bridge Port' }));

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({ bridge_port_override: null });
    });
  });

  it('groups bridge connector secret and download actions in a menu', async () => {
    render(<UserSettingsPage />);

    await screen.findByRole('heading', { name: 'User Settings' });

    expect(screen.queryByRole('button', { name: 'Rotate Secret' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Revoke Secret' })).toBeNull();

    const menuButton = screen.getByRole('button', { name: 'Bridge connector actions' });
    expect(menuButton).toHaveAttribute('aria-expanded', 'false');

    fireEvent.click(menuButton);

    expect(menuButton).toHaveAttribute('aria-expanded', 'true');
    const menu = screen.getByRole('menu', { name: 'Bridge connector actions' });
    expect(within(menu).getByRole('menuitem', { name: 'Rotate Secret' })).toBeInTheDocument();
    expect(within(menu).getByRole('menuitem', { name: 'Revoke Secret' })).toBeInTheDocument();
    expect(within(menu).getByRole('menuitem', { name: 'Download Windows x64' })).toHaveAttribute(
      'href',
      '/api/v1/settings/bridge/connector/download/windows/amd64',
    );
  });

  it('falls back to textarea copy for the one-time bridge secret when clipboard is unavailable', async () => {
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: execCommand,
    });
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: undefined,
    });
    vi.mocked(fetchUserSettings).mockResolvedValue({
      ...activeSettings,
      bridge: {
        configured: false,
        credential: null,
      },
    });
    vi.mocked(generateBridgeSecret).mockResolvedValue({ secret: 'theia_bridge_secret_once' });

    render(<UserSettingsPage />);

    await screen.findByRole('heading', { name: 'User Settings' });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Generate Bridge Secret' }));
    });
    await screen.findByText('theia_bridge_secret_once');

    vi.useFakeTimers();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Copy Bridge Secret' }));
    });
    await act(async () => {});

    expect(execCommand).toHaveBeenCalledWith('copy');
    expect(screen.getByText('Bridge Secret copied')).toBeInTheDocument();
    const copiedButton = screen.getByRole('button', { name: 'Bridge Secret copied' });
    expect(within(copiedButton).getByText('check')).toBeInTheDocument();
    expect(copiedButton).toHaveClass('scale-105');

    act(() => {
      vi.advanceTimersByTime(1500);
    });

    const resetButton = screen.getByRole('button', { name: 'Copy Bridge Secret' });
    expect(within(resetButton).getByText('content_copy')).toBeInTheDocument();
    expect(resetButton).not.toHaveClass('scale-105');
  });

  it('opens the local connector setup wizard on the default bridge port', async () => {
    render(<UserSettingsPage />);

    await screen.findByRole('heading', { name: 'User Settings' });

    fireEvent.click(screen.getByRole('button', { name: 'Configure Local Connector' }));

    expect(openSpy).toHaveBeenCalledWith(
      'http://localhost:1337/setup',
      '_blank',
      'noopener,noreferrer',
    );
  });

  it('opens the local connector setup wizard on the effective bridge port', async () => {
    vi.mocked(fetchUserSettings).mockResolvedValue({
      ...activeSettings,
      preferences: {
        ...activeSettings.preferences,
        bridge_port: 9000,
        global_bridge_port: 1337,
        bridge_port_override: 9000,
      },
    });

    render(<UserSettingsPage />);

    await screen.findByRole('heading', { name: 'User Settings' });

    fireEvent.click(screen.getByRole('button', { name: 'Configure Local Connector' }));

    expect(openSpy).toHaveBeenCalledWith(
      'http://localhost:9000/setup',
      '_blank',
      'noopener,noreferrer',
    );
  });
});
