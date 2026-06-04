package canvasmap

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestMaterializeMembershipProjectsBaseGhostLinksAndAreas(t *testing.T) {
	areaID := uuid.New()
	otherAreaID := uuid.New()
	baseID := uuid.New()
	ghostID := uuid.New()
	excludedID := uuid.New()
	linkID := uuid.New()
	excludedLinkID := uuid.New()

	membership := MaterializeMembership(
		[]domain.Device{
			{ID: baseID, AreaIDs: []uuid.UUID{areaID, otherAreaID}},
			{ID: ghostID, AreaIDs: []uuid.UUID{otherAreaID}},
			{ID: excludedID, AreaIDs: []uuid.UUID{otherAreaID}},
		},
		[]domain.Link{
			{ID: linkID, SourceDeviceID: baseID, TargetDeviceID: ghostID},
			{ID: excludedLinkID, SourceDeviceID: ghostID, TargetDeviceID: excludedID},
		},
		[]domain.AreaWithCount{
			{Area: domain.Area{ID: areaID, Name: "Core", Description: "Core area", Color: "#00E676"}},
			{Area: domain.Area{ID: otherAreaID, Name: "Edge", Description: "Edge area", Color: "#2979FF"}},
		},
		domain.CanvasMapFilter{
			AreaID:                &areaID,
			IncludeCrossAreaLinks: true,
			IncludeGhostDevices:   true,
		},
	)

	if got, want := len(membership.Devices), 2; got != want {
		t.Fatalf("device membership count = %d, want %d", got, want)
	}
	if membership.Devices[0].DeviceID != baseID || membership.Devices[0].Role != domain.CanvasMapDeviceRoleBase {
		t.Fatalf("first member = %+v, want base device %s", membership.Devices[0], baseID)
	}
	if got, want := membership.Devices[0].AreaIDs, []uuid.UUID{areaID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("base area IDs = %v, want %v", got, want)
	}
	if membership.Devices[1].DeviceID != ghostID || membership.Devices[1].Role != domain.CanvasMapDeviceRoleGhost {
		t.Fatalf("second member = %+v, want ghost device %s", membership.Devices[1], ghostID)
	}
	if got, want := membership.LinkIDs, []uuid.UUID{linkID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("link IDs = %v, want %v", got, want)
	}
	if got, want := len(membership.Areas), 1; got != want {
		t.Fatalf("area count = %d, want %d", got, want)
	}
	if membership.Areas[0].AreaID != areaID || membership.Areas[0].Name != "Core" {
		t.Fatalf("area snapshot = %+v, want Core area", membership.Areas[0])
	}
}

func TestMaterializeMembershipFromSourceMapUsesSourceAreaAssignments(t *testing.T) {
	areaID := uuid.New()
	baseID := uuid.New()
	ghostID := uuid.New()
	linkID := uuid.New()

	sourceMembership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: baseID, Role: domain.CanvasMapDeviceRoleBase, AreaIDs: []uuid.UUID{areaID}},
			{DeviceID: ghostID, Role: domain.CanvasMapDeviceRoleGhost},
		},
		LinkIDs: []uuid.UUID{linkID},
		Areas: []domain.CanvasMapAreaMembership{
			{AreaID: areaID, Name: "Map local", Description: "source snapshot", Color: "#FFAB00"},
		},
	}

	membership := MaterializeMembershipFromSourceMap(
		[]domain.Device{
			{ID: baseID, AreaIDs: []uuid.UUID{uuid.New()}},
			{ID: ghostID},
		},
		[]domain.Link{{ID: linkID, SourceDeviceID: baseID, TargetDeviceID: ghostID}},
		sourceMembership,
		sourceMembership.Areas,
		domain.CanvasMapFilter{DeviceIDs: []uuid.UUID{baseID}},
	)

	if got, want := len(membership.Devices), 1; got != want {
		t.Fatalf("device membership count = %d, want %d", got, want)
	}
	if got, want := membership.Devices[0].AreaIDs, []uuid.UUID{areaID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("source area IDs = %v, want %v", got, want)
	}
	if got, want := membership.Areas, sourceMembership.Areas; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("area snapshots = %+v, want %+v", got, want)
	}
}

func TestConnectedBaseLinkIDsIncludesNewDeviceAndDeduplicates(t *testing.T) {
	existingID := uuid.New()
	newID := uuid.New()
	ghostID := uuid.New()
	baseLinkID := uuid.New()
	ghostLinkID := uuid.New()

	got := ConnectedBaseLinkIDs(
		newID,
		domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{
				{DeviceID: existingID, Role: domain.CanvasMapDeviceRoleBase},
				{DeviceID: ghostID, Role: domain.CanvasMapDeviceRoleGhost},
			},
		},
		[]domain.Link{
			{ID: baseLinkID, SourceDeviceID: existingID, TargetDeviceID: newID},
			{ID: baseLinkID, SourceDeviceID: newID, TargetDeviceID: existingID},
			{ID: ghostLinkID, SourceDeviceID: newID, TargetDeviceID: ghostID},
		},
	)

	if want := []uuid.UUID{baseLinkID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("connected base link IDs = %v, want %v", got, want)
	}
}

func TestMissingLinkIDsKeepsOnlyCandidatesNotAlreadyInMembership(t *testing.T) {
	existingID := uuid.New()
	firstMissingID := uuid.New()
	secondMissingID := uuid.New()

	got := MissingLinkIDs(
		[]uuid.UUID{existingID},
		[]uuid.UUID{existingID, firstMissingID, firstMissingID, secondMissingID},
	)

	if want := []uuid.UUID{firstMissingID, firstMissingID, secondMissingID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("missing link IDs = %v, want %v", got, want)
	}
}

func TestDuplicateDeviceAddressDetectsExistingMemberAddress(t *testing.T) {
	newID := uuid.New()
	otherID := uuid.New()

	duplicate := HasDuplicateDeviceAddress(
		domain.Device{ID: newID, IP: " Router.EXAMPLE.com "},
		[]domain.Device{
			{ID: otherID, IP: "router.example.com"},
			{ID: uuid.New(), IP: "other.example.com"},
		},
	)

	if !duplicate {
		t.Fatal("HasDuplicateDeviceAddress() = false, want true")
	}

	if HasDuplicateDeviceAddress(domain.Device{ID: newID, IP: ""}, []domain.Device{{ID: otherID, IP: ""}}) {
		t.Fatal("HasDuplicateDeviceAddress() matched blank address, want false")
	}

	if HasDuplicateDeviceAddress(domain.Device{ID: newID, IP: "router.example.com"}, []domain.Device{{ID: newID, IP: "router.example.com"}}) {
		t.Fatal("HasDuplicateDeviceAddress() matched the same device, want false")
	}
}

func TestDuplicateDeviceAddressMessagePreservesHTTPErrorText(t *testing.T) {
	if got, want := DuplicateDeviceAddressMessage(" Router.EXAMPLE.com "), `a device with IP/host "Router.EXAMPLE.com" already exists in this map`; got != want {
		t.Fatalf("DuplicateDeviceAddressMessage() = %q, want %q", got, want)
	}
	if got, want := DuplicateDeviceAddressMessage(" "), "a device with that address already exists in this map"; got != want {
		t.Fatalf("DuplicateDeviceAddressMessage() = %q, want %q", got, want)
	}
}

func TestAreasToMembershipPreservesAreaSnapshots(t *testing.T) {
	firstID := uuid.New()
	secondID := uuid.New()

	got := AreasToMembership([]domain.Area{
		{ID: firstID, Name: "Core", Description: "Core area", Color: "#00E676"},
		{ID: secondID, Name: "Edge", Description: "Edge area", Color: "#2979FF"},
	})

	want := []domain.CanvasMapAreaMembership{
		{AreaID: firstID, Name: "Core", Description: "Core area", Color: "#00E676"},
		{AreaID: secondID, Name: "Edge", Description: "Edge area", Color: "#2979FF"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("AreasToMembership() = %+v, want %+v", got, want)
	}
}

func TestBaseDeviceMembershipCopiesAreaIDs(t *testing.T) {
	areaID := uuid.New()
	device := domain.Device{ID: uuid.New(), AreaIDs: []uuid.UUID{areaID}}

	got := BaseDeviceMembership(device)
	device.AreaIDs[0] = uuid.New()

	if got.DeviceID != device.ID {
		t.Fatalf("BaseDeviceMembership().DeviceID = %s, want %s", got.DeviceID, device.ID)
	}
	if got.Role != domain.CanvasMapDeviceRoleBase {
		t.Fatalf("BaseDeviceMembership().Role = %s, want base", got.Role)
	}
	if !uuidSlicesEqual(got.AreaIDs, []uuid.UUID{areaID}) {
		t.Fatalf("BaseDeviceMembership().AreaIDs = %v, want copied %s", got.AreaIDs, areaID)
	}
}

func TestShouldCopyDefaultPositionsSkipsDefaultMap(t *testing.T) {
	defaultMapID := uuid.New()
	if ShouldCopyDefaultPositions(defaultMapID, defaultMapID) {
		t.Fatal("ShouldCopyDefaultPositions() = true for default map, want false")
	}
	if !ShouldCopyDefaultPositions(uuid.New(), defaultMapID) {
		t.Fatal("ShouldCopyDefaultPositions() = false for non-default map, want true")
	}
}

func TestDefaultPositionCandidatesFallbackToLegacyOnlyWhenDefaultEmpty(t *testing.T) {
	defaultDeviceID := uuid.New()
	legacyDeviceID := uuid.New()
	defaultPositions := []domain.DevicePosition{{DeviceID: defaultDeviceID, X: 10, Y: 20}}
	legacyPositions := []domain.DevicePosition{{DeviceID: legacyDeviceID, X: 30, Y: 40}}

	got := DefaultPositionCandidates(defaultPositions, legacyPositions)
	if len(got) != 1 || got[0].DeviceID != defaultDeviceID {
		t.Fatalf("DefaultPositionCandidates() = %+v, want default positions", got)
	}

	got = DefaultPositionCandidates(nil, legacyPositions)
	if len(got) != 1 || got[0].DeviceID != legacyDeviceID {
		t.Fatalf("DefaultPositionCandidates() fallback = %+v, want legacy positions", got)
	}
}

func TestDefaultPositionsForMembershipPrunesNonMembers(t *testing.T) {
	memberID := uuid.New()
	otherID := uuid.New()
	positions := DefaultPositionsForMembership(
		[]domain.DevicePosition{
			{DeviceID: memberID, X: 10, Y: 20},
			{DeviceID: otherID, X: 30, Y: 40},
		},
		[]domain.CanvasMapDeviceMembership{{DeviceID: memberID}},
	)

	if len(positions) != 1 || positions[0].DeviceID != memberID {
		t.Fatalf("DefaultPositionsForMembership() = %+v, want only member position", positions)
	}
}

func TestValidateDeleteRejectsDefaultMap(t *testing.T) {
	err := ValidateDelete(domain.CanvasMap{IsDefault: true})
	if err == nil {
		t.Fatal("ValidateDelete() error = nil, want default map conflict")
	}
	if got, want := err.Error(), "cannot delete default canvas map"; got != want {
		t.Fatalf("ValidateDelete() error = %q, want %q", got, want)
	}
	if err := ValidateDelete(domain.CanvasMap{IsDefault: false}); err != nil {
		t.Fatalf("ValidateDelete() non-default error = %v", err)
	}
}

func TestPlanCreateEmptyMembership(t *testing.T) {
	plan := PlanCreate(domain.CanvasMapFilter{}, nil, nil)

	if !plan.CreateEmptyMembership {
		t.Fatal("PlanCreate().CreateEmptyMembership = false, want true")
	}
	if plan.SourceMapID != nil {
		t.Fatalf("PlanCreate().SourceMapID = %v, want nil", plan.SourceMapID)
	}
	if plan.PersistedSourceAreaID != nil {
		t.Fatalf("PlanCreate().PersistedSourceAreaID = %v, want nil", plan.PersistedSourceAreaID)
	}
	if plan.Filter.AreaID != nil {
		t.Fatalf("PlanCreate().Filter.AreaID = %v, want nil", plan.Filter.AreaID)
	}
}

func TestPlanCreateFromSourceAreaPersistsSourceAreaAndMaterializationFilter(t *testing.T) {
	areaID := uuid.New()

	plan := PlanCreate(domain.CanvasMapFilter{}, &areaID, nil)

	if plan.CreateEmptyMembership {
		t.Fatal("PlanCreate().CreateEmptyMembership = true, want false")
	}
	if plan.PersistedSourceAreaID == nil || *plan.PersistedSourceAreaID != areaID {
		t.Fatalf("PlanCreate().PersistedSourceAreaID = %v, want %s", plan.PersistedSourceAreaID, areaID)
	}
	if plan.Filter.AreaID == nil || *plan.Filter.AreaID != areaID {
		t.Fatalf("PlanCreate().Filter.AreaID = %v, want %s", plan.Filter.AreaID, areaID)
	}
}

func TestPlanCreateFromSourceMapDoesNotPersistSourceArea(t *testing.T) {
	areaID := uuid.New()
	sourceMapID := uuid.New()

	plan := PlanCreate(domain.CanvasMapFilter{}, &areaID, &sourceMapID)

	if plan.CreateEmptyMembership {
		t.Fatal("PlanCreate().CreateEmptyMembership = true, want false")
	}
	if plan.SourceMapID == nil || *plan.SourceMapID != sourceMapID {
		t.Fatalf("PlanCreate().SourceMapID = %v, want %s", plan.SourceMapID, sourceMapID)
	}
	if plan.PersistedSourceAreaID != nil {
		t.Fatalf("PlanCreate().PersistedSourceAreaID = %v, want nil for source map", plan.PersistedSourceAreaID)
	}
	if plan.Filter.AreaID == nil || *plan.Filter.AreaID != areaID {
		t.Fatalf("PlanCreate().Filter.AreaID = %v, want %s", plan.Filter.AreaID, areaID)
	}
}

func TestRemapPositionsForDeviceClonesPrunesNonMembers(t *testing.T) {
	originalID := uuid.New()
	cloneID := uuid.New()
	keptID := uuid.New()
	prunedID := uuid.New()

	got := RemapPositionsForDeviceClones(
		[]domain.DevicePosition{
			{DeviceID: originalID, X: 10, Y: 20},
			{DeviceID: keptID, X: 30, Y: 40},
			{DeviceID: prunedID, X: 50, Y: 60},
		},
		map[uuid.UUID]uuid.UUID{originalID: cloneID},
		[]domain.CanvasMapDeviceMembership{
			{DeviceID: cloneID},
			{DeviceID: keptID},
		},
	)

	want := []domain.DevicePosition{
		{DeviceID: cloneID, X: 10, Y: 20},
		{DeviceID: keptID, X: 30, Y: 40},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("RemapPositionsForDeviceClones() = %+v, want %+v", got, want)
	}
}

func uuidSlicesEqual(got, want []uuid.UUID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
