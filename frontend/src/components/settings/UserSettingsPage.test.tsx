import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  type UserSettingsResponse,
  fetchBridgeConnectorConfig,
  fetchUserSettings,
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

describe('UserSettingsPage', () => {
  beforeEach(() => {
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
  });

  it('moves bridge port editing into the bridge connector section', async () => {
    render(<UserSettingsPage />);

    await screen.findByRole('heading', { name: 'User Settings' });

    const accountProfile = screen.getByRole('region', { name: 'Account Profile' });
    const bridgeConnector = screen.getByRole('region', { name: 'Bridge Connector' });

    expect(within(accountProfile).queryByRole('spinbutton', { name: 'Bridge port' })).toBeNull();
    expect(within(bridgeConnector).getByRole('spinbutton', { name: 'Bridge port' })).toHaveValue(
      1337,
    );

    fireEvent.change(within(bridgeConnector).getByRole('spinbutton', { name: 'Bridge port' }), {
      target: { value: '1444' },
    });
    fireEvent.click(within(bridgeConnector).getByRole('button', { name: 'Save Bridge Port' }));

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({ bridge_port: 1444 });
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
});
