package api

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
	gossh "golang.org/x/crypto/ssh"
)

const duplicateAssignmentDriverName = "theia_duplicate_assignment_driver"

func init() {
	sql.Register(duplicateAssignmentDriverName, duplicateAssignmentDriver{})
}

type duplicateAssignmentDriver struct{}

func (duplicateAssignmentDriver) Open(string) (driver.Conn, error) {
	return duplicateAssignmentConn{}, nil
}

type duplicateAssignmentConn struct{}

func (duplicateAssignmentConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not implemented")
}

func (duplicateAssignmentConn) Close() error {
	return nil
}

func (duplicateAssignmentConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("begin not implemented")
}

func (duplicateAssignmentConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, fmt.Errorf(`ERROR: duplicate key value violates unique constraint "device_credential_profiles_pkey" (SQLSTATE 23505)`)
}

func seedDeviceCredentialProfileDevice(t *testing.T, db *sql.DB, hostname, ip string) uuid.UUID {
	t.Helper()
	deviceID := uuid.New()
	now := time.Now().UTC()
	_, err := db.Exec(
		`INSERT INTO devices (id, hostname, ip, device_type, status, sys_name, sys_descr,
		 sys_object_id, hardware_model, vendor, managed, snmp_credentials_json,
		 metrics_source, prometheus_label_name, prometheus_label_value, tags_json,
		 created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		deviceID.String(), hostname, ip, "router", "up", "",
		"", "", "", "default", 1, `{}`, "none", "", "", `{}`, now, now,
	)
	if err != nil {
		t.Fatalf("seeding device %q: %v", hostname, err)
	}
	return deviceID
}

// setupDeviceCredentialProfileTest creates an in-memory SQLite DB, runs migrations, seeds
// a device and a credential profile, and returns a fully-wired DeviceCredentialProfileHandler.
func setupDeviceCredentialProfileTest(t *testing.T) (
	*DeviceCredentialProfileHandler,
	*sqlite.CredentialProfileRepo,
	*sql.DB,
	uuid.UUID, // deviceID
	uuid.UUID, // profileID
	[]byte, // encKey (for test crypto ops)
) {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	credentialProfileRepo := sqlite.NewCredentialProfileRepo(db)
	encKey := crypto.DeriveKey("test-key-for-device-cred-tests")

	// Build minimal vendor registry
	defaultCfg := vendor.DBVendorRecord{
		Name: "default",
		ConfigJSON: `{
			"vendor": {"name": "default", "display_name": "Generic"},
			"detection": {},
			"backup": {"supported": false}
		}`,
	}
	reg, err := vendor.LoadRegistryFromDB([]vendor.DBVendorRecord{defaultCfg})
	if err != nil {
		t.Fatalf("building vendor registry: %v", err)
	}

	// Seed a device via SQL directly (device repo needs encryption key, use SQL for simplicity)
	deviceID := seedDeviceCredentialProfileDevice(t, db, "test-device", "192.168.1.1")

	// Seed a credential profile via repo
	profile := &domain.CredentialProfile{
		ID:          uuid.New(),
		Name:        "Test Profile",
		Description: "test",
		Username:    "admin",
		Port:        22,
		AuthMethod:  domain.SSHAuthPassword,
		Role:        "Admin",
	}
	if err := credentialProfileRepo.Create(profile); err != nil {
		t.Fatalf("seeding profile: %v", err)
	}
	profileID := profile.ID

	// Build BackupService (uses domain.DeviceRepository interface)
	deviceRepo := sqlite.NewDeviceRepo(db, encKey, make(chan struct{}, 1))

	backupSvc := service.NewBackupService(
		newMockBackupJobRepo(),
		newMockBackupFileRepo(),
		credentialProfileRepo,
		deviceRepo,
		newMockSettingsRepo(),
		reg,
		&mockSSHDialer{},
		encKey,
		t.TempDir(),
		gossh.InsecureIgnoreHostKey(),
	)

	handler := NewDeviceCredentialProfileHandler(backupSvc, credentialProfileRepo)
	return handler, credentialProfileRepo, db, deviceID, profileID, encKey
}

// --- HandleListAssignments ---

func TestDeviceCredentialProfile_ListAssignments_Empty(t *testing.T) {
	handler, _, _, deviceID, _, _ := setupDeviceCredentialProfileTest(t)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/devices/%s/credential-profiles", deviceID), nil)
	rec := httptest.NewRecorder()
	handler.HandleListAssignments(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["data"]; !ok {
		t.Fatal("expected 'data' key in response")
	}
	var data []interface{}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty array, got %d items", len(data))
	}
}

func TestDeviceCredentialProfile_ListAssignments_AfterAssign(t *testing.T) {
	handler, repo, _, deviceID, profileID, _ := setupDeviceCredentialProfileTest(t)

	// Assign first
	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("assign profile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/devices/%s/credential-profiles", deviceID), nil)
	rec := httptest.NewRecorder()
	handler.HandleListAssignments(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var data []map[string]interface{}
	if err := json.Unmarshal(resp["data"], &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(data))
	}
	if _, ok := data[0]["is_winbox"]; !ok {
		t.Fatal("expected is_winbox field in response")
	}
}

// --- HandleAssign ---

func TestDeviceCredentialProfile_Assign_HappyPath(t *testing.T) {
	handler, _, _, deviceID, profileID, _ := setupDeviceCredentialProfileTest(t)

	body := fmt.Sprintf(`{"profile_id":"%s"}`, profileID)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/devices/%s/credential-profiles", deviceID),
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleAssign(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["data"]["profile_id"]; !ok {
		t.Fatal("expected profile_id in data")
	}
}

func TestDeviceCredentialProfile_Assign_SameProfileToMultipleDevices(t *testing.T) {
	handler, repo, db, deviceID, profileID, _ := setupDeviceCredentialProfileTest(t)
	otherDeviceID := seedDeviceCredentialProfileDevice(t, db, "test-device-2", "192.168.1.2")

	for _, targetDeviceID := range []uuid.UUID{deviceID, otherDeviceID} {
		body := fmt.Sprintf(`{"profile_id":"%s"}`, profileID)
		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/v1/devices/%s/credential-profiles", targetDeviceID),
			strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler.HandleAssign(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 for device %s, got %d; body: %s", targetDeviceID, rec.Code, rec.Body.String())
		}
	}

	for _, targetDeviceID := range []uuid.UUID{deviceID, otherDeviceID} {
		rows, err := repo.ListAssignedProfiles(targetDeviceID)
		if err != nil {
			t.Fatalf("listing assignments for device %s: %v", targetDeviceID, err)
		}
		if len(rows) != 1 || rows[0].ProfileID != profileID {
			t.Fatalf("expected profile %s assigned to device %s, got %+v", profileID, targetDeviceID, rows)
		}
	}
}

func TestDeviceCredentialProfile_Assign_Duplicate_IsIdempotent(t *testing.T) {
	handler, repo, _, deviceID, profileID, _ := setupDeviceCredentialProfileTest(t)

	// Assign once
	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("pre-assign profile: %v", err)
	}

	// Try to assign again
	body := fmt.Sprintf(`{"profile_id":"%s"}`, profileID)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/devices/%s/credential-profiles", deviceID),
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleAssign(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestDeviceCredentialProfile_Assign_PostgresDuplicate_IsIdempotent(t *testing.T) {
	db, err := sql.Open(duplicateAssignmentDriverName, "")
	if err != nil {
		t.Fatalf("opening duplicate assignment driver: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	handler := NewDeviceCredentialProfileHandler(nil, sqlite.NewCredentialProfileRepo(db))
	deviceID := uuid.New()
	profileID := uuid.New()

	body := fmt.Sprintf(`{"profile_id":"%s"}`, profileID)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/devices/%s/credential-profiles", deviceID),
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleAssign(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), profileID.String()) {
		t.Fatalf("expected assigned profile response, got body: %s", rec.Body.String())
	}
}

func TestDeviceCredentialProfile_Assign_MissingProfileID_Returns400(t *testing.T) {
	handler, _, _, deviceID, _, _ := setupDeviceCredentialProfileTest(t)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/devices/%s/credential-profiles", deviceID),
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleAssign(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// --- HandleUnassign ---

func TestDeviceCredentialProfile_Unassign_HappyPath(t *testing.T) {
	handler, repo, _, deviceID, profileID, _ := setupDeviceCredentialProfileTest(t)

	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("assign profile: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/devices/%s/credential-profiles/%s", deviceID, profileID), nil)
	rec := httptest.NewRecorder()
	handler.HandleUnassign(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestDeviceCredentialProfile_Unassign_NotAssigned_Returns404(t *testing.T) {
	handler, _, _, deviceID, profileID, _ := setupDeviceCredentialProfileTest(t)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/devices/%s/credential-profiles/%s", deviceID, profileID), nil)
	rec := httptest.NewRecorder()
	handler.HandleUnassign(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// --- HandleSetWinbox ---

func TestDeviceCredentialProfile_SetWinbox_HappyPath(t *testing.T) {
	handler, repo, _, deviceID, profileID, _ := setupDeviceCredentialProfileTest(t)

	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("assign profile: %v", err)
	}

	body := fmt.Sprintf(`{"profile_id":"%s"}`, profileID)
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/v1/devices/%s/winbox-profile", deviceID),
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleSetWinbox(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["data"]["is_winbox"] != true {
		t.Fatalf("expected is_winbox=true, got %v", resp["data"]["is_winbox"])
	}
}

func TestDeviceCredentialProfile_SetWinbox_NotAssigned_Returns404(t *testing.T) {
	handler, _, _, deviceID, profileID, _ := setupDeviceCredentialProfileTest(t)

	body := fmt.Sprintf(`{"profile_id":"%s"}`, profileID)
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/v1/devices/%s/winbox-profile", deviceID),
		strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleSetWinbox(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// --- HandleClearWinbox ---

func TestDeviceCredentialProfile_ClearWinbox_Idempotent(t *testing.T) {
	handler, _, _, deviceID, _, _ := setupDeviceCredentialProfileTest(t)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/devices/%s/winbox-profile", deviceID), nil)
	rec := httptest.NewRecorder()
	handler.HandleClearWinbox(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// --- HandleGetWinboxCredentials / HandleRevealWinboxCredentials ---

func TestDeviceCredentialProfile_GetWinboxCredentials_Gone(t *testing.T) {
	handler, _, _, deviceID, _, _ := setupDeviceCredentialProfileTest(t)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/devices/%s/winbox-credentials", deviceID), nil)
	rec := httptest.NewRecorder()
	handler.HandleGetWinboxCredentials(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestDeviceCredentialProfile_RevealWinboxCredentials_HappyPath(t *testing.T) {
	handler, repo, db, deviceID, profileID, encKey := setupDeviceCredentialProfileTest(t)

	// Update the profile to have an encrypted password
	encryptedPwd, err := crypto.Encrypt([]byte("test-pass"), encKey)
	if err != nil {
		t.Fatalf("encrypt password: %v", err)
	}
	_, err = db.Exec(
		`UPDATE credential_profiles SET encrypted_secret = ? WHERE id = ?`,
		string(encryptedPwd), profileID.String(),
	)
	if err != nil {
		t.Fatalf("update profile secret: %v", err)
	}

	// Assign and set as WinBox
	if err := repo.AssignProfile(deviceID, profileID); err != nil {
		t.Fatalf("assign profile: %v", err)
	}
	if err := repo.SetWinboxProfile(deviceID, profileID); err != nil {
		t.Fatalf("set winbox profile: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/devices/%s/winbox-credentials/reveal", deviceID),
		strings.NewReader(`{"reason":"manual diagnostic reveal"}`))
	rec := httptest.NewRecorder()
	handler.HandleRevealWinboxCredentials(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", got)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["password"] != "test-pass" {
		t.Fatalf("expected password='test-pass', got %v", data["password"])
	}
	if data["ip"] != "192.168.1.1" {
		t.Fatalf("expected ip='192.168.1.1', got %v", data["ip"])
	}
	if data["username"] != "admin" {
		t.Fatalf("expected username='admin', got %v", data["username"])
	}
}

func TestDeviceCredentialProfile_RevealWinboxCredentials_RequiresReason(t *testing.T) {
	handler, _, _, deviceID, _, _ := setupDeviceCredentialProfileTest(t)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/devices/%s/winbox-credentials/reveal", deviceID),
		strings.NewReader(`{"reason":" "}`))
	rec := httptest.NewRecorder()
	handler.HandleRevealWinboxCredentials(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestDeviceCredentialProfile_RevealWinboxCredentials_GetReturns405(t *testing.T) {
	handler, _, _, deviceID, _, _ := setupDeviceCredentialProfileTest(t)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/devices/%s/winbox-credentials/reveal", deviceID), nil)
	rec := httptest.NewRecorder()
	handler.HandleRevealWinboxCredentials(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRouterPath_WinboxCredentialsRevealMatchesAnyMethod(t *testing.T) {
	deviceID := uuid.New()
	if !isWinboxCredentialsRevealPath("/api/v1/devices/" + deviceID.String() + "/winbox-credentials/reveal") {
		t.Fatal("expected exact WinBox credentials reveal path to match")
	}
	if isWinboxCredentialsRevealPath("/api/v1/devices/" + deviceID.String() + "/winbox-credentials/reveal/extra") {
		t.Fatal("expected reveal path with extra segment not to match")
	}
}

func TestDeviceCredentialProfile_RevealWinboxCredentials_NoWinboxProfile_Returns404(t *testing.T) {
	handler, _, _, deviceID, _, _ := setupDeviceCredentialProfileTest(t)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/devices/%s/winbox-credentials/reveal", deviceID),
		strings.NewReader(`{"reason":"manual diagnostic reveal"}`))
	rec := httptest.NewRecorder()
	handler.HandleRevealWinboxCredentials(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
