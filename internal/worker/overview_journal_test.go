package worker

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/ws"
)

func TestOverviewJournalReplayMergesContiguousEntries(t *testing.T) {
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	journal.Append(10, 11, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {"operational_status": "up"},
		},
	})
	journal.Append(11, 12, &ws.RuntimeDeltaPayload{
		Links: map[string]map[string]any{
			"link-1": {"utilization": 0.5},
		},
	})
	journal.Append(12, 13, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-2": {"cpu_usage": 42.0},
		},
	})

	got, ok := journal.Replay(10, 13)
	if !ok {
		t.Fatal("expected contiguous range to be replayable")
	}
	want := &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {"operational_status": "up"},
			"device-2": {"cpu_usage": 42.0},
		},
		Links: map[string]map[string]any{
			"link-1": {"utilization": 0.5},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Replay(10, 13) = %#v, want %#v", got, want)
	}
}

func TestOverviewJournalReplayUsesFieldWiseLastWriteWins(t *testing.T) {
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	journal.Append(20, 21, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {
				"operational_status": "up",
				"cpu_usage":          10.0,
			},
		},
		Links: map[string]map[string]any{
			"link-1": {
				"in_utilization":  0.25,
				"out_utilization": 0.5,
			},
		},
	})
	journal.Append(21, 22, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {
				"operational_status": "down",
				"memory_usage":       75.0,
			},
		},
		Links: map[string]map[string]any{
			"link-1": {
				"in_utilization": 0.75,
			},
		},
	})

	got, ok := journal.Replay(20, 22)
	if !ok {
		t.Fatal("expected contiguous range to be replayable")
	}
	want := &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {
				"operational_status": "down",
				"cpu_usage":          10.0,
				"memory_usage":       75.0,
			},
		},
		Links: map[string]map[string]any{
			"link-1": {
				"in_utilization":  0.75,
				"out_utilization": 0.5,
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compacted replay = %#v, want %#v", got, want)
	}
}

func TestOverviewJournalGapResetsReplayability(t *testing.T) {
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	journal.Append(30, 31, ws.EmptyRuntimeDeltaPayload())
	journal.Append(32, 33, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {"operational_status": "up"},
		},
	})

	if got, ok := journal.Replay(30, 33); ok || got != nil {
		t.Fatalf("Replay(30, 33) = (%#v, %t), want (nil, false)", got, ok)
	}
	if len(journal.entries) != 1 {
		t.Fatalf("journal retained %d entries after gap, want 1", len(journal.entries))
	}
	if got, ok := journal.Replay(32, 33); !ok || got == nil {
		t.Fatalf("Replay(32, 33) = (%#v, %t), want a replay", got, ok)
	}
}

func TestOverviewJournalCapacityEvictsOldestEntry(t *testing.T) {
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	for version := uint64(0); version <= overviewJournalCapacity; version++ {
		journal.Append(version, version+1, &ws.RuntimeDeltaPayload{
			Devices: map[string]map[string]any{
				"device-1": {"sequence": version + 1},
			},
		})
	}

	if len(journal.entries) != overviewJournalCapacity {
		t.Fatalf("journal contains %d entries, want %d", len(journal.entries), overviewJournalCapacity)
	}
	if got := journal.entries[0].baseVersion; got != 1 {
		t.Fatalf("oldest base version = %d, want 1", got)
	}
	if got := journal.entries[len(journal.entries)-1].version; got != overviewJournalCapacity+1 {
		t.Fatalf("newest version = %d, want %d", got, overviewJournalCapacity+1)
	}
	if got, ok := journal.Replay(0, overviewJournalCapacity+1); ok || got != nil {
		t.Fatalf("evicted range replay = (%#v, %t), want (nil, false)", got, ok)
	}
	if got, ok := journal.Replay(1, overviewJournalCapacity+1); !ok || got == nil {
		t.Fatalf("retained range replay = (%#v, %t), want a replay", got, ok)
	}
}

func TestOverviewJournalByteLimitEvictsEntriesAndRejectsOversizeEntry(t *testing.T) {
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	halfBudgetValue := strings.Repeat("x", overviewJournalMaxBytes/2)
	journal.Append(40, 41, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {"payload": halfBudgetValue},
		},
	})
	journal.Append(41, 42, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {"payload": halfBudgetValue},
		},
	})

	if journal.sizeBytes > overviewJournalMaxBytes {
		t.Fatalf("journal size = %d bytes, exceeds %d-byte limit", journal.sizeBytes, overviewJournalMaxBytes)
	}
	if len(journal.entries) != 1 {
		t.Fatalf("journal contains %d entries after byte eviction, want 1", len(journal.entries))
	}
	if got := journal.entries[0].baseVersion; got != 41 {
		t.Fatalf("retained base version = %d, want 41", got)
	}

	oversizeValue := strings.Repeat("x", overviewJournalMaxBytes)
	journal.Append(42, 43, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {"payload": oversizeValue},
		},
	})
	if len(journal.entries) != 0 || journal.sizeBytes != 0 {
		t.Fatalf("oversize append left %d entries and %d bytes, want an empty journal", len(journal.entries), journal.sizeBytes)
	}
}

func TestOverviewJournalClonesInputsAndReplayResults(t *testing.T) {
	input := &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {
				"operational_status": "up",
				"nullable_metric":    nil,
				"runtime_flags":      []string{"reachable"},
				"field_states":       map[string]string{"cpu_usage": "fresh"},
			},
		},
		Links: map[string]map[string]any{
			"link-1": {"in_utilization": 0.5},
		},
	}
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	journal.Append(50, 51, input)

	input.Devices["device-1"]["operational_status"] = "mutated"
	input.Devices["device-1"]["runtime_flags"].([]string)[0] = "mutated"
	input.Devices["device-1"]["field_states"].(map[string]string)["cpu_usage"] = "mutated"
	input.Devices["device-2"] = map[string]any{"operational_status": "added"}
	input.Links["link-1"]["in_utilization"] = 1.0

	first, ok := journal.Replay(50, 51)
	if !ok {
		t.Fatal("expected appended entry to be replayable")
	}
	assertOriginalOverviewJournalPayload(t, first)

	first.Devices["device-1"]["operational_status"] = "replay-mutated"
	first.Devices["device-1"]["runtime_flags"].([]string)[0] = "replay-mutated"
	first.Devices["device-1"]["field_states"].(map[string]string)["cpu_usage"] = "replay-mutated"
	first.Devices["device-3"] = map[string]any{"operational_status": "added"}
	first.Links["link-1"]["in_utilization"] = 0.0

	second, ok := journal.Replay(50, 51)
	if !ok {
		t.Fatal("expected entry to remain replayable after mutating returned data")
	}
	assertOriginalOverviewJournalPayload(t, second)
}

func TestOverviewJournalAppendDeepClonesJSONValueGraph(t *testing.T) {
	input := nestedMutableOverviewJournalPayload()
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	journal.Append(55, 56, input)

	mutateNestedOverviewJournalPatch(t, input.Devices["device-1"], "input-mutated")

	replay, ok := journal.Replay(55, 56)
	if !ok {
		t.Fatal("expected appended entry to be replayable")
	}
	assertOriginalNestedOverviewJournalPatch(t, replay.Devices["device-1"])
}

func TestOverviewJournalReplayDeepClonesJSONValueGraph(t *testing.T) {
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	journal.Append(57, 58, nestedMutableOverviewJournalPayload())

	first, ok := journal.Replay(57, 58)
	if !ok {
		t.Fatal("expected appended entry to be replayable")
	}
	mutateNestedOverviewJournalPatch(t, first.Devices["device-1"], "replay-mutated")

	second, ok := journal.Replay(57, 58)
	if !ok {
		t.Fatal("expected entry to remain replayable after mutating returned data")
	}
	assertOriginalNestedOverviewJournalPatch(t, second.Devices["device-1"])
}

func TestOverviewJournalClonePreservesEmptyAndNullJSONValues(t *testing.T) {
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	journal.Append(59, 60, &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {
				"empty_bytes":         []byte{},
				"nil_bytes":           []byte(nil),
				"empty_strings":       []string{},
				"nil_strings":         []string(nil),
				"empty_list":          []any{},
				"nil_list":            []any(nil),
				"empty_object":        map[string]any{},
				"nil_object":          map[string]any(nil),
				"empty_string_object": map[string]string{},
				"nil_string_object":   map[string]string(nil),
				"explicit_nil":        nil,
			},
		},
		Links: map[string]map[string]any{},
	})

	replay, ok := journal.Replay(59, 60)
	if !ok {
		t.Fatal("expected appended entry to be replayable")
	}
	serialized, err := json.Marshal(replay.Devices["device-1"])
	if err != nil {
		t.Fatalf("Marshal replay patch: %v", err)
	}
	const want = `{"empty_bytes":"","empty_list":[],"empty_object":{},"empty_string_object":{},"empty_strings":[],"explicit_nil":null,"nil_bytes":null,"nil_list":null,"nil_object":null,"nil_string_object":null,"nil_strings":null}`
	if string(serialized) != want {
		t.Fatalf("replay patch = %s, want %s", serialized, want)
	}
}

func TestOverviewJournalReplaySameVersionDoesNotProduceDelta(t *testing.T) {
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	journal.Append(60, 61, ws.EmptyRuntimeDeltaPayload())

	if got, ok := journal.Replay(61, 61); ok || got != nil {
		t.Fatalf("Replay(61, 61) = (%#v, %t), want (nil, false) so the caller can send ready", got, ok)
	}
}

func BenchmarkOverviewJournalReplay512(b *testing.B) {
	journal := newOverviewJournal(overviewJournalCapacity, overviewJournalMaxBytes)
	for version := uint64(0); version < overviewJournalCapacity; version++ {
		journal.Append(version, version+1, &ws.RuntimeDeltaPayload{
			Devices: map[string]map[string]any{
				"device-1": {
					"operational_status": "up",
					"sequence":           version + 1,
				},
			},
			Links: map[string]map[string]any{
				"link-1": {
					"in_utilization":  version,
					"out_utilization": version + 1,
				},
			},
		})
	}
	if len(journal.entries) != overviewJournalCapacity {
		b.Fatalf("journal contains %d entries, want %d", len(journal.entries), overviewJournalCapacity)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		delta, ok := journal.Replay(0, overviewJournalCapacity)
		if !ok {
			b.Fatal("full journal is not replayable")
		}
		if got := delta.Devices["device-1"]["sequence"]; got != uint64(overviewJournalCapacity) {
			b.Fatalf("final sequence = %#v, want %d", got, overviewJournalCapacity)
		}
	}
}

func assertOriginalOverviewJournalPayload(t *testing.T, delta *ws.RuntimeDeltaPayload) {
	t.Helper()

	device := delta.Devices["device-1"]
	if got := device["operational_status"]; got != "up" {
		t.Fatalf("operational_status = %#v, want up", got)
	}
	value, exists := device["nullable_metric"]
	if !exists || value != nil {
		t.Fatalf("nullable_metric = (%#v, %t), want explicit nil", value, exists)
	}
	if got := device["runtime_flags"].([]string); !reflect.DeepEqual(got, []string{"reachable"}) {
		t.Fatalf("runtime_flags = %#v, want [reachable]", got)
	}
	if got := device["field_states"].(map[string]string); !reflect.DeepEqual(got, map[string]string{"cpu_usage": "fresh"}) {
		t.Fatalf("field_states = %#v, want original state", got)
	}
	if _, exists := delta.Devices["device-2"]; exists {
		t.Fatal("input-only device leaked into journal")
	}
	if _, exists := delta.Devices["device-3"]; exists {
		t.Fatal("replay-only device leaked into journal")
	}
	if got := delta.Links["link-1"]["in_utilization"]; got != 0.5 {
		t.Fatalf("in_utilization = %#v, want 0.5", got)
	}
}

func nestedMutableOverviewJournalPayload() *ws.RuntimeDeltaPayload {
	pointed := map[string]any{"state": "original"}
	return &ws.RuntimeDeltaPayload{
		Devices: map[string]map[string]any{
			"device-1": {
				"bytes": []byte{1, 2, 3},
				"nested": map[string]any{
					"items": []any{map[string]any{"state": "original"}},
				},
				"pointed": &pointed,
			},
		},
		Links: map[string]map[string]any{},
	}
}

func mutateNestedOverviewJournalPatch(t *testing.T, patch map[string]any, state string) {
	t.Helper()

	bytesValue, ok := patch["bytes"].([]byte)
	if !ok {
		t.Fatalf("bytes = %T, want []byte", patch["bytes"])
	}
	bytesValue[0] = 9

	nested, ok := patch["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested = %T, want map[string]any", patch["nested"])
	}
	items, ok := nested["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("nested.items = %#v, want a non-empty []any", nested["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("nested.items[0] = %T, want map[string]any", items[0])
	}
	item["state"] = state

	switch pointed := patch["pointed"].(type) {
	case *map[string]any:
		(*pointed)["state"] = state
	case map[string]any:
		pointed["state"] = state
	default:
		t.Fatalf("pointed = %T, want *map[string]any or normalized map[string]any", patch["pointed"])
	}
}

func assertOriginalNestedOverviewJournalPatch(t *testing.T, patch map[string]any) {
	t.Helper()

	serialized, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("Marshal nested patch: %v", err)
	}
	const want = `{"bytes":"AQID","nested":{"items":[{"state":"original"}]},"pointed":{"state":"original"}}`
	if string(serialized) != want {
		t.Fatalf("nested patch = %s, want %s", serialized, want)
	}
}
