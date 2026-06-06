package observability

// This file defines registry observability registry behavior.

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

var (
	defaultRegistryMu sync.RWMutex
	defaultRegistry   = NewRegistry()
)

var (
	durationBucketsSeconds = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120}
	payloadBucketsBytes    = []float64{128, 512, 1024, 4096, 16384, 65536, 262144, 1048576}
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

type schedulerBackpressureKey struct {
	VolatilityClass string
	Reason          string
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

	schedulerReadyDepth        map[domain.VolatilityClass]float64
	schedulerQueueLagSeconds   map[domain.VolatilityClass]float64
	schedulerInFlight          float64
	schedulerTaskDispatchTotal map[domain.VolatilityClass]uint64
	schedulerBackpressureTotal map[schedulerBackpressureKey]uint64
	schedulerTaskDuration      map[domain.VolatilityClass]*histogram
	bulkOperationInFlight      map[bulkOperationInFlightKey]float64
	bulkOperationLimits        map[bulkOperationLimitKey]float64
	bulkOperationRejections    map[bulkOperationRejectionKey]uint64
	bulkOperationCompletions   map[bulkOperationCompletionKey]uint64
	bulkOperationDuration      map[bulkOperationCompletionKey]*histogram
	bulkOperationDevices       map[bulkOperationCompletionKey]uint64
	bulkOperationFiles         map[bulkOperationCompletionKey]uint64
	bulkOperationBytes         map[bulkOperationCompletionKey]uint64
	pollingEssentialOverloaded float64
	pollingDeadlineMissTotal   uint64
	pollResultsTotal           map[taskResultKey]uint64
	discoveryNeighbors         map[deviceProtocolKey]float64
	linkUpsertsTotal           map[linkUpsertKey]uint64
	cacheInvalidationsTotal    map[string]uint64
	cacheReloadTotal           uint64
	topologyMaterialization    map[string]*histogram
	refreshSnapshotBuild       map[refreshSnapshotBuildKey]*histogram
	refreshTopologyReloadTotal map[string]uint64
	prometheusRuntimeRequests  map[prometheusRuntimeRequestKey]uint64
	prometheusRuntimeDuration  map[prometheusRuntimeRequestKey]*histogram
	snmpCollectorOperations    map[snmpCollectorOperationKey]uint64
	snmpCollectorDuration      map[snmpCollectorOperationKey]*histogram
	snmpCollectorEarlyExit     map[snmpCollectorEarlyExitKey]uint64
	wsConnectedClients         float64
	wsConnectionsTotal         map[string]uint64
	wsMessagesTotal            map[wsMetricKey]uint64
	wsBackpressureTotal        map[wsBackpressureKey]uint64
	wsClientResyncTotal        map[wsClientResyncKey]uint64
	wsOverviewMailboxClear     map[string]uint64
	wsOverviewResyncSuppressed map[string]uint64
	wsPayloadBytes             map[wsMetricKey]*histogram
	unknownNeighborsTotal      map[deviceProtocolKey]uint64
	stateChangesDroppedTotal   uint64
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
		schedulerTaskDispatchTotal: map[domain.VolatilityClass]uint64{
			domain.VolatilityClassPerformance: 0,
			domain.VolatilityClassOperational: 0,
			domain.VolatilityClassStatic:      0,
		},
		schedulerBackpressureTotal: make(map[schedulerBackpressureKey]uint64),
		bulkOperationInFlight:      make(map[bulkOperationInFlightKey]float64),
		bulkOperationLimits:        make(map[bulkOperationLimitKey]float64),
		bulkOperationRejections:    make(map[bulkOperationRejectionKey]uint64),
		bulkOperationCompletions:   make(map[bulkOperationCompletionKey]uint64),
		bulkOperationDuration:      make(map[bulkOperationCompletionKey]*histogram),
		bulkOperationDevices:       make(map[bulkOperationCompletionKey]uint64),
		bulkOperationFiles:         make(map[bulkOperationCompletionKey]uint64),
		bulkOperationBytes:         make(map[bulkOperationCompletionKey]uint64),
		schedulerTaskDuration: map[domain.VolatilityClass]*histogram{
			domain.VolatilityClassPerformance: newHistogram(durationBucketsSeconds),
			domain.VolatilityClassOperational: newHistogram(durationBucketsSeconds),
			domain.VolatilityClassStatic:      newHistogram(durationBucketsSeconds),
		},
		pollResultsTotal:        make(map[taskResultKey]uint64),
		discoveryNeighbors:      make(map[deviceProtocolKey]float64),
		linkUpsertsTotal:        make(map[linkUpsertKey]uint64),
		cacheInvalidationsTotal: make(map[string]uint64),
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
		snmpCollectorEarlyExit:     make(map[snmpCollectorEarlyExitKey]uint64),
		wsConnectionsTotal:         make(map[string]uint64),
		wsMessagesTotal:            make(map[wsMetricKey]uint64),
		wsBackpressureTotal:        make(map[wsBackpressureKey]uint64),
		wsClientResyncTotal:        make(map[wsClientResyncKey]uint64),
		wsOverviewMailboxClear:     make(map[string]uint64),
		wsOverviewResyncSuppressed: make(map[string]uint64),
		wsPayloadBytes:             make(map[wsMetricKey]*histogram),
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
	return Default()
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
		"Total scheduled tasks dispatched by volatility class.",
		sortedDispatchRows(r.schedulerTaskDispatchTotal),
	)
	writeCounterVec(&b,
		"theia_scheduler_backpressure_total",
		"Scheduler backpressure events by volatility class and reason.",
		sortedSchedulerBackpressureRows(r.schedulerBackpressureTotal),
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedulerTaskDispatchTotal[volatility]++
}

func (r *Registry) IncSchedulerBackpressure(volatility domain.VolatilityClass, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedulerBackpressureTotal[schedulerBackpressureKey{
		VolatilityClass: string(volatility),
		Reason:          reason,
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

func sortedDispatchRows(values map[domain.VolatilityClass]uint64) []counterRow {
	order := []domain.VolatilityClass{
		domain.VolatilityClassPerformance,
		domain.VolatilityClassOperational,
		domain.VolatilityClassStatic,
	}
	rows := make([]counterRow, 0, len(order))
	for _, volatility := range order {
		rows = append(rows, counterRow{
			labels: map[string]string{"volatility_class": string(volatility)},
			value:  values[volatility],
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
			labels := copyLabels(row.labels)
			labels["le"] = formatFloat(bucket)
			fmt.Fprintf(b, "%s_bucket%s %d\n", name, formatLabels(labels), row.value.counts[i])
		}
		labels := copyLabels(row.labels)
		labels["le"] = "+Inf"
		fmt.Fprintf(b, "%s_bucket%s %d\n", name, formatLabels(labels), row.value.count)
		fmt.Fprintf(b, "%s_sum%s %s\n", name, formatLabels(row.labels), formatFloat(row.value.sum))
		fmt.Fprintf(b, "%s_count%s %d\n", name, formatLabels(row.labels), row.value.count)
	}
}

func copyLabels(labels map[string]string) map[string]string {
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
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

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
