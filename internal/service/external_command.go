package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode"
)

var lookupExternalCommand = exec.LookPath

var runExternalCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return runExternalCommandWithEnv(ctx, nil, name, args...)
}

var runExternalCommandWithEnv = func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return output, nil
	}

	return output, externalCommandError(name, args, err, output)
}

func ensureExternalCommand(name string) error {
	if _, err := lookupExternalCommand(name); err != nil {
		return fmt.Errorf("required command %q not found in PATH: %w", name, err)
	}
	return nil
}

const externalCommandRedactedValue = "[redacted]"

var (
	externalCommandPasswordAssignmentPattern = regexp.MustCompile(`(?i)(password\s*=\s*)('[^']*'|"[^"]*"|[^&\s]+)`)
	externalCommandPGPasswordPattern         = regexp.MustCompile(`(?i)(PGPASSWORD=)[^\s]+`)
)

func externalCommandError(name string, args []string, err error, output []byte) error {
	command := formatExternalCommand(name, args)
	trimmedOutput := strings.TrimSpace(redactExternalCommandText(string(output)))
	if trimmedOutput == "" {
		return fmt.Errorf("%s: %w", command, err)
	}
	return fmt.Errorf("%s: %w: %s", command, err, trimmedOutput)
}

func formatExternalCommand(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(redactExternalCommandArgs(args), " ")
}

func redactExternalCommandArgs(args []string) []string {
	redacted := make([]string, 0, len(args))
	redactNext := false
	for _, arg := range args {
		if redactNext {
			redacted = append(redacted, externalCommandRedactedValue)
			redactNext = false
			continue
		}
		if isSensitiveExternalCommandFlag(arg) {
			redacted = append(redacted, arg)
			redactNext = true
			continue
		}
		if flag, _, ok := strings.Cut(arg, "="); ok && isSensitiveExternalCommandFlag(flag) {
			redacted = append(redacted, flag+"="+externalCommandRedactedValue)
			continue
		}
		redacted = append(redacted, redactExternalCommandText(arg))
	}
	return redacted
}

func isSensitiveExternalCommandFlag(flag string) bool {
	switch flag {
	case "--dbname", "-d", "--password":
		return true
	default:
		return false
	}
}

func redactExternalCommandText(text string) string {
	redacted := redactPostgresURLTokens(text)
	redacted = externalCommandPasswordAssignmentPattern.ReplaceAllString(redacted, "${1}"+externalCommandRedactedValue)
	redacted = externalCommandPGPasswordPattern.ReplaceAllString(redacted, "${1}"+externalCommandRedactedValue)
	return redacted
}

func redactPostgresURLTokens(text string) string {
	var b strings.Builder
	inToken := false
	tokenStart := 0

	writeToken := func(token string) {
		lower := strings.ToLower(token)
		if strings.Contains(lower, "postgres://") || strings.Contains(lower, "postgresql://") {
			b.WriteString(externalCommandRedactedValue)
			return
		}
		b.WriteString(token)
	}

	for index, r := range text {
		if unicode.IsSpace(r) {
			if inToken {
				writeToken(text[tokenStart:index])
				inToken = false
			}
			b.WriteRune(r)
			continue
		}
		if !inToken {
			tokenStart = index
			inToken = true
		}
	}
	if inToken {
		writeToken(text[tokenStart:])
	}

	return b.String()
}
