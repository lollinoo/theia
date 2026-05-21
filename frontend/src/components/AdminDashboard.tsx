import { type FormEvent, useEffect, useMemo, useState } from 'react';
import {
  type AdminAuditLog,
  type AdminDashboardResponse,
  type AdminRole,
  type AuthUser,
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
import { MaterialIcon } from './MaterialIcon';

type AdminTab = 'overview' | 'users' | 'roles' | 'audit';

const tabs: Array<{ id: AdminTab; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'users', label: 'Users' },
  { id: 'roles', label: 'Roles' },
  { id: 'audit', label: 'Audit Logs' },
];

const emptyDashboard: AdminDashboardResponse = {
  stats: {
    total_users: 0,
    active_users: 0,
    disabled_users: 0,
    locked_users: 0,
    recent_logins: 0,
    recent_failed_login_attempts: 0,
  },
  recent_audit_logs: [],
};

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

function StatTile({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border border-outline-subtle bg-surface-container px-4 py-3">
      <div className="text-xs font-medium text-on-bg-secondary">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums text-on-bg">{value}</div>
    </div>
  );
}

function rolesLabel(user: AuthUser): string {
  return user.roles.length > 0 ? user.roles.join(', ') : 'No roles';
}

interface AdminDashboardProps {
  visible?: boolean;
}

export function AdminDashboard({ visible = true }: AdminDashboardProps = {}) {
  const [activeTab, setActiveTab] = useState<AdminTab>('overview');
  const [dashboard, setDashboard] = useState<AdminDashboardResponse>(emptyDashboard);
  const [users, setUsers] = useState<AuthUser[]>([]);
  const [roles, setRoles] = useState<AdminRole[]>([]);
  const [permissions, setPermissions] = useState<string[]>([]);
  const [auditLogs, setAuditLogs] = useState<AdminAuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [newUser, setNewUser] = useState({
    username: '',
    email: '',
    display_name: '',
    password: '',
    must_change_password: true,
  });
  const [selectedRoleByUser, setSelectedRoleByUser] = useState<Record<string, string>>({});
  const [savingUserId, setSavingUserId] = useState<string | null>(null);
  const [creatingUser, setCreatingUser] = useState(false);
  const [resetToken, setResetToken] = useState<{
    username: string;
    token: string;
  } | null>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    setResetToken(null);
    try {
      const [nextDashboard, nextUsers, nextRoles, nextPermissions, nextAuditLogs] =
        await Promise.all([
          fetchAdminDashboard(),
          fetchAdminUsers(),
          fetchAdminRoles(),
          fetchAdminPermissions(),
          fetchAdminAuditLogs(),
        ]);
      setDashboard(nextDashboard);
      setUsers(nextUsers);
      setRoles(nextRoles);
      setPermissions(nextPermissions);
      setAuditLogs(nextAuditLogs);
    } catch (loadError) {
      setError(errorMessage(loadError, 'Failed to load admin data'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    if (!visible) {
      setResetToken(null);
    }
  }, [visible]);

  useEffect(() => {
    if (activeTab !== 'users') {
      setResetToken(null);
    }
  }, [activeTab]);

  const filteredUsers = useMemo(() => {
    const query = search.trim().toLowerCase();
    return users.filter((user) => {
      const matchesStatus = statusFilter === 'all' || user.status === statusFilter;
      const haystack = `${user.username} ${user.email} ${user.display_name}`.toLowerCase();
      return matchesStatus && (query === '' || haystack.includes(query));
    });
  }, [search, statusFilter, users]);

  async function handleCreateUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreatingUser(true);
    setError(null);
    try {
      const createdUser = await createAdminUser({
        username: newUser.username.trim(),
        email: newUser.email.trim(),
        display_name: newUser.display_name.trim(),
        password: newUser.password,
        must_change_password: newUser.must_change_password,
      });
      setUsers((current) => [...current, createdUser]);
      setNewUser({
        username: '',
        email: '',
        display_name: '',
        password: '',
        must_change_password: true,
      });
      void load();
    } catch (createError) {
      setError(errorMessage(createError, 'Failed to create user'));
    } finally {
      setCreatingUser(false);
    }
  }

  async function patchUser(user: AuthUser, payload: { display_name?: string; email?: string }) {
    setSavingUserId(user.id);
    setError(null);
    try {
      const updatedUser = await updateAdminUser(user.id, payload);
      setUsers((current) =>
        current.map((candidate) => (candidate.id === updatedUser.id ? updatedUser : candidate)),
      );
    } catch (updateError) {
      setError(errorMessage(updateError, 'Failed to update user'));
    } finally {
      setSavingUserId(null);
    }
  }

  async function changeStatus(user: AuthUser, status: string) {
    if (status === user.status) {
      return;
    }
    if (!window.confirm(`Change ${user.username} status to ${status}?`)) {
      return;
    }
    setSavingUserId(user.id);
    setError(null);
    try {
      const updatedUser = await setAdminUserStatus(user.id, status);
      setUsers((current) =>
        current.map((candidate) => (candidate.id === updatedUser.id ? updatedUser : candidate)),
      );
    } catch (statusError) {
      setError(errorMessage(statusError, 'Failed to update status'));
    } finally {
      setSavingUserId(null);
    }
  }

  async function assignRole(user: AuthUser) {
    const roleId = selectedRoleByUser[user.id];
    if (!roleId) {
      return;
    }
    const role = roles.find((candidate) => candidate.id === roleId);
    if (!window.confirm(`Assign ${role?.name ?? 'role'} to ${user.username}?`)) {
      return;
    }
    setSavingUserId(user.id);
    setError(null);
    try {
      const updatedUser = await assignAdminUserRole(user.id, roleId);
      setUsers((current) =>
        current.map((candidate) => (candidate.id === updatedUser.id ? updatedUser : candidate)),
      );
    } catch (roleError) {
      setError(errorMessage(roleError, 'Failed to assign role'));
    } finally {
      setSavingUserId(null);
    }
  }

  async function removeRole(user: AuthUser, roleName: string) {
    const role = roles.find(
      (candidate) => candidate.name === roleName || candidate.id === roleName,
    );
    if (!role || !window.confirm(`Remove ${role.name} from ${user.username}?`)) {
      return;
    }
    setSavingUserId(user.id);
    setError(null);
    try {
      await removeAdminUserRole(user.id, role.id);
      setUsers((current) =>
        current.map((candidate) =>
          candidate.id === user.id
            ? {
                ...candidate,
                roles: candidate.roles.filter((name) => name !== roleName),
              }
            : candidate,
        ),
      );
    } catch (roleError) {
      setError(errorMessage(roleError, 'Failed to remove role'));
    } finally {
      setSavingUserId(null);
    }
  }

  async function resetPassword(user: AuthUser) {
    if (!window.confirm(`Create a one-time password reset token for ${user.username}?`)) {
      return;
    }
    setSavingUserId(user.id);
    setError(null);
    try {
      const response = await createAdminPasswordReset(user.id);
      setResetToken({ username: user.username, token: response.reset_token });
    } catch (resetError) {
      setError(errorMessage(resetError, 'Failed to create reset token'));
    } finally {
      setSavingUserId(null);
    }
  }

  return (
    <div className="min-h-full px-4 pt-24 pb-10 sm:px-6">
      <div className="mx-auto flex max-w-7xl flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-semibold text-on-bg">Admin</h1>
            <p className="text-sm text-on-bg-secondary">Users, roles, and access events.</p>
          </div>
          <button
            type="button"
            onClick={() => void load()}
            className="inline-flex items-center gap-2 rounded-md border border-outline-subtle bg-surface-container px-3 py-2 text-sm text-on-bg transition-colors hover:bg-surface-container-high"
          >
            <MaterialIcon name="refresh" size={18} />
            Refresh
          </button>
        </div>

        <div className="flex flex-wrap gap-2" role="tablist" aria-label="Admin sections">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={activeTab === tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`rounded-md border px-3 py-2 text-sm transition-colors ${
                activeTab === tab.id
                  ? 'border-outline-strong bg-surface-container-high font-semibold text-on-bg'
                  : 'border-outline-subtle bg-surface text-on-bg-secondary hover:bg-surface-container'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {error && (
          <div className="rounded-lg border border-warning/40 bg-warning/10 px-4 py-3 text-sm text-warning">
            {error}
          </div>
        )}

        {resetToken && (
          <div className="rounded-lg border border-outline-subtle bg-surface-container px-4 py-3">
            <div className="flex items-center justify-between gap-3">
              <div className="text-sm font-semibold text-on-bg">
                One-time reset token for {resetToken.username}
              </div>
              <button
                type="button"
                onClick={() => setResetToken(null)}
                className="inline-flex items-center gap-1 rounded-md border border-outline-subtle bg-surface px-2 py-1 text-xs text-on-bg transition-colors hover:bg-surface-container-high"
                aria-label="Dismiss reset token"
              >
                <MaterialIcon name="close" size={14} />
                Dismiss
              </button>
            </div>
            <div className="mt-2 rounded-md bg-bg px-3 py-2 font-mono text-sm text-on-bg">
              {resetToken.token}
            </div>
          </div>
        )}

        {loading ? (
          <div className="rounded-lg border border-outline-subtle bg-surface p-6 text-sm text-on-bg-secondary">
            Loading admin data
          </div>
        ) : (
          <>
            {activeTab === 'overview' && (
              <section className="flex flex-col gap-4">
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-6">
                  <StatTile label="Total users" value={dashboard.stats.total_users} />
                  <StatTile label="Active" value={dashboard.stats.active_users} />
                  <StatTile label="Disabled" value={dashboard.stats.disabled_users} />
                  <StatTile label="Locked" value={dashboard.stats.locked_users} />
                  <StatTile label="Recent logins" value={dashboard.stats.recent_logins} />
                  <StatTile
                    label="Failed attempts"
                    value={dashboard.stats.recent_failed_login_attempts}
                  />
                </div>
                <AuditTable logs={dashboard.recent_audit_logs} compact />
              </section>
            )}

            {activeTab === 'users' && (
              <section className="flex flex-col gap-4">
                <form
                  className="grid gap-3 rounded-lg border border-outline-subtle bg-surface p-4 md:grid-cols-6"
                  onSubmit={handleCreateUser}
                >
                  <input
                    aria-label="New username"
                    className="rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
                    placeholder="Username"
                    value={newUser.username}
                    onChange={(event) =>
                      setNewUser((current) => ({
                        ...current,
                        username: event.target.value,
                      }))
                    }
                  />
                  <input
                    aria-label="New email"
                    className="rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
                    placeholder="Email"
                    value={newUser.email}
                    onChange={(event) =>
                      setNewUser((current) => ({
                        ...current,
                        email: event.target.value,
                      }))
                    }
                  />
                  <input
                    aria-label="New display name"
                    className="rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
                    placeholder="Display name"
                    value={newUser.display_name}
                    onChange={(event) =>
                      setNewUser((current) => ({
                        ...current,
                        display_name: event.target.value,
                      }))
                    }
                  />
                  <input
                    aria-label="Initial password"
                    className="rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
                    placeholder="Initial password"
                    type="password"
                    value={newUser.password}
                    onChange={(event) =>
                      setNewUser((current) => ({
                        ...current,
                        password: event.target.value,
                      }))
                    }
                  />
                  <label className="flex items-center gap-2 text-sm text-on-bg-secondary">
                    <input
                      type="checkbox"
                      checked={newUser.must_change_password}
                      onChange={(event) =>
                        setNewUser((current) => ({
                          ...current,
                          must_change_password: event.target.checked,
                        }))
                      }
                    />
                    Require change
                  </label>
                  <button
                    type="submit"
                    disabled={creatingUser || !newUser.username.trim() || !newUser.password}
                    className="rounded-md bg-primary px-3 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    Create user
                  </button>
                </form>

                <div className="flex flex-wrap gap-2">
                  <input
                    aria-label="Search users"
                    className="min-w-64 rounded-md border border-outline-subtle bg-surface px-3 py-2 text-sm outline-none focus:border-primary"
                    placeholder="Search users"
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                  />
                  <select
                    aria-label="Filter users by status"
                    className="rounded-md border border-outline-subtle bg-surface px-3 py-2 text-sm outline-none focus:border-primary"
                    value={statusFilter}
                    onChange={(event) => setStatusFilter(event.target.value)}
                  >
                    <option value="all">All statuses</option>
                    <option value="active">Active</option>
                    <option value="disabled">Disabled</option>
                    <option value="locked">Locked</option>
                  </select>
                </div>

                <UserTable
                  users={filteredUsers}
                  roles={roles}
                  savingUserId={savingUserId}
                  selectedRoleByUser={selectedRoleByUser}
                  onSelectedRoleChange={(userId, roleId) =>
                    setSelectedRoleByUser((current) => ({
                      ...current,
                      [userId]: roleId,
                    }))
                  }
                  onPatchUser={patchUser}
                  onChangeStatus={changeStatus}
                  onAssignRole={assignRole}
                  onRemoveRole={removeRole}
                  onResetPassword={resetPassword}
                />
              </section>
            )}

            {activeTab === 'roles' && <RolesTable roles={roles} permissions={permissions} />}
            {activeTab === 'audit' && <AuditTable logs={auditLogs} />}
          </>
        )}
      </div>
    </div>
  );
}

function UserTable({
  users,
  roles,
  savingUserId,
  selectedRoleByUser,
  onSelectedRoleChange,
  onPatchUser,
  onChangeStatus,
  onAssignRole,
  onRemoveRole,
  onResetPassword,
}: {
  users: AuthUser[];
  roles: AdminRole[];
  savingUserId: string | null;
  selectedRoleByUser: Record<string, string>;
  onSelectedRoleChange: (userId: string, roleId: string) => void;
  onPatchUser: (user: AuthUser, payload: { display_name?: string; email?: string }) => void;
  onChangeStatus: (user: AuthUser, status: string) => void;
  onAssignRole: (user: AuthUser) => void;
  onRemoveRole: (user: AuthUser, roleName: string) => void;
  onResetPassword: (user: AuthUser) => void;
}) {
  if (users.length === 0) {
    return (
      <div className="rounded-lg border border-outline-subtle bg-surface p-6 text-sm text-on-bg-secondary">
        No users match the current filters.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-outline-subtle bg-surface">
      <table className="min-w-full text-sm">
        <thead className="bg-surface-container text-left text-xs uppercase text-on-bg-secondary">
          <tr>
            <th className="px-3 py-2 font-semibold">User</th>
            <th className="px-3 py-2 font-semibold">Profile</th>
            <th className="px-3 py-2 font-semibold">Status</th>
            <th className="px-3 py-2 font-semibold">Roles</th>
            <th className="px-3 py-2 font-semibold">Actions</th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => {
            const disabled = savingUserId === user.id;
            return (
              <tr key={user.id} className="border-t border-outline-subtle">
                <td className="px-3 py-3 align-top">
                  <div className="font-semibold text-on-bg">{user.username}</div>
                  <div className="text-xs text-on-bg-secondary">{user.id}</div>
                </td>
                <td className="px-3 py-3 align-top">
                  <div className="flex min-w-64 flex-col gap-2">
                    <input
                      aria-label={`Display name for ${user.username}`}
                      defaultValue={user.display_name}
                      className="rounded-md border border-outline-subtle bg-bg px-2 py-1.5 outline-none focus:border-primary"
                      onBlur={(event) =>
                        event.target.value !== user.display_name
                          ? onPatchUser(user, {
                              display_name: event.target.value,
                            })
                          : undefined
                      }
                    />
                    <input
                      aria-label={`Email for ${user.username}`}
                      defaultValue={user.email}
                      className="rounded-md border border-outline-subtle bg-bg px-2 py-1.5 outline-none focus:border-primary"
                      onBlur={(event) =>
                        event.target.value !== user.email
                          ? onPatchUser(user, { email: event.target.value })
                          : undefined
                      }
                    />
                  </div>
                </td>
                <td className="px-3 py-3 align-top">
                  <select
                    aria-label={`Status for ${user.username}`}
                    className="rounded-md border border-outline-subtle bg-bg px-2 py-1.5 outline-none focus:border-primary"
                    value={user.status}
                    disabled={disabled}
                    onChange={(event) => onChangeStatus(user, event.target.value)}
                  >
                    <option value="active">Active</option>
                    <option value="disabled">Disabled</option>
                    <option value="locked">Locked</option>
                  </select>
                  {user.must_change_password && (
                    <div className="mt-1 text-xs text-warning">Password change pending</div>
                  )}
                </td>
                <td className="px-3 py-3 align-top">
                  <div className="flex max-w-xs flex-wrap gap-1">
                    {user.roles.length === 0 ? (
                      <span className="text-on-bg-secondary">No roles</span>
                    ) : (
                      user.roles.map((roleName) => (
                        <button
                          key={roleName}
                          type="button"
                          disabled={disabled}
                          onClick={() => onRemoveRole(user, roleName)}
                          className="inline-flex items-center gap-1 rounded-md border border-outline-subtle bg-surface-container px-2 py-1 text-xs text-on-bg hover:bg-surface-container-high disabled:opacity-60"
                          aria-label={`Remove ${roleName} from ${user.username}`}
                          title={`Remove ${roleName}`}
                        >
                          {roleName}
                          <MaterialIcon name="close" size={14} />
                        </button>
                      ))
                    )}
                  </div>
                  <div className="mt-2 flex gap-2">
                    <select
                      aria-label={`Role to assign to ${user.username}`}
                      className="max-w-40 rounded-md border border-outline-subtle bg-bg px-2 py-1.5 outline-none focus:border-primary"
                      value={selectedRoleByUser[user.id] ?? ''}
                      onChange={(event) => onSelectedRoleChange(user.id, event.target.value)}
                    >
                      <option value="">Assign role</option>
                      {roles.map((role) => (
                        <option key={role.id} value={role.id}>
                          {role.name}
                        </option>
                      ))}
                    </select>
                    <button
                      type="button"
                      disabled={disabled || !selectedRoleByUser[user.id]}
                      onClick={() => onAssignRole(user)}
                      className="rounded-md border border-outline-subtle bg-surface-container px-2 py-1.5 text-xs text-on-bg disabled:opacity-60"
                    >
                      Assign
                    </button>
                  </div>
                  <div className="mt-1 text-xs text-on-bg-secondary">{rolesLabel(user)}</div>
                </td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap gap-2">
                    <button
                      type="button"
                      disabled={disabled}
                      onClick={() =>
                        onChangeStatus(user, user.status === 'active' ? 'disabled' : 'active')
                      }
                      className="inline-flex items-center gap-1 rounded-md border border-outline-subtle bg-surface-container px-2 py-1.5 text-xs text-on-bg disabled:opacity-60"
                      aria-label={`${user.status === 'active' ? 'Disable' : 'Enable'} user ${
                        user.username
                      }`}
                    >
                      <MaterialIcon
                        name={user.status === 'active' ? 'block' : 'check_circle'}
                        size={16}
                      />
                      {user.status === 'active' ? 'Disable' : 'Enable'}
                    </button>
                    <button
                      type="button"
                      disabled={disabled}
                      onClick={() => onResetPassword(user)}
                      className="inline-flex items-center gap-1 rounded-md border border-outline-subtle bg-surface-container px-2 py-1.5 text-xs text-on-bg disabled:opacity-60"
                      aria-label={`Reset password for ${user.username}`}
                    >
                      <MaterialIcon name="key" size={16} />
                      Reset
                    </button>
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function RolesTable({ roles, permissions }: { roles: AdminRole[]; permissions: string[] }) {
  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <div className="overflow-hidden rounded-lg border border-outline-subtle bg-surface">
        {roles.length === 0 ? (
          <div className="p-6 text-sm text-on-bg-secondary">No roles returned by the server.</div>
        ) : (
          <table className="min-w-full text-sm">
            <thead className="bg-surface-container text-left text-xs uppercase text-on-bg-secondary">
              <tr>
                <th className="px-3 py-2 font-semibold">Role</th>
                <th className="px-3 py-2 font-semibold">Type</th>
                <th className="px-3 py-2 font-semibold">Permissions</th>
              </tr>
            </thead>
            <tbody>
              {roles.map((role) => (
                <tr key={role.id} className="border-t border-outline-subtle">
                  <td className="px-3 py-3 align-top">
                    <div className="font-semibold text-on-bg">{role.name}</div>
                    <div className="text-xs text-on-bg-secondary">{role.description}</div>
                  </td>
                  <td className="px-3 py-3 align-top text-on-bg-secondary">
                    {role.is_system_role ? 'System' : 'Custom'}
                  </td>
                  <td className="px-3 py-3 align-top">
                    <div className="flex flex-wrap gap-1">
                      {role.permissions.map((permission) => (
                        <span
                          key={permission}
                          className="rounded-md bg-surface-container px-2 py-1 text-xs text-on-bg"
                        >
                          {permission}
                        </span>
                      ))}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      <div className="rounded-lg border border-outline-subtle bg-surface p-4">
        <div className="mb-3 text-sm font-semibold text-on-bg">Permissions</div>
        <div className="flex flex-wrap gap-1">
          {permissions.length === 0 ? (
            <span className="text-sm text-on-bg-secondary">No permissions returned.</span>
          ) : (
            permissions.map((permission) => (
              <span
                key={permission}
                className="rounded-md bg-surface-container px-2 py-1 text-xs text-on-bg"
              >
                {permission}
              </span>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

function AuditTable({ logs, compact = false }: { logs: AdminAuditLog[]; compact?: boolean }) {
  return (
    <div className="overflow-hidden rounded-lg border border-outline-subtle bg-surface">
      {logs.length === 0 ? (
        <div className="p-6 text-sm text-on-bg-secondary">No audit logs returned.</div>
      ) : (
        <table className="min-w-full text-sm">
          <thead className="bg-surface-container text-left text-xs uppercase text-on-bg-secondary">
            <tr>
              <th className="px-3 py-2 font-semibold">Time</th>
              <th className="px-3 py-2 font-semibold">Actor</th>
              <th className="px-3 py-2 font-semibold">Action</th>
              {!compact && <th className="px-3 py-2 font-semibold">Resource</th>}
            </tr>
          </thead>
          <tbody>
            {logs.map((log) => {
              const resourceLabel =
                [log.resource, log.resource_id].filter(Boolean).join(':') ||
                (log.target_user_id ? `user:${log.target_user_id}` : 'none');
              return (
                <tr key={log.id} className="border-t border-outline-subtle">
                  <td className="px-3 py-3 font-mono text-xs text-on-bg-secondary">
                    {log.created_at}
                  </td>
                  <td className="px-3 py-3 text-on-bg">{log.actor_user_id ?? 'system'}</td>
                  <td className="px-3 py-3 font-medium text-on-bg">{log.action}</td>
                  {!compact && <td className="px-3 py-3 text-on-bg-secondary">{resourceLabel}</td>}
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}
