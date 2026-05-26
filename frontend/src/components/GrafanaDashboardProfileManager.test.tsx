import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { GrafanaDashboardProfileManager } from './GrafanaDashboardProfileManager';

vi.mock('../api/client', () => ({
  fetchGrafanaDashboardConfig: vi.fn().mockResolvedValue({
    profiles: [
      {
        id: 'profile-1',
        name: 'RouterBoard shared',
        url_template: 'https://grafana.example/d/router?var-device={{hostname}}',
        variable_source: 'hostname',
      },
    ],
    default_profile_id: 'profile-1',
    device_overrides: {},
  }),
  createGrafanaDashboardProfile: vi.fn().mockResolvedValue({
    profiles: [],
    default_profile_id: '',
    device_overrides: {},
  }),
  updateGrafanaDashboardProfile: vi.fn(),
  deleteGrafanaDashboardProfile: vi.fn().mockResolvedValue(undefined),
}));

describe('GrafanaDashboardProfileManager', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('loads dashboard profile config and shows default profile', async () => {
    render(<GrafanaDashboardProfileManager />);

    expect(await screen.findByText('RouterBoard shared')).toBeInTheDocument();
    expect(screen.getByText(/Default/)).toBeInTheDocument();
  });

  it('keeps long dashboard URLs constrained inside the profile card', async () => {
    const { fetchGrafanaDashboardConfig } = await import('../api/client');
    const longUrl =
      'https://grafana.example/d/router-overview/very-long-dashboard-slug?orgId=1&var-routerboard={{hostname}}&var-site=main-core&var-link=a-very-long-link-identifier-that-should-not-resize-the-admin-settings-card';
    vi.mocked(fetchGrafanaDashboardConfig).mockResolvedValueOnce({
      profiles: [
        {
          id: 'profile-long-url',
          name: 'Long URL profile',
          url_template: longUrl,
          variable_source: 'hostname',
        },
      ],
      default_profile_id: 'profile-long-url',
      device_overrides: {},
    });

    render(<GrafanaDashboardProfileManager />);

    const url = await screen.findByText(longUrl);
    expect(url).toHaveClass('max-w-full', 'truncate');
    expect(screen.getByTestId('grafana-profile-card-profile-long-url')).toHaveClass(
      'max-w-full',
      'overflow-hidden',
    );
  });

  it('creates dashboard profile with URL template and default flag', async () => {
    const { createGrafanaDashboardProfile } = await import('../api/client');
    render(<GrafanaDashboardProfileManager />);

    fireEvent.click(await screen.findByRole('button', { name: /new/i }));
    fireEvent.change(screen.getByPlaceholderText('RouterBoard shared'), {
      target: { value: 'Edge routers' },
    });
    fireEvent.change(
      screen.getByPlaceholderText('https://grafana.example/d/router?var-device={{hostname}}'),
      {
        target: { value: 'https://grafana.example/d/router?var-device={{ip}}' },
      },
    );
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'ip' } });
    fireEvent.click(screen.getByLabelText(/use as default/i));
    fireEvent.click(screen.getByRole('button', { name: /create profile/i }));

    await waitFor(() => {
      expect(createGrafanaDashboardProfile).toHaveBeenCalledWith({
        name: 'Edge routers',
        url_template: 'https://grafana.example/d/router?var-device={{ip}}',
        variable_source: 'ip',
        is_default: true,
      });
    });
  });
});
