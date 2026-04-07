package api

import (
	"database/sql"
	"encoding/json"
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

	_ "github.com/mattn/go-sqlite3"
)

// --- mock BackupJobRepo (minimal, satisfies domain.BackupJobRepository) ---

type mockBackupJobRepo struct {
	jobs map[uuid.UUID]*domain.BackupJob
}

func newMockBackupJobRepo() *mockBackupJobRepo {
	return &mockBackupJobRepo{jobs: make(map[uuid.UUID]*domain.BackupJob)}
}

func (r *mockBackupJobRepo) Create(job *domain.BackupJob) error          { r.jobs[job.ID] = job; return nil }
func (r *mockBackupJobRepo) GetByID(id uuid.UUID) (*domain.BackupJob, error) {
	j, ok := r.jobs[id]
	if !ok {
		return nil, nil
	}
	return j, nil
}
func (r *mockBackupJobRepo) GetByDeviceID(uuid.UUID) ([]domain.BackupJob, error) {
	return nil, nil
}
func (r *mockBackupJobRepo) GetLatestByDeviceID(uuid.UUID) (*domain.BackupJob, error) {
	return nil, nil
}
func (r *mockBackupJobRepo) Update(job *domain.BackupJob) error { r.jobs[job.ID] = job; return nil }
func (r *mockBackupJobRepo) Delete(id uuid.UUID) error          { delete(r.jobs, id); return nil }
func (r *mockBackupJobRepo) DeleteByDeviceID(uuid.UUID) error   { return nil }
func (r *mockBackupJobRepo) ListSuccessfulByDeviceOldest(uuid.UUID) ([]domain.BackupJob, error) {
	return nil, nil
}
func (r *mockBackupJobRepo) ListAllDeviceIDs() ([]uuid.UUID, error) { return nil, nil }
func (r *mockBackupJobRepo) DeleteFailedOlderThan(time.Time) (int, error) {
	return 0, nil
}

// --- mock BackupFileRepo (minimal, satisfies domain.BackupFileRepository) ---

type mockBackupFileRepo struct {
	files map[uuid.UUID]*domain.BackupFile
}

func newMockBackupFileRepo() *mockBackupFileRepo {
	return &mockBackupFileRepo{files: make(map[uuid.UUID]*domain.BackupFile)}
}

func (r *mockBackupFileRepo) Create(f *domain.BackupFile) error { r.files[f.ID] = f; return nil }
func (r *mockBackupFileRepo) GetByJobID(uuid.UUID) ([]domain.BackupFile, error) {
	return nil, nil
}
func (r *mockBackupFileRepo) GetByID(id uuid.UUID) (*domain.BackupFile, error) {
	f, ok := r.files[id]
	if !ok {
		return nil, nil
	}
	return f, nil
}
func (r *mockBackupFileRepo) DeleteByJobID(uuid.UUID) error { return nil }

// --- mock ssh.Dialer (no-op, never called in profile CRUD tests) ---

type mockSSHDialer struct{}

func (d *mockSSHDialer) Dial(addr string, config *gossh.ClientConfig) (*gossh.Client, error) {
	return nil, nil
}

// setupCredentialProfileTest creates an in-memory SQLite DB, runs migrations, and
// builds a fully-wired CredentialProfileHandler backed by a real CredentialProfileRepo.
func setupCredentialProfileTest(t *testing.T) (*CredentialProfileHandler, *sqlite.CredentialProfileRepo, *sql.DB) {
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
	encKey := crypto.DeriveKey("test-key-for-handler-tests")

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

	backupSvc := service.NewBackupService(
		newMockBackupJobRepo(),
		newMockBackupFileRepo(),
		credentialProfileRepo,
		newMockDeviceRepo(),
		newMockSettingsRepo(),
		reg,
		&mockSSHDialer{},
		encKey,
		t.TempDir(),
		gossh.InsecureIgnoreHostKey(),
	)

	handler := NewCredentialProfileHandler(backupSvc, credentialProfileRepo)
	return handler, credentialProfileRepo, db
}

func TestCredentialProfileHandlerList(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ssh-profiles", nil)
	rec := httptest.NewRecorder()
	handler.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["data"]; !ok {
		t.Fatal("expected 'data' key in response")
	}
}

func TestCredentialProfileHandlerCreate_HappyPath(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	body := `{"name":"test-profile","username":"admin","port":22,"auth_method":"password","secret":"s3cret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["data"]; !ok {
		t.Fatal("expected 'data' key in response")
	}
}

func TestCredentialProfileHandlerCreate_MalformedJSON(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(`{invalid`))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCredentialProfileHandlerGet_HappyPath(t *testing.T) {
	handler, repo, _ := setupCredentialProfileTest(t)

	// Seed a profile via the repo
	profile := &domain.CredentialProfile{
		Name:       "get-test",
		Username:   "admin",
		Port:       22,
		AuthMethod: domain.SSHAuthPassword,
		Role:       "Admin",
	}
	if err := repo.Create(profile); err != nil {
		t.Fatalf("seeding profile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ssh-profiles/"+profile.ID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCredentialProfileHandlerGet_NotFound(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	randomID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ssh-profiles/"+randomID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleGet(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCredentialProfileHandlerUpdate_HappyPath(t *testing.T) {
	handler, repo, _ := setupCredentialProfileTest(t)

	profile := &domain.CredentialProfile{
		Name:       "update-test",
		Username:   "admin",
		Port:       22,
		AuthMethod: domain.SSHAuthPassword,
		Role:       "Admin",
	}
	if err := repo.Create(profile); err != nil {
		t.Fatalf("seeding profile: %v", err)
	}

	body := `{"name":"updated-name","username":"root","port":2222,"auth_method":"password"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/ssh-profiles/"+profile.ID.String(), strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCredentialProfileHandlerDelete_HappyPath(t *testing.T) {
	handler, repo, _ := setupCredentialProfileTest(t)

	profile := &domain.CredentialProfile{
		Name:       "delete-test",
		Username:   "admin",
		Port:       22,
		AuthMethod: domain.SSHAuthPassword,
		Role:       "Admin",
	}
	if err := repo.Create(profile); err != nil {
		t.Fatalf("seeding profile: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/ssh-profiles/"+profile.ID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// D-05: Credential profile port range validation
// =============================================================================

func TestCredentialProfile_PortOutOfRange_400(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	body := `{"name":"test-profile","username":"admin","port":70000,"auth_method":"password","secret":"s3cret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for port 70000, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "port must be between 1 and 65535") {
		t.Errorf("expected port range error, got: %s", rec.Body.String())
	}
}

func TestCredentialProfile_PortNegative_400(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	body := `{"name":"test-profile","username":"admin","port":-1,"auth_method":"password","secret":"s3cret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for port -1, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "port must be between 1 and 65535") {
		t.Errorf("expected port range error, got: %s", rec.Body.String())
	}
}

func TestCredentialProfile_PortZero_UsesDefault(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	// Port 0 means "use default" (22) — should be accepted
	body := `{"name":"test-default-port","username":"admin","port":0,"auth_method":"password","secret":"s3cret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for port 0 (default 22), got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCredentialProfile_ValidPort_201(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	body := `{"name":"test-port-22","username":"admin","port":22,"auth_method":"password","secret":"s3cret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for port 22, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// =============================================================================
// D-03: Credential profile string length validation
// =============================================================================

func TestCredentialProfile_NameTooLong_400(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	longName := strings.Repeat("n", 256)
	body := `{"name":"` + longName + `","username":"admin","port":22,"auth_method":"password","secret":"s3cret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for name > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "name too long") {
		t.Errorf("expected error about name length, got: %s", rec.Body.String())
	}
}

func TestCredentialProfile_UsernameTooLong_400(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	longUsername := strings.Repeat("u", 256)
	body := `{"name":"valid-name","username":"` + longUsername + `","port":22,"auth_method":"password","secret":"s3cret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for username > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "username too long") {
		t.Errorf("expected error about username length, got: %s", rec.Body.String())
	}
}

func TestCredentialProfile_DescriptionTooLong_400(t *testing.T) {
	handler, _, _ := setupCredentialProfileTest(t)

	longDesc := strings.Repeat("d", 256)
	body := `{"name":"valid-name","description":"` + longDesc + `","username":"admin","port":22,"auth_method":"password","secret":"s3cret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for description > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "description too long") {
		t.Errorf("expected error about description length, got: %s", rec.Body.String())
	}
}

func TestCredentialProfileHandlerDelete_InUse(t *testing.T) {
	handler, repo, db := setupCredentialProfileTest(t)

	// Create a profile
	profile := &domain.CredentialProfile{
		Name:       "in-use-profile",
		Username:   "admin",
		Port:       22,
		AuthMethod: domain.SSHAuthPassword,
		Role:       "Admin",
	}
	if err := repo.Create(profile); err != nil {
		t.Fatalf("seeding profile: %v", err)
	}

	// Insert a device that references this credential profile via raw SQL
	deviceID := uuid.New()
	_, err := db.Exec(
		`INSERT INTO devices (id, hostname, ip, snmp_credentials_json, device_type, status, managed, tags_json, metrics_source, prometheus_label_name, prometheus_label_value, vendor, ssh_profile_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		deviceID.String(), "test-device", "10.0.0.1", `{"version":"2c","v2c":{"community":"public"}}`, "router", "up", 1, "{}", "prometheus", "instance", "10.0.0.1", "default", profile.ID.String(),
	)
	if err != nil {
		t.Fatalf("inserting device referencing credential profile: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/ssh-profiles/"+profile.ID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 (conflict), got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify the error message mentions the rejection reason
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err == nil {
		if !strings.Contains(errResp["error"], "still assigned") {
			t.Fatalf("expected conflict error to mention 'still assigned', got: %s", errResp["error"])
		}
	}
}
