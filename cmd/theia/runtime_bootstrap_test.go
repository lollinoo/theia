package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

type stubRuntimeStopper struct {
	name  string
	stops *[]string
}

func (s stubRuntimeStopper) Stop() {
	*s.stops = append(*s.stops, s.name)
}

type stubRuntimeServer struct {
	listenErr error
}

func (s stubRuntimeServer) ListenAndServe() error {
	return s.listenErr
}

func (stubRuntimeServer) Shutdown(context.Context) error {
	return nil
}

func TestRuntimeBootstrapRunWrapsLoadConfigError(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	original := loadRuntimeConfig
	loadRuntimeConfig = func(path string) (*runtimeConfig, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { loadRuntimeConfig = original })

	err := bootstrap.Run("/tmp/theia.yaml")
	if err == nil {
		t.Fatal("Run() error = nil, want wrapped load config error")
	}
	if got, want := err.Error(), "load config: boom"; got != want {
		t.Fatalf("Run() error = %q, want %q", got, want)
	}
}

func TestRuntimeBootstrapStopRuntimeStopsChildrenInReverseOrder(t *testing.T) {
	bootstrap := &runtimeBootstrap{}
	var order []string
	children := runtimeChildren{
		stubRuntimeStopper{name: "pipeline", stops: &order},
		stubRuntimeStopper{name: "instance-backups", stops: &order},
		stubRuntimeStopper{name: "device-backups", stops: &order},
	}

	bootstrap.stopRuntime(children)

	if got, want := fmt.Sprint(order), "[device-backups instance-backups pipeline]"; got != want {
		t.Fatalf("stop order = %s, want %s", got, want)
	}
}

func TestRuntimeBootstrapServeTreatsServerClosedAsSuccess(t *testing.T) {
	bootstrap := &runtimeBootstrap{}

	if err := bootstrap.serve(stubRuntimeServer{listenErr: http.ErrServerClosed}); err != nil {
		t.Fatalf("serve() error = %v, want nil", err)
	}
	if err := bootstrap.serve(stubRuntimeServer{}); err != nil {
		t.Fatalf("serve() unexpected error = %v", err)
	}

	boom := errors.New("boom")
	if err := bootstrap.serve(stubRuntimeServer{listenErr: boom}); !reflect.DeepEqual(err, boom) {
		t.Fatalf("serve() error = %v, want %v", err, boom)
	}
}
