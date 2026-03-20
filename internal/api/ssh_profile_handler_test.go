package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// setupSSHProfileTest creates an in-memory SQLite DB, runs migrations, and
// builds a fully-wired SSHProfileHandler backed by a real SSHProfileRepo.
func setupSSHProfileTest(t *testing.T) (*SSHProfileHandler, *sqlite.SSHProfileRepo, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	sshProfileRepo := sqlite.NewSSHProfileRepo(db)
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
		sshProfileRepo,
		newMockDeviceRepo(),
		newMockSettingsRepo(),
		reg,
		&mockSSHDialer{},
		encKey,
		t.TempDir(),
		gossh.InsecureIgnoreHostKey(),
	)

	handler := NewSSHProfileHandler(backupSvc, sshProfileRepo)
	return handler, sshProfileRepo, db
}

func TestSSHProfileHandlerList(t *testing.T) {
	handler, _, _ := setupSSHProfileTest(t)

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

func TestSSHProfileHandlerCreate_HappyPath(t *testing.T) {
	handler, _, _ := setupSSHProfileTest(t)

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

func TestSSHProfileHandlerCreate_MalformedJSON(t *testing.T) {
	handler, _, _ := setupSSHProfileTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh-profiles", strings.NewReader(`{invalid`))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSSHProfileHandlerGet_HappyPath(t *testing.T) {
	handler, repo, _ := setupSSHProfileTest(t)

	// Seed a profile via the repo
	profile := &domain.SSHProfile{
		Name:       "get-test",
		Username:   "admin",
		Port:       22,
		AuthMethod: domain.SSHAuthPassword,
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

func TestSSHProfileHandlerGet_NotFound(t *testing.T) {
	handler, _, _ := setupSSHProfileTest(t)

	randomID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ssh-profiles/"+randomID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleGet(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSSHProfileHandlerUpdate_HappyPath(t *testing.T) {
	handler, repo, _ := setupSSHProfileTest(t)

	profile := &domain.SSHProfile{
		Name:       "update-test",
		Username:   "admin",
		Port:       22,
		AuthMethod: domain.SSHAuthPassword,
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

func TestSSHProfileHandlerDelete_HappyPath(t *testing.T) {
	handler, repo, _ := setupSSHProfileTest(t)

	profile := &domain.SSHProfile{
		Name:       "delete-test",
		Username:   "admin",
		Port:       22,
		AuthMethod: domain.SSHAuthPassword,
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

func TestSSHProfileHandlerDelete_InUse(t *testing.T) {
	handler, repo, db := setupSSHProfileTest(t)

	// Create a profile
	profile := &domain.SSHProfile{
		Name:       "in-use-profile",
		Username:   "admin",
		Port:       22,
		AuthMethod: domain.SSHAuthPassword,
	}
	if err := repo.Create(profile); err != nil {
		t.Fatalf("seeding profile: %v", err)
	}

	// Insert a device that references this SSH profile via raw SQL
	deviceID := uuid.New()
	_, err := db.Exec(
		`INSERT INTO devices (id, hostname, ip, snmp_credentials_json, device_type, status, managed, tags_json, metrics_source, prometheus_label_name, prometheus_label_value, vendor, ssh_profile_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		deviceID.String(), "test-device", "10.0.0.1", `{"version":"2c","v2c":{"community":"public"}}`, "router", "up", 1, "{}", "prometheus", "instance", "10.0.0.1", "default", profile.ID.String(),
	)
	if err != nil {
		t.Fatalf("inserting device referencing SSH profile: %v", err)
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
