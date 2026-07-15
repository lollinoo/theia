package worker

import (
	"bytes"
	"encoding/json"

	"github.com/lollinoo/theia/internal/ws"
)

const overviewJournalCapacity = 512
const overviewJournalMaxBytes = 16 << 20

type overviewJournalEntry struct {
	baseVersion uint64
	version     uint64
	delta       *ws.RuntimeDeltaPayload
	sizeBytes   int
}

type overviewJournal struct {
	capacity  int
	maxBytes  int
	sizeBytes int
	entries   []overviewJournalEntry
}

func newOverviewJournal(capacity, maxBytes int) *overviewJournal {
	return &overviewJournal{
		capacity: capacity,
		maxBytes: maxBytes,
	}
}

func (j *overviewJournal) Reset() {
	if j == nil {
		return
	}
	for index := range j.entries {
		j.entries[index] = overviewJournalEntry{}
	}
	j.entries = nil
	j.sizeBytes = 0
}

func (j *overviewJournal) Append(baseVersion, version uint64, delta *ws.RuntimeDeltaPayload) {
	if j == nil {
		return
	}
	if version <= baseVersion || version-baseVersion != 1 {
		j.Reset()
		return
	}
	if len(j.entries) > 0 && j.entries[len(j.entries)-1].version != baseVersion {
		j.Reset()
	}

	cloned, sizeBytes, err := cloneOverviewJournalDelta(delta)
	if err != nil || sizeBytes > j.maxBytes || j.capacity <= 0 {
		j.Reset()
		return
	}

	entry := overviewJournalEntry{
		baseVersion: baseVersion,
		version:     version,
		delta:       cloned,
		sizeBytes:   sizeBytes,
	}
	j.entries = append(j.entries, entry)
	j.sizeBytes += entry.sizeBytes

	for len(j.entries) > j.capacity || j.sizeBytes > j.maxBytes {
		j.sizeBytes -= j.entries[0].sizeBytes
		j.entries[0] = overviewJournalEntry{}
		j.entries = j.entries[1:]
	}
}

func (j *overviewJournal) Replay(fromVersion, targetVersion uint64) (*ws.RuntimeDeltaPayload, bool) {
	if j == nil || fromVersion >= targetVersion {
		return nil, false
	}

	compacted := ws.EmptyRuntimeDeltaPayload()
	expectedBaseVersion := fromVersion
	for _, entry := range j.entries {
		if entry.version <= fromVersion {
			continue
		}
		if entry.baseVersion != expectedBaseVersion || entry.version > targetVersion {
			return nil, false
		}

		mergeOverviewJournalPatches(compacted.Devices, entry.delta.Devices)
		mergeOverviewJournalPatches(compacted.Links, entry.delta.Links)
		expectedBaseVersion = entry.version
		if expectedBaseVersion == targetVersion {
			cloned, _, err := cloneOverviewJournalDelta(compacted)
			if err != nil {
				return nil, false
			}
			return cloned, true
		}
	}

	return nil, false
}

func cloneOverviewJournalDelta(delta *ws.RuntimeDeltaPayload) (*ws.RuntimeDeltaPayload, int, error) {
	if delta == nil {
		delta = ws.EmptyRuntimeDeltaPayload()
	}
	// Validate cycles and unsupported values before recursively cloning open-ended patch data.
	if _, err := json.Marshal(delta); err != nil {
		return nil, 0, err
	}

	cloned := &ws.RuntimeDeltaPayload{}
	if delta.Devices != nil {
		cloned.Devices = make(map[string]map[string]any, len(delta.Devices))
		for entityID, patch := range delta.Devices {
			clonedPatch, err := cloneOverviewJournalPatch(patch)
			if err != nil {
				return nil, 0, err
			}
			cloned.Devices[entityID] = clonedPatch
		}
	}
	if delta.Links != nil {
		cloned.Links = make(map[string]map[string]any, len(delta.Links))
		for entityID, patch := range delta.Links {
			clonedPatch, err := cloneOverviewJournalPatch(patch)
			if err != nil {
				return nil, 0, err
			}
			cloned.Links[entityID] = clonedPatch
		}
	}

	serialized, err := json.Marshal(cloned)
	if err != nil {
		return nil, 0, err
	}
	return cloned, len(serialized), nil
}

func cloneOverviewJournalPatch(patch map[string]any) (map[string]any, error) {
	if patch == nil {
		return nil, nil
	}
	cloned := make(map[string]any, len(patch))
	for field, value := range patch {
		clonedValue, err := cloneOverviewJournalValue(value)
		if err != nil {
			return nil, err
		}
		cloned[field] = clonedValue
	}
	return cloned, nil
}

func cloneOverviewJournalValue(value any) (any, error) {
	switch typed := value.(type) {
	case nil,
		bool,
		string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64,
		json.Number:
		return typed, nil
	case []byte:
		if typed == nil {
			return []byte(nil), nil
		}
		cloned := make([]byte, len(typed))
		copy(cloned, typed)
		return cloned, nil
	case []string:
		if typed == nil {
			return []string(nil), nil
		}
		cloned := make([]string, len(typed))
		copy(cloned, typed)
		return cloned, nil
	case map[string]string:
		if typed == nil {
			return map[string]string(nil), nil
		}
		cloned := make(map[string]string, len(typed))
		for key, nestedValue := range typed {
			cloned[key] = nestedValue
		}
		return cloned, nil
	case []any:
		if typed == nil {
			return []any(nil), nil
		}
		cloned := make([]any, len(typed))
		for index, nestedValue := range typed {
			valueClone, err := cloneOverviewJournalValue(nestedValue)
			if err != nil {
				return nil, err
			}
			cloned[index] = valueClone
		}
		return cloned, nil
	case map[string]any:
		if typed == nil {
			return map[string]any(nil), nil
		}
		cloned := make(map[string]any, len(typed))
		for key, nestedValue := range typed {
			valueClone, err := cloneOverviewJournalValue(nestedValue)
			if err != nil {
				return nil, err
			}
			cloned[key] = valueClone
		}
		return cloned, nil
	default:
		// Normalize custom JSON containers and pointers into an alias-free, wire-equivalent graph.
		return normalizeOverviewJournalJSONValue(typed)
	}
}

func normalizeOverviewJournalJSONValue(value any) (any, error) {
	serialized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(serialized))
	decoder.UseNumber()
	var cloned any
	if err := decoder.Decode(&cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func mergeOverviewJournalPatches(destination, source map[string]map[string]any) {
	for entityID, sourcePatch := range source {
		destinationPatch, exists := destination[entityID]
		if !exists {
			destinationPatch = make(map[string]any, len(sourcePatch))
			destination[entityID] = destinationPatch
		}
		for field, value := range sourcePatch {
			destinationPatch[field] = value
		}
	}
}
