package service

import (
	"archive/tar"
	"strings"
	"testing"
)

// TestValidateRestoreArchiveHeaderClassifiesAllowedEntries keeps tar header validation focused and fail-closed.
func TestValidateRestoreArchiveHeaderClassifiesAllowedEntries(t *testing.T) {
	tests := []struct {
		name      string
		header    *tar.Header
		wantName  string
		wantDir   bool
		wantError string
	}{
		{
			name:     "regular backup file",
			header:   &tar.Header{Name: "backups/router/config.rsc", Typeflag: tar.TypeReg},
			wantName: "backups/router/config.rsc",
		},
		{
			name:     "backup directory",
			header:   &tar.Header{Name: "backups/router", Typeflag: tar.TypeDir},
			wantName: "backups/router",
			wantDir:  true,
		},
		{
			name:      "symlink",
			header:    &tar.Header{Name: "known_hosts", Typeflag: tar.TypeSymlink, Linkname: "database.dump"},
			wantError: "disallowed link entry",
		},
		{
			name:      "unsupported type",
			header:    &tar.Header{Name: "known_hosts", Typeflag: tar.TypeFifo},
			wantError: "unsupported restore archive entry type",
		},
		{
			name:      "traversal",
			header:    &tar.Header{Name: "backups/../database.dump", Typeflag: tar.TypeReg},
			wantError: "path traversal",
		},
		{
			name:      "disallowed file",
			header:    &tar.Header{Name: "secret.txt", Typeflag: tar.TypeReg},
			wantError: "disallowed restore archive entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateRestoreArchiveHeader(tt.header)
			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("validateRestoreArchiveHeader() error = nil, want %q", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("validateRestoreArchiveHeader() error = %q, want %q", err.Error(), tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateRestoreArchiveHeader() error = %v", err)
			}
			if got.cleanName != tt.wantName {
				t.Fatalf("validateRestoreArchiveHeader().cleanName = %q, want %q", got.cleanName, tt.wantName)
			}
			if got.directory != tt.wantDir {
				t.Fatalf("validateRestoreArchiveHeader().directory = %v, want %v", got.directory, tt.wantDir)
			}
		})
	}
}
