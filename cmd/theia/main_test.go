package main

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/service"
)

type stubBootstrapRunner struct {
	runCalls []string
	runErr   error
}

func (s *stubBootstrapRunner) Run(configPath string) error {
	s.runCalls = append(s.runCalls, configPath)
	return s.runErr
}

func TestRunMainDelegatesToBootstrapRunner(t *testing.T) {
	runner := &stubBootstrapRunner{}
	original := newBootstrapRunner
	newBootstrapRunner = func() bootstrapRunner {
		return runner
	}
	t.Cleanup(func() { newBootstrapRunner = original })

	err := runMain([]string{"-config", "/tmp/theia.yaml"})
	if err != nil {
		t.Fatalf("runMain() error = %v, want nil", err)
	}
	if !slices.Equal(runner.runCalls, []string{"/tmp/theia.yaml"}) {
		t.Fatalf("runner calls = %v, want [/tmp/theia.yaml]", runner.runCalls)
	}
}

type stubSettingsRepo struct {
	values map[string]string
}

func (r stubSettingsRepo) Get(key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", errors.New("setting not found")
}

func (r stubSettingsRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r stubSettingsRepo) GetAll() (map[string]string, error) {
	cloned := make(map[string]string, len(r.values))
	for key, value := range r.values {
		cloned[key] = value
	}
	return cloned, nil
}

type fakeCollectorSNMPClient struct{}

func (fakeCollectorSNMPClient) Connect() error { return nil }
func (fakeCollectorSNMPClient) Close() error   { return nil }
func (fakeCollectorSNMPClient) Get([]string) ([]gosnmp.SnmpPDU, error) {
	return nil, nil
}
func (fakeCollectorSNMPClient) BulkWalk(string) ([]gosnmp.SnmpPDU, error) {
	return nil, nil
}

func TestMainSNMPRuntimeHelpersRemainConstructibleAfterPipelineCutover(t *testing.T) {
	t.Run("uses caller timeout and retries when settings are invalid", func(t *testing.T) {
		var (
			gotTimeout time.Duration
			gotRetries int
		)

		original := newCollectorSNMPClient
		newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
			gotTimeout = timeout
			gotRetries = retries
			return fakeCollectorSNMPClient{}, nil
		}
		t.Cleanup(func() { newCollectorSNMPClient = original })

		factory := newCollectorSNMPClientFunc(stubSettingsRepo{
			values: map[string]string{
				domain.SettingSNMPTimeout: "bad-timeout",
				domain.SettingSNMPRetries: "bad-retries",
			},
		})
		client, err := factory("10.0.0.1", domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c: &domain.SNMPv2cCredentials{
				Community: "public",
			},
		}, 12*time.Second, 4)
		if err != nil {
			t.Fatalf("factory() error = %v", err)
		}
		if client == nil {
			t.Fatal("factory() returned nil client")
		}
		if gotTimeout != 12*time.Second {
			t.Fatalf("timeout = %v, want caller timeout 12s", gotTimeout)
		}
		if gotRetries != 4 {
			t.Fatalf("retries = %d, want caller retries 4", gotRetries)
		}
	})

	t.Run("defaults when caller inputs are invalid and settings are missing", func(t *testing.T) {
		var (
			gotTimeout time.Duration
			gotRetries int
		)

		original := newCollectorSNMPClient
		newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
			gotTimeout = timeout
			gotRetries = retries
			return fakeCollectorSNMPClient{}, nil
		}
		t.Cleanup(func() { newCollectorSNMPClient = original })

		factory := newCollectorSNMPClientFunc(stubSettingsRepo{
			values: map[string]string{
				domain.SettingSNMPTimeout: "-1",
				domain.SettingSNMPRetries: "nope",
			},
		})
		client, err := factory("10.0.0.1", domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c: &domain.SNMPv2cCredentials{
				Community: "public",
			},
		}, 0, -1)
		if err != nil {
			t.Fatalf("factory() error = %v", err)
		}
		if client == nil {
			t.Fatal("factory() returned nil client")
		}
		if gotTimeout != 10*time.Second {
			t.Fatalf("timeout = %v, want 10s fallback", gotTimeout)
		}
		if gotRetries != 2 {
			t.Fatalf("retries = %d, want 2 fallback", gotRetries)
		}
	})

	t.Run("preserves explicit caller timeout and retries over legacy settings", func(t *testing.T) {
		var (
			gotTimeout time.Duration
			gotRetries int
		)

		original := newCollectorSNMPClient
		newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
			gotTimeout = timeout
			gotRetries = retries
			return fakeCollectorSNMPClient{}, nil
		}
		t.Cleanup(func() { newCollectorSNMPClient = original })

		factory := newCollectorSNMPClientFunc(stubSettingsRepo{
			values: map[string]string{
				domain.SettingSNMPTimeout: "9",
				domain.SettingSNMPRetries: "3",
			},
		})
		client, err := factory("10.0.0.2", domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c: &domain.SNMPv2cCredentials{
				Community: "public",
			},
		}, 800*time.Millisecond, 0)
		if err != nil {
			t.Fatalf("factory() error = %v", err)
		}
		if client == nil {
			t.Fatal("factory() returned nil client")
		}
		if gotTimeout != 800*time.Millisecond {
			t.Fatalf("timeout = %v, want explicit caller timeout 800ms", gotTimeout)
		}
		if gotRetries != 0 {
			t.Fatalf("retries = %d, want explicit caller retries 0", gotRetries)
		}
	})

	t.Run("parses settings overrides when caller inputs are invalid", func(t *testing.T) {
		var (
			gotTimeout time.Duration
			gotRetries int
		)

		original := newCollectorSNMPClient
		newCollectorSNMPClient = func(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (collector.SNMPClient, error) {
			gotTimeout = timeout
			gotRetries = retries
			return fakeCollectorSNMPClient{}, nil
		}
		t.Cleanup(func() { newCollectorSNMPClient = original })

		factory := newCollectorSNMPClientFunc(stubSettingsRepo{
			values: map[string]string{
				domain.SettingSNMPTimeout: "9",
				domain.SettingSNMPRetries: "3",
			},
		})
		client, err := factory("10.0.0.2", domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c: &domain.SNMPv2cCredentials{
				Community: "public",
			},
		}, 0, -1)
		if err != nil {
			t.Fatalf("factory() error = %v", err)
		}
		if client == nil {
			t.Fatal("factory() returned nil client")
		}
		if gotTimeout != 9*time.Second {
			t.Fatalf("timeout = %v, want 9s", gotTimeout)
		}
		if gotRetries != 3 {
			t.Fatalf("retries = %d, want 3", gotRetries)
		}
	})
}

type stubDeviceSource struct{}

func (stubDeviceSource) GetDevices() ([]domain.Device, error) {
	return nil, nil
}

type stubRuntimeResetter struct{}

func (stubRuntimeResetter) ResetDeviceRuntime(uuid.UUID) {}

func TestWirePollRescheduler_AttachesSchedulerToDeviceService(t *testing.T) {
	deviceService := service.NewDeviceService(nil, nil, nil, nil, nil)
	sched := scheduler.NewScheduler(stubDeviceSource{}, nil)

	wirePollRescheduler(deviceService, sched)

	field := reflect.ValueOf(deviceService).Elem().FieldByName("pollRescheduler")
	if !field.IsValid() {
		t.Fatal("pollRescheduler field missing on DeviceService")
	}
	if field.IsNil() {
		t.Fatal("pollRescheduler field is nil after wirePollRescheduler")
	}

	attached := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	attachedScheduler, ok := attached.(*scheduler.Scheduler)
	if !ok {
		t.Fatalf("pollRescheduler concrete type = %T, want *scheduler.Scheduler", attached)
	}
	if attachedScheduler != sched {
		t.Fatalf("attached scheduler = %p, want %p", attachedScheduler, sched)
	}

	bootstrapField := reflect.ValueOf(deviceService).Elem().FieldByName("bootstrapScheduler")
	if !bootstrapField.IsValid() {
		t.Fatal("bootstrapScheduler field missing on DeviceService")
	}
	if bootstrapField.IsNil() {
		t.Fatal("bootstrapScheduler field is nil after wirePollRescheduler")
	}

	attachedBootstrap := reflect.NewAt(bootstrapField.Type(), unsafe.Pointer(bootstrapField.UnsafeAddr())).Elem().Interface()
	bootstrapScheduler, ok := attachedBootstrap.(*scheduler.Scheduler)
	if !ok {
		t.Fatalf("bootstrapScheduler concrete type = %T, want *scheduler.Scheduler", attachedBootstrap)
	}
	if bootstrapScheduler != sched {
		t.Fatalf("attached bootstrap scheduler = %p, want %p", bootstrapScheduler, sched)
	}
}

func TestWireRuntimeResetter_AttachesPipelineToDeviceService(t *testing.T) {
	deviceService := service.NewDeviceService(nil, nil, nil, nil, nil)
	resetter := &stubRuntimeResetter{}

	wireRuntimeResetter(deviceService, resetter)

	field := reflect.ValueOf(deviceService).Elem().FieldByName("runtimeResetter")
	if !field.IsValid() {
		t.Fatal("runtimeResetter field missing on DeviceService")
	}
	if field.IsNil() {
		t.Fatal("runtimeResetter field is nil after wireRuntimeResetter")
	}

	attached := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	attachedResetter, ok := attached.(*stubRuntimeResetter)
	if !ok {
		t.Fatalf("runtimeResetter concrete type = %T, want *stubRuntimeResetter", attached)
	}
	if attachedResetter != resetter {
		t.Fatalf("attached runtime resetter = %p, want %p", attachedResetter, resetter)
	}
}
