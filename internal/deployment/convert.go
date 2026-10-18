package deployment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConvertLegacy preserves the input and publishes only a restore-verified
// encrypted replacement. It does not stop or modify the live database.
func (a *Admin) ConvertLegacy(ctx context.Context, input, output string) error {
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		return fmt.Errorf("conversion output already exists or cannot be inspected")
	}
	input, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	destination, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if strings.ContainsAny(input+destination, ":$\r\n") {
		return fmt.Errorf("conversion paths cannot be mounted")
	}
	info, err := os.Lstat(input)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("legacy archive must be a regular file")
	}
	temporary, err := os.MkdirTemp(filepath.Dir(destination), ".theia-converted-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := a.compose(ctx, "run", "--rm", "--no-deps", "-T", "-v", input+":/run/theia/legacy.tar.gz:ro", "-v", temporary+":/converted", "backend", "maintenance", "convert-legacy", "-archive", "/run/theia/legacy.tar.gz", "-output", "/converted/archive.age"); err != nil {
		return err
	}
	if err := os.Link(filepath.Join(temporary, "archive.age"), destination); err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Output, "Verified encrypted conversion exported to %s; original archive retained.\n", destination)
	return err
}
