package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const supportedPostgresCLIToolMajorVersion = 17

var postgresCLIToolVersionPattern = regexp.MustCompile(`(?i)\(PostgreSQL\)\s+([0-9]+)(?:\.[0-9]+)?`)

func ensureSupportedPostgresCLITools(ctx context.Context, names ...string) error {
	for _, name := range names {
		if err := ensureExternalCommand(name); err != nil {
			return fmt.Errorf("%s is required but was not found; install PostgreSQL %d.x client tools and ensure %q is on PATH: %s", name, supportedPostgresCLIToolMajorVersion, name, redactExternalCommandText(err.Error()))
		}

		output, err := runExternalCommand(ctx, name, "--version")
		if err != nil {
			return fmt.Errorf("%s --version failed; expected PostgreSQL %d.x client tools: %s", name, supportedPostgresCLIToolMajorVersion, redactExternalCommandText(err.Error()))
		}

		major, ok := parsePostgresCLIToolMajorVersion(output)
		if !ok {
			return fmt.Errorf("could not parse %s PostgreSQL client version from --version output %q; expected PostgreSQL %d.x client tools", name, redactedPostgresCLIToolOutput(output), supportedPostgresCLIToolMajorVersion)
		}
		if major != supportedPostgresCLIToolMajorVersion {
			return fmt.Errorf("%s reports unsupported PostgreSQL client version %q; expected PostgreSQL %d.x client tools", name, redactedPostgresCLIToolOutput(output), supportedPostgresCLIToolMajorVersion)
		}
	}
	return nil
}

func parsePostgresCLIToolMajorVersion(output []byte) (int, bool) {
	matches := postgresCLIToolVersionPattern.FindSubmatch(output)
	if len(matches) != 2 {
		return 0, false
	}
	major, err := strconv.Atoi(string(matches[1]))
	if err != nil {
		return 0, false
	}
	return major, true
}

func redactedPostgresCLIToolOutput(output []byte) string {
	return strings.TrimSpace(redactExternalCommandText(string(output)))
}
