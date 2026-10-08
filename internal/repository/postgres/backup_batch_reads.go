package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

const backupReadBatchSize = 256

func backupBatchArgs(ids []uuid.UUID) (string, []interface{}) {
	args := make([]interface{}, len(ids))
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		args[i] = id.String()
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ","), args
}

// GetByJobIDsContext loads files for bounded batches of backup jobs.
func (r *BackupFileRepo) GetByJobIDsContext(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domain.BackupFile, error) {
	ids = uniqueBackupReadIDs(ids)
	files := make(map[uuid.UUID][]domain.BackupFile, len(ids))
	for start := 0; start < len(ids); start += backupReadBatchSize {
		placeholders, args := backupBatchArgs(ids[start:min(start+backupReadBatchSize, len(ids))])
		rows, err := r.db.QueryContext(ctx, `SELECT id,job_id,file_type,file_name,file_path,file_hash,size_bytes,created_at
			FROM backup_files WHERE job_id IN (`+placeholders+`) ORDER BY job_id,file_type`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var file domain.BackupFile
			if err := rows.Scan(&file.ID, &file.JobID, &file.FileType, &file.FileName, &file.FilePath, &file.FileHash, &file.SizeBytes, &file.CreatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			files[file.JobID] = append(files[file.JobID], file)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

// GetTotalsByJobIDsContext counts files and positive stored sizes in SQL.
func (r *BackupFileRepo) GetTotalsByJobIDsContext(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.BackupFileTotals, error) {
	ids = uniqueBackupReadIDs(ids)
	totals := make(map[uuid.UUID]domain.BackupFileTotals, len(ids))
	for start := 0; start < len(ids); start += backupReadBatchSize {
		placeholders, args := backupBatchArgs(ids[start:min(start+backupReadBatchSize, len(ids))])
		rows, err := r.db.QueryContext(ctx, `SELECT job_id,COUNT(*),SUM(GREATEST(size_bytes,0)::bigint)
			FROM backup_files WHERE job_id IN (`+placeholders+`) GROUP BY job_id`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id uuid.UUID
			var total domain.BackupFileTotals
			if err := rows.Scan(&id, &total.FileCount, &total.ByteCount); err != nil {
				rows.Close()
				return nil, err
			}
			totals[id] = total
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return totals, nil
}

// GetLatestByDeviceIDsContext finds each selected device's latest successful backup.
func (r *BackupJobRepo) GetLatestByDeviceIDsContext(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.BackupJob, error) {
	ids = uniqueBackupReadIDs(ids)
	jobs := make(map[uuid.UUID]domain.BackupJob, len(ids))
	for start := 0; start < len(ids); start += backupReadBatchSize {
		placeholders, args := backupBatchArgs(ids[start:min(start+backupReadBatchSize, len(ids))])
		rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT ON (device_id) id,device_id,status,error_message,created_at
			FROM backup_jobs WHERE device_id IN (`+placeholders+`) AND status='success'
			ORDER BY device_id,created_at DESC`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			job, err := scanJobRows(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			jobs[job.DeviceID] = *job
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

// GetBackupDownloadDevicesContext loads only the identity and naming fields used by downloads.
func (r *DeviceRepo) GetBackupDownloadDevicesContext(ctx context.Context, ids []uuid.UUID) ([]domain.Device, error) {
	ids = uniqueBackupReadIDs(ids)
	devices := make([]domain.Device, 0, len(ids))
	for start := 0; start < len(ids); start += backupReadBatchSize {
		placeholders, args := backupBatchArgs(ids[start:min(start+backupReadBatchSize, len(ids))])
		rows, err := r.db.QueryContext(ctx, `SELECT id,hostname,ip,sys_name,tags_json FROM devices WHERE id IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var device domain.Device
			var tags string
			if err := rows.Scan(&device.ID, &device.Hostname, &device.IP, &device.SysName, &tags); err != nil {
				rows.Close()
				return nil, err
			}
			if err := json.Unmarshal([]byte(tags), &device.Tags); err != nil {
				rows.Close()
				return nil, fmt.Errorf("decoding backup device tags: %w", err)
			}
			devices = append(devices, device)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return devices, nil
}

// ListRetentionCandidatesContext returns a bounded page of obsolete successful
// jobs. The UUID cursor lets subsequent sweeps resume without scanning each device.
func (r *BackupJobRepo) ListRetentionCandidatesContext(ctx context.Context, keep int, after uuid.UUID, limit int) ([]domain.BackupJob, error) {
	if keep < 0 || limit <= 0 || limit > 100 {
		return nil, fmt.Errorf("invalid retention batch bounds")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,device_id,status,error_message,created_at FROM (
		SELECT id,device_id,status,error_message,created_at,
		ROW_NUMBER() OVER(PARTITION BY device_id ORDER BY created_at DESC,id DESC) AS rank
		FROM backup_jobs WHERE status='success'
	) ranked WHERE rank>? AND id>? ORDER BY id LIMIT ?`, keep, after.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]domain.BackupJob, 0, limit)
	for rows.Next() {
		job, err := scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}
	return jobs, rows.Err()
}

func uniqueBackupReadIDs(ids []uuid.UUID) []uuid.UUID {
	unique := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	return unique
}
