package observability

// This file defines registry observability registry behavior.

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

const runtimeGCCPUFractionMetricName = "go_memstats_gc_cpu_fraction"

var (
	defaultRegistryMu sync.RWMutex
	defaultRegistry   = NewRegistry()
)

var runtimeMetricsGatherer = newRuntimeMetricsGatherer()

var (
	durationBucketsSeconds = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120}
	payloadBucketsBytes    = []float64{128, 512, 1024, 4096, 16384, 65536, 262144, 1048576}
	runtimeVersionBuckets  = []float64{0, 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}
)

type taskResultKey struct {
	VolatilityClass string
	Outcome         string
}

type deviceProtocolKey struct {
	DeviceID string
	Protocol string
}

type linkUpsertKey struct {
	Protocol string
	Result   string
}

type staticCollectionSkipKey struct {
	Operation string
	Reason    string
}

type schedulerBackpressureKey struct {
	VolatilityClass string
	Reason          string
}

type schedulerScopedBackpressureKey struct {
	TaskKind        string
	VolatilityClass string
	Reason          string
	Scope           string
	ScopeID         string
	ScopeName       string
}

type schedulerDispatchKey struct {
	TaskKind        string
	VolatilityClass string
}

type bulkOperationRejectionKey struct {
	Operation string
	Reason    string
	Source    string
}

type bulkOperationInFlightKey struct {
	Operation string
	Source    string
}

type bulkOperationLimitKey struct {
	Operation string
	Scope     string
	Source    string
}

type bulkOperationCompletionKey struct {
	Operation string
	Source    string
	Result    string
}

type wsMetricKey struct {
	Scope string
	Type  string
}

type wsBackpressureKey struct {
	Scope  string
	Reason string
}

type wsClientResyncKey struct {
	Scope     string
	Reason    string
	Bootstrap string
}

type wsRuntimeRecoveryKey struct {
	Mode    string
	Reason  string
	Outcome string
}

type wsRuntimeRecoveryDurationKey struct {
	Mode    string
	Outcome string
}

type refreshSnapshotBuildKey struct {
	Mode   string
	Result string
}

type prometheusRuntimeRequestKey struct {
	Operation string
	Result    string
}

type snmpCollectorOperationKey struct {
	Collector string
	Operation string
	Result    string
}

type snmpCollectorDeviceOperationKey struct {
	DeviceID  string
	Device    string
	Target    string
	Collector string
	Operation string
	Result    string
}

type snmpCollectorEarlyExitKey struct {
	Collector string
	Reason    string
}

type histogram struct {
	buckets []float64
	counts  []uint64
	count   uint64
	sum     float64
}

type histogramSnapshot struct {
	buckets []float64
	counts  []uint64
	count   uint64
	sum     float64
}

// Registry represents registry data used by the package.
type Registry struct {
	mu sync.RWMutex

	schedulerReadyDepth               map[domain.VolatilityClass]float64
	schedulerQueueLagSeconds          map[domain.VolatilityClass]float64
	schedulerInFlight                 float64
	schedulerTaskDispatchTotal        map[schedulerDispatchKey]uint64
	schedulerBackpressureTotal        map[schedulerBackpressureKey]uint64
	schedulerScopedBackpressureTotal  map[schedulerScopedBackpressureKey]uint64
	schedulerTaskDuration             map[domain.VolatilityClass]*histogram
	bulkOperationInFlight             map[bulkOperationInFlightKey]float64
	bulkOperationLimits               map[bulkOperationLimitKey]float64
	bulkOperationRejections           map[bulkOperationRejectionKey]uint64
	bulkOperationCompletions          map[bulkOperationCompletionKey]uint64
	bulkOperationDuration             map[bulkOperationCompletionKey]*histogram
	bulkOperationDevices              map[bulkOperationCompletionKey]uint64
	bulkOperationFiles                map[bulkOperationCompletionKey]uint64
	bulkOperationBytes                map[bulkOperationCompletionKey]uint64
	pollingEssentialOverloaded        float64
	pollingDeadlineMissTotal          uint64
	runtimeWorkerSettings             map[string]float64
	pollResultsTotal                  map[taskResultKey]uint64
	discoveryNeighbors                map[deviceProtocolKey]float64
	linkUpsertsTotal                  map[linkUpsertKey]uint64
	cacheInvalidationsTotal           map[string]uint64
	cacheReloadTotal                  uint64
	topologyMaterialization           map[string]*histogram
	topologyMaterializationSkipsTotal map[string]uint64
	staticPersistenceSkipsTotal       map[string]uint64
	staticCollectionSkipsTotal        map[staticCollectionSkipKey]uint64
	refreshSnapshotBuild              map[refreshSnapshotBuildKey]*histogram
	refreshTopologyReloadTotal        map[string]uint64
	prometheusRuntimeRequests         map[prometheusRuntimeRequestKey]uint64
	prometheusRuntimeDuration         map[prometheusRuntimeRequestKey]*histogram
	snmpCollectorOperations           map[snmpCollectorOperationKey]uint64
	snmpCollectorDuration             map[snmpCollectorOperationKey]*histogram
	snmpCollectorDeviceLast           map[snmpCollectorDeviceOperationKey]float64
	snmpCollectorDeviceSlow           map[snmpCollectorDeviceOperationKey]uint64
	snmpCollectorEarlyExit            map[snmpCollectorEarlyExitKey]uint64
	wsConnectedClients                float64
	wsConnectionsTotal                map[string]uint64
	wsMessagesTotal                   map[wsMetricKey]uint64
	wsBackpressureTotal               map[wsBackpressureKey]uint64
	wsClientResyncTotal               map[wsClientResyncKey]uint64
	wsOverviewMailboxClear            map[string]uint64
	wsOverviewResyncSuppressed        map[string]uint64
	wsPayloadBytes                    map[wsMetricKey]*histogram
	wsRuntimeRecoveryTotal            map[wsRuntimeRecoveryKey]uint64
	wsRuntimeRecoveryDuration         map[wsRuntimeRecoveryDurationKey]*histogram
	wsRuntimeAckLag                   *histogram
	wsRuntimeReplayVersions           *histogram
	unknownNeighborsTotal             map[deviceProtocolKey]uint64
	stateChangesDroppedTotal          uint64
}

// NewRegistry constructs registry state for the package.
func NewRegistry() *Registry {
	return &Registry{
		schedulerReadyDepth: map[domain.VolatilityClass]float64{
			domain.VolatilityClassPerformance: 0,
			domain.VolatilityClassOperational: 0,
			domain.VolatilityClassStatic:      0,
		},
		schedulerQueueLagSeconds: map[domain.VolatilityClass]float64{
			domain.VolatilityClassPerformance: 0,
			domain.VolatilityClassOperational: 0,
			domain.VolatilityClassStatic:      0,
		},
		schedulerTaskDispatchTotal:       make(map[schedulerDispatchKey]uint64),
		schedulerBackpressureTotal:       make(map[schedulerBackpressureKey]uint64),
		schedulerScopedBackpressureTotal: make(map[schedulerScopedBackpressureKey]uint64),
		bulkOperationInFlight:            make(map[bulkOperationInFlightKey]float64),
		bulkOperationLimits:              make(map[bulkOperationLimitKey]float64),
		bulkOperationRejections:          make(map[bulkOperationRejectionKey]uint64),
		bulkOperationCompletions:         make(map[bulkOperationCompletionKey]uint64),
		bulkOperationDuration:            make(map[bulkOperationCompletionKey]*histogram),
		bulkOperationDevices:             make(map[bulkOperationCompletionKey]uint64),
		bulkOperationFiles:               make(map[bulkOperationCompletionKey]uint64),
		bulkOperationBytes:               make(map[bulkOperationCompletionKey]uint64),
		schedulerTaskDuration: map[domain.VolatilityClass]*histogram{
			domain.VolatilityClassPerformance: newHistogram(durationBucketsSeconds),
			domain.VolatilityClassOperational: newHistogram(durationBucketsSeconds),
			domain.VolatilityClassStatic:      newHistogram(durationBucketsSeconds),
		},
		runtimeWorkerSettings:             make(map[string]float64),
		pollResultsTotal:                  make(map[taskResultKey]uint64),
		discoveryNeighbors:                make(map[deviceProtocolKey]float64),
		linkUpsertsTotal:                  make(map[linkUpsertKey]uint64),
		cacheInvalidationsTotal:           make(map[string]uint64),
		staticPersistenceSkipsTotal:       make(map[string]uint64),
		topologyMaterializationSkipsTotal: make(map[string]uint64),
		staticCollectionSkipsTotal:        make(map[staticCollectionSkipKey]uint64),
		topologyMaterialization: map[string]*histogram{
			"success": newHistogram(durationBucketsSeconds),
			"error":   newHistogram(durationBucketsSeconds),
		},
		refreshSnapshotBuild: map[refreshSnapshotBuildKey]*histogram{
			{Mode: "dirty", Result: "error"}:   newHistogram(durationBucketsSeconds),
			{Mode: "dirty", Result: "success"}: newHistogram(durationBucketsSeconds),
			{Mode: "full", Result: "error"}:    newHistogram(durationBucketsSeconds),
			{Mode: "full", Result: "success"}:  newHistogram(durationBucketsSeconds),
		},
		refreshTopologyReloadTotal: make(map[string]uint64),
		prometheusRuntimeRequests:  make(map[prometheusRuntimeRequestKey]uint64),
		prometheusRuntimeDuration:  make(map[prometheusRuntimeRequestKey]*histogram),
		snmpCollectorOperations:    make(map[snmpCollectorOperationKey]uint64),
		snmpCollectorDuration:      make(map[snmpCollectorOperationKey]*histogram),
		snmpCollectorDeviceLast:    make(map[snmpCollectorDeviceOperationKey]float64),
		snmpCollectorDeviceSlow:    make(map[snmpCollectorDeviceOperationKey]uint64),
		snmpCollectorEarlyExit:     make(map[snmpCollectorEarlyExitKey]uint64),
		wsConnectionsTotal:         make(map[string]uint64),
		wsMessagesTotal:            make(map[wsMetricKey]uint64),
		wsBackpressureTotal:        make(map[wsBackpressureKey]uint64),
		wsClientResyncTotal:        make(map[wsClientResyncKey]uint64),
		wsOverviewMailboxClear:     make(map[string]uint64),
		wsOverviewResyncSuppressed: make(map[string]uint64),
		wsPayloadBytes:             make(map[wsMetricKey]*histogram),
		wsRuntimeRecoveryTotal:     make(map[wsRuntimeRecoveryKey]uint64),
		wsRuntimeRecoveryDuration:  make(map[wsRuntimeRecoveryDurationKey]*histogram),
		wsRuntimeAckLag:            newHistogram(runtimeVersionBuckets),
		wsRuntimeReplayVersions:    newHistogram(runtimeVersionBuckets),
		unknownNeighborsTotal:      make(map[deviceProtocolKey]uint64),
	}
}

func Default() *Registry {
	defaultRegistryMu.RLock()
	defer defaultRegistryMu.RUnlock()
	return defaultRegistry
}

func ResetDefaultForTest() *Registry {
	defaultRegistryMu.Lock()
	defer defaultRegistryMu.Unlock()
	defaultRegistry = NewRegistry()
	return defaultRegistry
}

func Handler() http.Handler {
	return metricsHandler{
		registry:        Default(),
		runtimeGatherer: runtimeMetricsGatherer,
	}
}

type metricsHandler struct {
	registry        *Registry
	runtimeGatherer prometheus.Gatherer
}

func (h metricsHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	runtimeMetrics, err := marshalRuntimeMetrics(h.runtimeGatherer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write(runtimeMetrics)
	_, _ = w.Write(h.registry.MarshalPrometheus())
}

func newRuntimeMetricsGatherer() prometheus.Gatherer {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	return registry
}

func marshalRuntimeMetrics(gatherer prometheus.Gatherer) ([]byte, error) {
	families, err := gatherer.Gather()
	if err != nil {
		return nil, fmt.Errorf("gather runtime metrics: %w", err)
	}

	var b strings.Builder
	hasGCCPUFraction := false
	for _, family := range families {
		if family.GetName() == runtimeGCCPUFractionMetricName {
			hasGCCPUFraction = true
		}
		if _, err := expfmt.MetricFamilyToText(&b, family); err != nil {
			return nil, fmt.Errorf("encode runtime metric %s: %w", family.GetName(), err)
		}
	}
	if !hasGCCPUFraction {
		writeRuntimeGCCPUFractionMetric(&b)
	}
	return []byte(b.String()), nil
}

func writeRuntimeGCCPUFractionMetric(b *strings.Builder) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	writeGaugeSingle(
		b,
		runtimeGCCPUFractionMetricName,
		"The fraction of this program's available CPU time used by the GC since the program started.",
		stats.GCCPUFraction,
	)
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write(r.MarshalPrometheus())
}

func (r *Registry) MarshalPrometheus() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var b strings.Builder

	writeGaugeVec(&b,
		"theia_scheduler_ready_queue_depth",
		"Current ready queue depth by volatility class.",
		sortedVolatilityGaugeRows(r.schedulerReadyDepth),
	)
	writeGaugeSingle(&b,
		"theia_scheduler_in_flight_tasks",
		"Current number of scheduler tasks in flight.",
		r.schedulerInFlight,
	)
	writeGaugeVec(&b,
		"theia_scheduler_queue_lag_seconds",
		"Current overdue queue lag by volatility class.",
		sortedVolatilityGaugeRows(r.schedulerQueueLagSeconds),
	)
	writeCounterVec(&b,
		"theia_scheduler_task_dispatch_total",
		"Total scheduled tasks dispatched by task kind and volatility class.",
		sortedDispatchRows(r.schedulerTaskDispatchTotal),
	)
	writeCounterVec(&b,
		"theia_scheduler_backpressure_total",
		"Scheduler backpressure events by volatility class and reason.",
		sortedSchedulerBackpressureRows(r.schedulerBackpressureTotal),
	)
	writeCounterVec(&b,
		"theia_scheduler_scoped_backpressure_total",
		"Scheduler backpressure events by task kind, volatility class, reason, and isolation scope.",
		sortedSchedulerScopedBackpressureRows(r.schedulerScopedBackpressureTotal),
	)
	writeGaugeVec(&b,
		"theia_runtime_worker_setting_effective",
		"Effective runtime worker and polling tuning settings after defaults and bounds are applied.",
		sortedRuntimeWorkerSettingRows(r.runtimeWorkerSettings),
	)
	writeCounterVec(&b,
		"theia_bulk_operation_rejections_total",
		"Bulk operation request rejections by operation, reason, and source.",
		sortedBulkOperationRejectionRows(r.bulkOperationRejections),
	)
	writeGaugeVec(&b,
		"theia_bulk_operation_in_flight",
		"Current in-flight bulk operations by operation and source.",
		sortedBulkOperationInFlightRows(r.bulkOperationInFlight),
	)
	writeGaugeVec(&b,
		"theia_bulk_operation_concurrency_limit",
		"Configured bulk operation concurrency limits by operation, scope, and source.",
		sortedBulkOperationLimitRows(r.bulkOperationLimits),
	)
	writeCounterVec(&b,
		"theia_bulk_operation_completions_total",
		"Bulk operation completions by operation, result, and source.",
		sortedBulkOperationCompletionCounterRows(r.bulkOperationCompletions),
	)
	writeHistogramVec(&b,
		"theia_bulk_operation_duration_seconds",
		"Bulk operation completion duration by operation, result, and source.",
		sortedBulkOperationCompletionHistogramRows(r.bulkOperationDuration),
	)
	writeCounterVec(&b,
		"theia_bulk_operation_selected_devices_total",
		"Bulk operation selected device totals by operation, result, and source.",
		sortedBulkOperationCompletionCounterRows(r.bulkOperationDevices),
	)
	writeCounterVec(&b,
		"theia_bulk_operation_selected_files_total",
		"Bulk operation selected file totals by operation, result, and source.",
		sortedBulkOperationCompletionCounterRows(r.bulkOperationFiles),
	)
	writeCounterVec(&b,
		"theia_bulk_operation_selected_bytes_total",
		"Bulk operation selected byte totals by operation, result, and source.",
		sortedBulkOperationCompletionCounterRows(r.bulkOperationBytes),
	)
	writeHistogramVec(&b,
		"theia_scheduler_task_duration_seconds",
		"Task completion latency by volatility class.",
		sortedVolatilityHistogramRows(r.schedulerTaskDuration),
	)
	writeGaugeSingle(&b,
		"theia_polling_essential_overloaded",
		"Whether the essential lane is overloaded.",
		r.pollingEssentialOverloaded,
	)
	writeCounterSingle(&b,
		"theia_polling_deadline_miss_total",
		"Total essential polling deadline misses.",
		r.pollingDeadlineMissTotal,
	)
	writeCounterVec(&b,
		"theia_poll_results_total",
		"Poll success and failure totals by volatility class.",
		sortedTaskResultRows(r.pollResultsTotal),
	)
	writeGaugeVec(&b,
		"theia_discovery_neighbors",
		"Current discovered neighbor count by device and protocol.",
		sortedDiscoveryNeighborRows(r.discoveryNeighbors),
	)
	writeCounterVec(&b,
		"theia_link_upserts_total",
		"Link upsert totals by discovery protocol and result.",
		sortedLinkUpsertRows(r.linkUpsertsTotal),
	)
	writeCounterVec(&b,
		"theia_cache_invalidation_total",
		"Successful device/link cache invalidation signals emitted by source.",
		sortedStringCounterRows("source", r.cacheInvalidationsTotal),
	)
	writeCounterSingle(&b,
		"theia_cache_reload_total",
		"Total full device/link cache reloads.",
		r.cacheReloadTotal,
	)
	writeHistogramVec(&b,
		"theia_topology_materialization_seconds",
		"Static discovery materialization latency.",
		sortedStringHistogramRows("result", r.topologyMaterialization),
	)
	writeCounterVec(&b,
		"theia_topology_materialization_skips_total",
		"Topology materialization skips by reason.",
		sortedStringCounterRows("reason", r.topologyMaterializationSkipsTotal),
	)
	writeCounterVec(&b,
		"theia_static_persistence_skips_total",
		"Static discovery persistence skips by reason.",
		sortedStringCounterRows("reason", r.staticPersistenceSkipsTotal),
	)
	writeCounterVec(&b,
		"theia_static_collection_skips_total",
		"Static discovery collection skips by operation and reason.",
		sortedStaticCollectionSkipRows(r.staticCollectionSkipsTotal),
	)
	writeHistogramVec(&b,
		"theia_refresh_snapshot_build_seconds",
		"Refresh snapshot build latency by build mode and result.",
		sortedRefreshSnapshotBuildRows(r.refreshSnapshotBuild),
	)
	writeCounterVec(&b,
		"theia_refresh_topology_reload_total",
		"Full topology reload decisions by reason.",
		sortedStringCounterRows("reason", r.refreshTopologyReloadTotal),
	)
	writeCounterVec(&b,
		"theia_prometheus_runtime_requests_total",
		"Prometheus runtime requests by operation and result.",
		sortedPrometheusRuntimeCounterRows(r.prometheusRuntimeRequests),
	)
	writeHistogramVec(&b,
		"theia_prometheus_runtime_request_seconds",
		"Prometheus runtime request latency by operation and result.",
		sortedPrometheusRuntimeHistogramRows(r.prometheusRuntimeDuration),
	)
	writeCounterVec(&b,
		"theia_snmp_collector_operations_total",
		"SNMP collector operation totals by collector, operation, and result.",
		sortedSNMPCollectorOperationCounterRows(r.snmpCollectorOperations),
	)
	writeHistogramVec(&b,
		"theia_snmp_collector_operation_seconds",
		"SNMP collector operation latency by collector, operation, and result.",
		sortedSNMPCollectorOperationHistogramRows(r.snmpCollectorDuration),
	)
	writeGaugeVec(&b,
		"theia_snmp_collector_device_operation_last_duration_seconds",
		"Most recent SNMP collector operation latency by device, collector, operation, and result.",
		sortedSNMPCollectorDeviceOperationGaugeRows(r.snmpCollectorDeviceLast),
	)
	writeCounterVec(&b,
		"theia_snmp_collector_device_slow_operations_total",
		"Slow SNMP collector operation totals by device, collector, operation, and result.",
		sortedSNMPCollectorDeviceOperationCounterRows(r.snmpCollectorDeviceSlow),
	)
	writeCounterVec(&b,
		"theia_snmp_collector_early_exit_total",
		"SNMP collector early exits by collector and reason.",
		sortedSNMPCollectorEarlyExitRows(r.snmpCollectorEarlyExit),
	)
	writeCounterVec(&b,
		"theia_ws_messages_total",
		"WebSocket messages emitted by scope and type.",
		sortedWSRows(r.wsMessagesTotal),
	)
	writeCounterVec(&b,
		"theia_ws_connections_total",
		"WebSocket connection lifecycle events by event.",
		sortedStringCounterRows("event", r.wsConnectionsTotal),
	)
	writeCounterVec(&b,
		"theia_ws_backpressure_total",
		"WebSocket backpressure events by scope and reason.",
		sortedWSBackpressureRows(r.wsBackpressureTotal),
	)
	writeCounterVec(&b,
		"theia_ws_client_resync_required_total",
		"WebSocket resync markers emitted by scope, reason, and client bootstrap mode.",
		sortedWSClientResyncRows(r.wsClientResyncTotal),
	)
	writeCounterVec(&b,
		"theia_ws_overview_mailbox_clear_total",
		"Overview mailbox messages dropped while replacing stale backlog by reason.",
		sortedStringCounterRows("reason", r.wsOverviewMailboxClear),
	)
	writeCounterVec(&b,
		"theia_ws_overview_resync_suppressed_total",
		"Overview resync markers suppressed because one is already pending by reason.",
		sortedStringCounterRows("reason", r.wsOverviewResyncSuppressed),
	)
	writeGaugeSingle(&b,
		"theia_ws_connected_clients",
		"Current connected WebSocket clients.",
		r.wsConnectedClients,
	)
	writeHistogramVec(&b,
		"theia_ws_message_payload_bytes",
		"WebSocket payload sizes emitted by scope and type.",
		sortedWSHistogramRows(r.wsPayloadBytes),
	)
	writeCounterVec(&b,
		"theia_ws_runtime_recovery_total",
		"Runtime recovery attempts by bounded mode, reason, and outcome.",
		sortedWSRuntimeRecoveryRows(r.wsRuntimeRecoveryTotal),
	)
	writeHistogramVec(&b,
		"theia_ws_runtime_recovery_duration_seconds",
		"Runtime recovery completion latency by bounded mode and outcome.",
		sortedWSRuntimeRecoveryDurationRows(r.wsRuntimeRecoveryDuration),
	)
	writeHistogramVec(&b,
		"theia_ws_runtime_ack_lag_versions",
		"Runtime cursor lag in versions observed on valid client acknowledgements.",
		[]histogramRow{{value: r.wsRuntimeAckLag.snapshot()}},
	)
	writeHistogramVec(&b,
		"theia_ws_runtime_replay_versions",
		"Runtime version span carried by installed replay recoveries.",
		[]histogramRow{{value: r.wsRuntimeReplayVersions.snapshot()}},
	)
	writeCounterVec(&b,
		"theia_unknown_neighbors_total",
		"Unknown neighbor observations by local device and protocol.",
		sortedUnknownNeighborRows(r.unknownNeighborsTotal),
	)
	writeCounterSingle(&b,
		"theia_state_changes_dropped_total",
		"Dropped state-store change batches when the non-blocking channel is full.",
		r.stateChangesDroppedTotal,
	)

	return []byte(b.String())
}

func (r *Registry) SetSchedulerReadyDepth(volatility domain.VolatilityClass, depth int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedulerReadyDepth[volatility] = float64(depth)
}

func (r *Registry) SetSchedulerInFlight(count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedulerInFlight = float64(count)
}

func (r *Registry) SetSchedulerQueueLag(volatility domain.VolatilityClass, lag time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if lag < 0 {
		lag = 0
	}
	r.schedulerQueueLagSeconds[volatility] = lag.Seconds()
}

func (r *Registry) IncSchedulerTaskDispatch(volatility domain.VolatilityClass) {
	r.IncSchedulerTaskDispatchForTask("unknown", volatility)
}

func (r *Registry) IncSchedulerTaskDispatchForTask(taskKind string, volatility domain.VolatilityClass) {
	taskKind = strings.TrimSpace(taskKind)
	if taskKind == "" {
		taskKind = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedulerTaskDispatchTotal[schedulerDispatchKey{
		TaskKind:        taskKind,
		VolatilityClass: string(volatility),
	}]++
}

func (r *Registry) IncSchedulerBackpressure(volatility domain.VolatilityClass, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedulerBackpressureTotal[schedulerBackpressureKey{
		VolatilityClass: string(volatility),
		Reason:          reason,
	}]++
}

func (r *Registry) IncSchedulerScopedBackpressure(taskKind string, volatility domain.VolatilityClass, reason, scope, scopeID, scopeName string) {
	taskKind = strings.TrimSpace(taskKind)
	if taskKind == "" {
		taskKind = "unknown"
	}
	reason = strings.TrimSpace(reason)
	scope = strings.TrimSpace(scope)
	scopeID = strings.TrimSpace(scopeID)
	scopeName = strings.TrimSpace(scopeName)
	if reason == "" || scope == "" || scopeID == "" {
		return
	}
	if scopeName == "" {
		scopeName = scopeID
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedulerScopedBackpressureTotal[schedulerScopedBackpressureKey{
		TaskKind:        taskKind,
		VolatilityClass: string(volatility),
		Reason:          reason,
		Scope:           scope,
		ScopeID:         scopeID,
		ScopeName:       scopeName,
	}]++
}

func (r *Registry) IncBulkOperationRejection(operation, reason, source string) {
	if operation == "" || reason == "" || source == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.bulkOperationRejections[bulkOperationRejectionKey{
		Operation: operation,
		Reason:    reason,
		Source:    source,
	}]++
}

func (r *Registry) SetBulkOperationInFlight(operation, source string, count int) {
	if operation == "" || source == "" {
		return
	}
	if count < 0 {
		count = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.bulkOperationInFlight[bulkOperationInFlightKey{
		Operation: operation,
		Source:    source,
	}] = float64(count)
}

func (r *Registry) SetBulkOperationConcurrencyLimit(operation, scope, source string, limit int) {
	if operation == "" || scope == "" || source == "" {
		return
	}
	if limit < 0 {
		limit = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.bulkOperationLimits[bulkOperationLimitKey{
		Operation: operation,
		Scope:     scope,
		Source:    source,
	}] = float64(limit)
}

func (r *Registry) ObserveBulkOperationCompletion(operation, source, result string, duration time.Duration, selectedDevices, selectedFiles int, selectedBytes int64) {
	if operation == "" || source == "" || result == "" {
		return
	}
	if duration < 0 {
		duration = 0
	}
	if selectedDevices < 0 {
		selectedDevices = 0
	}
	if selectedFiles < 0 {
		selectedFiles = 0
	}
	if selectedBytes < 0 {
		selectedBytes = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	key := bulkOperationCompletionKey{
		Operation: operation,
		Source:    source,
		Result:    result,
	}
	r.bulkOperationCompletions[key]++
	h, ok := r.bulkOperationDuration[key]
	if !ok {
		h = newHistogram(durationBucketsSeconds)
		r.bulkOperationDuration[key] = h
	}
	h.observe(duration.Seconds())
	if selectedDevices > 0 {
		r.bulkOperationDevices[key] += uint64(selectedDevices)
	}
	if selectedFiles > 0 {
		r.bulkOperationFiles[key] += uint64(selectedFiles)
	}
	if selectedBytes > 0 {
		r.bulkOperationBytes[key] += uint64(selectedBytes)
	}
}

func (r *Registry) ObserveSchedulerTaskDuration(volatility domain.VolatilityClass, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.schedulerTaskDuration[volatility]
	if !ok {
		h = newHistogram(durationBucketsSeconds)
		r.schedulerTaskDuration[volatility] = h
	}
	h.observe(duration.Seconds())
}

func (r *Registry) SetPollingEssentialOverloaded(overloaded bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if overloaded {
		r.pollingEssentialOverloaded = 1
		return
	}
	r.pollingEssentialOverloaded = 0
}

func (r *Registry) SetRuntimeWorkerSettingEffective(setting string, value float64) {
	if !runtimeWorkerSettingAllowed(setting) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.runtimeWorkerSettings[setting] = value
}

func (r *Registry) SetRuntimeWorkerSettingsEffective(settings []RuntimeWorkerSettingEffective) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, setting := range settings {
		if !runtimeWorkerSettingAllowed(setting.Setting) {
			continue
		}
		r.runtimeWorkerSettings[setting.Setting] = setting.Value
	}
}

func (r *Registry) IncPollingDeadlineMiss() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pollingDeadlineMissTotal++
}

func (r *Registry) IncPollResult(volatility domain.VolatilityClass, success bool) {
	outcome := "failure"
	if success {
		outcome = "success"
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.pollResultsTotal[taskResultKey{
		VolatilityClass: string(volatility),
		Outcome:         outcome,
	}]++
}

func (r *Registry) SetDiscoveryNeighborCounts(deviceID uuid.UUID, counts map[domain.DiscoveryProtocol]int) {
	device := deviceID.String()

	r.mu.Lock()
	defer r.mu.Unlock()

	for key := range r.discoveryNeighbors {
		if key.DeviceID == device {
			delete(r.discoveryNeighbors, key)
		}
	}
	for protocol, count := range counts {
		r.discoveryNeighbors[deviceProtocolKey{
			DeviceID: device,
			Protocol: string(protocol),
		}] = float64(count)
	}
}

func (r *Registry) IncLinkUpsert(protocol domain.DiscoveryProtocol, result domain.LinkUpsertKind) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.linkUpsertsTotal[linkUpsertKey{
		Protocol: string(protocol),
		Result:   string(result),
	}]++
}

func (r *Registry) IncCacheInvalidation(source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cacheInvalidationsTotal[source]++
}

func (r *Registry) IncCacheReload() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cacheReloadTotal++
}

func (r *Registry) IncStaticPersistenceSkip(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.staticPersistenceSkipsTotal[reason]++
}

func (r *Registry) IncTopologyMaterializationSkip(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.topologyMaterializationSkipsTotal[reason]++
}

func (r *Registry) IncStaticCollectionSkip(operation, reason string) {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "unknown"
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.staticCollectionSkipsTotal[staticCollectionSkipKey{
		Operation: operation,
		Reason:    reason,
	}]++
}

func (r *Registry) ObserveTopologyMaterialization(duration time.Duration, success bool) {
	result := "error"
	if success {
		result = "success"
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.topologyMaterialization[result]
	if !ok {
		h = newHistogram(durationBucketsSeconds)
		r.topologyMaterialization[result] = h
	}
	h.observe(duration.Seconds())
}

func (r *Registry) ObserveRefreshSnapshotBuild(mode string, duration time.Duration, success bool) {
	result := "error"
	if success {
		result = "success"
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	key := refreshSnapshotBuildKey{Mode: mode, Result: result}
	h, ok := r.refreshSnapshotBuild[key]
	if !ok {
		h = newHistogram(durationBucketsSeconds)
		r.refreshSnapshotBuild[key] = h
	}
	h.observe(duration.Seconds())
}

func (r *Registry) IncRefreshTopologyReload(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshTopologyReloadTotal[reason]++
}

func (r *Registry) ObservePrometheusRuntimeRequest(operation, result string, duration time.Duration) {
	if operation == "" || result == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := prometheusRuntimeRequestKey{
		Operation: operation,
		Result:    result,
	}
	r.prometheusRuntimeRequests[key]++
	h, ok := r.prometheusRuntimeDuration[key]
	if !ok {
		h = newHistogram(durationBucketsSeconds)
		r.prometheusRuntimeDuration[key] = h
	}
	h.observe(duration.Seconds())
}

func (r *Registry) ObserveSNMPCollectorOperation(collector, operation, result string, duration time.Duration) {
	if collector == "" || operation == "" || result == "" {
		return
	}
	if duration < 0 {
		duration = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := snmpCollectorOperationKey{
		Collector: collector,
		Operation: operation,
		Result:    result,
	}
	r.snmpCollectorOperations[key]++
	h, ok := r.snmpCollectorDuration[key]
	if !ok {
		h = newHistogram(durationBucketsSeconds)
		r.snmpCollectorDuration[key] = h
	}
	h.observe(duration.Seconds())
}

// ObserveSNMPCollectorDeviceOperation records the latest per-device SNMP
// operation latency and optionally increments the slow-operation counter.
func (r *Registry) ObserveSNMPCollectorDeviceOperation(deviceID, device, target, collector, operation, result string, duration time.Duration, slow bool) {
	if collector == "" || operation == "" || result == "" {
		return
	}
	if duration < 0 {
		duration = 0
	}
	if deviceID == "" {
		deviceID = "unknown"
	}
	if device == "" {
		device = "unknown"
	}
	if target == "" {
		target = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := snmpCollectorDeviceOperationKey{
		DeviceID:  deviceID,
		Device:    device,
		Target:    target,
		Collector: collector,
		Operation: operation,
		Result:    result,
	}
	r.snmpCollectorDeviceLast[key] = duration.Seconds()
	if slow {
		r.snmpCollectorDeviceSlow[key]++
	}
}

func (r *Registry) IncSNMPCollectorEarlyExit(collector, reason string) {
	if collector == "" || reason == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.snmpCollectorEarlyExit[snmpCollectorEarlyExitKey{
		Collector: collector,
		Reason:    reason,
	}]++
}

func (r *Registry) ObserveWSMessage(scope, messageType string, payloadBytes int) {
	key := wsMetricKey{Scope: scope, Type: messageType}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsMessagesTotal[key]++
	h, ok := r.wsPayloadBytes[key]
	if !ok {
		h = newHistogram(payloadBucketsBytes)
		r.wsPayloadBytes[key] = h
	}
	h.observe(float64(payloadBytes))
}

func (r *Registry) IncWSBackpressure(scope, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsBackpressureTotal[wsBackpressureKey{
		Scope:  scope,
		Reason: reason,
	}]++
}

func (r *Registry) IncWSClientResyncRequired(scope, reason, bootstrap string) {
	if scope == "" || reason == "" || bootstrap == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsClientResyncTotal[wsClientResyncKey{
		Scope:     scope,
		Reason:    reason,
		Bootstrap: bootstrap,
	}]++
}

func (r *Registry) AddWSOverviewMailboxCleared(reason string, count int) {
	if reason == "" || count <= 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsOverviewMailboxClear[reason] += uint64(count)
}

func (r *Registry) IncWSOverviewResyncSuppressed(reason string) {
	if reason == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsOverviewResyncSuppressed[reason]++
}

// IncWSRuntimeRecovery records one bounded runtime recovery lifecycle event.
func (r *Registry) IncWSRuntimeRecovery(mode, reason, outcome string) {
	if !wsRuntimeRecoveryModeAllowed(mode) ||
		!wsRuntimeRecoveryReasonAllowed(reason) ||
		!wsRuntimeRecoveryOutcomeAllowed(outcome) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsRuntimeRecoveryTotal[wsRuntimeRecoveryKey{
		Mode:    mode,
		Reason:  reason,
		Outcome: outcome,
	}]++
}

// ObserveWSRuntimeRecoveryDuration records terminal recovery latency.
func (r *Registry) ObserveWSRuntimeRecoveryDuration(mode, outcome string, duration time.Duration) {
	if !wsRuntimeRecoveryModeAllowed(mode) ||
		(outcome != "completed" && outcome != "failed") ||
		duration < 0 {
		return
	}

	key := wsRuntimeRecoveryDurationKey{Mode: mode, Outcome: outcome}
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.wsRuntimeRecoveryDuration[key]
	if !ok {
		h = newHistogram(durationBucketsSeconds)
		r.wsRuntimeRecoveryDuration[key] = h
	}
	h.observe(duration.Seconds())
}

// ObserveWSRuntimeAckLag records lag for a validated current-stream cursor.
func (r *Registry) ObserveWSRuntimeAckLag(versions uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsRuntimeAckLag.observe(float64(versions))
}

// ObserveWSRuntimeReplayVersions records the bounded journal span selected for replay.
func (r *Registry) ObserveWSRuntimeReplayVersions(versions uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsRuntimeReplayVersions.observe(float64(versions))
}

func wsRuntimeRecoveryModeAllowed(mode string) bool {
	switch mode {
	case "current", "replay", "snapshot", "http_fallback":
		return true
	default:
		return false
	}
}

func wsRuntimeRecoveryOutcomeAllowed(outcome string) bool {
	switch outcome {
	case "scheduled", "completed", "failed":
		return true
	default:
		return false
	}
}

func wsRuntimeRecoveryReasonAllowed(reason string) bool {
	switch reason {
	case "client_resync_scheduled",
		"client_missing_runtime_snapshot",
		"state_changes_dropped",
		"hub_buffer_full",
		"connect",
		"client_gap",
		"stream_mismatch",
		"timeout":
		return true
	default:
		return false
	}
}

func (r *Registry) SetWSConnectedClients(count int) {
	if count < 0 {
		count = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsConnectedClients = float64(count)
}

func (r *Registry) IncWSConnectionEvent(event string) {
	if event == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.wsConnectionsTotal[event]++
}

func (r *Registry) AddUnknownNeighbors(deviceID uuid.UUID, protocol domain.DiscoveryProtocol, count int) {
	if count <= 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.unknownNeighborsTotal[deviceProtocolKey{
		DeviceID: deviceID.String(),
		Protocol: string(protocol),
	}] += uint64(count)
}

func (r *Registry) AddDroppedStateChanges(count int) {
	if count <= 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.stateChangesDroppedTotal += uint64(count)
}

func newHistogram(buckets []float64) *histogram {
	cloned := append([]float64(nil), buckets...)
	return &histogram{
		buckets: cloned,
		counts:  make([]uint64, len(cloned)),
	}
}

func (h *histogram) observe(value float64) {
	h.count++
	h.sum += value
	for i, bucket := range h.buckets {
		if value <= bucket {
			h.counts[i]++
		}
	}
}

func (h *histogram) snapshot() histogramSnapshot {
	return histogramSnapshot{
		buckets: append([]float64(nil), h.buckets...),
		counts:  append([]uint64(nil), h.counts...),
		count:   h.count,
		sum:     h.sum,
	}
}

// RuntimeWorkerSettingEffective is one fixed worker or polling tuning value after runtime coercion.
type RuntimeWorkerSettingEffective struct {
	Setting string
	Value   float64
}

var runtimeWorkerSettingOrder = []string{
	domain.SettingPollingEssentialWorkers,
	domain.SettingSNMPWorkerPoolPerformance,
	domain.SettingSNMPWorkerPoolOperational,
	domain.SettingSNMPWorkerPoolStatic,
	domain.SettingPollingMaxWorkersPerDevice,
	domain.SettingPollingMaxWorkersPerSite,
	domain.SettingPollingMaxWorkersPerSubnet,
	domain.SettingPollingMaxInflightPerProfile,
	domain.SettingPollingWebSocketCoalesceMS,
	domain.SettingPollingPersistenceBatchMS,
	domain.SettingPollingEssentialTimeoutMillis,
	domain.SettingPollingEssentialRetries,
}

type gaugeRow struct {
	labels map[string]string
	value  float64
}

type counterRow struct {
	labels map[string]string
	value  uint64
}

type histogramRow struct {
	labels map[string]string
	value  histogramSnapshot
}

func sortedVolatilityGaugeRows(values map[domain.VolatilityClass]float64) []gaugeRow {
	order := []domain.VolatilityClass{
		domain.VolatilityClassPerformance,
		domain.VolatilityClassOperational,
		domain.VolatilityClassStatic,
	}
	rows := make([]gaugeRow, 0, len(order))
	for _, volatility := range order {
		rows = append(rows, gaugeRow{
			labels: map[string]string{"volatility_class": string(volatility)},
			value:  values[volatility],
		})
	}
	return rows
}

func sortedRuntimeWorkerSettingRows(values map[string]float64) []gaugeRow {
	rows := make([]gaugeRow, 0, len(values))
	for _, setting := range runtimeWorkerSettingOrder {
		value, ok := values[setting]
		if !ok {
			continue
		}
		rows = append(rows, gaugeRow{
			labels: map[string]string{"setting": setting},
			value:  value,
		})
	}
	return rows
}

func runtimeWorkerSettingAllowed(setting string) bool {
	for _, allowed := range runtimeWorkerSettingOrder {
		if setting == allowed {
			return true
		}
	}
	return false
}

func sortedDispatchRows(values map[schedulerDispatchKey]uint64) []counterRow {
	keys := make([]schedulerDispatchKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].TaskKind != keys[j].TaskKind {
			return keys[i].TaskKind < keys[j].TaskKind
		}
		return keys[i].VolatilityClass < keys[j].VolatilityClass
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"task_kind":        key.TaskKind,
				"volatility_class": key.VolatilityClass,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedSchedulerBackpressureRows(values map[schedulerBackpressureKey]uint64) []counterRow {
	keys := make([]schedulerBackpressureKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].VolatilityClass != keys[j].VolatilityClass {
			return keys[i].VolatilityClass < keys[j].VolatilityClass
		}
		return keys[i].Reason < keys[j].Reason
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"volatility_class": key.VolatilityClass,
				"reason":           key.Reason,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedSchedulerScopedBackpressureRows(values map[schedulerScopedBackpressureKey]uint64) []counterRow {
	keys := make([]schedulerScopedBackpressureKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].TaskKind != keys[j].TaskKind {
			return keys[i].TaskKind < keys[j].TaskKind
		}
		if keys[i].VolatilityClass != keys[j].VolatilityClass {
			return keys[i].VolatilityClass < keys[j].VolatilityClass
		}
		if keys[i].Reason != keys[j].Reason {
			return keys[i].Reason < keys[j].Reason
		}
		if keys[i].Scope != keys[j].Scope {
			return keys[i].Scope < keys[j].Scope
		}
		if keys[i].ScopeID != keys[j].ScopeID {
			return keys[i].ScopeID < keys[j].ScopeID
		}
		return keys[i].ScopeName < keys[j].ScopeName
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"task_kind":        key.TaskKind,
				"volatility_class": key.VolatilityClass,
				"reason":           key.Reason,
				"scope":            key.Scope,
				"scope_id":         key.ScopeID,
				"scope_name":       key.ScopeName,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedBulkOperationRejectionRows(values map[bulkOperationRejectionKey]uint64) []counterRow {
	keys := make([]bulkOperationRejectionKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		if keys[i].Reason != keys[j].Reason {
			return keys[i].Reason < keys[j].Reason
		}
		return keys[i].Source < keys[j].Source
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"operation": key.Operation,
				"reason":    key.Reason,
				"source":    key.Source,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedBulkOperationInFlightRows(values map[bulkOperationInFlightKey]float64) []gaugeRow {
	keys := make([]bulkOperationInFlightKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		return keys[i].Source < keys[j].Source
	})

	rows := make([]gaugeRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, gaugeRow{
			labels: map[string]string{
				"operation": key.Operation,
				"source":    key.Source,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedBulkOperationLimitRows(values map[bulkOperationLimitKey]float64) []gaugeRow {
	keys := make([]bulkOperationLimitKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		if keys[i].Scope != keys[j].Scope {
			return keys[i].Scope < keys[j].Scope
		}
		return keys[i].Source < keys[j].Source
	})

	rows := make([]gaugeRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, gaugeRow{
			labels: map[string]string{
				"operation": key.Operation,
				"scope":     key.Scope,
				"source":    key.Source,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedBulkOperationCompletionCounterRows(values map[bulkOperationCompletionKey]uint64) []counterRow {
	keys := make([]bulkOperationCompletionKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortBulkOperationCompletionKeys(keys)

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"operation": key.Operation,
				"result":    key.Result,
				"source":    key.Source,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedBulkOperationCompletionHistogramRows(values map[bulkOperationCompletionKey]*histogram) []histogramRow {
	keys := make([]bulkOperationCompletionKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortBulkOperationCompletionKeys(keys)

	rows := make([]histogramRow, 0, len(keys))
	for _, key := range keys {
		h := values[key]
		if h == nil {
			continue
		}
		rows = append(rows, histogramRow{
			labels: map[string]string{
				"operation": key.Operation,
				"result":    key.Result,
				"source":    key.Source,
			},
			value: h.snapshot(),
		})
	}
	return rows
}

func sortBulkOperationCompletionKeys(keys []bulkOperationCompletionKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		if keys[i].Result != keys[j].Result {
			return keys[i].Result < keys[j].Result
		}
		return keys[i].Source < keys[j].Source
	})
}

func sortedVolatilityHistogramRows(values map[domain.VolatilityClass]*histogram) []histogramRow {
	order := []domain.VolatilityClass{
		domain.VolatilityClassPerformance,
		domain.VolatilityClassOperational,
		domain.VolatilityClassStatic,
	}
	rows := make([]histogramRow, 0, len(order))
	for _, volatility := range order {
		if values[volatility] == nil {
			continue
		}
		rows = append(rows, histogramRow{
			labels: map[string]string{"volatility_class": string(volatility)},
			value:  values[volatility].snapshot(),
		})
	}
	return rows
}

func sortedTaskResultRows(values map[taskResultKey]uint64) []counterRow {
	keys := make([]taskResultKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].VolatilityClass != keys[j].VolatilityClass {
			return keys[i].VolatilityClass < keys[j].VolatilityClass
		}
		return keys[i].Outcome < keys[j].Outcome
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"volatility_class": key.VolatilityClass,
				"outcome":          key.Outcome,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedDiscoveryNeighborRows(values map[deviceProtocolKey]float64) []gaugeRow {
	keys := make([]deviceProtocolKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].DeviceID != keys[j].DeviceID {
			return keys[i].DeviceID < keys[j].DeviceID
		}
		return keys[i].Protocol < keys[j].Protocol
	})

	rows := make([]gaugeRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, gaugeRow{
			labels: map[string]string{
				"device_id": key.DeviceID,
				"protocol":  key.Protocol,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedLinkUpsertRows(values map[linkUpsertKey]uint64) []counterRow {
	keys := make([]linkUpsertKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Protocol != keys[j].Protocol {
			return keys[i].Protocol < keys[j].Protocol
		}
		return keys[i].Result < keys[j].Result
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"protocol": key.Protocol,
				"result":   key.Result,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedStringCounterRows(label string, values map[string]uint64) []counterRow {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{label: key},
			value:  values[key],
		})
	}
	return rows
}

func sortedStaticCollectionSkipRows(values map[staticCollectionSkipKey]uint64) []counterRow {
	keys := make([]staticCollectionSkipKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		return keys[i].Reason < keys[j].Reason
	})
	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"operation": key.Operation,
				"reason":    key.Reason,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedStringHistogramRows(label string, values map[string]*histogram) []histogramRow {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rows := make([]histogramRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, histogramRow{
			labels: map[string]string{label: key},
			value:  values[key].snapshot(),
		})
	}
	return rows
}

func sortedRefreshSnapshotBuildRows(values map[refreshSnapshotBuildKey]*histogram) []histogramRow {
	keys := make([]refreshSnapshotBuildKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Mode != keys[j].Mode {
			return keys[i].Mode < keys[j].Mode
		}
		return keys[i].Result < keys[j].Result
	})

	rows := make([]histogramRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, histogramRow{
			labels: map[string]string{
				"mode":   key.Mode,
				"result": key.Result,
			},
			value: values[key].snapshot(),
		})
	}
	return rows
}

func sortedPrometheusRuntimeCounterRows(values map[prometheusRuntimeRequestKey]uint64) []counterRow {
	keys := make([]prometheusRuntimeRequestKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		return keys[i].Result < keys[j].Result
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"operation": key.Operation,
				"result":    key.Result,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedPrometheusRuntimeHistogramRows(values map[prometheusRuntimeRequestKey]*histogram) []histogramRow {
	keys := make([]prometheusRuntimeRequestKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		return keys[i].Result < keys[j].Result
	})

	rows := make([]histogramRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, histogramRow{
			labels: map[string]string{
				"operation": key.Operation,
				"result":    key.Result,
			},
			value: values[key].snapshot(),
		})
	}
	return rows
}

func sortedSNMPCollectorOperationCounterRows(values map[snmpCollectorOperationKey]uint64) []counterRow {
	keys := make([]snmpCollectorOperationKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortSNMPCollectorOperationKeys(keys)

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"collector": key.Collector,
				"operation": key.Operation,
				"result":    key.Result,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedSNMPCollectorOperationHistogramRows(values map[snmpCollectorOperationKey]*histogram) []histogramRow {
	keys := make([]snmpCollectorOperationKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortSNMPCollectorOperationKeys(keys)

	rows := make([]histogramRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, histogramRow{
			labels: map[string]string{
				"collector": key.Collector,
				"operation": key.Operation,
				"result":    key.Result,
			},
			value: values[key].snapshot(),
		})
	}
	return rows
}

func sortedSNMPCollectorDeviceOperationGaugeRows(values map[snmpCollectorDeviceOperationKey]float64) []gaugeRow {
	keys := make([]snmpCollectorDeviceOperationKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortSNMPCollectorDeviceOperationKeys(keys)

	rows := make([]gaugeRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, gaugeRow{
			labels: snmpCollectorDeviceOperationLabels(key),
			value:  values[key],
		})
	}
	return rows
}

func sortedSNMPCollectorDeviceOperationCounterRows(values map[snmpCollectorDeviceOperationKey]uint64) []counterRow {
	keys := make([]snmpCollectorDeviceOperationKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortSNMPCollectorDeviceOperationKeys(keys)

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: snmpCollectorDeviceOperationLabels(key),
			value:  values[key],
		})
	}
	return rows
}

func sortSNMPCollectorOperationKeys(keys []snmpCollectorOperationKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Collector != keys[j].Collector {
			return keys[i].Collector < keys[j].Collector
		}
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		return keys[i].Result < keys[j].Result
	})
}

func sortSNMPCollectorDeviceOperationKeys(keys []snmpCollectorDeviceOperationKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Device != keys[j].Device {
			return keys[i].Device < keys[j].Device
		}
		if keys[i].Target != keys[j].Target {
			return keys[i].Target < keys[j].Target
		}
		if keys[i].Collector != keys[j].Collector {
			return keys[i].Collector < keys[j].Collector
		}
		if keys[i].Operation != keys[j].Operation {
			return keys[i].Operation < keys[j].Operation
		}
		if keys[i].Result != keys[j].Result {
			return keys[i].Result < keys[j].Result
		}
		return keys[i].DeviceID < keys[j].DeviceID
	})
}

func snmpCollectorDeviceOperationLabels(key snmpCollectorDeviceOperationKey) map[string]string {
	return map[string]string{
		"device_id": key.DeviceID,
		"device":    key.Device,
		"target":    key.Target,
		"collector": key.Collector,
		"operation": key.Operation,
		"result":    key.Result,
	}
}

func sortedSNMPCollectorEarlyExitRows(values map[snmpCollectorEarlyExitKey]uint64) []counterRow {
	keys := make([]snmpCollectorEarlyExitKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Collector != keys[j].Collector {
			return keys[i].Collector < keys[j].Collector
		}
		return keys[i].Reason < keys[j].Reason
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"collector": key.Collector,
				"reason":    key.Reason,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedWSRows(values map[wsMetricKey]uint64) []counterRow {
	keys := make([]wsMetricKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Scope != keys[j].Scope {
			return keys[i].Scope < keys[j].Scope
		}
		return keys[i].Type < keys[j].Type
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"scope": key.Scope,
				"type":  key.Type,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedWSBackpressureRows(values map[wsBackpressureKey]uint64) []counterRow {
	keys := make([]wsBackpressureKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Scope != keys[j].Scope {
			return keys[i].Scope < keys[j].Scope
		}
		return keys[i].Reason < keys[j].Reason
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"scope":  key.Scope,
				"reason": key.Reason,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedWSClientResyncRows(values map[wsClientResyncKey]uint64) []counterRow {
	keys := make([]wsClientResyncKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Bootstrap != keys[j].Bootstrap {
			return keys[i].Bootstrap < keys[j].Bootstrap
		}
		if keys[i].Reason != keys[j].Reason {
			return keys[i].Reason < keys[j].Reason
		}
		return keys[i].Scope < keys[j].Scope
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"bootstrap": key.Bootstrap,
				"reason":    key.Reason,
				"scope":     key.Scope,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedWSRuntimeRecoveryRows(values map[wsRuntimeRecoveryKey]uint64) []counterRow {
	keys := make([]wsRuntimeRecoveryKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Mode != keys[j].Mode {
			return keys[i].Mode < keys[j].Mode
		}
		if keys[i].Outcome != keys[j].Outcome {
			return keys[i].Outcome < keys[j].Outcome
		}
		return keys[i].Reason < keys[j].Reason
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"mode":    key.Mode,
				"outcome": key.Outcome,
				"reason":  key.Reason,
			},
			value: values[key],
		})
	}
	return rows
}

func sortedWSRuntimeRecoveryDurationRows(values map[wsRuntimeRecoveryDurationKey]*histogram) []histogramRow {
	keys := make([]wsRuntimeRecoveryDurationKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Mode != keys[j].Mode {
			return keys[i].Mode < keys[j].Mode
		}
		return keys[i].Outcome < keys[j].Outcome
	})

	rows := make([]histogramRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, histogramRow{
			labels: map[string]string{
				"mode":    key.Mode,
				"outcome": key.Outcome,
			},
			value: values[key].snapshot(),
		})
	}
	return rows
}

func sortedWSHistogramRows(values map[wsMetricKey]*histogram) []histogramRow {
	keys := make([]wsMetricKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Scope != keys[j].Scope {
			return keys[i].Scope < keys[j].Scope
		}
		return keys[i].Type < keys[j].Type
	})

	rows := make([]histogramRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, histogramRow{
			labels: map[string]string{
				"scope": key.Scope,
				"type":  key.Type,
			},
			value: values[key].snapshot(),
		})
	}
	return rows
}

func sortedUnknownNeighborRows(values map[deviceProtocolKey]uint64) []counterRow {
	keys := make([]deviceProtocolKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].DeviceID != keys[j].DeviceID {
			return keys[i].DeviceID < keys[j].DeviceID
		}
		return keys[i].Protocol < keys[j].Protocol
	})

	rows := make([]counterRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, counterRow{
			labels: map[string]string{
				"device_id": key.DeviceID,
				"protocol":  key.Protocol,
			},
			value: values[key],
		})
	}
	return rows
}

func writeGaugeSingle(b *strings.Builder, name, help string, value float64) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
	fmt.Fprintf(b, "%s %s\n", name, formatFloat(value))
}

func writeCounterSingle(b *strings.Builder, name, help string, value uint64) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s counter\n", name)
	fmt.Fprintf(b, "%s %d\n", name, value)
}

func writeGaugeVec(b *strings.Builder, name, help string, rows []gaugeRow) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
	for _, row := range rows {
		fmt.Fprintf(b, "%s%s %s\n", name, formatLabels(row.labels), formatFloat(row.value))
	}
}

func writeCounterVec(b *strings.Builder, name, help string, rows []counterRow) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s counter\n", name)
	for _, row := range rows {
		fmt.Fprintf(b, "%s%s %d\n", name, formatLabels(row.labels), row.value)
	}
}

func writeHistogramVec(b *strings.Builder, name, help string, rows []histogramRow) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s histogram\n", name)
	for _, row := range rows {
		for i, bucket := range row.value.buckets {
			fmt.Fprintf(b, "%s_bucket%s %d\n", name, formatBucketLabels(row.labels, formatFloat(bucket)), row.value.counts[i])
		}
		fmt.Fprintf(b, "%s_bucket%s %d\n", name, formatBucketLabels(row.labels, "+Inf"), row.value.count)
		fmt.Fprintf(b, "%s_sum%s %s\n", name, formatLabels(row.labels), formatFloat(row.value.sum))
		fmt.Fprintf(b, "%s_count%s %d\n", name, formatLabels(row.labels), row.value.count)
	}
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, escapeLabelValue(labels[key])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func formatBucketLabels(labels map[string]string, le string) string {
	if len(labels) == 0 {
		return fmt.Sprintf(`{le="%s"}`, escapeLabelValue(le))
	}

	return strings.TrimSuffix(formatLabels(labels), "}") + fmt.Sprintf(`,le="%s"}`, escapeLabelValue(le))
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
