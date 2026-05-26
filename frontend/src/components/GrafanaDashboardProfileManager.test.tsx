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
