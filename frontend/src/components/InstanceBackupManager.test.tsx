import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerError, ValidationError } from '../api/errors';
import { InstanceBackupManager } from './InstanceBackupManager';

// Mock all API calls used by InstanceBackupManager
vi.mock('../api/client', () => ({
  fetchInstanceBackups: vi.fn().mockResolvedValue([]),
  fetchSettings: vi.fn().mockResolvedValue({}),
  updateSetting: vi.fn().mockResolvedValue(undefined),
  createInstanceBackup: vi
    .fn()
    .mockResolvedValue({ id: 'backup-1', status: 'running', created_at: new Date().toISOString() }),
  cancelInstanceBackup: vi.fn().mockResolvedValue({
    id: 'backup-1',
    status: 'cancelled',
    created_at: new Date().toISOString(),
  }),
  deleteInstanceBackup: vi.fn().mockResolvedValue(undefined),
  fetchRestoreStatus: vi.fn().mockResolvedValue(null),
  instanceBackupDownloadUrl: vi.fn().mockReturnValue('/api/v1/instance-backups/download'),
  restoreInstanceBackup: vi.fn().mockResolvedValue({ success: true }),
}));

beforeEach(() => {
  vi.clearAllMocks();
});

// Helper: render and wait for initial loading to complete
async function renderAndWait() {
  render(<InstanceBackupManager />);
  await waitFor(() => {
    expect(screen.queryByText('Loading backups...')).not.toBeInTheDocument();
  });
}

// --- Gap 9: InstanceBackupManager validation ---

describe('InstanceBackupManager — retention count validated (1-50)', () => {
  it('shows error when retention count is set to 0', async () => {
    await renderAndWait();

    const retentionInput = screen.getByDisplayValue('5');
    fireEvent.change(retentionInput, { target: { value: '0' } });

    await waitFor(() => {
      expect(screen.getByText('Retention count must be between 1 and 50')).toBeInTheDocument();
    });
  });

  it('shows error when retention count is set to 51', async () => {
    await renderAndWait();

    const retentionInput = screen.getByDisplayValue('5');
    fireEvent.change(retentionInput, { target: { value: '51' } });

    await waitFor(() => {
      expect(screen.getByText('Retention count must be between 1 and 50')).toBeInTheDocument();
    });
  });

  it('shows error for non-numeric retention count', async () => {
    await renderAndWait();

    const retentionInput = screen.getByDisplayValue('5');
    fireEvent.change(retentionInput, { target: { value: 'abc' } });

    await waitFor(() => {
      expect(screen.getByText('Retention count must be a number')).toBeInTheDocument();
    });
  });

  it('does not call updateSetting when retention count is invalid', async () => {
    const { updateSetting } = await import('../api/client');
    await renderAndWait();

    // Flush any prior calls from initial settings load
    vi.clearAllMocks();

    const retentionInput = screen.getByDisplayValue('5');
    fireEvent.change(retentionInput, { target: { value: '0' } });

    // handleRetentionChange returns early when invalid — no timer is set
    // Give a short synchronous tick to confirm no call was scheduled
    await new Promise((r) => setTimeout(r, 0));

    const calls = (updateSetting as ReturnType<typeof vi.fn>).mock.calls;
    const retentionCalls = calls.filter(
      ([key]: string[]) => key === 'instance_backup_retention_count',
    );
    expect(retentionCalls).toHaveLength(0);
  });
});

describe('InstanceBackupManager — schedule interval validated against allowlist', () => {
  it('rejects invalid interval and does not save (gates updateSetting)', async () => {
    const { updateSetting } = await import('../api/client');
    await renderAndWait();

    vi.clearAllMocks();

    // The select only has valid options in real UI; we simulate an invalid value programmatically
    const scheduleSelect = screen.getByDisplayValue('Disabled');
    fireEvent.change(scheduleSelect, { target: { value: '7' } });

    // handleScheduleChange returns early for invalid values — no timer is set, no updateSetting call
    await new Promise((r) => setTimeout(r, 0));
    const intervalCalls = (updateSetting as ReturnType<typeof vi.fn>).mock.calls.filter(
      ([key]: string[]) => key === 'instance_backup_interval_hours',
    );
    expect(intervalCalls).toHaveLength(0);
  });

  it('does not call updateSetting for interval when value is invalid', async () => {
    const { updateSetting } = await import('../api/client');
    await renderAndWait();

    vi.clearAllMocks();

    const scheduleSelect = screen.getByDisplayValue('Disabled');
    fireEvent.change(scheduleSelect, { target: { value: '7' } });

    // handleScheduleChange returns early when invalid — no timer is set
    await new Promise((r) => setTimeout(r, 0));

    const calls = (updateSetting as ReturnType<typeof vi.fn>).mock.calls;
    const intervalCalls = calls.filter(
      ([key]: string[]) => key === 'instance_backup_interval_hours',
    );
    expect(intervalCalls).toHaveLength(0);
  });
});

describe('InstanceBackupManager — typed errors in handleCreate', () => {
  it('shows ServerError ref when createInstanceBackup throws ServerError', async () => {
    const { createInstanceBackup } = await import('../api/client');
    (createInstanceBackup as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ServerError('internal error, ref: err001', 'err001'),
    );

    await renderAndWait();

    fireEvent.click(screen.getByText('Create Backup'));

    await waitFor(() => {
      expect(screen.getByText('Something went wrong (ref: err001)')).toBeInTheDocument();
    });
  });

  it('shows ValidationError message when createInstanceBackup throws ValidationError', async () => {
    const { createInstanceBackup } = await import('../api/client');
    (createInstanceBackup as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ValidationError('backup limit reached'),
    );

    await renderAndWait();

    fireEvent.click(screen.getByText('Create Backup'));

    await waitFor(() => {
      expect(screen.getByText('backup limit reached')).toBeInTheDocument();
    });
  });

  it('shows plain error message when createInstanceBackup throws plain Error', async () => {
    const { createInstanceBackup } = await import('../api/client');
    (createInstanceBackup as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error('network timeout'),
    );

    await renderAndWait();

    fireEvent.click(screen.getByText('Create Backup'));

    await waitFor(() => {
      expect(screen.getByText('network timeout')).toBeInTheDocument();
    });
  });
});

// --- SC-5: Restore UI ---

// A valid RestoreReport fixture that matches the backend dry-run response shape.
const mockRestoreReport = {
  valid: true,
  app_version: '1.4.0',
  git_commit: 'abc123',
  migration_version: 10,
  created_at: '2026-04-05T20:00:00Z',
  db_size_bytes: 1234567,
  backup_file_count: 3,
  total_size_bytes: 2345678,
  needs_migration: false,
  current_migration_version: 10,
  message: 'Validation passed. Archive is ready to restore.',
};

// Helper: simulate file selection on the hidden file input.
// The component's file input has accept=".tar.gz" and type="file".
function selectRestoreFile(file: File) {
  const input = document.querySelector('input[type="file"]') as HTMLInputElement;
  if (!input) throw new Error('file input not found in DOM');
  // Use Object.defineProperty to set files on the read-only HTMLInputElement.files
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  fireEvent.change(input);
}

describe('InstanceBackupManager — SC-5: restore UI', () => {
  it.each([
    ['staged_restart_pending', 'Restore staged. Restart pending.'],
    ['startup_restore_detected', 'Restore applying on startup.'],
    ['applying_postgres', 'Restore applying on startup.'],
    ['completed', 'Restore completed.'],
    ['failed_retryable', 'Restore failed but can retry on restart.'],
  ])('renders restore status message for %s', async (phase, message) => {
    const { fetchRestoreStatus } = await import('../api/client');
    (fetchRestoreStatus as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      operation_id: 'restore-1',
      phase,
      attempt_count: 1,
      last_error: phase === 'failed_retryable' ? 'pg_restore failed' : '',
      missing_key_id: '',
      created_at: '2026-06-05T00:00:00Z',
      updated_at: '2026-06-05T00:01:00Z',
    });

    await renderAndWait();

    expect(screen.getByText(message)).toBeInTheDocument();
  });

  it('renders missing key restore status with legacy recovery guidance', async () => {
    const { fetchRestoreStatus } = await import('../api/client');
    (fetchRestoreStatus as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      operation_id: 'restore-1',
      phase: 'failed_operator_action_required',
      attempt_count: 1,
      last_error: '',
      missing_key_id: 'legacy',
      created_at: '2026-06-05T00:00:00Z',
      updated_at: '2026-06-05T00:01:00Z',
    });

    await renderAndWait();

    expect(
      screen.getByText('Restore blocked because key id legacy is missing from THEIA_ENCRYPTION_KEYS.'),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'Add legacy=<old secret> to THEIA_ENCRYPTION_KEYS or set THEIA_ENCRYPTION_KEY as fallback, restart, then create and restore-test a fresh backup.',
      ),
    ).toBeInTheDocument();
  });

  it('renders the Restore Backup button', async () => {
    await renderAndWait();
    expect(screen.getByRole('button', { name: 'Restore Backup' })).toBeInTheDocument();
  });

  it('file input has accept=".tar.gz"', async () => {
    await renderAndWait();
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    expect(input).not.toBeNull();
    expect(input.accept).toBe('.tar.gz');
  });

  it('dry-run validation is called when a file is selected', async () => {
    const { restoreInstanceBackup } = await import('../api/client');
    (restoreInstanceBackup as ReturnType<typeof vi.fn>).mockResolvedValueOnce(mockRestoreReport);

    await renderAndWait();

    const file = new File(['dummy'], 'backup.tar.gz', { type: 'application/gzip' });
    selectRestoreFile(file);

    await waitFor(() => {
      const calls = (restoreInstanceBackup as ReturnType<typeof vi.fn>).mock.calls;
      expect(calls.length).toBeGreaterThan(0);
      // First call must be dry-run (second argument = true)
      expect(calls[0][1]).toBe(true);
    });
  });

  it('confirmation modal appears with manifest details after file selection', async () => {
    const { restoreInstanceBackup } = await import('../api/client');
    (restoreInstanceBackup as ReturnType<typeof vi.fn>).mockResolvedValueOnce(mockRestoreReport);

    await renderAndWait();

    const file = new File(['dummy'], 'backup.tar.gz', { type: 'application/gzip' });
    selectRestoreFile(file);

    await waitFor(() => {
      expect(screen.getByText('Confirm Restore')).toBeInTheDocument();
    });

    // Modal should display manifest details
    expect(screen.getByText('v1.4.0')).toBeInTheDocument();
    expect(screen.getByText('10')).toBeInTheDocument(); // migration_version
  });

  it('Restore Now button is disabled until checkbox is checked', async () => {
    const { restoreInstanceBackup } = await import('../api/client');
    (restoreInstanceBackup as ReturnType<typeof vi.fn>).mockResolvedValueOnce(mockRestoreReport);

    await renderAndWait();

    const file = new File(['dummy'], 'backup.tar.gz', { type: 'application/gzip' });
    selectRestoreFile(file);

    await waitFor(() => {
      expect(screen.getByText('Confirm Restore')).toBeInTheDocument();
    });

    const restoreButton = screen.getByRole('button', { name: 'Restore Now' });
    expect(restoreButton).toBeDisabled();

    // Check the acknowledgement checkbox
    const checkbox = screen.getByRole('checkbox');
    fireEvent.click(checkbox);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Restore Now' })).not.toBeDisabled();
    });
  });

  it('confirmed restore failure stays visible instead of showing restart success', async () => {
    const { restoreInstanceBackup } = await import('../api/client');
    (restoreInstanceBackup as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(mockRestoreReport)
      .mockRejectedValueOnce(new ValidationError('restore archive invalid'));

    await renderAndWait();

    const file = new File(['dummy'], 'backup.tar.gz', { type: 'application/gzip' });
    selectRestoreFile(file);

    await waitFor(() => {
      expect(screen.getByText('Confirm Restore')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('checkbox'));
    fireEvent.click(screen.getByRole('button', { name: 'Restore Now' }));

    await waitFor(() => {
      expect(screen.getByText('restore archive invalid')).toBeInTheDocument();
    });
    expect(screen.getByText('Confirm Restore')).toBeInTheDocument();
    expect(screen.queryByText(/Restore initiated/)).not.toBeInTheDocument();
  });

  it('Cancel closes the confirmation modal', async () => {
    const { restoreInstanceBackup } = await import('../api/client');
    (restoreInstanceBackup as ReturnType<typeof vi.fn>).mockResolvedValueOnce(mockRestoreReport);

    await renderAndWait();

    const file = new File(['dummy'], 'backup.tar.gz', { type: 'application/gzip' });
    selectRestoreFile(file);

    await waitFor(() => {
      expect(screen.getByText('Confirm Restore')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    await waitFor(() => {
      expect(screen.queryByText('Confirm Restore')).not.toBeInTheDocument();
    });
  });

  it('validation error is shown below header when dry-run fails', async () => {
    const { restoreInstanceBackup } = await import('../api/client');
    (restoreInstanceBackup as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error('encryption key mismatch'),
    );

    await renderAndWait();

    const file = new File(['dummy'], 'backup.tar.gz', { type: 'application/gzip' });
    selectRestoreFile(file);

    await waitFor(() => {
      expect(screen.getByText('encryption key mismatch')).toBeInTheDocument();
    });

    // Modal should NOT appear
    expect(screen.queryByText('Confirm Restore')).not.toBeInTheDocument();
  });
});

// --- G2: computeNextBackupText helper text ---

describe('InstanceBackupManager — computeNextBackupText helper text', () => {
  it('shows "Scheduling disabled" when interval is 0 (default)', async () => {
    // fetchSettings returns no instance_backup_interval_hours so scheduleInterval stays '0'
    await renderAndWait();
    expect(screen.getByText('Scheduling disabled')).toBeInTheDocument();
  });

  it('shows "First backup in ~Xh" when interval is set but no successful backups exist', async () => {
    const { fetchSettings, fetchInstanceBackups } = await import('../api/client');
    (fetchSettings as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      instance_backup_interval_hours: '24',
    });
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);

    render(<InstanceBackupManager />);
    await waitFor(() => {
      expect(screen.queryByText('Loading backups...')).not.toBeInTheDocument();
    });

    expect(screen.getByText('First backup in ~24h')).toBeInTheDocument();
  });

  it('shows "Next backup: in ~Xh" when a successful backup exists and next run is in the future', async () => {
    const { fetchSettings, fetchInstanceBackups } = await import('../api/client');
    const intervalHours = 24;
    // Last backup was 1 hour ago — next is in ~23 hours
    const lastBackupTime = new Date(Date.now() - 1 * 3600 * 1000).toISOString();

    (fetchSettings as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      instance_backup_interval_hours: String(intervalHours),
    });
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'b1',
        file_name: 'theia-backup-20260407-120000-v1.4.0.tar.gz',
        size_bytes: 1024,
        sha256: 'abc',
        app_version: '1.4.0',
        migration_version: 11,
        status: 'success',
        error_message: '',
        trigger: 'manual',
        created_at: lastBackupTime,
      },
    ]);

    render(<InstanceBackupManager />);
    await waitFor(() => {
      expect(screen.queryByText('Loading backups...')).not.toBeInTheDocument();
    });

    // Should show "Next backup: in ~23h" (approximately — just check prefix)
    const helperText = screen.getByText(/Next backup: in ~/);
    expect(helperText).toBeInTheDocument();
  });

  it('shows "Next backup: soon" when last backup was longer ago than the interval', async () => {
    const { fetchSettings, fetchInstanceBackups } = await import('../api/client');
    const intervalHours = 6;
    // Last backup was 10 hours ago (beyond the 6h interval) — next is overdue
    const lastBackupTime = new Date(Date.now() - 10 * 3600 * 1000).toISOString();

    (fetchSettings as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      instance_backup_interval_hours: String(intervalHours),
    });
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'b1',
        file_name: 'theia-backup-20260407-000000-v1.4.0.tar.gz',
        size_bytes: 1024,
        sha256: 'abc',
        app_version: '1.4.0',
        migration_version: 11,
        status: 'success',
        error_message: '',
        trigger: 'manual',
        created_at: lastBackupTime,
      },
    ]);

    render(<InstanceBackupManager />);
    await waitFor(() => {
      expect(screen.queryByText('Loading backups...')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Next backup: soon')).toBeInTheDocument();
  });
});

// --- G3: Trigger badge on backup rows ---

describe('InstanceBackupManager — trigger badge on backup rows', () => {
  it('shows "scheduled" badge when backup trigger is "scheduled"', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'b-sched',
        file_name: 'theia-backup-20260407-120000-v1.4.0.tar.gz',
        size_bytes: 2048,
        sha256: 'def',
        app_version: '1.4.0',
        migration_version: 11,
        status: 'success',
        error_message: '',
        trigger: 'scheduled',
        created_at: new Date().toISOString(),
      },
    ]);

    await renderAndWait();

    expect(screen.getByText('scheduled')).toBeInTheDocument();
  });

  it('does not show "scheduled" badge when backup trigger is "manual"', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'b-manual',
        file_name: 'theia-backup-20260407-120000-v1.4.0.tar.gz',
        size_bytes: 2048,
        sha256: 'def',
        app_version: '1.4.0',
        migration_version: 11,
        status: 'success',
        error_message: '',
        trigger: 'manual',
        created_at: new Date().toISOString(),
      },
    ]);

    await renderAndWait();

    expect(screen.queryByText('scheduled')).not.toBeInTheDocument();
  });

  it('shows badge only for scheduled backup when both manual and scheduled are listed', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      {
        id: 'b-manual',
        file_name: 'theia-backup-manual.tar.gz',
        size_bytes: 1024,
        sha256: 'aaa',
        app_version: '1.4.0',
        migration_version: 11,
        status: 'success',
        error_message: '',
        trigger: 'manual',
        created_at: new Date(Date.now() - 3600 * 1000).toISOString(),
      },
      {
        id: 'b-sched',
        file_name: 'theia-backup-scheduled.tar.gz',
        size_bytes: 2048,
        sha256: 'bbb',
        app_version: '1.4.0',
        migration_version: 11,
        status: 'success',
        error_message: '',
        trigger: 'scheduled',
        created_at: new Date().toISOString(),
      },
    ]);

    await renderAndWait();

    // Exactly one "scheduled" badge should appear
    const badges = screen.getAllByText('scheduled');
    expect(badges).toHaveLength(1);
  });
});

// --- Phase 16 gap tests (SC-5 base UI behaviors) ---

// Helper: build a complete InstanceBackup fixture
function mockBackup(overrides: Record<string, unknown> = {}) {
  return {
    id: 'backup-abc',
    file_name: 'theia-backup-20260407-120000-v1.4.0.tar.gz',
    size_bytes: 1024 * 1024 * 12,
    sha256: 'abc123',
    app_version: '1.4.0',
    migration_version: 11,
    status: 'success',
    error_message: '',
    trigger: 'manual',
    created_at: '2026-04-07T12:00:00Z',
    ...overrides,
  };
}

// Gap 8: Table renders backup rows with status, filename, size, version, date
describe('InstanceBackupManager — SC-5: table renders backup row fields', () => {
  it('renders the backup filename in the row', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([mockBackup()]);

    await renderAndWait();

    expect(screen.getByText('theia-backup-20260407-120000-v1.4.0.tar.gz')).toBeInTheDocument();
  });

  it('renders the app version prefixed with v', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([mockBackup()]);

    await renderAndWait();

    expect(screen.getByText('v1.4.0')).toBeInTheDocument();
  });

  it('renders the status icon for a successful backup', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ status: 'success' }),
    ]);

    await renderAndWait();

    // Checkmark character for success
    expect(screen.getByText('\u2713')).toBeInTheDocument();
  });

  it('renders "Backup in progress..." filename when backup is running', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ status: 'running', file_name: '' }),
    ]);

    await renderAndWait();

    expect(screen.getByText('Backup in progress...')).toBeInTheDocument();
  });

  it('renders running backup progress details', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({
        status: 'running',
        file_name: '',
        progress: {
          phase: 'archiving',
          message: 'Writing backup archive',
          current: 50,
          total: 100,
        },
      }),
    ]);

    await renderAndWait();

    expect(screen.getByText('Writing backup archive (50%)')).toBeInTheDocument();
  });

  it('renders size in MB for large backups', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ size_bytes: 1024 * 1024 * 12, status: 'success' }),
    ]);

    await renderAndWait();

    expect(screen.getByText('12.0 MB')).toBeInTheDocument();
  });
});

// Gap 9: Create button triggers backup + shows spinner (creating state)
describe('InstanceBackupManager — SC-5: create button shows Creating... while backup runs', () => {
  it('shows "Creating..." spinner text after clicking Create Backup', async () => {
    const { createInstanceBackup, fetchInstanceBackups } = await import('../api/client');
    // Initial load returns empty list so the Create Backup button is enabled
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);
    // createInstanceBackup resolves with a running backup — triggers startPolling
    (createInstanceBackup as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBackup({ id: 'new-backup', status: 'running', file_name: '' }),
    );
    // Subsequent fetchInstanceBackups calls (from polling) return running
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValue([
      mockBackup({ id: 'new-backup', status: 'running', file_name: '' }),
    ]);

    await renderAndWait();

    // Button should show "Create Backup" because initial list is empty
    fireEvent.click(screen.getByRole('button', { name: 'Create Backup' }));

    await waitFor(() => {
      expect(screen.getByText('Creating...')).toBeInTheDocument();
    });
  });
});

// Gap 10: Download link renders for successful backups
describe('InstanceBackupManager — SC-5: download link for successful backups', () => {
  it('renders a Download anchor link for a successful backup', async () => {
    const { fetchInstanceBackups, instanceBackupDownloadUrl } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ id: 'dl-backup', status: 'success' }),
    ]);
    (instanceBackupDownloadUrl as ReturnType<typeof vi.fn>).mockReturnValue(
      '/api/v1/instance-backups/dl-backup/download',
    );

    await renderAndWait();

    const link = screen.getByRole('link', { name: 'Download' });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute('href', '/api/v1/instance-backups/dl-backup/download');
  });

  it('does NOT render a Download link for a failed backup', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ id: 'fail-backup', status: 'failed', error_message: 'disk full' }),
    ]);

    await renderAndWait();

    expect(screen.queryByRole('link', { name: 'Download' })).not.toBeInTheDocument();
  });

  it('does NOT render a Download link for a running backup', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ id: 'run-backup', status: 'running', file_name: '' }),
    ]);

    await renderAndWait();

    expect(screen.queryByRole('link', { name: 'Download' })).not.toBeInTheDocument();
  });
});

// Gap 11: Inline delete confirm — first click shows "Confirm?", reverts after 3s
describe('InstanceBackupManager — SC-5: inline delete confirm behavior', () => {
  it('first click on Delete changes button text to "Confirm?"', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ id: 'del-backup', status: 'success' }),
    ]);

    await renderAndWait();

    const deleteBtn = screen.getByRole('button', { name: 'Delete' });
    fireEvent.click(deleteBtn);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Confirm?' })).toBeInTheDocument();
    });
  });

  it('second click on Confirm? executes delete and removes the row', async () => {
    const { fetchInstanceBackups, deleteInstanceBackup } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ id: 'del-backup-2', status: 'success' }),
    ]);
    (deleteInstanceBackup as ReturnType<typeof vi.fn>).mockResolvedValueOnce(undefined);

    await renderAndWait();

    // First click
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Confirm?' })).toBeInTheDocument();
    });

    // Second click
    fireEvent.click(screen.getByRole('button', { name: 'Confirm?' }));

    await waitFor(() => {
      expect(
        screen.queryByText('theia-backup-20260407-120000-v1.4.0.tar.gz'),
      ).not.toBeInTheDocument();
    });
  });
});

// Gap 12: Create button disabled when backup is in progress
describe('InstanceBackupManager — SC-5: Create Backup button disabled when backup in progress', () => {
  it('Create Backup button is disabled when a running backup exists in the list', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ id: 'running-backup', status: 'running', file_name: '' }),
    ]);

    await renderAndWait();

    // When a backup is running, the component sets creating=true so the button
    // renders as "Creating..." and is disabled (hasRunning = true).
    const createBtn = screen.getByRole('button', { name: /creating\.\.\./i });
    expect(createBtn).toBeDisabled();
  });
});

describe('InstanceBackupManager — running backup cancellation', () => {
  it('calls cancelInstanceBackup when Cancel is clicked for a running backup', async () => {
    const { cancelInstanceBackup, fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ id: 'running-backup', status: 'running', file_name: '' }),
    ]);
    (cancelInstanceBackup as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      mockBackup({
        id: 'running-backup',
        status: 'running',
        file_name: '',
        progress: { phase: 'cancelling', message: 'Cancellation requested', current: 0, total: 0 },
      }),
    );

    await renderAndWait();

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    await waitFor(() => {
      expect(cancelInstanceBackup).toHaveBeenCalledWith('running-backup');
    });
    expect(screen.getByRole('button', { name: /creating\.\.\./i })).toBeDisabled();
  });
});

// Gap 13: Failed backup shows expandable error message
describe('InstanceBackupManager — SC-5: failed backup shows error message', () => {
  it('renders the error message for a failed backup', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({
        id: 'failed-backup',
        status: 'failed',
        error_message: 'archive creation failed: disk quota exceeded',
        file_name: 'theia-backup-fail.tar.gz',
      }),
    ]);

    await renderAndWait();

    expect(screen.getByText('archive creation failed: disk quota exceeded')).toBeInTheDocument();
  });

  it('does NOT render error text for a successful backup', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ id: 'ok-backup', status: 'success', error_message: '' }),
    ]);

    await renderAndWait();

    expect(screen.queryByText('archive creation failed')).not.toBeInTheDocument();
  });

  it('shows X status icon for a failed backup', async () => {
    const { fetchInstanceBackups } = await import('../api/client');
    (fetchInstanceBackups as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      mockBackup({ id: 'failed-backup-2', status: 'failed', error_message: 'oops' }),
    ]);

    await renderAndWait();

    // X mark character for failed
    expect(screen.getByText('\u2717')).toBeInTheDocument();
  });
});
