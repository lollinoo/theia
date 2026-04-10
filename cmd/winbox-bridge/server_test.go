package main

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

// freeCfg returns a Config whose ListenPort is a free TCP port.
// Uses net.Listen(":0") to let the OS pick an available port, then closes that
// listener so ServerManager can bind it.
func freeCfg(t *testing.T) Config {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("freeCfg: find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return Config{
		ListenPort:  port,
		TheiaOrigin: "http://localhost:3000",
		WinBoxPath:  "",
	}
}

// waitForServer polls the /health endpoint until it responds or timeout.
func waitForServer(t *testing.T, port int) {
	t.Helper()
	url := fmt.Sprintf("http://localhost:%d/health", port)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Origin", "http://localhost:3000")
		req.Host = fmt.Sprintf("localhost:%d", port)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("waitForServer: server on port %d did not become ready within 3s", port)
}

// --- ServerManager lifecycle tests ---

func TestServerManager_StartRunningTrue(t *testing.T) {
	cfg := freeCfg(t)
	mgr := &ServerManager{}

	if err := mgr.Start(cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })

	if !mgr.Running() {
		t.Error("expected Running()=true after Start()")
	}
}

func TestServerManager_StopRunningFalse(t *testing.T) {
	cfg := freeCfg(t)
	mgr := &ServerManager{}

	if err := mgr.Start(cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if mgr.Running() {
		t.Error("expected Running()=false after Stop()")
	}
}

func TestServerManager_StartAlreadyRunningIsNoop(t *testing.T) {
	cfg := freeCfg(t)
	mgr := &ServerManager{}

	if err := mgr.Start(cfg); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })

	// Second Start on same manager while running — must be a no-op (no error, still running)
	if err := mgr.Start(cfg); err != nil {
		t.Errorf("second Start should be no-op, got error: %v", err)
	}
	if !mgr.Running() {
		t.Error("expected Running()=true after second no-op Start()")
	}
}

func TestServerManager_StopWhenNotRunningIsNoop(t *testing.T) {
	mgr := &ServerManager{}
	// Stop before ever starting — must be a no-op
	if err := mgr.Stop(); err != nil {
		t.Errorf("Stop on idle manager should be no-op, got error: %v", err)
	}
}

func TestServerManager_StopThenStartSucceeds(t *testing.T) {
	cfg := freeCfg(t)
	mgr := &ServerManager{}

	if err := mgr.Start(cfg); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Re-bind on same port after Stop
	if err := mgr.Start(cfg); err != nil {
		t.Fatalf("re-Start: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })

	if !mgr.Running() {
		t.Error("expected Running()=true after re-Start()")
	}
}

func TestServerManager_RespondsToHealthAfterStart(t *testing.T) {
	cfg := freeCfg(t)
	mgr := &ServerManager{}

	if err := mgr.Start(cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })

	waitForServer(t, cfg.ListenPort)

	url := fmt.Sprintf("http://localhost:%d/health", cfg.ListenPort)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Origin", cfg.TheiaOrigin)
	req.Host = fmt.Sprintf("localhost:%d", cfg.ListenPort)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServerManager_RejectsRequestsAfterStop(t *testing.T) {
	cfg := freeCfg(t)
	mgr := &ServerManager{}

	if err := mgr.Start(cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForServer(t, cfg.ListenPort)

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Give the OS a moment to release the port
	time.Sleep(50 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d/health", cfg.ListenPort)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Origin", cfg.TheiaOrigin)
	req.Host = fmt.Sprintf("localhost:%d", cfg.ListenPort)

	_, err := http.DefaultClient.Do(req)
	if err == nil {
		t.Error("expected connection error after Stop(), got nil")
	}
}

func TestServerManager_HostCheckUsesDynamicPort(t *testing.T) {
	cfg := freeCfg(t)
	mgr := &ServerManager{}

	if err := mgr.Start(cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { mgr.Stop() })

	waitForServer(t, cfg.ListenPort)

	// Request with correct dynamic host → 200
	url := fmt.Sprintf("http://localhost:%d/health", cfg.ListenPort)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Origin", cfg.TheiaOrigin)
	req.Host = fmt.Sprintf("localhost:%d", cfg.ListenPort)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /health with correct host: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with correct host, got %d", resp.StatusCode)
	}
}
