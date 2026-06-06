/**
 * Exercises backup API boundary behavior so refactors preserve the documented contract.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  fetchBackupFileContent,
  fetchBackupJobs,
  fetchBulkOperationStatus,
  startBulkBackupRun,
} from './backup';

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
  document.cookie = 'theia_csrf=backup-csrf';
});

describe('backup client', () => {
  it('fetches backup jobs and preserves empty-file defaults', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({
          data: [
            {
              id: 'job-1',
              device_id: 'dev-1',
              status: 'success',
              created_at: '2026-01-01T00:00:00Z',
              files: [{ id: 'file-1', file_name: 'router.rsc' }],
            },
          ],
        }),
      ),
    );

    const jobs = await fetchBackupJobs('dev-1');

    expect(jobs).toHaveLength(1);
    expect(jobs[0].files[0]).toEqual({
      id: 'file-1',
      job_id: '',
      file_type: '',
      file_name: 'router.rsc',
      file_hash: '',
      size_bytes: 0,
      created_at: '',
    });
  });

  it('parses bulk operation status metadata defaults', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse({
          data: {
            bulk_backup_run: { can_pause: true, can_resume: true, can_cancel: true },
            bulk_download: { max_files: 250, distributed: true },
          },
        }),
      ),
    );

    await expect(fetchBulkOperationStatus()).resolves.toMatchObject({
      bulk_backup_run: { can_pause: true, can_resume: true, can_cancel: true },
      bulk_download: { max_files: 250, distributed: true },
    });
  });

  it('treats 409 bulk-run responses as the current run payload', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        mockResponse(
          {
            data: {
              id: 'run-1',
              status: 'running',
              batch_size: 25,
              total_count: 2,
              created_at: '2026-01-01T00:00:00Z',
              items: [],
            },
          },
          { ok: false, status: 409, statusText: 'Conflict' },
        ),
      ),
    );

    await expect(startBulkBackupRun(['dev-1', 'dev-2'])).resolves.toMatchObject({
      id: 'run-1',
      status: 'running',
      batch_size: 25,
      total_count: 2,
    });
  });

  it('defaults backup file content download URLs to the file download endpoint', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockResponse({ data: { content: 'export compact', inline: true, size_bytes: 14 } }),
        ),
    );

    await expect(fetchBackupFileContent('file-1')).resolves.toEqual({
      content: 'export compact',
      inline: true,
      download_url: '/api/v1/backup-files/file-1/download',
      size_bytes: 14,
      max_inline_size_bytes: 0,
    });
  });
});
