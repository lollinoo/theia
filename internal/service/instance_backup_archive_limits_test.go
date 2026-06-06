package service

// This file exercises instance backup archive limits behavior so refactors preserve the documented contract.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
)

type archiveLimitTestEntry struct {
	name     string
	body     []byte
	typeflag byte
	linkname string
}

func writeArchiveLimitTestTarGz(t *testing.T, entries ...archiveLimitTestEntry) string {
	t.Helper()

	archivePath := filepath.Join(t.TempDir(), "archive.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Create archive: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{
			Name:     entry.name,
			Typeflag: typeflag,
			Mode:     0o644,
			Linkname: entry.linkname,
		}
		if typeflag == tar.TypeReg || typeflag == tar.TypeRegA {
			header.Size = int64(len(entry.body))
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%s): %v", entry.name, err)
		}
		if header.Size > 0 {
			if _, err := tw.Write(entry.body); err != nil {
				t.Fatalf("Write(%s): %v", entry.name, err)
			}
		}
	}

	return archivePath
}

func assertRestoreLimitError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want RestoreLimitError containing %q", want)
	}
	var limitErr *RestoreLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %T %v, want RestoreLimitError", err, err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err.Error(), want)
	}
}

func TestValidateRestoreArchiveFileRejectsCompressedLimit(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "oversized.tar.gz")
	if err := os.WriteFile(archivePath, []byte("too large"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := validateRestoreArchiveFile(archivePath, RestoreArchiveLimits{
		MaxCompressedBytes: 3,
		MaxTotalBytes:      1 << 20,
		MaxEntryBytes:      1 << 20,
		MaxFileEntries:     10,
	})

	assertRestoreLimitError(t, err, "compressed archive exceeds restore limit")
}

func TestExtractArchiveContextEnforcesRestoreQuotas(t *testing.T) {
	tests := []struct {
		name    string
		limits  RestoreArchiveLimits
		entries []archiveLimitTestEntry
		want    string
	}{
		{
			name: "expanded total limit",
			limits: RestoreArchiveLimits{
				MaxCompressedBytes: 1 << 20,
				MaxTotalBytes:      6,
				MaxEntryBytes:      10,
				MaxFileEntries:     10,
			},
			entries: []archiveLimitTestEntry{
				{name: "manifest.json", body: []byte("abc")},
				{name: postgresArchiveDBEntry, body: []byte("defg")},
			},
			want: "expanded archive exceeds restore limit",
		},
		{
			name: "per entry limit",
			limits: RestoreArchiveLimits{
				MaxCompressedBytes: 1 << 20,
				MaxTotalBytes:      1 << 20,
				MaxEntryBytes:      3,
				MaxFileEntries:     10,
			},
			entries: []archiveLimitTestEntry{
				{name: postgresArchiveDBEntry, body: []byte("abcd")},
			},
			want: "exceeds per-entry restore limit",
		},
		{
			name: "file entry count limit",
			limits: RestoreArchiveLimits{
				MaxCompressedBytes: 1 << 20,
				MaxTotalBytes:      1 << 20,
				MaxEntryBytes:      1 << 20,
				MaxFileEntries:     2,
			},
			entries: []archiveLimitTestEntry{
				{name: "manifest.json", body: []byte("{}")},
				{name: postgresArchiveDBEntry, body: []byte("db")},
				{name: "known_hosts", body: []byte("host key")},
			},
			want: "archive file count exceeds restore limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archivePath := writeArchiveLimitTestTarGz(t, tt.entries...)
			err := extractArchiveContext(context.Background(), archivePath, t.TempDir(), tt.limits)
			assertRestoreLimitError(t, err, tt.want)
		})
	}
}

func TestExtractArchiveContextRejectsUnsafeRestoreEntries(t *testing.T) {
	limits := RestoreArchiveLimits{
		MaxCompressedBytes: 1 << 20,
		MaxTotalBytes:      1 << 20,
		MaxEntryBytes:      1 << 20,
		MaxFileEntries:     10,
	}
	tests := []struct {
		name  string
		entry archiveLimitTestEntry
		want  string
	}{
		{
			name:  "path traversal",
			entry: archiveLimitTestEntry{name: "backups/../database.dump", body: []byte("db")},
			want:  "path traversal",
		},
		{
			name:  "absolute path",
			entry: archiveLimitTestEntry{name: "/database.dump", body: []byte("db")},
			want:  "absolute path",
		},
		{
			name:  "symlink",
			entry: archiveLimitTestEntry{name: "known_hosts", typeflag: tar.TypeSymlink, linkname: "database.dump"},
			want:  "disallowed link entry",
		},
		{
			name:  "hard link",
			entry: archiveLimitTestEntry{name: "known_hosts", typeflag: tar.TypeLink, linkname: "database.dump"},
			want:  "disallowed link entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archivePath := writeArchiveLimitTestTarGz(t, tt.entry)
			err := extractArchiveContext(context.Background(), archivePath, t.TempDir(), limits)
			if err == nil {
				t.Fatalf("extractArchiveContext() error = nil, want %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("extractArchiveContext() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestExtractArchiveContextNormalizesSafeWindowsBackupSeparators(t *testing.T) {
	archivePath := writeArchiveLimitTestTarGz(t, archiveLimitTestEntry{
		name: "backups\\router\\config.rsc",
		body: []byte("backup-data"),
	})
	destDir := t.TempDir()

	err := extractArchiveContext(context.Background(), archivePath, destDir, RestoreArchiveLimits{
		MaxCompressedBytes: 1 << 20,
		MaxTotalBytes:      1 << 20,
		MaxEntryBytes:      1 << 20,
		MaxFileEntries:     10,
	})
	if err != nil {
		t.Fatalf("extractArchiveContext() error = %v", err)
	}

	normalizedPath := filepath.Join(destDir, "backups", "router", "config.rsc")
	data, err := os.ReadFile(normalizedPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", normalizedPath, err)
	}
	if string(data) != "backup-data" {
		t.Fatalf("normalized backup data = %q, want backup-data", string(data))
	}
	if os.PathSeparator != '\\' {
		if _, err := os.Stat(filepath.Join(destDir, "backups\\router\\config.rsc")); !os.IsNotExist(err) {
			t.Fatalf("restore should not leave unnormalized Windows-style path, stat err = %v", err)
		}
	}
}

func TestCollectArchiveSourceFilesEnforcesBackupQuotas(t *testing.T) {
	tests := []struct {
		name         string
		initialBytes int64
		fileBytes    int
		limits       BackupArchiveLimits
		want         string
	}{
		{
			name:         "expanded total limit",
			initialBytes: 3,
			fileBytes:    4,
			limits: BackupArchiveLimits{
				MaxTotalBytes:  6,
				MaxEntryBytes:  10,
				MaxFileEntries: 10,
			},
			want: "backup archive exceeds expanded backup limit",
		},
		{
			name:         "per entry limit",
			initialBytes: 1,
			fileBytes:    4,
			limits: BackupArchiveLimits{
				MaxTotalBytes:  100,
				MaxEntryBytes:  3,
				MaxFileEntries: 10,
			},
			want: "exceeds per-entry backup limit",
		},
		{
			name:         "file entry count limit",
			initialBytes: 1,
			fileBytes:    1,
			limits: BackupArchiveLimits{
				MaxTotalBytes:  100,
				MaxEntryBytes:  10,
				MaxFileEntries: 1,
			},
			want: "backup archive file count exceeds backup limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			deviceBackupDir := filepath.Join(tmpDir, "device-backups")
			if err := os.MkdirAll(filepath.Join(deviceBackupDir, "router"), 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(filepath.Join(deviceBackupDir, "router", "config.rsc"), []byte(strings.Repeat("x", tt.fileBytes)), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			svc := NewInstanceBackupService(nil, nil, nil, filepath.Join(tmpDir, "instance-backups"), deviceBackupDir, filepath.Join(tmpDir, "known_hosts"), "", "", nil)

			_, _, _, _, _, err := svc.collectArchiveSourceFiles(context.Background(), tt.limits, tt.initialBytes)
			assertRestoreLimitError(t, err, tt.want)
		})
	}
}

func TestCollectInstanceBackupArchiveSourceFilesIncludesAllowedSourcesAndSkipsInstanceBackups(t *testing.T) {
	tmpDir := t.TempDir()
	deviceBackupDir := filepath.Join(tmpDir, "device-backups")
	instanceBackupDir := filepath.Join(deviceBackupDir, "instance-backups")
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")
	if err := os.MkdirAll(filepath.Join(deviceBackupDir, "router"), 0o700); err != nil {
		t.Fatalf("creating router backup dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(instanceBackupDir, "old-instance"), 0o700); err != nil {
		t.Fatalf("creating instance backup dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deviceBackupDir, "router", "config.rsc"), []byte("device"), 0o600); err != nil {
		t.Fatalf("writing device backup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(instanceBackupDir, "old-instance", "archive.tar.gz"), []byte("instance"), 0o600); err != nil {
		t.Fatalf("writing nested instance backup: %v", err)
	}
	if err := os.WriteFile(knownHostsPath, []byte("known-hosts"), 0o600); err != nil {
		t.Fatalf("writing known_hosts: %v", err)
	}

	sources, err := collectInstanceBackupArchiveSourceFiles(
		context.Background(),
		instanceBackupDir,
		deviceBackupDir,
		knownHostsPath,
		DefaultBackupArchiveLimits,
		4,
	)

	if err != nil {
		t.Fatalf("collectInstanceBackupArchiveSourceFiles: %v", err)
	}
	if sources.backupFileCount != 1 {
		t.Fatalf("backupFileCount = %d, want 1", sources.backupFileCount)
	}
	if len(sources.deviceBackupFiles) != 1 {
		t.Fatalf("deviceBackupFiles = %d, want 1", len(sources.deviceBackupFiles))
	}
	if got := sources.deviceBackupFiles[0].archiveName; got != "backups/router/config.rsc" {
		t.Fatalf("device archiveName = %q, want backups/router/config.rsc", got)
	}
	if strings.Contains(sources.deviceBackupFiles[0].diskPath, "old-instance") {
		t.Fatalf("instance backup source should be skipped, got %s", sources.deviceBackupFiles[0].diskPath)
	}
	if sources.knownHostsFile == nil {
		t.Fatal("knownHostsFile = nil, want known_hosts source")
	}
	if got := sources.knownHostsFile.archiveName; got != "known_hosts" {
		t.Fatalf("known_hosts archiveName = %q, want known_hosts", got)
	}
	if sources.totalBytes != int64(4+len("device")+len("known-hosts")) {
		t.Fatalf("totalBytes = %d, want %d", sources.totalBytes, 4+len("device")+len("known-hosts"))
	}
	if sources.fileEntries != 3 {
		t.Fatalf("fileEntries = %d, want 3", sources.fileEntries)
	}
}

func TestWriteInstanceBackupArchiveWritesEntriesAndReportsProgress(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "instance.tar.gz")
	dbPath := filepath.Join(tmpDir, "database.dump")
	devicePath := filepath.Join(tmpDir, "router.rsc")
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")
	if err := os.WriteFile(dbPath, []byte("database"), 0o600); err != nil {
		t.Fatalf("writing database dump: %v", err)
	}
	if err := os.WriteFile(devicePath, []byte("device-backup"), 0o600); err != nil {
		t.Fatalf("writing device backup: %v", err)
	}
	if err := os.WriteFile(knownHostsPath, []byte("known-hosts"), 0o600); err != nil {
		t.Fatalf("writing known_hosts: %v", err)
	}
	manifestJSON := []byte(`{"version":1}`)
	manifest := &backupManifest{TotalSizeBytes: int64(len(manifestJSON) + len("database") + len("device-backup") + len("known-hosts"))}
	var progress []domain.InstanceBackupProgress

	total, err := writeInstanceBackupArchive(context.Background(), instanceBackupArchiveWriteRequest{
		archivePath: archivePath,
		dbArtifact: databaseBackupArtifact{
			tempPath:         dbPath,
			archiveEntryName: postgresArchiveDBEntry,
		},
		deviceBackupFiles: []archiveSourceFile{{
			archiveName: "backups/router/config.rsc",
			diskPath:    devicePath,
			sizeBytes:   int64(len("device-backup")),
		}},
		knownHostsFile: &archiveSourceFile{
			archiveName: "known_hosts",
			diskPath:    knownHostsPath,
			sizeBytes:   int64(len("known-hosts")),
		},
		manifestJSON: manifestJSON,
		manifest:     manifest,
		limits:       DefaultBackupArchiveLimits,
		progress: func(update domain.InstanceBackupProgress) {
			progress = append(progress, update)
		},
	})

	if err != nil {
		t.Fatalf("writeInstanceBackupArchive: %v", err)
	}
	if total != manifest.TotalSizeBytes {
		t.Fatalf("total = %d, want %d", total, manifest.TotalSizeBytes)
	}
	entries := readArchiveEntries(t, archivePath)
	for name, want := range map[string]string{
		"manifest.json":             string(manifestJSON),
		postgresArchiveDBEntry:      "database",
		"backups/router/config.rsc": "device-backup",
		"known_hosts":               "known-hosts",
	} {
		if got := string(entries[name]); got != want {
			t.Fatalf("entry %s = %q, want %q", name, got, want)
		}
	}
	if len(progress) != 4 {
		t.Fatalf("progress updates = %d, want 4", len(progress))
	}
	if progress[0].Message != "Archived manifest" {
		t.Fatalf("first progress message = %q, want Archived manifest", progress[0].Message)
	}
	last := progress[len(progress)-1]
	if last.Message != "Archived known_hosts" {
		t.Fatalf("last progress message = %q, want Archived known_hosts", last.Message)
	}
	if last.Current != manifest.TotalSizeBytes || last.Total != manifest.TotalSizeBytes {
		t.Fatalf("last progress current/total = %d/%d, want %d/%d", last.Current, last.Total, manifest.TotalSizeBytes, manifest.TotalSizeBytes)
	}
}

func TestCheckedArchiveByteTotalRejectsOverflowBeforeAdding(t *testing.T) {
	_, err := checkedArchiveByteTotal(math.MaxInt64-1, 10, math.MaxInt64)
	assertRestoreLimitError(t, err, "backup archive exceeds expanded backup limit")
}
