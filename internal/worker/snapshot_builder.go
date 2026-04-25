package worker

import (
	"fmt"
	"hash/fnv"
	"math"
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
type sectionHashes struct {
	devices        map[string]uint64
	links          map[string]uint64
	deviceMetrics  map[string]uint64
	linkMetrics    map[string]uint64
	deviceStatuses map[string]uint64
}

func buildPipelineSnapshot(
	devices []domain.Device,
	links []domain.Link,
	states map[uuid.UUID]state.DeviceState,
	alerts map[uuid.UUID][]domain.AlertState,
	promStatus ws.PrometheusStatusPayload,
) *ws.SnapshotPayload {
	snapshot := ws.EmptySnapshot()

	devicesByID := make(map[uuid.UUID]domain.Device, len(devices))

	for _, device := range devices {
		devicesByID[device.ID] = device
		snapshot.Devices[device.ID.String()] = normalizeDeviceRuntimeDTO(device, states[device.ID], alerts[device.ID], promStatus)
	}

	for _, link := range links {
		sourceRuntime := snapshot.Devices[link.SourceDeviceID.String()]
		targetRuntime := snapshot.Devices[link.TargetDeviceID.String()]
		linkMetric := selectNormalizedLinkMetric(
			link,
			devicesByID[link.SourceDeviceID],
			devicesByID[link.TargetDeviceID],
			states[link.SourceDeviceID].LinkMetrics,
			states[link.TargetDeviceID].LinkMetrics,
		)
		snapshot.Links[link.ID.String()] = normalizeLinkRuntimeDTO(link, linkMetric, sourceRuntime, targetRuntime)
	}
	syncSnapshotCompatibility(snapshot)

	return snapshot
}

func buildDeviceDetailDelta(
	device domain.Device,
	deviceState state.DeviceState,
	alerts []domain.AlertState,
	promStatus ws.PrometheusStatusPayload,
) *ws.SnapshotPayload {
	return buildDeviceDetailDeltaWithLinks(device, deviceState, nil, alerts, promStatus)
}

func buildDeviceDetailDeltaWithLinks(
	device domain.Device,
	deviceState state.DeviceState,
	linkRuntimes []ws.LinkRuntimeDTO,
	alerts []domain.AlertState,
	promStatus ws.PrometheusStatusPayload,
) *ws.SnapshotPayload {
	delta := ws.EmptySnapshot()
	deviceID := device.ID.String()
	delta.Devices[deviceID] = normalizeDeviceRuntimeDTO(device, deviceState, alerts, promStatus)
	for _, linkRuntime := range linkRuntimes {
		delta.Links[linkRuntime.LinkID] = linkRuntime
	}
	syncSnapshotCompatibility(delta)

	return delta
}

func buildDeviceLinkRuntimeDTOs(
	device domain.Device,
	deviceState state.DeviceState,
	devicesByID map[uuid.UUID]domain.Device,
	states map[uuid.UUID]state.DeviceState,
	links []domain.Link,
	promStatus ws.PrometheusStatusPayload,
) []ws.LinkRuntimeDTO {
	deviceID := device.ID
	sourceRuntime := normalizeDeviceRuntimeDTO(device, deviceState, nil, promStatus)
	linkRuntimes := make([]ws.LinkRuntimeDTO, 0, len(links))

	for _, link := range links {
		if link.SourceDeviceID != deviceID && link.TargetDeviceID != deviceID {
			continue
		}

		sourceDevice := devicesByID[link.SourceDeviceID]
		targetDevice := devicesByID[link.TargetDeviceID]
		sourceState := states[link.SourceDeviceID]
		targetState := states[link.TargetDeviceID]
		linkMetric := selectNormalizedLinkMetric(link, sourceDevice, targetDevice, sourceState.LinkMetrics, targetState.LinkMetrics)

		linkSourceRuntime := sourceRuntime
		if link.SourceDeviceID != deviceID {
			linkSourceRuntime = normalizeDeviceRuntimeDTO(sourceDevice, sourceState, nil, promStatus)
		}
		linkTargetRuntime := sourceRuntime
		if link.TargetDeviceID != deviceID {
			linkTargetRuntime = normalizeDeviceRuntimeDTO(targetDevice, targetState, nil, promStatus)
		}

		linkRuntimes = append(linkRuntimes, normalizeLinkRuntimeDTO(link, linkMetric, linkSourceRuntime, linkTargetRuntime))
	}

	return linkRuntimes
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

func computeSnapshotHashes(snapshot *ws.SnapshotPayload) *sectionHashes {
	sh := &sectionHashes{
		devices:        make(map[string]uint64, len(snapshot.Devices)),
		links:          make(map[string]uint64, len(snapshot.Links)),
		deviceMetrics:  make(map[string]uint64, len(snapshot.Devices)),
		linkMetrics:    make(map[string]uint64, len(snapshot.Links)),
		deviceStatuses: make(map[string]uint64, len(snapshot.Devices)),
	}

	for id, dm := range snapshot.Devices {
		key := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%d|%s|%s|%s|%s|%s|%s|%s",
			dm.DeviceID,
			dm.OperationalStatus,
			dm.PrimaryHealth,
			strings.Join(dm.RuntimeFlags, ","),
			fieldStateHashPart(dm.FieldStates),
			dm.NetworkReachable,
			dm.SNMPReachable,
			dm.Reachability,
			dm.Health,
			dm.Freshness,
			dm.PrimaryReason,
			dm.MetricsStatus,
			dm.MetricsReason,
			dm.AlertStatus,
			dm.FiringAlertCount,
			formatStringPtr(dm.LastCollectedAt),
			formatStringPtr(dm.LastPolledAt),
			formatFloatPtr(dm.ExpectedPollIntervalSeconds),
			formatFloatPtr(dm.CPUPercent),
			formatFloatPtr(dm.MemPercent),
			formatFloatPtr(dm.TempCelsius),
			formatFloatPtr(dm.UptimeSecs),
		)
		sh.devices[id] = computeSectionHash(key)
		sh.deviceMetrics[id] = sh.devices[id]
		sh.deviceStatuses[id] = computeSectionHash(compatibilityOperationalStatus(dm.OperationalStatus))
	}

	for id, lm := range snapshot.Links {
		key := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
			lm.LinkID,
			lm.SourceDeviceID,
			lm.TargetDeviceID,
			lm.SourceIfName,
			lm.TargetIfName,
			lm.MetricsStatus,
			lm.MetricsReason,
			formatStringPtr(lm.LastCollectedAt),
			formatFloatPtr(lm.TxBps),
			formatFloatPtr(lm.RxBps)+"|"+formatFloatPtr(lm.Utilization),
		)
		sh.links[id] = computeSectionHash(key)
		sh.linkMetrics[id] = sh.links[id]
	}

	return sh
}

func fieldStateHashPart(fields map[string]string) string {
	return strings.Join([]string{
		fields["uptime"],
		fields["cpu"],
		fields["memory"],
	}, ",")
}

func buildDelta(current *ws.SnapshotPayload, currentHashes, previousHashes *sectionHashes) *ws.SnapshotPayload {
	delta := &ws.SnapshotPayload{
		Devices: make(map[string]ws.DeviceRuntimeDTO),
		Links:   make(map[string]ws.LinkRuntimeDTO),
	}

	anyChanged := false

	for id, hash := range currentHashes.devices {
		if previousHash, ok := previousHashes.devices[id]; !ok || previousHash != hash {
			delta.Devices[id] = current.Devices[id]
			anyChanged = true
		}
	}

	for id, hash := range currentHashes.links {
		if previousHash, ok := previousHashes.links[id]; !ok || previousHash != hash {
			delta.Links[id] = current.Links[id]
			anyChanged = true
		}
	}

	if !anyChanged {
		return nil
	}

	return delta
}

func durationSecondsPtr(value time.Duration) *float64 {
	if value <= 0 {
		return nil
	}

	seconds := value.Seconds()
	return &seconds
}

func formatStringPtr(value *string) string {
	if value == nil {
		return "nil"
	}
	return *value
}

func selectNormalizedLinkMetric(link domain.Link, sourceDevice domain.Device, targetDevice domain.Device, sourceMetrics []domain.LinkMetrics, targetMetrics []domain.LinkMetrics) *domain.LinkMetrics {
	sourceCandidates := buildDeviceLinkMetrics(sourceDevice, []domain.Link{link}, sourceMetrics)
	for _, candidate := range sourceCandidates {
		if candidate.LinkID == link.ID.String() {
			return &candidate
		}
	}
	targetCandidates := buildDeviceLinkMetrics(targetDevice, []domain.Link{link}, targetMetrics)
	for _, candidate := range targetCandidates {
		if candidate.LinkID == link.ID.String() {
			return &candidate
		}
	}
	return nil
}

func syncSnapshotCompatibility(snapshot *ws.SnapshotPayload) {
	if snapshot == nil {
		return
	}
	if snapshot.DeviceMetrics == nil {
		snapshot.DeviceMetrics = make(map[string]ws.DeviceRuntimeDTO, len(snapshot.Devices))
	}
	if snapshot.LinkMetrics == nil {
		snapshot.LinkMetrics = make(map[string][]ws.LinkRuntimeDTO)
	}
	if snapshot.DeviceStatuses == nil {
		snapshot.DeviceStatuses = make(map[string]string, len(snapshot.Devices))
	}

	clear(snapshot.DeviceMetrics)
	clear(snapshot.LinkMetrics)
	clear(snapshot.DeviceStatuses)

	for key, value := range snapshot.Devices {
		snapshot.DeviceMetrics[key] = value
		snapshot.DeviceStatuses[key] = compatibilityOperationalStatus(value.OperationalStatus)
	}
	for _, value := range snapshot.Links {
		if value.DeviceID == "" {
			value.DeviceID = value.SourceDeviceID
		}
		snapshot.LinkMetrics[value.DeviceID] = append(snapshot.LinkMetrics[value.DeviceID], value)
	}
}

func compatibilityOperationalStatus(status string) string {
	if status == "unmonitored" {
		return string(domain.DeviceStatusUnknown)
	}
	return status
}
