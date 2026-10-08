package observability

import (
	"github.com/lollinoo/theia/internal/domain"
	"strings"
	"testing"
)

func TestRegistryDoesNotReportUnusedPersistenceBatchAsEffective(t *testing.T) {
	registry := NewRegistry()
	registry.SetRuntimeWorkerSettingsEffective([]RuntimeWorkerSettingEffective{
		{Setting: domain.SettingPollingPersistenceBatchMS, Value: 1500},
		{Setting: domain.SettingPollingEssentialWorkers, Value: 42},
	})
	metrics := string(registry.MarshalPrometheus())
	if strings.Contains(metrics, `setting="polling_persistence_batch_ms"`) {
		t.Fatalf("obsolete effective metric: %s", metrics)
	}
	if !strings.Contains(metrics, `theia_runtime_worker_setting_effective{setting="polling_essential_workers"} 42`) {
		t.Fatal("active setting missing from effective metrics")
	}
}
