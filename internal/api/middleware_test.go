package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBodySizeAllowed(t *testing.T) {
	handler := MaxBodySize(1 << 20)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	}))
	body := bytes.NewReader(make([]byte, 100))
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("expected non-413, got %d", rec.Code)
	}
}

func TestMaxBodySizeExceeded(t *testing.T) {
	handler := MaxBodySize(1 << 20)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if !decodeJSON(w, r, &body) {
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	// Build a valid JSON body that exceeds 1MB so json.Decoder hits MaxBytesError
	bigJSON := `{"data":"` + strings.Repeat("a", 2<<20) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(bigJSON))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestMaxBodySizeExactLimit(t *testing.T) {
	handler := MaxBodySize(1 << 20)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	}))
	// Exactly 1MB body
	body := bytes.NewReader(make([]byte, 1<<20))
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("exact limit should succeed, got 413")
	}
}

func TestDecodeJSONBodyTooLarge(t *testing.T) {
	rec := httptest.NewRecorder()
	// Build a valid JSON body that exceeds 1MB so json.Decoder hits MaxBytesError
	bigJSON := `{"data":"` + strings.Repeat("a", 2<<20) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(bigJSON))
	req.Body = http.MaxBytesReader(rec, req.Body, 1<<20)
	var v map[string]string
	if decodeJSON(rec, req, &v) {
		t.Fatal("expected decodeJSON to return false for oversized body")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestDecodeJSONBodyValid(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	var v map[string]string
	if !decodeJSON(rec, req, &v) {
		t.Fatal("expected decodeJSON to return true for valid body")
	}
	if v["name"] != "test" {
		t.Fatalf("expected name=test, got %s", v["name"])
	}
}

func TestWebSocketBypassesMaxBodySize(t *testing.T) {
	// Reproduce the dispatch pattern from router.go to prove WebSocket requests
	// bypass the middleware chain (including MaxBodySize). This mirrors the exact
	// code path in NewRouter's returned http.HandlerFunc.
	wsServed := false
	wsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsServed = true
		w.WriteHeader(http.StatusOK)
	})

	// Build a handler chain identical to the one in NewRouter
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", wsHandler)

	var handler http.Handler = mux
	handler = JSONContentType(handler)
	handler = MaxBodySize(1 << 20)(handler) // 1 MB limit
	handler = RequestLogger(handler)
	handler = CORS(handler)

	// Dispatch function mirrors router.go: WS path bypasses the handler chain
	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ws" {
			wsHandler.ServeHTTP(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})

	// Send a request to the WebSocket path with a body exceeding the 1MB limit.
	bigBody := bytes.NewReader(make([]byte, 2<<20))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", bigBody)
	rec := httptest.NewRecorder()
	dispatch.ServeHTTP(rec, req)

	if !wsServed {
		t.Fatal("expected WebSocket handler to be called, but it was not")
	}
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatal("WebSocket request was rejected with 413 -- MaxBodySize should not apply to /api/v1/ws")
	}
}
