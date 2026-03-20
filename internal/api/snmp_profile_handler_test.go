package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// mockSNMPProfileRepo implements domain.SNMPProfileRepository backed by an in-memory map.
type mockSNMPProfileRepo struct {
	profiles map[uuid.UUID]*domain.SNMPProfile
}

func newMockSNMPProfileRepo() *mockSNMPProfileRepo {
	return &mockSNMPProfileRepo{profiles: make(map[uuid.UUID]*domain.SNMPProfile)}
}

func (m *mockSNMPProfileRepo) Create(profile *domain.SNMPProfile) error {
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	m.profiles[profile.ID] = profile
	return nil
}

func (m *mockSNMPProfileRepo) GetByID(id uuid.UUID) (*domain.SNMPProfile, error) {
	p, ok := m.profiles[id]
	if !ok {
		return nil, fmt.Errorf("profile not found")
	}
	return p, nil
}

func (m *mockSNMPProfileRepo) GetAll() ([]domain.SNMPProfile, error) {
	result := make([]domain.SNMPProfile, 0, len(m.profiles))
	for _, p := range m.profiles {
		result = append(result, *p)
	}
	return result, nil
}

func (m *mockSNMPProfileRepo) Update(profile *domain.SNMPProfile) error {
	if _, ok := m.profiles[profile.ID]; !ok {
		return fmt.Errorf("profile not found")
	}
	profile.UpdatedAt = time.Now().UTC()
	m.profiles[profile.ID] = profile
	return nil
}

func (m *mockSNMPProfileRepo) Delete(id uuid.UUID) error {
	if _, ok := m.profiles[id]; !ok {
		return fmt.Errorf("profile not found")
	}
	delete(m.profiles, id)
	return nil
}

// seedV2cProfile adds a v2c SNMP profile to the mock repo and returns its ID.
func seedV2cProfile(t *testing.T, repo *mockSNMPProfileRepo) uuid.UUID {
	t.Helper()
	p := &domain.SNMPProfile{
		Name:        "test-v2c",
		Description: "test v2c profile",
		Credentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(p); err != nil {
		t.Fatalf("failed to seed v2c profile: %v", err)
	}
	return p.ID
}

// seedV3Profile adds a v3 SNMP profile with known secrets to the mock repo and returns its ID.
func seedV3Profile(t *testing.T, repo *mockSNMPProfileRepo) uuid.UUID {
	t.Helper()
	p := &domain.SNMPProfile{
		Name:        "test-v3",
		Description: "test v3 profile",
		Credentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV3,
			V3: &domain.SNMPv3Credentials{
				Username:      "admin",
				AuthProtocol:  "SHA",
				AuthPassword:  "super-secret-auth-pass",
				PrivProtocol:  "AES",
				PrivPassword:  "super-secret-priv-pass",
				SecurityLevel: "authPriv",
			},
		},
	}
	if err := repo.Create(p); err != nil {
		t.Fatalf("failed to seed v3 profile: %v", err)
	}
	return p.ID
}

func TestSNMPProfileHandlerList(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	seedV2cProfile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snmp-profiles", nil)
	rec := httptest.NewRecorder()

	h.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Data []snmpProfileResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "test-v2c" {
		t.Fatalf("expected name=test-v2c, got %s", resp.Data[0].Name)
	}
}

func TestSNMPProfileHandlerCreate_HappyPath(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	h := NewSNMPProfileHandler(repo)

	body := `{"name":"new-profile","snmp":{"version":"2c","community":"public"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data snmpProfileResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data.Name != "new-profile" {
		t.Fatalf("expected name=new-profile, got %s", resp.Data.Name)
	}
	if resp.Data.SNMP.Version != "2c" {
		t.Fatalf("expected version=2c, got %s", resp.Data.SNMP.Version)
	}
}

func TestSNMPProfileHandlerCreate_Errors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "malformed JSON",
			body:       `{invalid`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing name",
			body:       `{"snmp":{"version":"2c","community":"public"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty name",
			body:       `{"name":"  ","snmp":{"version":"2c","community":"public"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid version",
			body:       `{"name":"test","snmp":{"version":"4"}}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockSNMPProfileRepo()
			h := NewSNMPProfileHandler(repo)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			h.HandleCreate(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body=%s", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSNMPProfileHandlerGet_HappyPath(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV2cProfile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snmp-profiles/"+id.String(), nil)
	rec := httptest.NewRecorder()

	h.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data snmpProfileResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data.ID != id.String() {
		t.Fatalf("expected ID=%s, got %s", id, resp.Data.ID)
	}
}

func TestSNMPProfileHandlerGet_NotFound(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snmp-profiles/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()

	h.HandleGet(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSNMPProfileHandlerGet_InvalidID(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snmp-profiles/not-a-uuid", nil)
	rec := httptest.NewRecorder()

	h.HandleGet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSNMPProfileHandlerUpdate_HappyPath(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV2cProfile(t, repo)
	h := NewSNMPProfileHandler(repo)

	body := `{"name":"updated-name","snmp":{"version":"2c","community":"private"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/snmp-profiles/"+id.String(), strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	updated := repo.profiles[id]
	if updated.Name != "updated-name" {
		t.Fatalf("expected name=updated-name, got %s", updated.Name)
	}
}

func TestSNMPProfileHandlerDelete_HappyPath(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV2cProfile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/snmp-profiles/"+id.String(), nil)
	rec := httptest.NewRecorder()

	h.HandleDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}

	if _, ok := repo.profiles[id]; ok {
		t.Fatal("expected profile to be deleted from repo")
	}
}

func TestSNMPProfileHandlerDelete_NotFound(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/snmp-profiles/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()

	h.HandleDelete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// --- Phase 2 security hardening: v3 password redaction tests ---

func TestSNMPProfileHandlerGet_V3PasswordRedaction(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV3Profile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snmp-profiles/"+id.String(), nil)
	rec := httptest.NewRecorder()

	h.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()

	// Passwords must NOT appear in GET response
	if strings.Contains(body, "super-secret-auth-pass") {
		t.Fatal("GET response contains plaintext auth_password -- v3 passwords must be redacted")
	}
	if strings.Contains(body, "super-secret-priv-pass") {
		t.Fatal("GET response contains plaintext priv_password -- v3 passwords must be redacted")
	}

	// Non-secret fields must still be present
	if !strings.Contains(body, "admin") {
		t.Fatal("GET response missing username -- non-secret fields should be present")
	}
	if !strings.Contains(body, "SHA") {
		t.Fatal("GET response missing auth_protocol -- non-secret fields should be present")
	}
	if !strings.Contains(body, "AES") {
		t.Fatal("GET response missing priv_protocol -- non-secret fields should be present")
	}
	if !strings.Contains(body, "authPriv") {
		t.Fatal("GET response missing security_level -- non-secret fields should be present")
	}
}

func TestSNMPProfileHandlerList_V3PasswordRedaction(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	seedV3Profile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snmp-profiles", nil)
	rec := httptest.NewRecorder()

	h.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Passwords must NOT appear in list response
	if strings.Contains(body, "super-secret-auth-pass") {
		t.Fatal("list response contains plaintext auth_password -- v3 passwords must be redacted")
	}
	if strings.Contains(body, "super-secret-priv-pass") {
		t.Fatal("list response contains plaintext priv_password -- v3 passwords must be redacted")
	}

	// Non-secret fields must still be present
	if !strings.Contains(body, "admin") {
		t.Fatal("list response missing username")
	}
}
