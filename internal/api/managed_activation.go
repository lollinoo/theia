package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/service"
)

//go:embed activation.html
var activationPage string

// WithManagedActivation adds first-administrator activation and probe endpoints
// outside the authenticated application routes. It does not enable legacy setup.
func WithManagedActivation(next http.Handler, activation *service.ManagedActivation, db *sql.DB) http.Handler {
	script := strings.Split(strings.Split(activationPage, "<script>")[1], "</script>")[0]
	digest := sha256.Sum256([]byte(script))
	csp := "default-src 'none'; connect-src 'self'; img-src 'self'; style-src 'unsafe-inline'; script-src 'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/livez" || r.URL.Path == "/readyz" {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.URL.Path == "/readyz" {
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				if err := db.PingContext(ctx); err != nil {
					http.Error(w, "database unavailable", http.StatusServiceUnavailable)
					return
				}
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if activation == nil || (r.URL.Path != "/activate" && !strings.HasPrefix(r.URL.Path, "/api/v1/setup/")) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/activate" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Security-Policy", csp)
			io.WriteString(w, activationPage)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || (u.Scheme != "https" && u.Scheme != "http") {
				http.Error(w, "origin rejected", http.StatusForbidden)
				return
			}
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "JSON required", http.StatusUnsupportedMediaType)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == r.Header.Get("Authorization") {
			token = ""
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var err error
		var result any = map[string]bool{"activated": true}
		switch r.URL.Path {
		case "/api/v1/setup/recovery":
			result, err = activation.Recovery(ctx, token)
		case "/api/v1/setup/complete":
			var input service.ActivateInstanceInput
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
				http.Error(w, "invalid activation request", http.StatusBadRequest)
				return
			}
			err = activation.Complete(ctx, token, input)
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			if errors.Is(err, service.ErrActivationDenied) {
				http.Error(w, service.ErrActivationDenied.Error(), http.StatusForbidden)
			} else {
				http.Error(w, "Activation could not finish. Check your details and recovery file, then retry.", http.StatusBadRequest)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})
}
