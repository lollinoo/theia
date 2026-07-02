/**
 * Exercises sshcredential form operations dashboard behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { CredentialProfile } from '../../types/api';
import { SSHCredentialForm } from './SSHCredentialForm';

// Mock api/client — SSHCredentialForm must use assignCredentialProfile and
// unassignCredentialProfile, NOT updateDevice (T-27-07 mitigation).
vi.mock('../../api/client', () => ({
  fetchCredentialProfiles: vi.fn(),
  createCredentialProfile: vi.fn(),
  assignCredentialProfile: vi.fn(),
  unassignCredentialProfile: vi.fn(),
  testSSHConnection: vi.fn(),
  resetSSHHostKey: vi.fn(),
}));

function mockProfile(overrides: Partial<CredentialProfile> = {}): CredentialProfile {
  return {
    id: 'profile-1',
    name: 'Admin Profile',
    description: '',
    username: 'admin',
    port: 22,
    auth_method: 'password',
    role: 'Admin',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('SSHCredentialForm — renders SSH profile dropdown', () => {
  it('renders a profile selector populated with fetched profiles', async () => {
    const { fetchCredentialProfiles } = await import('../../api/client');
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValue([
      mockProfile({ id: 'profile-1', name: 'Admin Profile' }),
      mockProfile({ id: 'profile-2', name: 'Backup Profile' }),
    ]);

    render(<SSHCredentialForm deviceId="dev-1" />);

    await waitFor(() => {
      expect(screen.getByText('Admin Profile (admin:22)')).toBeInTheDocument();
      expect(screen.getByText('Backup Profile (admin:22)')).toBeInTheDocument();
    });
  });

  it('renders a "No SSH Profile" option as default', async () => {
    const { fetchCredentialProfiles } = await import('../../api/client');
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValue([]);

    render(<SSHCredentialForm deviceId="dev-1" />);

    await waitFor(() => {
      expect(screen.getByText('-- No SSH Profile --')).toBeInTheDocument();
    });
  });
});

describe('SSHCredentialForm — Save calls assignCredentialProfile not updateDevice', () => {
  it('calls assignCredentialProfile with correct args when a profile is selected and saved', async () => {
    const { fetchCredentialProfiles, assignCredentialProfile } = await import('../../api/client');
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValue([
      mockProfile({ id: 'profile-1', name: 'Admin Profile' }),
    ]);
    (assignCredentialProfile as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);

    render(<SSHCredentialForm deviceId="dev-1" currentProfileId={undefined} />);

    // Wait for profiles to load
    await waitFor(() => {
      expect(screen.getByText('Admin Profile (admin:22)')).toBeInTheDocument();
    });

    // Select the profile
    const select = screen.getByRole('combobox');
    fireEvent.change(select, { target: { value: 'profile-1' } });

    // Click Save
    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => {
      expect(assignCredentialProfile).toHaveBeenCalledWith('dev-1', 'profile-1');
    });
  });

  it('does not call updateDevice when saving a credential profile assignment', async () => {
    const { fetchCredentialProfiles, assignCredentialProfile } = await import('../../api/client');
    // updateDevice is deliberately NOT in the mock — if the component tried to
    // import and call it, the test would throw a "not a function" error.
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValue([
      mockProfile({ id: 'profile-1', name: 'Admin Profile' }),
    ]);
    (assignCredentialProfile as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);

    render(<SSHCredentialForm deviceId="dev-1" />);

    await waitFor(() => {
      expect(screen.getByText('Admin Profile (admin:22)')).toBeInTheDocument();
    });

    const select = screen.getByRole('combobox');
    fireEvent.change(select, { target: { value: 'profile-1' } });

    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => {
      // assignCredentialProfile was called — updateDevice was never needed
      expect(assignCredentialProfile).toHaveBeenCalledTimes(1);
    });
  });

  it('calls unassignCredentialProfile with the previous profile before assigning a new one', async () => {
    const { fetchCredentialProfiles, assignCredentialProfile, unassignCredentialProfile } =
      await import('../../api/client');
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValue([
      mockProfile({ id: 'profile-1', name: 'Admin Profile' }),
      mockProfile({ id: 'profile-2', name: 'Backup Profile' }),
    ]);
    (unassignCredentialProfile as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
    (assignCredentialProfile as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);

    // currentProfileId = 'profile-1' means profile-1 is already assigned
    render(<SSHCredentialForm deviceId="dev-1" currentProfileId="profile-1" />);

    await waitFor(() => {
      expect(screen.getByText('Backup Profile (admin:22)')).toBeInTheDocument();
    });

    // Switch to profile-2
    const select = screen.getByRole('combobox');
    fireEvent.change(select, { target: { value: 'profile-2' } });

    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => {
      // Must unassign old profile first, then assign new one
      expect(unassignCredentialProfile).toHaveBeenCalledWith('dev-1', 'profile-1');
      expect(assignCredentialProfile).toHaveBeenCalledWith('dev-1', 'profile-2');
    });
  });

  it('calls unassignCredentialProfile only when selecting no profile (unassign path)', async () => {
    const { fetchCredentialProfiles, assignCredentialProfile, unassignCredentialProfile } =
      await import('../../api/client');
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValue([
      mockProfile({ id: 'profile-1', name: 'Admin Profile' }),
    ]);
    (unassignCredentialProfile as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
    (assignCredentialProfile as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);

    // currentProfileId is set — a profile is currently assigned
    render(<SSHCredentialForm deviceId="dev-1" currentProfileId="profile-1" />);

    await waitFor(() => {
      expect(screen.getByText('Admin Profile (admin:22)')).toBeInTheDocument();
    });

    // Deselect — choose "No SSH Profile"
    const select = screen.getByRole('combobox');
    fireEvent.change(select, { target: { value: '' } });

    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => {
      // Only unassign — no new assign
      expect(unassignCredentialProfile).toHaveBeenCalledWith('dev-1', 'profile-1');
      expect(assignCredentialProfile).not.toHaveBeenCalled();
    });
  });
});

describe('SSHCredentialForm — SSH host-key mismatch recovery', () => {
  it('offers a confirmed SSH host key reset from the Test button result', async () => {
    const { fetchCredentialProfiles, resetSSHHostKey, testSSHConnection } = await import(
      '../../api/client'
    );
    (fetchCredentialProfiles as ReturnType<typeof vi.fn>).mockResolvedValue([
      mockProfile({ id: 'profile-1', name: 'Admin Profile' }),
    ]);
    (testSSHConnection as ReturnType<typeof vi.fn>).mockResolvedValue({
      success: false,
      error: 'SSH connection to 10.8.20.1 failed: SSH host key mismatch for 10.8.20.1:22',
      error_code: 'ssh_host_key_mismatch',
    });
    (resetSSHHostKey as ReturnType<typeof vi.fn>).mockResolvedValue({
      target: '10.8.20.1',
      port: 22,
      removed: true,
    });
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    render(<SSHCredentialForm deviceId="dev-1" currentProfileId="profile-1" />);

    await waitFor(() => {
      expect(screen.getByText('Admin Profile (admin:22)')).toBeInTheDocument();
    });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Test' }));
    });

    expect(await screen.findByText('SSH host key changed')).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /reset ssh host key/i }));
    });

    expect(resetSSHHostKey).toHaveBeenCalledWith('dev-1');
    expect(
      screen.getByText('SSH host key reset. Run Test again to trust the new key.'),
    ).toBeInTheDocument();
  });
});
