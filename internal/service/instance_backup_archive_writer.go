package service

// This file defines instance backup archive writer backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/instance"
)

type instanceBackupArchiveWriteRequest struct {
	privateState      []byte
	archivePath       string
	dbArtifact        databaseBackupArtifact
	deviceBackupFiles []archiveSourceFile
	knownHostsFile    *archiveSourceFile
	manifestJSON      []byte
	manifest          *backupManifest
	limits            BackupArchiveLimits
	progress          func(domain.InstanceBackupProgress)
}

// createArchive builds a .tar.gz archive containing manifest, database, device backups, and known_hosts.
// Returns the total size of all archived file data.
func (s *InstanceBackupService) createArchive(
	ctx context.Context,
	archivePath string,
	dbArtifact databaseBackupArtifact,
	deviceBackupFiles []archiveSourceFile,
	knownHostsFile *archiveSourceFile,
	manifestJSON []byte,
	manifest *backupManifest,
	backupID uuid.UUID,
) (int64, error) {
	size, _, err := s.createArchiveWithProtection(ctx, archivePath, dbArtifact, deviceBackupFiles, knownHostsFile, manifestJSON, manifest, backupID, nil)
	return size, err
}

func (s *InstanceBackupService) createArchiveWithProtection(ctx context.Context, archivePath string, dbArtifact databaseBackupArtifact, deviceBackupFiles []archiveSourceFile, knownHostsFile *archiveSourceFile, manifestJSON []byte, manifest *backupManifest, backupID uuid.UUID, privateState []byte) (int64, *age.X25519Identity, error) {
	req := instanceBackupArchiveWriteRequest{
		privateState:      privateState,
		archivePath:       archivePath,
		dbArtifact:        dbArtifact,
		deviceBackupFiles: deviceBackupFiles,
		knownHostsFile:    knownHostsFile,
		manifestJSON:      manifestJSON,
		manifest:          manifest,
		limits:            s.BackupArchiveLimits(),
		progress: func(update domain.InstanceBackupProgress) {
			s.updateInstanceBackupProgress(backupID, update)
		},
	}
	if len(privateState) == 0 {
		size, err := writeInstanceBackupArchive(ctx, req)
		return size, nil, err
	}
	var state instance.State
	if err := json.Unmarshal(privateState, &state); err != nil {
		return 0, nil, fmt.Errorf("invalid private archive state")
	}
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return 0, nil, err
	}
	protected, identity, err := instance.ProtectWriter(f, state.RecoveryRecipient)
	if err != nil {
		f.Close()
		return 0, nil, err
	}
	size, err := writeInstanceBackupArchiveTo(ctx, req, protected)
	return size, identity, err
}

// writeInstanceBackupArchive writes the full tar.gz archive and reports progress after each entry.
func writeInstanceBackupArchive(ctx context.Context, req instanceBackupArchiveWriteRequest) (int64, error) {
	f, err := os.OpenFile(req.archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, fmt.Errorf("creating archive file: %w", err)
	}
	return writeInstanceBackupArchiveTo(ctx, req, f)
}

// writeInstanceBackupArchiveTo owns the destination and finishes each archive layer
// before returning success. Closing an outer layer can still write to the next one.
func writeInstanceBackupArchiveTo(ctx context.Context, req instanceBackupArchiveWriteRequest, dst io.WriteCloser) (totalSize int64, err error) {
	gw := gzip.NewWriter(dst)

	tw := tar.NewWriter(gw)
	defer func() {
		for _, layer := range []struct {
			name  string
			close func() error
		}{
			{"tar", tw.Close},
			{"gzip", gw.Close},
			{"file", dst.Close},
		} {
			if closeErr := layer.close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("closing archive %s: %w", layer.name, closeErr))
				totalSize = 0
			}
		}
	}()

	// Add manifest.json
	if err := addBytesToTar(tw, "manifest.json", req.manifestJSON, time.Now().UTC()); err != nil {
		return 0, fmt.Errorf("adding manifest to archive: %w", err)
	}
	totalSize += int64(len(req.manifestJSON))
	reportInstanceBackupArchiveProgress(req, "Archived manifest", totalSize)
	if len(req.privateState) > 0 {
		if err := addBytesToTar(tw, "instance-secrets.json", req.privateState, time.Now().UTC()); err != nil {
			return 0, err
		}
		totalSize += int64(len(req.privateState))
	}

	// Add the PostgreSQL dump.
	dbSize, err := addFileToTarContext(ctx, tw, req.dbArtifact.archiveEntryName, req.dbArtifact.tempPath)
	if err != nil {
		return 0, fmt.Errorf("adding database to archive: %w", err)
	}
	totalSize += dbSize
	reportInstanceBackupArchiveProgress(req, "Archived database snapshot", totalSize)

	// Add device backup files under backups/
	for _, bf := range req.deviceBackupFiles {
		size, err := addCollectedFileToTarContext(ctx, tw, bf, req.limits)
		if err != nil {
			if len(req.privateState) > 0 {
				return 0, fmt.Errorf("archiving complete instance: %w", err)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isArchiveQuotaError(err) {
				return 0, err
			}
			log.Printf("Warning: skipping device backup file %s: %v", bf.diskPath, err)
			continue
		}
		totalSize += size
		reportInstanceBackupArchiveProgress(req, "Archived "+bf.archiveName, totalSize)
	}

	// Add known_hosts if it exists
	if req.knownHostsFile != nil {
		size, err := addCollectedFileToTarContext(ctx, tw, *req.knownHostsFile, req.limits)
		if err != nil {
			if len(req.privateState) > 0 {
				return 0, fmt.Errorf("archiving complete instance: %w", err)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isArchiveQuotaError(err) {
				return 0, err
			}
			log.Printf("Warning: failed to add known_hosts to archive: %v", err)
		} else {
			totalSize += size
			reportInstanceBackupArchiveProgress(req, "Archived known_hosts", totalSize)
		}
	}

	return totalSize, nil
}

// reportInstanceBackupArchiveProgress publishes an archiving phase update when a tracker is present.
func reportInstanceBackupArchiveProgress(req instanceBackupArchiveWriteRequest, message string, current int64) {
	if req.progress == nil || req.manifest == nil {
		return
	}
	req.progress(domain.InstanceBackupProgress{
		Phase:   "archiving",
		Message: message,
		Current: current,
		Total:   req.manifest.TotalSizeBytes,
	})
}

// addFileToTar adds a file from disk to the tar archive. Returns the file size.
func addFileToTar(tw *tar.Writer, name string, sourcePath string) (int64, error) {
	return addFileToTarContext(context.Background(), tw, name, sourcePath)
}

// addFileToTarContext streams one file into the archive and verifies the expected byte count.
func addFileToTarContext(ctx context.Context, tw *tar.Writer, name string, sourcePath string) (int64, error) {
	f, err := os.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", sourcePath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("statting %s: %w", sourcePath, err)
	}

	header := &tar.Header{
		Name:    name,
		Size:    info.Size(),
		Mode:    0644,
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return 0, fmt.Errorf("writing header for %s: %w", name, err)
	}

	written, err := copyWithContext(ctx, tw, f)
	if err != nil {
		return 0, fmt.Errorf("writing data for %s: %w", name, err)
	}
	if written != info.Size() {
		return 0, fmt.Errorf("writing data for %s: wrote %d bytes, expected %d bytes", name, written, info.Size())
	}
	return written, nil
}

// addCollectedFileToTarContext rechecks quotas before archiving a previously collected file.
func addCollectedFileToTarContext(ctx context.Context, tw *tar.Writer, source archiveSourceFile, limits BackupArchiveLimits) (int64, error) {
	f, err := os.Open(source.diskPath)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", source.diskPath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("statting %s: %w", source.diskPath, err)
	}
	if err := checkBackupArchiveEntryQuota(source.archiveName, info.Size(), limits); err != nil {
		return 0, err
	}
	if info.Size() > source.sizeBytes {
		return 0, newRestoreLimitError("backup archive entry %s grew after collection: %d bytes > %d bytes", source.archiveName, info.Size(), source.sizeBytes)
	}

	header := &tar.Header{
		Name:    source.archiveName,
		Size:    info.Size(),
		Mode:    0644,
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return 0, fmt.Errorf("writing header for %s: %w", source.archiveName, err)
	}

	written, err := copyWithContext(ctx, tw, f)
	if err != nil {
		return 0, fmt.Errorf("writing data for %s: %w", source.archiveName, err)
	}
	if written != info.Size() {
		return 0, fmt.Errorf("writing data for %s: wrote %d bytes, expected %d bytes", source.archiveName, written, info.Size())
	}
	return written, nil
}

// addBytesToTar adds raw bytes as a tar entry.
func addBytesToTar(tw *tar.Writer, name string, data []byte, modTime time.Time) error {
	header := &tar.Header{
		Name:    name,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: modTime,
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("writing header for %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("writing data for %s: %w", name, err)
	}
	return nil
}
