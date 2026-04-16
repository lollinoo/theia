package worker

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lollinoo/theia/internal/collector"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/state"
	"github.com/lollinoo/theia/internal/ws"
)

// sectionHashes stores FNV-64a hashes for each section of the snapshot, keyed by device_id.
// Alerts use a whole-set hash so any alert change resends the entire array.
type sectionHashes struct {
	deviceMetrics   map[string]uint64
	linkMetrics     map[string]uint64
	deviceStatuses  map[string]uint64
	deviceHostnames map[string]uint64
	deviceModels    map[string]uint64
	alertsHash      uint64
}

func buildPipelineSnapshot(
	devices []domain.Device,
	links []domain.Link,
	states map[uuid.UUID]state.DeviceState,
	alerts map[uuid.UUID][]domain.AlertState,
	hostnameOverrides map[uuid.UUID]string,
) *ws.SnapshotPayload {
	snapshot := ws.EmptySnapshot()
	deviceMetrics := make(map[string]domain.DeviceMetrics, len(devices))
	linkMetrics := make(map[string][]domain.LinkMetrics, len(devices))
	statuses := make(map[string]string, len(devices))
	hostnames := make(map[string]string, len(devices))
	models := make(map[string]string, len(devices))

	linksByDevice := make(map[uuid.UUID][]domain.Link, len(links)*2)
	for _, link := range links {
		linksByDevice[link.SourceDeviceID] = append(linksByDevice[link.SourceDeviceID], link)
		linksByDevice[link.TargetDeviceID] = append(linksByDevice[link.TargetDeviceID], link)
	}

	for _, device := range devices {
		deviceKey := device.ID.String()
		deviceState := states[device.ID]

		metric := deviceState.Metrics
		metric.DeviceID = device.ID
		deviceMetrics[deviceKey] = metric
		linkMetrics[deviceKey] = buildDeviceLinkMetrics(device, linksByDevice[device.ID], deviceState.LinkMetrics)
		statuses[deviceKey] = effectiveSnapshotDeviceStatus(device, deviceState)

		if hostname := strings.TrimSpace(device.SysName); hostname != "" {
			hostnames[deviceKey] = hostname
		} else if hostname := strings.TrimSpace(hostnameOverrides[device.ID]); hostname != "" {
			hostnames[deviceKey] = hostname
		}

		if model := strings.TrimSpace(device.HardwareModel); model != "" && model != "Unknown" {
			models[deviceKey] = model
		}
	}

	snapshot.DeviceMetrics = ws.DeviceMetricsToDTOs(deviceMetrics)
	for _, device := range devices {
		deviceID := device.ID.String()
		dto, ok := snapshot.DeviceMetrics[deviceID]
		if !ok {
			continue
		}

		deviceState := states[device.ID]
		if deviceState.Health == "" {
			dto.Health = string(state.HealthStatusUnknown)
		} else {
			dto.Health = string(deviceState.Health)
		}
		if deviceState.Reachability != "" {
			dto.Reachability = string(deviceState.Reachability)
		}
		dto.Stale = boolPtr(deviceState.Stale)
		if !deviceState.LastPolledAt.IsZero() {
			dto.LastPolledAt = deviceState.LastPolledAt.UTC().Format(time.RFC3339)
		}

		expectedInterval := deviceState.ExpectedInterval
		if expectedInterval <= 0 {
			if device.PollIntervalOverride != nil {
				expectedInterval = time.Duration(*device.PollIntervalOverride) * time.Second
			} else {
				expectedInterval = device.PollClass.Interval()
			}
		}
		dto.ExpectedPollIntervalSeconds = int64Ptr(int64(expectedInterval / time.Second))

		snapshot.DeviceMetrics[deviceID] = dto
	}
	snapshot.LinkMetrics = ws.LinkMetricsToDTOs(linkMetrics)
	snapshot.Alerts = ws.AlertsToDTOs(flattenAlerts(alerts))
	snapshot.DeviceStatuses = statuses
	snapshot.DeviceHostnames = hostnames
	snapshot.DeviceModels = models

	return snapshot
}

func buildDeviceDetailDelta(device domain.Device, deviceState state.DeviceState) *ws.SnapshotPayload {
	delta := ws.EmptySnapshot()
	deviceID := device.ID.String()

	deviceMetrics := deviceState.Metrics
	deviceMetrics.DeviceID = device.ID
	dto := ws.DeviceMetricsToDTOs(map[string]domain.DeviceMetrics{
		deviceID: deviceMetrics,
	})[deviceID]

	dto.Health = string(deviceState.Health)
	dto.Reachability = string(deviceState.Reachability)
	dto.Stale = boolPtr(deviceState.Stale)
	if !deviceState.LastPolledAt.IsZero() {
		dto.LastPolledAt = deviceState.LastPolledAt.UTC().Format(time.RFC3339)
	}
	if deviceState.ExpectedInterval > 0 {
		dto.ExpectedPollIntervalSeconds = int64Ptr(int64(deviceState.ExpectedInterval / time.Second))
	}

	delta.DeviceMetrics[deviceID] = dto
	delta.DeviceStatuses[deviceID] = effectiveSnapshotDeviceStatus(device, deviceState)
	if len(deviceState.LinkMetrics) > 0 {
		copiedLinkMetrics := make([]domain.LinkMetrics, 0, len(deviceState.LinkMetrics))
		for _, metric := range deviceState.LinkMetrics {
			mapped := metric
			mapped.DeviceID = device.ID
			copiedLinkMetrics = append(copiedLinkMetrics, mapped)
		}

		linkMetrics := ws.LinkMetricsToDTOs(map[string][]domain.LinkMetrics{
			deviceID: copiedLinkMetrics,
		})
		if deviceLinkMetrics, ok := linkMetrics[deviceID]; ok {
			delta.LinkMetrics[deviceID] = deviceLinkMetrics
		}
	}

	return delta
}

func buildDeviceLinkMetrics(device domain.Device, links []domain.Link, metrics []domain.LinkMetrics) []domain.LinkMetrics {
	built := make([]domain.LinkMetrics, 0, len(metrics))
	for _, metric := range metrics {
		linkID := matchLinkID(device, links, metric.IfName)
		if linkID == "" {
			continue
		}

		mapped := metric
		mapped.DeviceID = device.ID
		mapped.LinkID = linkID
		if utilization := computeUtilization(device, metric.IfName, metric); utilization != nil {
			mapped.Utilization = utilization
		}
		built = append(built, mapped)
	}

	sort.Slice(built, func(i, j int) bool {
		if built[i].IfName != built[j].IfName {
			return built[i].IfName < built[j].IfName
		}
		return built[i].LinkID < built[j].LinkID
	})

	return built
}

func flattenAlerts(alertsByDevice map[uuid.UUID][]domain.AlertState) []domain.AlertState {
	total := 0
	for _, alerts := range alertsByDevice {
		total += len(alerts)
	}

	flattened := make([]domain.AlertState, 0, total)
	for deviceID, alerts := range alertsByDevice {
		for _, alert := range alerts {
			mapped := alert
			if mapped.DeviceID == uuid.Nil {
				mapped.DeviceID = deviceID
			}
			flattened = append(flattened, mapped)
		}
	}

	return flattened
}

func mapDeviceStatus(fallback domain.DeviceStatus, reachability state.ReachabilityStatus) string {
	switch reachability {
	case state.ReachabilityUp:
		return string(domain.DeviceStatusUp)
	case state.ReachabilitySoftDown, state.ReachabilityHardDown:
		return string(domain.DeviceStatusDown)
	default:
		return string(fallback)
	}
}

func effectiveSnapshotDeviceStatus(device domain.Device, deviceState state.DeviceState) string {
	if domain.IsVirtualNoIPDevice(device) {
		return string(domain.DeviceStatusUnknown)
	}

	return mapDeviceStatus(device.Status, deviceState.Reachability)
}

func matchLinkID(device domain.Device, links []domain.Link, metricIfName string) string {
	for _, link := range links {
		switch {
		case link.SourceDeviceID == device.ID && sameInterface(device, metricIfName, link.SourceIfName):
			return link.ID.String()
		case link.TargetDeviceID == device.ID && sameInterface(device, metricIfName, link.TargetIfName):
			return link.ID.String()
		}
	}
	return ""
}

func sameInterface(device domain.Device, observedIfName, linkIfName string) bool {
	observed := normalizeInterfaceName(observedIfName)
	linkName := normalizeInterfaceName(linkIfName)
	if observed == "" || linkName == "" {
		return false
	}
	if observed == linkName {
		return true
	}

	for _, iface := range device.Interfaces {
		ifaceName := normalizeInterfaceName(iface.IfName)
		ifaceDescr := normalizeInterfaceName(iface.IfDescr)

		if (observed == ifaceName || observed == ifaceDescr) &&
			(linkName == ifaceName || linkName == ifaceDescr) {
			return true
		}
	}

	return false
}

func computeUtilization(device domain.Device, observedIfName string, metric domain.LinkMetrics) *float64 {
	speed := interfaceSpeed(device, observedIfName)
	if speed <= 0 {
		return metric.Utilization
	}

	var (
		maxRate float64
		hasRate bool
	)

	if metric.TxBps != nil {
		maxRate = *metric.TxBps
		hasRate = true
	}
	if metric.RxBps != nil && (!hasRate || *metric.RxBps > maxRate) {
		maxRate = *metric.RxBps
		hasRate = true
	}

	if !hasRate {
		return nil
	}

	utilization := math.Max(0, maxRate/float64(speed))
	return &utilization
}

func interfaceSpeed(device domain.Device, observedIfName string) int64 {
	observed := normalizeInterfaceName(observedIfName)
	for _, iface := range device.Interfaces {
		if observed == normalizeInterfaceName(iface.IfName) || observed == normalizeInterfaceName(iface.IfDescr) {
			return iface.Speed
		}
	}
	return 0
}

func interfaceCounterSnapshots(device domain.Device, counters []SNMPIfCounter) []collector.InterfaceCounterSnapshot {
	snapshots := make([]collector.InterfaceCounterSnapshot, 0, len(counters))
	for _, counter := range counters {
		snapshots = append(snapshots, collector.InterfaceCounterSnapshot{
			IfName:    counter.IfName,
			InOctets:  counter.InOctets,
			OutOctets: counter.OutOctets,
			SpeedBps:  interfaceSpeed(device, counter.IfName),
		})
	}
	return snapshots
}

func normalizeInterfaceName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func computeSectionHash(data string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(data))
	return h.Sum64()
}

func formatFloatPtr(value *float64) string {
	if value == nil {
		return "nil"
	}

	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func formatBoolPtr(value *bool) string {
	if value == nil {
		return "nil"
	}

	return strconv.FormatBool(*value)
}

func formatInt64Ptr(value *int64) string {
	if value == nil {
		return "nil"
	}

	return strconv.FormatInt(*value, 10)
}

func computeSnapshotHashes(snapshot *ws.SnapshotPayload) *sectionHashes {
	sh := &sectionHashes{
		deviceMetrics:   make(map[string]uint64, len(snapshot.DeviceMetrics)),
		linkMetrics:     make(map[string]uint64, len(snapshot.LinkMetrics)),
		deviceStatuses:  make(map[string]uint64, len(snapshot.DeviceStatuses)),
		deviceHostnames: make(map[string]uint64, len(snapshot.DeviceHostnames)),
		deviceModels:    make(map[string]uint64, len(snapshot.DeviceModels)),
	}

	for id, dm := range snapshot.DeviceMetrics {
		key := fmt.Sprintf("%s|%v|%v|%v|%v|%s",
			dm.DeviceID,
			formatFloatPtr(dm.CPUPercent),
			formatFloatPtr(dm.MemPercent),
			formatFloatPtr(dm.TempCelsius),
			formatFloatPtr(dm.UptimeSecs),
			dm.CollectedAt,
		)
		key = fmt.Sprintf("%s|%s|%s|%s|%s|%s",
			key,
			dm.Health,
			dm.Reachability,
			formatBoolPtr(dm.Stale),
			dm.LastPolledAt,
			formatInt64Ptr(dm.ExpectedPollIntervalSeconds),
		)
		sh.deviceMetrics[id] = computeSectionHash(key)
	}

	for id, lms := range snapshot.LinkMetrics {
		var sb strings.Builder
		for _, lm := range lms {
			sb.WriteString(fmt.Sprintf("%s|%s|%v|%v|%v|%s",
				lm.DeviceID,
				lm.IfName,
				formatFloatPtr(lm.TxBps),
				formatFloatPtr(lm.RxBps),
				formatFloatPtr(lm.Utilization),
				lm.CollectedAt,
			))
		}
		sh.linkMetrics[id] = computeSectionHash(sb.String())
	}

	for id, status := range snapshot.DeviceStatuses {
		sh.deviceStatuses[id] = computeSectionHash(status)
	}

	for id, hostname := range snapshot.DeviceHostnames {
		sh.deviceHostnames[id] = computeSectionHash(hostname)
	}

	for id, model := range snapshot.DeviceModels {
		sh.deviceModels[id] = computeSectionHash(model)
	}

	var alertSb strings.Builder
	for _, a := range snapshot.Alerts {
		alertSb.WriteString(fmt.Sprintf("%s|%s|%s|%s|%s",
			a.DeviceID,
			a.Severity,
			a.AlertName,
			a.State,
			a.Summary,
		))
	}
	sh.alertsHash = computeSectionHash(alertSb.String())

	return sh
}

func buildDelta(current *ws.SnapshotPayload, currentHashes, previousHashes *sectionHashes) *ws.SnapshotPayload {
	delta := &ws.SnapshotPayload{
		DeviceMetrics:   make(map[string]ws.DeviceMetricsDTO),
		LinkMetrics:     make(map[string][]ws.LinkMetricsDTO),
		DeviceStatuses:  make(map[string]string),
		DeviceHostnames: make(map[string]string),
		DeviceModels:    make(map[string]string),
	}

	anyChanged := false

	for id, hash := range currentHashes.deviceMetrics {
		if previousHash, ok := previousHashes.deviceMetrics[id]; !ok || previousHash != hash {
			delta.DeviceMetrics[id] = current.DeviceMetrics[id]
			anyChanged = true
		}
	}

	for id, hash := range currentHashes.linkMetrics {
		if previousHash, ok := previousHashes.linkMetrics[id]; !ok || previousHash != hash {
			delta.LinkMetrics[id] = current.LinkMetrics[id]
			anyChanged = true
		}
	}

	for id, hash := range currentHashes.deviceStatuses {
		if previousHash, ok := previousHashes.deviceStatuses[id]; !ok || previousHash != hash {
			delta.DeviceStatuses[id] = current.DeviceStatuses[id]
			anyChanged = true
		}
	}

	for id, hash := range currentHashes.deviceHostnames {
		if previousHash, ok := previousHashes.deviceHostnames[id]; !ok || previousHash != hash {
			delta.DeviceHostnames[id] = current.DeviceHostnames[id]
			anyChanged = true
		}
	}

	for id, hash := range currentHashes.deviceModels {
		if previousHash, ok := previousHashes.deviceModels[id]; !ok || previousHash != hash {
			delta.DeviceModels[id] = current.DeviceModels[id]
			anyChanged = true
		}
	}

	if currentHashes.alertsHash != previousHashes.alertsHash {
		delta.Alerts = current.Alerts
		anyChanged = true
	}

	if !anyChanged {
		return nil
	}

	return delta
}

func boolPtr(value bool) *bool {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
