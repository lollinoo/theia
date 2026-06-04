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
