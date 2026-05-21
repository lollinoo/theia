import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  assignAdminUserRole,
  createAdminPasswordReset,
  createAdminUser,
  fetchAdminAuditLogs,
  fetchAdminDashboard,
  fetchAdminPermissions,
  fetchAdminRoles,
  fetchAdminUsers,
  removeAdminUserRole,
  setAdminUserStatus,
  updateAdminUser,
} from '../api/client';
import { AdminDashboard } from './AdminDashboard';

const settingsPanelPropsMock = vi.hoisted(() => vi.fn());

vi.mock('../api/client', () => ({
  fetchAdminDashboard: vi.fn(),
  fetchAdminUsers: vi.fn(),
  fetchAdminRoles: vi.fn(),
  fetchAdminPermissions: vi.fn(),
  fetchAdminAuditLogs: vi.fn(),
  createAdminUser: vi.fn(),
  updateAdminUser: vi.fn(),
  setAdminUserStatus: vi.fn(),
  assignAdminUserRole: vi.fn(),
  removeAdminUserRole: vi.fn(),
  createAdminPasswordReset: vi.fn(),
}));

vi.mock('./SettingsPanel', () => ({
  SettingsPanel: (props: { onSettingsChange?: () => void }) => {
    settingsPanelPropsMock(props);
    return <div data-testid="global-settings-panel">Global settings</div>;
  },
}));

const adminUser = {
  id: 'user-1',
  username: 'alice',
  email: 'alice@example.test',
  display_name: 'Alice',
  status: 'active',
  must_change_password: false,
  roles: ['operator'],
  permissions: ['topology:read'],
};

const adminRole = {
  id: 'role-1',
  name: 'operator',
  description: 'Operators',
  is_system_role: true,
  permissions: ['topology:read'],
};

describe('AdminDashboard', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.mocked(fetchAdminDashboard).mockReset();
    vi.mocked(fetchAdminUsers).mockReset();
    vi.mocked(fetchAdminRoles).mockReset();
    vi.mocked(fetchAdminPermissions).mockReset();
    vi.mocked(fetchAdminAuditLogs).mockReset();
    vi.mocked(createAdminUser).mockReset();
    vi.mocked(updateAdminUser).mockReset();
    vi.mocked(setAdminUserStatus).mockReset();
    vi.mocked(assignAdminUserRole).mockReset();
    vi.mocked(removeAdminUserRole).mockReset();
    vi.mocked(createAdminPasswordReset).mockReset();
    settingsPanelPropsMock.mockClear();

    vi.mocked(fetchAdminDashboard).mockResolvedValue({
      stats: {
        total_users: 1,
        active_users: 1,
        disabled_users: 0,
        locked_users: 0,
        recent_logins: 3,
        recent_failed_login_attempts: 1,
      },
      recent_audit_logs: [
        {
          id: 'audit-1',
          actor_user_id: 'admin-user-1',
          action: 'auth.login',
          resource: 'session',
          resource_id: 'session-1',
          created_at: '2026-05-21T10:00:00Z',
        },
      ],
    });
    vi.mocked(fetchAdminUsers).mockResolvedValue([adminUser]);
    vi.mocked(fetchAdminRoles).mockResolvedValue([adminRole]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['topology:read', 'admin:dashboard:read']);
    vi.mocked(fetchAdminAuditLogs).mockResolvedValue([
      {
        id: 'audit-2',
        actor_user_id: 'admin-user-1',
        action: 'user.update',
        target_user_id: 'user-1',
        resource: 'user',
        resource_id: 'user-1',
        created_at: '2026-05-21T11:00:00Z',
      },
    ]);
    vi.mocked(createAdminUser).mockResolvedValue(adminUser);
    vi.mocked(updateAdminUser).mockResolvedValue(adminUser);
    vi.mocked(setAdminUserStatus).mockResolvedValue({
      ...adminUser,
      status: 'disabled',
    });
    vi.mocked(assignAdminUserRole).mockResolvedValue(adminUser);
    vi.mocked(removeAdminUserRole).mockResolvedValue(adminUser);
    vi.mocked(createAdminPasswordReset).mockResolvedValue({
      reset_token: 'reset-token-1',
    });
  });

  it('fetches and renders overview, users, roles, and audit logs', async () => {
    render(<AdminDashboard />);

    expect(await screen.findByText('Admin')).toBeInTheDocument();
    expect(screen.getByText('Total users')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: 'Users' }));
    expect(screen.getByText('alice')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: 'Roles' }));
    expect(await screen.findByText('operator')).toBeInTheDocument();
    expect(screen.getAllByText('topology:read').length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole('tab', { name: 'Audit Logs' }));
    expect(await screen.findByText('user.update')).toBeInTheDocument();
    expect(screen.getByText('admin-user-1')).toBeInTheDocument();
    expect(screen.getByText('user:user-1')).toBeInTheDocument();
  });

  it('exposes global settings inside admin only when the user can read and update settings', async () => {
    vi.mocked(fetchAdminPermissions).mockResolvedValue([
      'admin:dashboard:read',
      'settings:read',
      'settings:update',
    ]);

    render(<AdminDashboard />);

    fireEvent.click(await screen.findByRole('tab', { name: 'Settings' }));

    expect(screen.getByTestId('global-settings-panel')).toBeInTheDocument();
    expect(settingsPanelPropsMock).toHaveBeenLastCalledWith({});
  });

  it('hides global settings from admin users without settings update permission', async () => {
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['admin:dashboard:read']);

    render(<AdminDashboard />);

    expect(await screen.findByText('Total users')).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Settings' })).not.toBeInTheDocument();
  });

  it('searches users and confirms privilege-changing actions', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    render(<AdminDashboard />);

    fireEvent.click(await screen.findByRole('tab', { name: 'Users' }));
    fireEvent.change(screen.getByLabelText('Search users'), {
      target: { value: 'alice' },
    });
    expect(screen.getByText('alice')).toBeInTheDocument();

    const row = screen.getByRole('row', { name: /alice/i });
    fireEvent.click(within(row).getByRole('button', { name: 'Disable user alice' }));

    await waitFor(() => {
      expect(setAdminUserStatus).toHaveBeenCalledWith('user-1', 'disabled');
    });
    expect(confirmSpy).toHaveBeenCalled();

    fireEvent.click(within(row).getByRole('button', { name: 'Reset password for alice' }));
    expect(await screen.findByText('reset-token-1')).toBeInTheDocument();
  });

  it('clears one-time reset tokens when leaving users, refreshing, dismissing, or hiding admin', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const { rerender } = render(<AdminDashboard visible />);

    fireEvent.click(await screen.findByRole('tab', { name: 'Users' }));
    fireEvent.click(screen.getByRole('button', { name: 'Reset password for alice' }));
    expect(await screen.findByText('reset-token-1')).toBeInTheDocument();
    expect(confirmSpy).toHaveBeenCalled();

    fireEvent.click(screen.getByRole('tab', { name: 'Roles' }));
    expect(screen.queryByText('reset-token-1')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: 'Users' }));
    fireEvent.click(screen.getByRole('button', { name: 'Reset password for alice' }));
    expect(await screen.findByText('reset-token-1')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss reset token' }));
    expect(screen.queryByText('reset-token-1')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Reset password for alice' }));
    expect(await screen.findByText('reset-token-1')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(screen.queryByText('reset-token-1')).not.toBeInTheDocument();
    await screen.findByRole('button', { name: 'Reset password for alice' });

    fireEvent.click(screen.getByRole('button', { name: 'Reset password for alice' }));
    expect(await screen.findByText('reset-token-1')).toBeInTheDocument();
    rerender(<AdminDashboard visible={false} />);
    expect(screen.queryByText('reset-token-1')).not.toBeInTheDocument();
  });

  it('confirms enabling disabled users before changing status to active', async () => {
    const disabledUser = { ...adminUser, status: 'disabled' };
    vi.mocked(fetchAdminUsers).mockResolvedValue([disabledUser]);
    vi.mocked(setAdminUserStatus).mockResolvedValue({ ...disabledUser, status: 'active' });
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    render(<AdminDashboard />);

    fireEvent.click(await screen.findByRole('tab', { name: 'Users' }));
    const row = screen.getByRole('row', { name: /alice/i });
    fireEvent.click(within(row).getByRole('button', { name: 'Enable user alice' }));

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledWith('Change alice status to active?');
      expect(setAdminUserStatus).toHaveBeenCalledWith('user-1', 'active');
    });
  });
});
