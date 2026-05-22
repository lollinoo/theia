import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  type UserSettingsResponse,
  fetchBridgeConnectorConfig,
  fetchUserSettings,
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

describe('UserSettingsPage bridge connector downloads', () => {
  beforeEach(() => {
    vi.mocked(fetchUserSettings).mockResolvedValue(activeSettings);
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

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Download Linux x64' })).toBeDisabled();
    });
    expect(screen.getByRole('link', { name: 'Download Windows x64' })).toHaveAttribute(
      'href',
      '/api/v1/settings/bridge/connector/download/windows/amd64',
    );
    expect(screen.getByRole('button', { name: 'Download macOS Intel' })).toBeDisabled();
  });
});
