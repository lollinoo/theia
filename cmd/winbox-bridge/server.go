package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// ServerManager owns the HTTP server lifecycle.
// Start and Stop can be called from any goroutine (mutex-protected).
// This satisfies T-29-02: mutex prevents concurrent Start/Stop races.
type ServerManager struct {
	mu     sync.Mutex
	server *http.Server
	port   int // current listening port, for status display
}

// Start creates and runs a new HTTP server with the given config.
// Re-discovers WinBox path from cfg.WinBoxPath on each call.
// The host header check uses fmt.Sprintf("localhost:%d", cfg.ListenPort) — NOT hardcoded 1337.
// Returns nil and is a no-op if the server is already running (T-29-02 mitigation).
func (m *ServerManager) Start(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		return nil // already running — no-op
	}
	winboxPath := discoverWinBox(cfg.WinBoxPath)
	expectedHost := fmt.Sprintf("localhost:%d", cfg.ListenPort)
	handler := buildMux(winboxPath, cfg.TheiaOrigin, expectedHost, cfg.BridgeSecret)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ListenPort),
		Handler: handler,
	}
	m.server = srv
	m.port = cfg.ListenPort
	// Capture srv in the goroutine — not m.server — so Stop() setting m.server=nil
	// does not cause a nil dereference inside ListenAndServe.
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("winbox-bridge: server error: %v", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the server with a 5-second timeout.
// Returns nil and is a no-op if the server is not running.
func (m *ServerManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server == nil {
		return nil // not running — no-op
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := m.server.Shutdown(ctx)
	m.server = nil
	m.port = 0
	return err
}

// Running reports whether the server is currently started.
func (m *ServerManager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.server != nil
}

// Port returns the port the server is listening on (0 if not running).
func (m *ServerManager) Port() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server == nil {
		return 0
	}
	return m.port
}
