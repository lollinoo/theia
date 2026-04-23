package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

var lookupExternalCommand = exec.LookPath

var runExternalCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return output, nil
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, trimmed)
}

func ensureExternalCommand(name string) error {
	if _, err := lookupExternalCommand(name); err != nil {
		return fmt.Errorf("required command %q not found in PATH: %w", name, err)
	}
	return nil
}
