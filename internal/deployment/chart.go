package deployment

import (
	"embed"
	"io/fs"
	"path/filepath"

	"github.com/lollinoo/theia/internal/instance"
)

//go:embed chart
var chartFiles embed.FS

// ExportChart exposes the same chart shipped in the standalone administration
// binary, so Docker and Kubernetes releases share their maintenance engine.
func ExportChart(dir string) error {
	return fs.WalkDir(chartFiles, "chart", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := chartFiles.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("chart", path)
		if err != nil {
			return err
		}
		return instance.WritePrivateFile(filepath.Join(dir, rel), data)
	})
}
