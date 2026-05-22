package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// ServerManager owns the HTTP server lifecycle.
// Start and Stop can be called from any goroutine (mutex-protected).
// This satisfies T-29-02: mutex prevents concurrent Start/Stop races.
type ServerManager struct {
	mu       sync.Mutex
	server   *http.Server
	listener net.Listener
	port     int // current listening port, for status display
	cfg      Config
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
	return m.startLocked(cfg)
}

func (m *ServerManager) startLocked(cfg Config) error {
	srv, listener, err := buildServer(cfg, m)
	if err != nil {
		return err
	}
	m.assignLocked(cfg, srv, listener)
	return nil
}

func buildServer(cfg Config, mgr *ServerManager) (*http.Server, net.Listener, error) {
	// T-29-01: reject invalid ports before binding — clearer error than net.Listen's OS message
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		return nil, nil, fmt.Errorf("invalid port %d: must be 1-65535", cfg.ListenPort)
	}
	winboxPath := discoverWinBox(cfg.WinBoxPath)
	expectedHost := fmt.Sprintf("localhost:%d", cfg.ListenPort)
	client := &TheiaClient{
		BaseURL: cfg.TheiaBaseURL,
		Secret:  cfg.BridgeSecret,
	}
	handler := buildMuxWithSetup(winboxPath, cfg.TheiaOrigin, expectedHost, client, setupOptions{
		Restart: setupRestartFunc(mgr),
	})
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ListenPort),
		Handler: handler,
	}
	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen on port %d: %w", cfg.ListenPort, err)
	}
	return srv, listener, nil
}

func (m *ServerManager) assignLocked(cfg Config, srv *http.Server, listener net.Listener) {
	m.server = srv
	m.listener = listener
	m.port = cfg.ListenPort
	m.cfg = cfg
	// Capture srv in the goroutine — not m.server — so Stop() setting m.server=nil
	// does not cause a nil dereference inside Serve.
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("winbox-bridge: server error: %v", err)
		}
	}()
}

// Stop gracefully shuts down the server with a 5-second timeout.
// Returns nil and is a no-op if the server is not running.
func (m *ServerManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *ServerManager) stopLocked() error {
	if m.server == nil {
		return nil // not running — no-op
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if m.listener != nil {
		_ = m.listener.Close()
	}
	err := m.server.Shutdown(ctx)
	m.server = nil
	m.listener = nil
	m.port = 0
	return err
}

// Restart reloads the HTTP server with cfg while preserving the previous server
// if the new bind fails.
func (m *ServerManager) Restart(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server == nil {
		return m.startLocked(cfg)
	}

	oldCfg := m.cfg
	oldPort := m.port
	if cfg.ListenPort != oldPort {
		srv, listener, err := buildServer(cfg, m)
		if err != nil {
			return err
		}
		if err := m.stopLocked(); err != nil {
			_ = listener.Close()
			return err
		}
		m.assignLocked(cfg, srv, listener)
		return nil
	}

	if err := m.stopLocked(); err != nil {
		return err
	}
	if err := m.startLocked(cfg); err != nil {
		if restoreErr := m.startLocked(oldCfg); restoreErr != nil {
			return fmt.Errorf("restart failed: %w; restore failed: %v", err, restoreErr)
		}
		return err
	}
	return nil
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
