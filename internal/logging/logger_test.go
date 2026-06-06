package logging

// This file exercises logger behavior so refactors preserve the documented contract.

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func restoreLogger(t *testing.T) {
	t.Helper()

	originalWriter := log.Writer()
	originalFlags := log.Flags()
	t.Cleanup(func() {
		Configure("info")
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})
}

func TestConfigureNormalizesKnownLevels(t *testing.T) {
	restoreLogger(t)

	cases := map[string]Level{
		"debug": LevelDebug,
		"info":  LevelInfo,
		"warn":  LevelWarn,
		"error": LevelError,
		"":      LevelInfo,
		"nope":  LevelInfo,
	}

	for raw, want := range cases {
		if got := Configure(raw); got != want {
			t.Fatalf("Configure(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestLoggerFiltersBelowConfiguredLevel(t *testing.T) {
	var buf bytes.Buffer
	restoreLogger(t)

	log.SetOutput(&buf)
	log.SetFlags(0)

	Configure("warn")
	Debugf("hidden debug")
	Infof("hidden info")
	Warnf("visible warn")
	Errorf("visible error")

	output := buf.String()
	if strings.Contains(output, "hidden") {
		t.Fatalf("output contains filtered message: %q", output)
	}
	if !strings.Contains(output, "WARN visible warn") {
		t.Fatalf("output missing warn message: %q", output)
	}
	if !strings.Contains(output, "ERROR visible error") {
		t.Fatalf("output missing error message: %q", output)
	}
}

func TestDebugShortFileReportsHelperCaller(t *testing.T) {
	var buf bytes.Buffer
	restoreLogger(t)

	log.SetOutput(&buf)
	Configure("debug")

	Debugf("caller check")

	output := buf.String()
	if !strings.Contains(output, "logger_test.go:") {
		t.Fatalf("output missing caller file: %q", output)
	}
	if strings.Contains(output, "logger.go:") {
		t.Fatalf("output points at logging helper instead of caller: %q", output)
	}
}
