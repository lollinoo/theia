/**
 * Exercises instance backup API boundary behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setDocumentCookie } from '../test/documentCookie';
import { ServerError, ValidationError } from './errors';
import {
  cancelInstanceBackup,
  fetchInstanceBackups,
  fetchRestoreStatus,
  instanceBackupDownloadUrl,
  restoreInstanceBackup,
} from './instanceBackup';

function mockResponse(
  body: unknown,
  init: { ok?: boolean; status?: number; statusText?: string } = {},
) {
  const { ok = true, status = 200, statusText = 'OK' } = init;
  return {
    ok,
    status,
    statusText,
    json: () => Promise.resolve(body),
    headers: new Headers(),
  } as unknown as Response;
}

beforeEach(() => {
  vi.restoreAllMocks();
  setDocumentCookie('theia_csrf=instance-csrf');
});

describe('instance backup client', () => {
  it('fetches instance backups and preserves progress defaults', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({
          data: [
            {
              id: 'backup-1',
              file_name: 'theia-backup.zip',
              size_bytes: 1024,
              sha256: 'abc',
              migration_version: 42,
              status: 'success',
              progress: { phase: 'archive', message: 'done' },
              trigger: 'scheduled',
              created_at: '2026-01-01T00:00:00Z',
            },
          ],
        }),
      ),
    );

    await expect(fetchInstanceBackups()).resolves.toEqual([
      {
        id: 'backup-1',
        file_name: 'theia-backup.zip',
        size_bytes: 1024,
        sha256: 'abc',
        migration_version: 42,
        status: 'success',
        error_message: '',
        progress: { phase: 'archive', message: 'done', current: 0, total: 0 },
        trigger: 'scheduled',
        created_at: '2026-01-01T00:00:00Z',
      },
    ]);
  });

  it('cancels an instance backup through the cancel endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: {
          id: 'backup-1',
          status: 'cancelled',
          trigger: 'manual',
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(cancelInstanceBackup('backup-1')).resolves.toMatchObject({
      id: 'backup-1',
      status: 'cancelled',
      trigger: 'manual',
    });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/instance-backups/backup-1/cancel',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'instance-csrf' }),
      }),
    );
  });

  it('builds encoded instance backup download URLs', () => {
    expect(instanceBackupDownloadUrl('backup/1')).toBe(
      '/api/v1/instance-backups/backup%2F1/download',
    );
  });

  it('fetches restore status and preserves missing-key details', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: {
          operation_id: 'restore-1',
          phase: 'failed_operator_action_required',
          attempt_count: 2,
          last_error: 'missing kid-old',
          missing_key_id: 'kid-old',
          created_at: '2026-06-05T00:00:00Z',
          updated_at: '2026-06-05T00:01:00Z',
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchRestoreStatus()).resolves.toEqual({
      operation_id: 'restore-1',
      phase: 'failed_operator_action_required',
      attempt_count: 2,
      last_error: 'missing kid-old',
      missing_key_id: 'kid-old',
      created_at: '2026-06-05T00:00:00Z',
      updated_at: '2026-06-05T00:01:00Z',
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/instance-backups/restore-status', {
      headers: expect.objectContaining({ Accept: 'application/json' }),
    });
  });

  it('restores backups with multipart form data and dry-run query', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockResponse({
        data: {
          valid: true,
          migration_version: 42,
          created_at: '2026-01-01T00:00:00Z',
          db_size_bytes: 2048,
          backup_file_count: 3,
          total_size_bytes: 4096,
          needs_migration: false,
          current_migration_version: 42,
          message: 'valid',
        },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const report = await restoreInstanceBackup(new File(['zip'], 'backup.zip'), true);

    expect(report.valid).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/instance-backups/restore?dry_run=true',
      expect.objectContaining({
        method: 'POST',
        body: expect.any(FormData),
        headers: expect.objectContaining({ 'X-CSRF-Token': 'instance-csrf' }),
      }),
    );
    expect(fetchMock.mock.calls[0][1]?.headers).not.toHaveProperty('Content-Type');
  });

  it('maps restore validation and server errors', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        mockResponse({ error: 'invalid backup archive' }, { ok: false, status: 400 }),
      )
      .mockResolvedValueOnce(
        mockResponse({ error: 'restore failed ref: restore-123' }, { ok: false, status: 500 }),
      );
    vi.stubGlobal('fetch', fetchMock);

    await expect(restoreInstanceBackup(new File(['zip'], 'backup.zip'), true)).rejects.toThrow(
      ValidationError,
    );
    await expect(restoreInstanceBackup(new File(['zip'], 'backup.zip'), true)).rejects.toThrow(
      ServerError,
    );
  });

  it.each([502, 503, 504])(
    'maps confirmed restore gateway status %d to an unknown outcome',
    async (status) => {
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValue(
          mockResponse(null, {
            ok: false,
            status,
            statusText: 'Gateway unavailable',
          }),
        ),
      );

      await expect(
        restoreInstanceBackup(new File(['archive'], 'backup.tar.gz'), false),
      ).rejects.toMatchObject({ name: 'RestoreOutcomeUnknownError' });
    },
  );

  it('maps a confirmed restore transport interruption to an unknown outcome', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));

    await expect(
      restoreInstanceBackup(new File(['archive'], 'backup.tar.gz'), false),
    ).rejects.toMatchObject({ name: 'RestoreOutcomeUnknownError' });
  });

  it('keeps a dry-run gateway response as a normal request failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse(null, { ok: false, status: 502, statusText: 'Bad Gateway' }),
        ),
    );

    try {
      await restoreInstanceBackup(new File(['archive'], 'backup.tar.gz'), true);
      throw new Error('expected dry-run restore to fail');
    } catch (error) {
      expect(error).toBeInstanceOf(Error);
      expect((error as Error).name).not.toBe('RestoreOutcomeUnknownError');
      expect((error as Error).message).toContain('502');
    }
  });
});
