package canvasmap

// This file exercises add device plan behavior so refactors preserve the documented contract.

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// TestPlanAddDeviceMembershipAddsOnlyMissingLinksForExistingDevice preserves the incremental-link path for existing members.
func TestPlanAddDeviceMembershipAddsOnlyMissingLinksForExistingDevice(t *testing.T) {
	deviceID := uuid.New()
	existingBaseID := uuid.New()
	existingLinkID := uuid.New()
	missingLinkID := uuid.New()
	areaID := uuid.New()
	color := "#112233"
	membership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{
				DeviceID:    deviceID,
				Role:        domain.CanvasMapDeviceRoleGhost,
				AreaIDs:     []uuid.UUID{areaID},
				VisualColor: &color,
			},
			{DeviceID: existingBaseID, Role: domain.CanvasMapDeviceRoleBase},
		},
		LinkIDs: []uuid.UUID{existingLinkID},
		Areas: []domain.CanvasMapAreaMembership{
			{AreaID: areaID, Name: "Existing Area", Description: "map-local", Color: "#00E676"},
		},
	}

	plan, err := PlanAddDeviceMembership(
		domain.Device{ID: deviceID, IP: "10.0.0.10"},
		membership,
		nil,
		[]domain.Link{
			{ID: existingLinkID, SourceDeviceID: existingBaseID, TargetDeviceID: deviceID},
			{ID: missingLinkID, SourceDeviceID: deviceID, TargetDeviceID: existingBaseID},
		},
		nil,
		true,
	)

	if err != nil {
		t.Fatalf("PlanAddDeviceMembership() error = %v", err)
	}
	if plan.Device.DeviceID != deviceID || plan.Device.Role != domain.CanvasMapDeviceRoleGhost {
		t.Fatalf("planned device = %+v, want existing ghost membership", plan.Device)
	}
	if plan.Device.VisualColor == nil || *plan.Device.VisualColor != color {
		t.Fatalf("planned visual color = %v, want existing color %q", plan.Device.VisualColor, color)
	}
	if !uuidSlicesEqual(plan.LinkIDs, []uuid.UUID{missingLinkID}) {
		t.Fatalf("planned link IDs = %v, want only missing link %s", plan.LinkIDs, missingLinkID)
	}
	if len(plan.Areas) != 1 || plan.Areas[0].AreaID != areaID {
		t.Fatalf("planned areas = %+v, want existing map-local areas", plan.Areas)
	}
}

// TestPlanAddDeviceMembershipConflictsWhenExistingDeviceHasNoMissingLinks preserves the add-device conflict behavior.
func TestPlanAddDeviceMembershipConflictsWhenExistingDeviceHasNoMissingLinks(t *testing.T) {
	deviceID := uuid.New()
	linkID := uuid.New()
	membership := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceID, Role: domain.CanvasMapDeviceRoleBase},
		},
		LinkIDs: []uuid.UUID{linkID},
	}

	_, err := PlanAddDeviceMembership(
		domain.Device{ID: deviceID},
		membership,
		nil,
		[]domain.Link{{ID: linkID, SourceDeviceID: deviceID, TargetDeviceID: uuid.New()}},
		nil,
		true,
	)

	if !errors.Is(err, ErrDeviceAlreadyInCanvasMap) {
		t.Fatalf("PlanAddDeviceMembership() error = %v, want ErrDeviceAlreadyInCanvasMap", err)
	}
}

// TestPlanAddDeviceMembershipBuildsNewBaseMemberWithLinksAndAreas characterizes new-member materialization.
func TestPlanAddDeviceMembershipBuildsNewBaseMemberWithLinksAndAreas(t *testing.T) {
	deviceID := uuid.New()
	existingBaseID := uuid.New()
	ghostID := uuid.New()
	linkID := uuid.New()
	ghostLinkID := uuid.New()
	areaID := uuid.New()
	areas := []domain.CanvasMapAreaMembership{
		{AreaID: areaID, Name: "Device Area", Description: "from global area", Color: "#2979FF"},
	}
	device := domain.Device{
		ID:      deviceID,
		IP:      "10.0.0.11",
		AreaIDs: []uuid.UUID{areaID},
	}

	plan, err := PlanAddDeviceMembership(
		device,
		domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{
				{DeviceID: existingBaseID, Role: domain.CanvasMapDeviceRoleBase},
				{DeviceID: ghostID, Role: domain.CanvasMapDeviceRoleGhost},
			},
		},
		[]domain.Device{{ID: existingBaseID, IP: "10.0.0.10"}},
		[]domain.Link{
			{ID: linkID, SourceDeviceID: existingBaseID, TargetDeviceID: deviceID},
			{ID: ghostLinkID, SourceDeviceID: ghostID, TargetDeviceID: deviceID},
		},
		areas,
		true,
	)

	if err != nil {
		t.Fatalf("PlanAddDeviceMembership() error = %v", err)
	}
	if plan.Device.DeviceID != deviceID || plan.Device.Role != domain.CanvasMapDeviceRoleBase {
		t.Fatalf("planned device = %+v, want new base member", plan.Device)
	}
	if !uuidSlicesEqual(plan.Device.AreaIDs, []uuid.UUID{areaID}) {
		t.Fatalf("planned device areas = %v, want %s", plan.Device.AreaIDs, areaID)
	}
	if !uuidSlicesEqual(plan.LinkIDs, []uuid.UUID{linkID}) {
		t.Fatalf("planned link IDs = %v, want only base connected link %s", plan.LinkIDs, linkID)
	}
	if len(plan.Areas) != 1 || plan.Areas[0].AreaID != areaID {
		t.Fatalf("planned areas = %+v, want supplied area snapshot", plan.Areas)
	}
}

// TestPlanAddDeviceMembershipRejectsDuplicateDeviceAddress preserves duplicate-address conflict text.
func TestPlanAddDeviceMembershipRejectsDuplicateDeviceAddress(t *testing.T) {
	device := domain.Device{ID: uuid.New(), IP: " Router.EXAMPLE.com "}

	_, err := PlanAddDeviceMembership(
		device,
		domain.CanvasMapMembership{},
		[]domain.Device{{ID: uuid.New(), IP: "router.example.com"}},
		nil,
		nil,
		false,
	)

	var duplicateErr DuplicateDeviceAddressError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("PlanAddDeviceMembership() error = %T %[1]v, want DuplicateDeviceAddressError", err)
	}
	if got, want := err.Error(), `a device with IP/host "Router.EXAMPLE.com" already exists in this map`; got != want {
		t.Fatalf("PlanAddDeviceMembership() duplicate error = %q, want %q", got, want)
	}
}
