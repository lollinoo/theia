package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/service"
)

func TestManagedActivationHTTPBoundaries(t *testing.T) {
	s, _ := instance.Generate(time.Now())
	s.BeginActivation(time.Now())
	store := instance.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	if err := store.Create(s); err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := WithManagedActivation(next, &service.ManagedActivation{Store: store}, nil)
	for _, tc := range []struct {
		method, path, origin, content string
		want                          int
	}{
		{"GET", "/activate", "", "", 200},
		{"GET", "/api/v1/setup/recovery", "", "", 405},
		{"POST", "/api/v1/setup/recovery", "https://evil.test", "application/json", 403},
		{"POST", "/api/v1/setup/recovery", "", "text/plain", 415},
		{"POST", "/api/v1/setup/recovery", "", "application/json", 403},
		{"POST", "/api/v1/setup/complete", "", "application/json", 403},
		{"GET", "/livez", "", "", 200},
		{"GET", "/api/v1/auth/me", "", "", 418},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s: %d", tc.method, tc.path, w.Code)
		}
		if tc.path == "/activate" && (!strings.Contains(w.Header().Get("Content-Security-Policy"), "sha256-") || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatal("activation page security headers missing")
		}
	}
}
