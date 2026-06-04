package service

import (
	"strings"
	"testing"
)

func TestValidateRestoreArchiveEntryForExtraction(t *testing.T) {
	tests := []struct {
		name      string
		entryName string
		isDir     bool
		wantName  string
		wantErr   string
	}{
		{
			name:      "normalizes backup file separators",
			entryName: "backups\\router\\config.rsc",
			wantName:  "backups/router/config.rsc",
		},
		{
			name:      "allows backup directories",
			entryName: "backups/router",
			isDir:     true,
			wantName:  "backups/router",
		},
		{
			name:      "rejects absolute paths",
			entryName: "/database.dump",
			wantErr:   "absolute path",
		},
		{
			name:      "rejects traversal",
			entryName: "backups/../database.dump",
			wantErr:   "path traversal",
		},
		{
			name:      "rejects disallowed files",
			entryName: "secrets.txt",
			wantErr:   "disallowed restore archive entry: secrets.txt",
		},
		{
			name:      "rejects disallowed directories",
			entryName: "secrets",
			isDir:     true,
			wantErr:   "disallowed restore archive entry: secrets",
		},
		{
			name:      "keeps legacy sqlite restore guidance",
			entryName: legacySQLiteArchiveDBEntry,
			wantErr:   "legacy SQLite instance backup archives",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateRestoreArchiveEntryForExtraction(tt.entryName, tt.isDir)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("validateRestoreArchiveEntryForExtraction() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("validateRestoreArchiveEntryForExtraction() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateRestoreArchiveEntryForExtraction() error = %v", err)
			}
			if got != tt.wantName {
				t.Fatalf("validateRestoreArchiveEntryForExtraction() = %q, want %q", got, tt.wantName)
			}
		})
	}
}
