package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestPostgresCLITools_AcceptsSupportedPostgreSQL17Tools(t *testing.T) {
	versions := map[string]string{
		"pg_dump":    "pg_dump (PostgreSQL) 17.4\n",
		"pg_restore": "pg_restore (PostgreSQL) 17.4 (Debian 17.4-1.pgdg120+2)\n",
		"psql":       "psql (PostgreSQL) 17.4\n",
	}
	seen := make(map[string]bool)
	stubPostgresCLIToolCommands(t,
		func(name string) (string, error) {
			if _, ok := versions[name]; !ok {
				return "", fmt.Errorf("unexpected lookup for %s", name)
			}
			return "/usr/bin/" + name, nil
		},
		func(_ context.Context, name string, args ...string) ([]byte, error) {
			if !reflect.DeepEqual(args, []string{"--version"}) {
				t.Fatalf("%s args = %v, want [--version]", name, args)
			}
			output, ok := versions[name]
			if !ok {
				t.Fatalf("unexpected version probe for %s", name)
			}
			seen[name] = true
			return []byte(output), nil
		},
	)

	if err := ensureSupportedPostgresCLITools(context.Background(), "pg_dump", "pg_restore", "psql"); err != nil {
		t.Fatalf("ensureSupportedPostgresCLITools() error = %v", err)
	}
	for name := range versions {
		if !seen[name] {
			t.Fatalf("expected %s version probe", name)
		}
	}
}

func TestPostgresCLITools_MissingToolReturnsActionableError(t *testing.T) {
	stubPostgresCLIToolCommands(t,
		func(name string) (string, error) {
			if name == "pg_dump" {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/" + name, nil
		},
		func(_ context.Context, name string, args ...string) ([]byte, error) {
			t.Fatalf("version probe called for missing tool %s with %v", name, args)
			return nil, nil
		},
	)

	err := ensureSupportedPostgresCLITools(context.Background(), "pg_dump")
	if err == nil {
		t.Fatal("ensureSupportedPostgresCLITools() error = nil, want missing tool error")
	}
	assertPostgresCLIActionableError(t, err.Error(), "pg_dump")
}

func TestPostgresCLITools_RejectsUnsupportedMajorVersion(t *testing.T) {
	stubPostgresCLIToolCommands(t,
		func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		func(_ context.Context, name string, args ...string) ([]byte, error) {
			return []byte("pg_dump (PostgreSQL) 16.10\n"), nil
		},
	)

	err := ensureSupportedPostgresCLITools(context.Background(), "pg_dump")
	if err == nil {
		t.Fatal("ensureSupportedPostgresCLITools() error = nil, want unsupported version error")
	}
	message := err.Error()
	assertPostgresCLIActionableError(t, message, "pg_dump")
	if !strings.Contains(message, "16.10") {
		t.Fatalf("error missing probed version: %q", message)
	}
}

func TestPostgresCLITools_RejectsUnparsableVersionOutput(t *testing.T) {
	stubPostgresCLIToolCommands(t,
		func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		func(_ context.Context, name string, args ...string) ([]byte, error) {
			return []byte("pg_dump version unavailable\n"), nil
		},
	)

	err := ensureSupportedPostgresCLITools(context.Background(), "pg_dump")
	if err == nil {
		t.Fatal("ensureSupportedPostgresCLITools() error = nil, want unparsable version error")
	}
	assertPostgresCLIActionableError(t, err.Error(), "pg_dump")
}

func TestPostgresCLITools_RejectsMalformedVersionToken(t *testing.T) {
	for _, output := range []string{
		"pg_dump (PostgreSQL) 17beta1\n",
		"pg_dump (PostgreSQL) 17foo\n",
	} {
		t.Run(strings.TrimSpace(output), func(t *testing.T) {
			stubPostgresCLIToolCommands(t,
				func(name string) (string, error) {
					return "/usr/bin/" + name, nil
				},
				func(_ context.Context, name string, args ...string) ([]byte, error) {
					return []byte(output), nil
				},
			)

			err := ensureSupportedPostgresCLITools(context.Background(), "pg_dump")
			if err == nil {
				t.Fatal("ensureSupportedPostgresCLITools() error = nil, want malformed version error")
			}
			assertPostgresCLIActionableError(t, err.Error(), "pg_dump")
		})
	}
}

func TestPostgresCLITools_RedactsFailedVersionProbeDiagnostics(t *testing.T) {
	const sensitive = "should-not-appear"
	stubPostgresCLIToolCommands(t,
		func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		func(_ context.Context, name string, args ...string) ([]byte, error) {
			output := []byte("PGPASSWORD=" + sensitive + " dsn=postgres://theia:" + sensitive + "@db/theia\n")
			return output, externalCommandError(name, args, errors.New("exit status 1"), output)
		},
	)

	err := ensureSupportedPostgresCLITools(context.Background(), "pg_dump")
	if err == nil {
		t.Fatal("ensureSupportedPostgresCLITools() error = nil, want failed probe error")
	}
	message := err.Error()
	assertPostgresCLIActionableError(t, message, "pg_dump")
	for _, forbidden := range []string{
		sensitive,
		"PGPASSWORD=" + sensitive,
		"postgres://theia:" + sensitive + "@db/theia",
	} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("failed probe error leaked %q in %q", forbidden, message)
		}
	}
	if !strings.Contains(message, "PGPASSWORD=[redacted]") {
		t.Fatalf("failed probe error missing redacted output context: %q", message)
	}
}

func stubPostgresCLIToolCommands(
	t *testing.T,
	lookup func(string) (string, error),
	runner func(context.Context, string, ...string) ([]byte, error),
) {
	t.Helper()

	originalLookup := lookupExternalCommand
	originalRunner := runExternalCommand
	lookupExternalCommand = lookup
	runExternalCommand = runner
	t.Cleanup(func() {
		lookupExternalCommand = originalLookup
		runExternalCommand = originalRunner
	})
}

func assertPostgresCLIActionableError(t *testing.T, message, tool string) {
	t.Helper()

	for _, want := range []string{tool, "PostgreSQL 17", "client tools"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q missing %q", message, want)
		}
	}
}
