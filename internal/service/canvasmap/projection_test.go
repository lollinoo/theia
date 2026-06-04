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

func TestRemapLinkForDeviceClonesPreservesLinkDetails(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()
	cloneID := uuid.New()
	linkID := uuid.New()
	link := domain.Link{
		ID:                linkID,
		SourceDeviceID:    sourceID,
		SourceIfName:      "ether1",
		TargetDeviceID:    targetID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: "lldp",
	}

	remapped, cloned := RemapLinkForDeviceClones(link, map[uuid.UUID]uuid.UUID{sourceID: cloneID})

	if !cloned {
		t.Fatal("RemapLinkForDeviceClones() cloned = false, want true")
	}
	if remapped.ID != linkID || remapped.SourceDeviceID != cloneID || remapped.TargetDeviceID != targetID ||
		remapped.SourceIfName != "ether1" || remapped.TargetIfName != "ether2" || remapped.DiscoveryProtocol != "lldp" {
		t.Fatalf("RemapLinkForDeviceClones() = %+v, want source clone with details preserved", remapped)
	}

	unchanged, cloned := RemapLinkForDeviceClones(link, map[uuid.UUID]uuid.UUID{})
	if cloned {
		t.Fatal("RemapLinkForDeviceClones() cloned = true for unchanged link, want false")
	}
	if unchanged != link {
		t.Fatalf("RemapLinkForDeviceClones() unchanged = %+v, want %+v", unchanged, link)
	}
}

func TestVirtualMemberDeviceIDsKeepsOnlyVirtualMembers(t *testing.T) {
	virtualID := uuid.New()
	physicalID := uuid.New()

	got, err := VirtualMemberDeviceIDs(
		domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{
				{DeviceID: physicalID, Role: domain.CanvasMapDeviceRoleBase},
				{DeviceID: virtualID, Role: domain.CanvasMapDeviceRoleGhost},
			},
		},
		[]domain.Device{
			{ID: virtualID, DeviceType: domain.DeviceTypeVirtual},
			{ID: physicalID, DeviceType: domain.DeviceTypeRouter},
		},
	)

	if err != nil {
		t.Fatalf("VirtualMemberDeviceIDs() error = %v", err)
	}
	if _, ok := got[virtualID]; !ok || len(got) != 1 {
		t.Fatalf("VirtualMemberDeviceIDs() = %v, want only virtual device %s", got, virtualID)
	}
}

func TestVirtualMemberDeviceIDsFailsClosedForMissingMemberDevice(t *testing.T) {
	missingID := uuid.New()

	_, err := VirtualMemberDeviceIDs(
		domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{{DeviceID: missingID}},
		},
		nil,
	)

	if err == nil {
		t.Fatal("VirtualMemberDeviceIDs() error = nil, want missing member device error")
	}
	if got, want := err.Error(), "canvas map member device "+missingID.String()+" not found"; got != want {
		t.Fatalf("VirtualMemberDeviceIDs() error = %q, want %q", got, want)
	}
}

func TestVirtualDeviceCloneCandidatesKeepsSharedVirtualMembersInMembershipOrder(t *testing.T) {
	firstVirtualID := uuid.New()
	secondVirtualID := uuid.New()
	physicalID := uuid.New()
	unsharedVirtualID := uuid.New()

	got, err := VirtualDeviceCloneCandidates(
		domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{
				{DeviceID: secondVirtualID},
				{DeviceID: physicalID},
				{DeviceID: unsharedVirtualID},
				{DeviceID: firstVirtualID},
			},
		},
		[]domain.Device{
			{ID: firstVirtualID, Hostname: "first", DeviceType: domain.DeviceTypeVirtual},
			{ID: secondVirtualID, Hostname: "second", DeviceType: domain.DeviceTypeVirtual},
			{ID: physicalID, Hostname: "physical", DeviceType: domain.DeviceTypeRouter},
			{ID: unsharedVirtualID, Hostname: "unshared", DeviceType: domain.DeviceTypeVirtual},
		},
		map[uuid.UUID]struct{}{
			firstVirtualID:  {},
			secondVirtualID: {},
			physicalID:      {},
		},
	)

	if err != nil {
		t.Fatalf("VirtualDeviceCloneCandidates() error = %v", err)
	}
	if gotIDs, want := deviceIDs(got), []uuid.UUID{secondVirtualID, firstVirtualID}; !uuidSlicesEqual(gotIDs, want) {
		t.Fatalf("VirtualDeviceCloneCandidates() IDs = %v, want %v", gotIDs, want)
	}
}

func TestMembershipWithDeviceClonesCopiesMembershipAndSwapsCloneIDs(t *testing.T) {
	originalID := uuid.New()
	cloneID := uuid.New()
	keptID := uuid.New()
	areaID := uuid.New()
	linkID := uuid.New()
	color := "#AABBCC"

	membership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{
				DeviceID:    originalID,
				Role:        domain.CanvasMapDeviceRoleGhost,
				AreaIDs:     []uuid.UUID{areaID},
				VisualColor: &color,
			},
			{DeviceID: keptID, Role: domain.CanvasMapDeviceRoleBase},
		},
		LinkIDs: []uuid.UUID{linkID},
		Areas: []domain.CanvasMapAreaMembership{
			{AreaID: areaID, Name: "Core", Description: "source area", Color: "#00E676"},
		},
	}

	got := MembershipWithDeviceClones(membership, map[uuid.UUID]uuid.UUID{originalID: cloneID})
	membership.Devices[0].DeviceID = uuid.New()
	membership.Devices[0].AreaIDs[0] = uuid.New()
	*membership.Devices[0].VisualColor = "#000000"
	membership.LinkIDs[0] = uuid.New()
	membership.Areas[0].Name = "mutated"

	if got.Devices[0].DeviceID != cloneID {
		t.Fatalf("MembershipWithDeviceClones() first device = %s, want clone %s", got.Devices[0].DeviceID, cloneID)
	}
	if got.Devices[0].Role != domain.CanvasMapDeviceRoleGhost {
		t.Fatalf("MembershipWithDeviceClones() first role = %s, want ghost", got.Devices[0].Role)
	}
	if !uuidSlicesEqual(got.Devices[0].AreaIDs, []uuid.UUID{areaID}) {
		t.Fatalf("MembershipWithDeviceClones() area IDs = %v, want copied %s", got.Devices[0].AreaIDs, areaID)
	}
	if got.Devices[0].VisualColor == nil || *got.Devices[0].VisualColor != "#AABBCC" {
		t.Fatalf("MembershipWithDeviceClones() visual color = %v, want copied #AABBCC", got.Devices[0].VisualColor)
	}
	if got.Devices[0].VisualColor == membership.Devices[0].VisualColor {
		t.Fatal("MembershipWithDeviceClones() reused visual color pointer, want copy")
	}
	if got.Devices[1].DeviceID != keptID {
		t.Fatalf("MembershipWithDeviceClones() second device = %s, want kept %s", got.Devices[1].DeviceID, keptID)
	}
	if !uuidSlicesEqual(got.LinkIDs, []uuid.UUID{linkID}) {
		t.Fatalf("MembershipWithDeviceClones() link IDs = %v, want copied %s", got.LinkIDs, linkID)
	}
	if len(got.Areas) != 1 || got.Areas[0].Name != "Core" {
		t.Fatalf("MembershipWithDeviceClones() areas = %+v, want copied source area", got.Areas)
	}
}

func TestBuildMaterializedTopologyResponsePlanKeepsCountsPositionsGhostsAndVisualMetadata(t *testing.T) {
	areaID := uuid.New()
	baseID := uuid.New()
	ghostID := uuid.New()
	prunedID := uuid.New()
	linkID := uuid.New()
	baseColor := "#112233"
	ghostColor := "#445566"

	plan := BuildMaterializedTopologyResponsePlan(
		domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{
				{
					DeviceID:    baseID,
					Role:        domain.CanvasMapDeviceRoleBase,
					AreaIDs:     []uuid.UUID{areaID},
					VisualColor: &baseColor,
				},
				{
					DeviceID:    ghostID,
					Role:        domain.CanvasMapDeviceRoleGhost,
					VisualColor: &ghostColor,
				},
			},
			LinkIDs: []uuid.UUID{linkID},
			Areas: []domain.CanvasMapAreaMembership{
				{AreaID: areaID, Name: "Core", Description: "core area", Color: "#00E676"},
			},
		},
		[]domain.Device{
			{ID: ghostID, Hostname: "ghost", AreaIDs: []uuid.UUID{uuid.New()}},
			{ID: baseID, Hostname: "base", AreaIDs: []uuid.UUID{uuid.New()}},
		},
		[]domain.Link{{ID: linkID, SourceDeviceID: baseID, TargetDeviceID: ghostID}},
		[]domain.DevicePosition{
			{DeviceID: baseID, X: 10, Y: 20, Pinned: true},
			{DeviceID: ghostID, X: 30, Y: 40},
			{DeviceID: prunedID, X: 50, Y: 60},
		},
	)

	if got, want := plan.DeviceCount, 1; got != want {
		t.Fatalf("DeviceCount = %d, want %d", got, want)
	}
	if got, want := plan.LinkCount, 1; got != want {
		t.Fatalf("LinkCount = %d, want %d", got, want)
	}
	if got, want := plan.PositionCount, 2; got != want {
		t.Fatalf("PositionCount = %d, want %d", got, want)
	}
	if got, want := deviceIDs(plan.Devices), []uuid.UUID{baseID, ghostID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("Devices = %v, want base then ghost %v", got, want)
	}
	if got, want := len(plan.Links), 1; got != want || plan.Links[0].ID != linkID {
		t.Fatalf("Links = %+v, want link %s", plan.Links, linkID)
	}
	if got, want := positionDeviceIDs(plan.Positions), []uuid.UUID{baseID, ghostID}; !uuidSlicesEqual(got, want) {
		t.Fatalf("Positions = %v, want %v", got, want)
	}
	if len(plan.Areas) != 1 || plan.Areas[0].ID != areaID || plan.Areas[0].DeviceCount != 1 {
		t.Fatalf("Areas = %+v, want area count for base device", plan.Areas)
	}
	if got := plan.VisualColors[baseID]; got != baseColor {
		t.Fatalf("VisualColors[%s] = %q, want %q", baseID, got, baseColor)
	}
	if got := plan.VisualColors[ghostID]; got != ghostColor {
		t.Fatalf("VisualColors[%s] = %q, want %q", ghostID, got, ghostColor)
	}
}

func TestEmptyTopologyResponsePlanReturnsEmptyValues(t *testing.T) {
	plan := EmptyTopologyResponsePlan()

	if plan.DeviceCount != 0 || plan.LinkCount != 0 || plan.PositionCount != 0 {
		t.Fatalf("empty counts = devices %d links %d positions %d, want zero", plan.DeviceCount, plan.LinkCount, plan.PositionCount)
	}
	if len(plan.Devices) != 0 || len(plan.Links) != 0 || len(plan.Positions) != 0 || len(plan.Areas) != 0 {
		t.Fatalf("empty plan = %+v, want empty slices", plan)
	}
	if len(plan.VisualColors) != 0 {
		t.Fatalf("empty VisualColors = %v, want empty map", plan.VisualColors)
	}
}

func deviceIDs(devices []domain.Device) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(devices))
	for _, device := range devices {
		ids = append(ids, device.ID)
	}
	return ids
}

func positionDeviceIDs(positions []domain.DevicePosition) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(positions))
	for _, position := range positions {
		ids = append(ids, position.DeviceID)
	}
	return ids
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
