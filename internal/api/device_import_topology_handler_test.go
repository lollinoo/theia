package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type fakeDeviceImportTopologyProvider struct {
	snapshot          domain.DeviceImportTopologyRunSnapshot
	getRunID          uuid.UUID
	getActiveMapID    uuid.UUID
	actorID           uuid.UUID
	retryRunID        uuid.UUID
	retryDeviceIDs    []uuid.UUID
	continuedRunID    uuid.UUID
	manualEditedRunID uuid.UUID
	layoutRunID       uuid.UUID
	layoutRequest     domain.DeviceImportTopologyLayoutApply
}

func (f *fakeDeviceImportTopologyProvider) GetRun(
	_ context.Context,
	runID, actorID uuid.UUID,
) (domain.DeviceImportTopologyRunSnapshot, error) {
	f.getRunID = runID
	f.actorID = actorID
	return f.snapshot, nil
}

func (f *fakeDeviceImportTopologyProvider) GetActiveRun(
	_ context.Context,
	mapID, actorID uuid.UUID,
) (domain.DeviceImportTopologyRunSnapshot, error) {
	f.getActiveMapID = mapID
	f.actorID = actorID
	return f.snapshot, nil
}

func (f *fakeDeviceImportTopologyProvider) Retry(
	_ context.Context,
	runID, actorID uuid.UUID,
	deviceIDs []uuid.UUID,
) error {
	f.retryRunID = runID
	f.actorID = actorID
	f.retryDeviceIDs = append([]uuid.UUID(nil), deviceIDs...)
	return nil
}

func (f *fakeDeviceImportTopologyProvider) Continue(_ context.Context, runID, actorID uuid.UUID) error {
	f.continuedRunID = runID
	f.actorID = actorID
	return nil
}

func (f *fakeDeviceImportTopologyProvider) MarkManualEdit(_ context.Context, runID, actorID uuid.UUID) error {
	f.manualEditedRunID = runID
	f.actorID = actorID
	return nil
}

func (f *fakeDeviceImportTopologyProvider) ApplyLayout(
	_ context.Context,
	runID, actorID uuid.UUID,
	request domain.DeviceImportTopologyLayoutApply,
) error {
	f.layoutRunID = runID
	f.actorID = actorID
	f.layoutRequest = request
	return nil
}

func TestDeviceImportTopologyHandlerGetsActiveRunForActor(t *testing.T) {
	mapID := uuid.New()
	runID := uuid.New()
	provider := &fakeDeviceImportTopologyProvider{snapshot: domain.DeviceImportTopologyRunSnapshot{
		Run: domain.DeviceImportTopologyRun{
			ID: runID, MapID: mapID, ActorUserID: uuid.New(),
			State: domain.DeviceImportTopologyRunStateDiscovering,
		},
		Items: []domain.DeviceImportTopologyRunItem{{
			DeviceID: uuid.New(), State: domain.DeviceImportTopologyItemStateRunning,
		}},
	}}
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/device-imports/topology-runs/active?map_id="+mapID.String(),
		nil,
	)
	user, auth := authorizeDeviceImportRequest(request, domain.PermissionCredentialsRead)
	response := httptest.NewRecorder()

	NewDeviceImportTopologyHandler(provider, auth).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if provider.getActiveMapID != mapID || provider.actorID != user.User.User.ID {
		t.Fatalf("provider map=%s actor=%s", provider.getActiveMapID, provider.actorID)
	}
	if strings.Contains(response.Body.String(), provider.snapshot.Run.ActorUserID.String()) {
		t.Fatalf("response exposed actor id: %s", response.Body.String())
	}
	var snapshot domain.DeviceImportTopologyRunSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapshot.Run.ID != runID || len(snapshot.Items) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestDeviceImportTopologyHandlerDispatchesActorScopedControls(t *testing.T) {
	runID := uuid.New()
	deviceID := uuid.New()
	tests := []struct {
		name   string
		action string
		body   string
		assert func(*testing.T, *fakeDeviceImportTopologyProvider)
	}{
		{
			name: "retry", action: "retry", body: `{"device_ids":["` + deviceID.String() + `"]}`,
			assert: func(t *testing.T, provider *fakeDeviceImportTopologyProvider) {
				if provider.retryRunID != runID || len(provider.retryDeviceIDs) != 1 || provider.retryDeviceIDs[0] != deviceID {
					t.Fatalf("retry run=%s devices=%v", provider.retryRunID, provider.retryDeviceIDs)
				}
			},
		},
		{
			name: "continue", action: "continue",
			assert: func(t *testing.T, provider *fakeDeviceImportTopologyProvider) {
				if provider.continuedRunID != runID {
					t.Fatalf("continued run=%s", provider.continuedRunID)
				}
			},
		},
		{
			name: "manual edit", action: "manual-edit",
			assert: func(t *testing.T, provider *fakeDeviceImportTopologyProvider) {
				if provider.manualEditedRunID != runID {
					t.Fatalf("manual-edit run=%s", provider.manualEditedRunID)
				}
			},
		},
		{
			name: "layout", action: "layout",
			body: `{"input_token":"sha256:layout","positions":[{"device_id":"` + deviceID.String() + `","x":120,"y":240,"pinned":false}],"reset_link_route_ids":[]}`,
			assert: func(t *testing.T, provider *fakeDeviceImportTopologyProvider) {
				if provider.layoutRunID != runID || provider.layoutRequest.InputToken != "sha256:layout" ||
					len(provider.layoutRequest.Positions) != 1 || provider.layoutRequest.Positions[0].DeviceID != deviceID {
					t.Fatalf("layout run=%s request=%#v", provider.layoutRunID, provider.layoutRequest)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeDeviceImportTopologyProvider{}
			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/admin/device-imports/topology-runs/"+runID.String()+"/"+tt.action,
				strings.NewReader(tt.body),
			)
			request.Header.Set("Content-Type", "application/json")
			user, auth := authorizeDeviceImportRequest(request, domain.PermissionCredentialsRead)
			response := httptest.NewRecorder()

			NewDeviceImportTopologyHandler(provider, auth).ServeHTTP(response, request)

			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204; body=%s", response.Code, response.Body.String())
			}
			if provider.actorID != user.User.User.ID {
				t.Fatalf("provider actor=%s", provider.actorID)
			}
			tt.assert(t, provider)
		})
	}
}
