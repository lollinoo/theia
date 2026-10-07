package observability

import (
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"maps"
)

type deviceMetricLabels struct{ device, target string }

func (r *Registry) snapshotForScrape() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return &Registry{
		schedulerReadyDepth:               maps.Clone(r.schedulerReadyDepth),
		schedulerQueueLagSeconds:          maps.Clone(r.schedulerQueueLagSeconds),
		schedulerInFlight:                 r.schedulerInFlight,
		schedulerTaskDispatchTotal:        maps.Clone(r.schedulerTaskDispatchTotal),
		schedulerBackpressureTotal:        maps.Clone(r.schedulerBackpressureTotal),
		schedulerScopedBackpressureTotal:  maps.Clone(r.schedulerScopedBackpressureTotal),
		schedulerTaskDuration:             cloneHistogramMap(r.schedulerTaskDuration),
		bulkOperationInFlight:             maps.Clone(r.bulkOperationInFlight),
		bulkOperationLimits:               maps.Clone(r.bulkOperationLimits),
		bulkOperationRejections:           maps.Clone(r.bulkOperationRejections),
		bulkOperationCompletions:          maps.Clone(r.bulkOperationCompletions),
		bulkOperationDuration:             cloneHistogramMap(r.bulkOperationDuration),
		bulkOperationDevices:              maps.Clone(r.bulkOperationDevices),
		bulkOperationFiles:                maps.Clone(r.bulkOperationFiles),
		bulkOperationBytes:                maps.Clone(r.bulkOperationBytes),
		pollingEssentialOverloaded:        r.pollingEssentialOverloaded,
		pollingDeadlineMissTotal:          r.pollingDeadlineMissTotal,
		runtimeWorkerSettings:             maps.Clone(r.runtimeWorkerSettings),
		pollResultsTotal:                  maps.Clone(r.pollResultsTotal),
		discoveryNeighbors:                maps.Clone(r.discoveryNeighbors),
		linkUpsertsTotal:                  maps.Clone(r.linkUpsertsTotal),
		deviceImportTopologyRunEvents:     maps.Clone(r.deviceImportTopologyRunEvents),
		deviceImportTopologyItemOutcomes:  maps.Clone(r.deviceImportTopologyItemOutcomes),
		deviceImportTopologyRetries:       maps.Clone(r.deviceImportTopologyRetries),
		deviceImportTopologyLayouts:       maps.Clone(r.deviceImportTopologyLayouts),
		cacheInvalidationsTotal:           maps.Clone(r.cacheInvalidationsTotal),
		cacheReloadTotal:                  r.cacheReloadTotal,
		topologyMaterialization:           cloneHistogramMap(r.topologyMaterialization),
		topologyMaterializationSkipsTotal: maps.Clone(r.topologyMaterializationSkipsTotal),
		staticPersistenceSkipsTotal:       maps.Clone(r.staticPersistenceSkipsTotal),
		staticCollectionSkipsTotal:        maps.Clone(r.staticCollectionSkipsTotal),
		refreshSnapshotBuild:              cloneHistogramMap(r.refreshSnapshotBuild),
		refreshTopologyReloadTotal:        maps.Clone(r.refreshTopologyReloadTotal),
		prometheusRuntimeRequests:         maps.Clone(r.prometheusRuntimeRequests),
		prometheusRuntimeDuration:         cloneHistogramMap(r.prometheusRuntimeDuration),
		snmpCollectorOperations:           maps.Clone(r.snmpCollectorOperations),
		snmpCollectorDuration:             cloneHistogramMap(r.snmpCollectorDuration),
		snmpCollectorDeviceLast:           maps.Clone(r.snmpCollectorDeviceLast),
		snmpCollectorDeviceSlow:           maps.Clone(r.snmpCollectorDeviceSlow),
		snmpCollectorEarlyExit:            maps.Clone(r.snmpCollectorEarlyExit),
		wsConnectedClients:                r.wsConnectedClients,
		wsConnectionsTotal:                maps.Clone(r.wsConnectionsTotal),
		wsMessagesTotal:                   maps.Clone(r.wsMessagesTotal),
		wsBackpressureTotal:               maps.Clone(r.wsBackpressureTotal),
		wsClientResyncTotal:               maps.Clone(r.wsClientResyncTotal),
		wsOverviewMailboxClear:            maps.Clone(r.wsOverviewMailboxClear),
		wsOverviewResyncSuppressed:        maps.Clone(r.wsOverviewResyncSuppressed),
		wsPayloadBytes:                    cloneHistogramMap(r.wsPayloadBytes),
		wsRuntimeRecoveryTotal:            maps.Clone(r.wsRuntimeRecoveryTotal),
		wsRuntimeRecoveryDuration:         cloneHistogramMap(r.wsRuntimeRecoveryDuration),
		wsRuntimeAckLag:                   cloneHistogram(r.wsRuntimeAckLag),
		wsRuntimeReplayVersions:           cloneHistogram(r.wsRuntimeReplayVersions),
		unknownNeighborsTotal:             maps.Clone(r.unknownNeighborsTotal),
		stateChangesDroppedTotal:          r.stateChangesDroppedTotal,
	}
}

func cloneHistogram(h *histogram) *histogram {
	if h == nil {
		return nil
	}
	return &histogram{buckets: h.buckets, counts: append([]uint64(nil), h.counts...), count: h.count, sum: h.sum}
}

func cloneHistogramMap[K comparable](values map[K]*histogram) map[K]*histogram {
	cloned := make(map[K]*histogram, len(values))
	for key, value := range values {
		cloned[key] = cloneHistogram(value)
	}
	return cloned
}

// RetainDeviceMetrics drops series for devices absent from a successful fleet refresh.
// Repeating this reconciliation also removes late results from polls finishing after deletion.
func (r *Registry) RetainDeviceMetrics(devices []domain.Device) {
	active := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		active[device.ID.String()] = struct{}{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.discoveryNeighbors {
		if _, ok := active[key.DeviceID]; !ok {
			delete(r.discoveryNeighbors, key)
		}
	}
	for key := range r.unknownNeighborsTotal {
		if _, ok := active[key.DeviceID]; !ok {
			delete(r.unknownNeighborsTotal, key)
		}
	}
	for key := range r.snmpCollectorDeviceLast {
		if _, ok := active[key.DeviceID]; !ok {
			delete(r.snmpCollectorDeviceLast, key)
		}
	}
	for key := range r.snmpCollectorDeviceSlow {
		if _, ok := active[key.DeviceID]; !ok {
			delete(r.snmpCollectorDeviceSlow, key)
		}
	}
	for key := range r.schedulerScopedBackpressureTotal {
		if key.Scope == "device" {
			if _, ok := active[key.ScopeID]; !ok {
				delete(r.schedulerScopedBackpressureTotal, key)
			}
		}
	}
	for id := range r.deviceMetricLabels {
		if _, ok := active[id]; !ok {
			delete(r.deviceMetricLabels, id)
		}
	}
	for id := range r.schedulerDeviceNames {
		if _, ok := active[id]; !ok {
			delete(r.schedulerDeviceNames, id)
		}
	}
}

// ForgetDevice drops per-device series immediately after a committed device deletion.
func (r *Registry) ForgetDevice(id uuid.UUID) {
	device := id.String()
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.discoveryNeighbors {
		if key.DeviceID == device {
			delete(r.discoveryNeighbors, key)
		}
	}
	for key := range r.unknownNeighborsTotal {
		if key.DeviceID == device {
			delete(r.unknownNeighborsTotal, key)
		}
	}
	for key := range r.snmpCollectorDeviceLast {
		if key.DeviceID == device {
			delete(r.snmpCollectorDeviceLast, key)
		}
	}
	for key := range r.snmpCollectorDeviceSlow {
		if key.DeviceID == device {
			delete(r.snmpCollectorDeviceSlow, key)
		}
	}
	for key := range r.schedulerScopedBackpressureTotal {
		if key.Scope == "device" && key.ScopeID == device {
			delete(r.schedulerScopedBackpressureTotal, key)
		}
	}
	delete(r.deviceMetricLabels, device)
	delete(r.schedulerDeviceNames, device)
}
