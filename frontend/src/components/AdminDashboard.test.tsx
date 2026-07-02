/**
 * Exercises admin dashboard component behavior so refactors preserve the documented contract.
 */
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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
  fetchSettings,
  removeAdminUserRole,
  setAdminUserStatus,
  updateAdminRolePermissions,
  updateAdminUser,
} from '../api/client';
import { AdminDashboard } from './AdminDashboard';

const settingsPanelPropsMock = vi.hoisted(() => vi.fn());
const authState = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock('../api/client', () => ({
  fetchAdminDashboard: vi.fn(),
  fetchAdminUsers: vi.fn(),
  fetchAdminRoles: vi.fn(),
  fetchAdminPermissions: vi.fn(),
  fetchAdminAuditLogs: vi.fn(),
  fetchSettings: vi.fn(),
  createAdminUser: vi.fn(),
  updateAdminUser: vi.fn(),
  setAdminUserStatus: vi.fn(),
  updateAdminRolePermissions: vi.fn(),
  assignAdminUserRole: vi.fn(),
  removeAdminUserRole: vi.fn(),
  createAdminPasswordReset: vi.fn(),
}));

vi.mock('./SettingsPanel', () => ({
  SettingsPanel: (props: { onSettingsChange?: (changed?: { timezone?: string }) => void }) => {
    settingsPanelPropsMock(props);
    return <div data-testid="global-settings-panel">Global settings</div>;
  },
}));

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    status: 'authenticated',
    user: { permissions: [...authState.permissions] },
    error: null,
    refresh: vi.fn(),
    login: vi.fn(),
    logout: vi.fn(),
    changePassword: vi.fn(),
    hasPermission: (permission: string) => authState.permissions.has(permission),
  }),
}));

function grantPermissions(...permissions: string[]) {
  authState.permissions.clear();
  for (const permission of permissions) {
    authState.permissions.add(permission);
  }
}

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

const actorUser = {
  ...adminUser,
  id: 'admin-user-1',
  username: 'administrator',
  email: 'administrator@example.test',
  display_name: 'Administrator',
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
    vi.mocked(fetchSettings).mockReset();
    vi.mocked(createAdminUser).mockReset();
    vi.mocked(updateAdminUser).mockReset();
    vi.mocked(setAdminUserStatus).mockReset();
    vi.mocked(updateAdminRolePermissions).mockReset();
    vi.mocked(assignAdminUserRole).mockReset();
    vi.mocked(removeAdminUserRole).mockReset();
    vi.mocked(createAdminPasswordReset).mockReset();
    settingsPanelPropsMock.mockClear();
    grantPermissions(
      'admin:dashboard:read',
      'users:read',
      'users:create',
      'users:update',
      'users:disable',
      'roles:read',
      'roles:assign',
      'roles:update',
      'audit_logs:read',
      'settings:read',
      'settings:update',
    );

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
    vi.mocked(fetchAdminUsers).mockResolvedValue([adminUser, actorUser]);
    vi.mocked(fetchAdminRoles).mockResolvedValue([adminRole]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['topology:read', 'admin:dashboard:read']);
    vi.mocked(fetchSettings).mockResolvedValue({ timezone: 'UTC' });
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
    expect(screen.getByText('Administrator')).toBeInTheDocument();
    expect(screen.getByText('User: Alice')).toBeInTheDocument();
  });

  it('keeps roles read-only without roles update permission', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read');
    vi.mocked(fetchAdminRoles).mockResolvedValue([
      { ...adminRole, id: 'user', name: 'user', permissions: ['account:manage'] },
    ]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['account:manage', 'bridge:token:create']);

    render(<AdminDashboard />);
    fireEvent.click(await screen.findByRole('tab', { name: 'Roles' }));

    expect(await screen.findByText('user')).toBeInTheDocument();
    expect(
      screen.queryByRole('checkbox', { name: 'bridge:token:create for user' }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /Save role permissions/i }),
    ).not.toBeInTheDocument();
  });

  it('saves editable role permissions and refreshes the displayed role', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read', 'roles:update');
    vi.mocked(fetchAdminRoles).mockResolvedValue([
      { ...adminRole, id: 'user', name: 'user', permissions: ['account:manage'] },
    ]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['account:manage', 'bridge:token:create']);
    vi.mocked(updateAdminRolePermissions).mockResolvedValue({
      ...adminRole,
      id: 'user',
      name: 'user',
      permissions: ['bridge:token:create'],
    });

    render(<AdminDashboard />);
    fireEvent.click(await screen.findByRole('tab', { name: 'Roles' }));
    const bridgeCheckbox = await screen.findByRole('checkbox', {
      name: 'bridge:token:create for user',
    });
    const accountCheckbox = screen.getByRole('checkbox', { name: 'account:manage for user' });
    fireEvent.click(bridgeCheckbox);
    const saveButton = screen.getByRole('button', { name: 'Save role permissions for user' });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(updateAdminRolePermissions).toHaveBeenCalledWith('user', [
        'account:manage',
        'bridge:token:create',
      ]);
    });
    expect(await screen.findByText('Role permissions saved.')).toBeInTheDocument();
    expect(accountCheckbox).not.toBeChecked();
    expect(bridgeCheckbox).toBeChecked();
    expect(saveButton).toBeDisabled();
  });

  it('saves an empty role permission set after unchecking the last permission', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read', 'roles:update');
    vi.mocked(fetchAdminRoles).mockResolvedValue([
      { ...adminRole, id: 'user', name: 'user', permissions: ['account:manage'] },
    ]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['account:manage']);
    vi.mocked(updateAdminRolePermissions).mockResolvedValue({
      ...adminRole,
      id: 'user',
      name: 'user',
      permissions: [],
    });

    render(<AdminDashboard />);
    act(() => {
      fireEvent.click(screen.getByRole('tab', { name: 'Roles' }));
    });
    const accountCheckbox = await screen.findByRole('checkbox', {
      name: 'account:manage for user',
    });
    act(() => {
      fireEvent.click(accountCheckbox);
    });
    expect(accountCheckbox).not.toBeChecked();
    const saveButton = screen.getByRole('button', { name: 'Save role permissions for user' });
    expect(saveButton).toBeEnabled();

    act(() => {
      fireEvent.click(saveButton);
    });

    await waitFor(() => {
      expect(updateAdminRolePermissions).toHaveBeenCalledWith('user', []);
    });
  });

  it('clears role permission save feedback when refreshing admin data', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read', 'roles:update');
    vi.mocked(fetchAdminRoles).mockResolvedValue([
      { ...adminRole, id: 'user', name: 'user', permissions: ['account:manage'] },
    ]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['account:manage', 'bridge:token:create']);
    vi.mocked(updateAdminRolePermissions).mockResolvedValue({
      ...adminRole,
      id: 'user',
      name: 'user',
      permissions: ['account:manage', 'bridge:token:create'],
    });

    render(<AdminDashboard />);
    fireEvent.click(await screen.findByRole('tab', { name: 'Roles' }));
    fireEvent.click(await screen.findByRole('checkbox', { name: 'bridge:token:create for user' }));
    fireEvent.click(screen.getByRole('button', { name: 'Save role permissions for user' }));

    expect(await screen.findByText('Role permissions saved.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));

    await waitFor(() => {
      expect(fetchAdminRoles).toHaveBeenCalledTimes(2);
    });
    expect(await screen.findByText('user')).toBeInTheDocument();
    expect(screen.queryByText('Role permissions saved.')).not.toBeInTheDocument();
  });

  it('shows super admin as protected in the roles editor', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read', 'roles:update');
    vi.mocked(fetchAdminRoles).mockResolvedValue([
      { ...adminRole, id: 'super_admin', name: 'super_admin', permissions: ['roles:update'] },
    ]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['roles:update']);

    render(<AdminDashboard />);
    fireEvent.click(await screen.findByRole('tab', { name: 'Roles' }));

    expect(await screen.findByText('Protected recovery role')).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /Save role permissions/i }),
    ).not.toBeInTheDocument();
  });

  it('keeps edited role permissions visible when save fails', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read', 'roles:update');
    vi.mocked(fetchAdminRoles).mockResolvedValue([
      { ...adminRole, id: 'user', name: 'user', permissions: ['account:manage'] },
    ]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['account:manage', 'bridge:token:create']);
    vi.mocked(updateAdminRolePermissions).mockRejectedValue(new Error('permission denied'));

    render(<AdminDashboard />);
    fireEvent.click(await screen.findByRole('tab', { name: 'Roles' }));
    const bridgeCheckbox = await screen.findByRole('checkbox', {
      name: 'bridge:token:create for user',
    });
    fireEvent.click(bridgeCheckbox);
    fireEvent.click(screen.getByRole('button', { name: 'Save role permissions for user' }));

    expect(await screen.findByText('permission denied')).toBeInTheDocument();
    expect(bridgeCheckbox).toBeChecked();
  });

  it('keeps the roles editor horizontally scrollable on narrow screens', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read', 'roles:update');
    vi.mocked(fetchAdminRoles).mockResolvedValue([
      { ...adminRole, id: 'user', name: 'user', permissions: ['account:manage'] },
    ]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue([
      'account:manage',
      'bridge:token:create',
      'very:long:permission:key:that:needs:room:on:narrow:screens',
    ]);

    render(<AdminDashboard />);
    fireEvent.click(await screen.findByRole('tab', { name: 'Roles' }));

    const rolesTable = await screen.findByRole('table');
    expect(rolesTable.parentElement?.className).toContain('overflow-x-auto');
    expect(rolesTable.parentElement?.className).not.toContain('overflow-hidden');
  });

  it('disables all role save buttons while one role is saving', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read', 'roles:update');
    vi.mocked(fetchAdminRoles).mockResolvedValue([
      { ...adminRole, id: 'user', name: 'user', permissions: ['account:manage'] },
      {
        ...adminRole,
        id: 'operator',
        name: 'operator',
        permissions: ['account:manage'],
      },
    ]);
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['account:manage', 'bridge:token:create']);
    let resolveSave!: (role: typeof adminRole) => void;
    const savePromise = new Promise<typeof adminRole>((resolve) => {
      resolveSave = resolve;
    });
    vi.mocked(updateAdminRolePermissions).mockReturnValue(savePromise);

    render(<AdminDashboard />);
    fireEvent.click(await screen.findByRole('tab', { name: 'Roles' }));
    const userBridgeCheckbox = await screen.findByRole('checkbox', {
      name: 'bridge:token:create for user',
    });
    const operatorBridgeCheckbox = screen.getByRole('checkbox', {
      name: 'bridge:token:create for operator',
    });
    fireEvent.click(userBridgeCheckbox);
    fireEvent.click(operatorBridgeCheckbox);
    const userSave = screen.getByRole('button', { name: 'Save role permissions for user' });
    const operatorSave = screen.getByRole('button', {
      name: 'Save role permissions for operator',
    });
    fireEvent.click(userSave);

    await waitFor(() => {
      expect(userSave).toBeDisabled();
    });
    expect(operatorSave).toBeDisabled();
    expect(userBridgeCheckbox).toBeDisabled();
    expect(operatorBridgeCheckbox).toBeDisabled();

    await act(async () => {
      resolveSave({
        ...adminRole,
        id: 'user',
        name: 'user',
        permissions: ['account:manage', 'bridge:token:create'],
      });
      await savePromise;
    });
  });

  it('renders audit names and normalizes audit times with the configured timezone', async () => {
    vi.mocked(fetchAdminUsers).mockResolvedValue([adminUser, actorUser]);
    vi.mocked(fetchSettings).mockResolvedValue({ timezone: 'Europe/Rome' });

    render(<AdminDashboard />);

    expect(await screen.findByText('2026-05-21 12:00:00 Europe/Rome')).toBeInTheDocument();
    expect(screen.getByText('Administrator')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: 'Audit Logs' }));

    expect(await screen.findByText('2026-05-21 13:00:00 Europe/Rome')).toBeInTheDocument();
    expect(screen.getByText('User: Alice')).toBeInTheDocument();
    expect(screen.queryByText('admin-user-1')).not.toBeInTheDocument();
    expect(screen.queryByText('user:user-1')).not.toBeInTheDocument();
  });

  it('keeps overview and audit tables horizontally scrollable on narrow screens', async () => {
    render(<AdminDashboard />);

    expect(await screen.findByText('auth.login')).toBeInTheDocument();
    const overviewTable = screen.getByText('auth.login').closest('table');
    expect(overviewTable?.className).toContain('min-w-[42rem]');
    expect(overviewTable?.parentElement?.className).toContain('overflow-x-auto');
    expect(overviewTable?.parentElement?.className).not.toContain('overflow-hidden');

    fireEvent.click(screen.getByRole('tab', { name: 'Audit Logs' }));

    expect(await screen.findByText('user.update')).toBeInTheDocument();
    const auditTable = screen.getByText('user.update').closest('table');
    expect(auditTable?.className).toContain('min-w-[48rem]');
    expect(auditTable?.parentElement?.className).toContain('overflow-x-auto');
    expect(auditTable?.parentElement?.className).not.toContain('overflow-hidden');
  });

  it('loads overview without requesting admin sections the user cannot read', async () => {
    grantPermissions('admin:dashboard:read');

    render(<AdminDashboard />);

    expect(await screen.findByText('Total users')).toBeInTheDocument();
    expect(fetchAdminUsers).not.toHaveBeenCalled();
    expect(fetchAdminRoles).not.toHaveBeenCalled();
    expect(fetchAdminPermissions).not.toHaveBeenCalled();
    expect(fetchAdminAuditLogs).not.toHaveBeenCalled();
    expect(screen.queryByRole('tab', { name: 'Users' })).not.toBeInTheDocument();
  });

  it('exposes global settings inside admin only when the user can read and update settings', async () => {
    render(<AdminDashboard />);

    fireEvent.click(await screen.findByRole('tab', { name: 'Settings' }));

    expect(screen.getByTestId('global-settings-panel')).toBeInTheDocument();
    expect(settingsPanelPropsMock).toHaveBeenLastCalledWith({
      onSettingsChange: expect.any(Function),
    });
  });

  it('updates audit timestamps when admin settings reports a timezone change', async () => {
    render(<AdminDashboard />);

    expect(await screen.findByText('2026-05-21 10:00:00 UTC')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: 'Settings' }));
    const props = settingsPanelPropsMock.mock.lastCall?.[0];
    act(() => {
      props?.onSettingsChange?.({ timezone: 'Europe/Rome' });
    });

    fireEvent.click(screen.getByRole('tab', { name: 'Overview' }));

    expect(screen.getByText('2026-05-21 12:00:00 Europe/Rome')).toBeInTheDocument();
  });

  it('hides global settings from admin users without settings update permission', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read');
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['admin:dashboard:read']);

    render(<AdminDashboard />);

    expect(await screen.findByText('Total users')).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Settings' })).not.toBeInTheDocument();
  });

  it('does not expose settings from the system permission catalog alone', async () => {
    grantPermissions('admin:dashboard:read', 'roles:read');
    vi.mocked(fetchAdminPermissions).mockResolvedValue(['settings:read', 'settings:update']);

    render(<AdminDashboard />);

    expect(await screen.findByText('Total users')).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Settings' })).not.toBeInTheDocument();
  });

  it('hides user mutation controls from users with read-only user access', async () => {
    grantPermissions('admin:dashboard:read', 'users:read');

    render(<AdminDashboard />);
    fireEvent.click(await screen.findByRole('tab', { name: 'Users' }));

    const row = screen.getByRole('row', { name: /alice/i });
    expect(screen.queryByRole('button', { name: 'Create user' })).not.toBeInTheDocument();
    expect(
      within(row).queryByRole('button', { name: 'Disable user alice' }),
    ).not.toBeInTheDocument();
    expect(
      within(row).queryByRole('button', { name: 'Reset password for alice' }),
    ).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: /Remove operator/ })).not.toBeInTheDocument();
    expect(within(row).queryByLabelText('Role to assign to alice')).not.toBeInTheDocument();
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
    expect(screen.getByText(/Use this token from the sign-in reset form/i)).toBeInTheDocument();
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
