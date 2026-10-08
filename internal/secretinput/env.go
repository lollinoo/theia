// Package secretinput reads environment secrets or Docker/Kubernetes secret files.
package secretinput

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Read accepts NAME or NAME_FILE, never both. File trailing line endings are
// removed; other whitespace is preserved for compatibility with original secrets.
func Read(name string) (string, error) {
	value, path := os.Getenv(name), os.Getenv(name+"_FILE")
	if value != "" && path != "" {
		return "", fmt.Errorf("set only one of %s and %s_FILE", name, name)
	}
	if path == "" {
		return value, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", name, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", name, err)
	}
	if len(data) > 64<<10 {
		return "", fmt.Errorf("%s_FILE exceeds secret size limit", name)
	}
	value = strings.TrimRight(string(data), "\r\n")
	if value == "" {
		return "", fmt.Errorf("%s_FILE is empty", name)
	}
	return value, nil
}
