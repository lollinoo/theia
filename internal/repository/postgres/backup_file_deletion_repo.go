package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

var _ domain.BackupFileDeletionRepository = (*BackupFileRepo)(nil)

// ListPendingFileDeletions returns a bounded batch of paths captured by deletion triggers.
func (r *BackupFileRepo) ListPendingFileDeletions(ctx context.Context, limit int) ([]domain.BackupFile, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("backup file deletion batch limit must be between 1 and 100")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,file_name,file_path FROM backup_file_deletions
		WHERE next_attempt_at <= CURRENT_TIMESTAMP ORDER BY next_attempt_at,id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []domain.BackupFile
	for rows.Next() {
		var file domain.BackupFile
		var id string
		if err := rows.Scan(&id, &file.FileName, &file.FilePath); err != nil {
			return nil, err
		}
		file.ID, err = uuid.Parse(id)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

// BackupFilePathsReferenced protects files still shared by surviving job records.
func (r *BackupFileRepo) BackupFilePathsReferenced(ctx context.Context, paths []string) (bool, error) {
	if len(paths) == 0 {
		return false, nil
	}
	args := make([]any, len(paths))
	for i, path := range paths {
		args[i] = path
	}
	var referenced bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM backup_files
		WHERE file_path IN (`+strings.TrimSuffix(strings.Repeat("?,", len(paths)), ",")+`))`, args...).Scan(&referenced)
	return referenced, err
}

// CompleteFileDeletion acknowledges only a successful or already absent disk file.
func (r *BackupFileRepo) CompleteFileDeletion(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM backup_file_deletions WHERE id=?", id.String())
	return err
}

// DeferFileDeletion prevents failed paths from monopolizing subsequent cleanup batches.
func (r *BackupFileRepo) DeferFileDeletion(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE backup_file_deletions
		SET next_attempt_at=CURRENT_TIMESTAMP + INTERVAL '5 minutes' WHERE id=?`, id.String())
	return err
}
