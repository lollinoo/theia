package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTheiaClientResolveLaunchSendsBridgeAuthorization(t *testing.T) {
	var gotAuth string
	var gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotToken = req["launch_token"]
		_ = json.NewEncoder(w).Encode(launchCredentials{
			IP:        "192.168.88.1",
			Username:  "admin",
			Password:  "secret",
			ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
		})
	}))
	defer server.Close()

	client := &TheiaClient{BaseURL: server.URL, Secret: "theia_bridge_public.raw-secret", HTTPClient: server.Client()}
	creds, err := client.ResolveLaunch(t.Context(), "launch-token")
	if err != nil {
		t.Fatalf("ResolveLaunch: %v", err)
	}
	if gotAuth != "Bridge theia_bridge_public.raw-secret" {
		t.Fatalf("Authorization header mismatch")
	}
	if gotToken != "launch-token" {
		t.Fatalf("launch_token mismatch")
	}
	if creds.IP != "192.168.88.1" || creds.Username != "admin" || creds.Password != "secret" {
		t.Fatalf("credentials mismatch")
	}
}

func TestTheiaClientResolveLaunchMapsBackendErrorsSafely(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bridge launch token already used"}`, http.StatusConflict)
	}))
	defer server.Close()

	client := &TheiaClient{BaseURL: server.URL, Secret: "theia_bridge_public.raw-secret", HTTPClient: server.Client()}
	_, err := client.ResolveLaunch(t.Context(), "launch-token")
	if err == nil {
		t.Fatal("expected backend error")
	}
	if strings.Contains(err.Error(), client.Secret) {
		t.Fatalf("error leaked bridge secret")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Fatalf("error = %v, want status code context", err)
	}
}
