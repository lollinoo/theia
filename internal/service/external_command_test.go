package service

// This file exercises external command behavior so refactors preserve the documented contract.

import (
	"errors"
	"strings"
	"testing"
)

func TestExternalCommandErrorRedactsSensitiveArgsAndOutput(t *testing.T) {
	const sensitive = "should-not-appear"

	err := externalCommandError(
		"pg_dump",
		[]string{
			"--format=custom",
			"--dbname",
			"host='db' user='theia' password='" + sensitive + "' dbname='theia'",
			"--file",
			"/tmp/theia.dump",
		},
		errors.New("exit status 1"),
		[]byte("FATAL: password="+sensitive+" dsn=postgres://theia:"+sensitive+"@db/theia"),
	)

	message := err.Error()
	for _, forbidden := range []string{
		sensitive,
		"password='" + sensitive + "'",
		"password=" + sensitive,
		"postgres://theia:" + sensitive + "@db/theia",
	} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("external command error leaked %q in %q", forbidden, message)
		}
	}
	if !strings.Contains(message, "pg_dump --format=custom --dbname [redacted] --file /tmp/theia.dump") {
		t.Fatalf("external command error missing redacted command context: %q", message)
	}
	if !strings.Contains(message, "password=[redacted]") {
		t.Fatalf("external command error missing redacted stderr context: %q", message)
	}
}
