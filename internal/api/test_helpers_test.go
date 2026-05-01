package api

import (
	"bytes"
	"errors"
	"log"
	"testing"

	"github.com/lollinoo/theia/internal/logging"
)

// errMock is a sentinel error used by mock repositories in tests.
var errMock = errors.New("mock error")

func captureAPIDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	logging.Configure("debug")
	t.Cleanup(func() {
		logging.Configure("info")
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})
	return &buf
}
