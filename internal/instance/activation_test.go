package instance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActivationTokenExpiryAndPersistence(t *testing.T) {
	now := time.Now().UTC()
	s, err := Generate(now)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.BeginActivation(now)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Activation.Accepts(token, now) || s.Activation.Accepts(token+"x", now) || s.Activation.Accepts(token, now.Add(time.Hour)) {
		t.Fatal("token verification/expiry failed")
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := (Store{Path: path}).Create(s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), token) {
		t.Fatal("raw activation token was persisted")
	}
	loaded, err := (Store{Path: path}).Load()
	if err != nil || !loaded.Activation.Accepts(token, now) {
		t.Fatalf("restart lost token digest: %v", err)
	}
}
