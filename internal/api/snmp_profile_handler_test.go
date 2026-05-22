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

// --- explicit reveal semantics ---

func TestSNMPProfileHandlerGet_RedactsCredentialSecrets(t *testing.T) {
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
	if strings.Contains(body, "super-secret-auth-pass") || strings.Contains(body, "super-secret-priv-pass") {
		t.Fatalf("metadata response leaked credential secret: %s", body)
	}

	var resp struct {
		Data snmpProfileResponse `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Data.SNMP.Username != "admin" {
		t.Fatalf("expected username=admin, got %s", resp.Data.SNMP.Username)
	}
	if resp.Data.SNMP.AuthProtocol != "SHA" {
		t.Fatalf("expected auth_protocol=SHA, got %s", resp.Data.SNMP.AuthProtocol)
	}
	if resp.Data.SNMP.AuthPassword != "" {
		t.Fatalf("expected auth_password to be redacted, got %q", resp.Data.SNMP.AuthPassword)
	}
	if resp.Data.SNMP.PrivProtocol != "AES" {
		t.Fatalf("expected priv_protocol=AES, got %s", resp.Data.SNMP.PrivProtocol)
	}
	if resp.Data.SNMP.PrivPassword != "" {
		t.Fatalf("expected priv_password to be redacted, got %q", resp.Data.SNMP.PrivPassword)
	}
	if resp.Data.SNMP.SecurityLevel != "authPriv" {
		t.Fatalf("expected security_level=authPriv, got %s", resp.Data.SNMP.SecurityLevel)
	}
	if !resp.Data.SNMP.AuthPasswordSet {
		t.Fatal("expected auth_password_set=true")
	}
	if !resp.Data.SNMP.PrivPasswordSet {
		t.Fatal("expected priv_password_set=true")
	}
}

func TestSNMPProfileHandlerList_RedactsCredentialSecrets(t *testing.T) {
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
	if strings.Contains(body, "super-secret-auth-pass") || strings.Contains(body, "super-secret-priv-pass") {
		t.Fatalf("metadata list leaked credential secret: %s", body)
	}

	var resp struct {
		Data []snmpProfileResponse `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(resp.Data))
	}

	if resp.Data[0].SNMP.AuthPassword != "" {
		t.Fatalf("expected auth_password to be redacted, got %q", resp.Data[0].SNMP.AuthPassword)
	}
	if resp.Data[0].SNMP.PrivPassword != "" {
		t.Fatalf("expected priv_password to be redacted, got %q", resp.Data[0].SNMP.PrivPassword)
	}
	if resp.Data[0].SNMP.Username != "admin" {
		t.Fatalf("expected username=admin, got %s", resp.Data[0].SNMP.Username)
	}
	if !resp.Data[0].SNMP.AuthPasswordSet || !resp.Data[0].SNMP.PrivPasswordSet {
		t.Fatalf("expected v3 secret metadata to be present, got %+v", resp.Data[0].SNMP)
	}
}

func TestSNMPProfileHandlerList_RedactsV2cCommunity(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	seedV2cProfile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snmp-profiles", nil)
	rec := httptest.NewRecorder()

	h.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "public") {
		t.Fatalf("metadata list leaked v2c community: %s", body)
	}

	var resp struct {
		Data []snmpProfileResponse `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(resp.Data))
	}
	if resp.Data[0].SNMP.Community != "" {
		t.Fatalf("expected community to be redacted, got %q", resp.Data[0].SNMP.Community)
	}
	if !resp.Data[0].SNMP.CommunitySet {
		t.Fatal("expected community_set=true")
	}
}

func TestSNMPProfileHandlerCreateAndUpdate_RedactCredentialSecrets(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	h := NewSNMPProfileHandler(repo)

	createBody := `{"name":"new-profile","snmp":{"version":"2c","community":"private-community"}}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles", strings.NewReader(createBody))
	createRec := httptest.NewRecorder()

	h.HandleCreate(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", createRec.Code, createRec.Body.String())
	}
	if strings.Contains(createRec.Body.String(), "private-community") {
		t.Fatalf("create response leaked v2c community: %s", createRec.Body.String())
	}

	var createResp struct {
		Data snmpProfileResponse `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if !createResp.Data.SNMP.CommunitySet {
		t.Fatal("expected community_set=true on create")
	}

	updateBody := `{"name":"updated-name","snmp":{"version":"3","username":"admin","auth_protocol":"SHA","auth_password":"auth-pass-value","priv_protocol":"AES","priv_password":"priv-pass-value","security_level":"authPriv"}}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/snmp-profiles/"+createResp.Data.ID, strings.NewReader(updateBody))
	updateRec := httptest.NewRecorder()

	h.HandleUpdate(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", updateRec.Code, updateRec.Body.String())
	}
	if strings.Contains(updateRec.Body.String(), "auth-pass-value") || strings.Contains(updateRec.Body.String(), "priv-pass-value") {
		t.Fatalf("update response leaked v3 password: %s", updateRec.Body.String())
	}
}

func TestSNMPProfileHandlerUpdate_PreservesExistingSecretsWhenRedactedFieldsOmitted(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV3Profile(t, repo)
	h := NewSNMPProfileHandler(repo)

	body := `{"name":"updated-v3","snmp":{"version":"3","username":"admin","auth_protocol":"SHA","priv_protocol":"AES","security_level":"authPriv"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/snmp-profiles/"+id.String(), strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	updated := repo.profiles[id]
	if updated.Credentials.V3.AuthPassword != "super-secret-auth-pass" {
		t.Fatalf("expected auth password to be preserved, got %q", updated.Credentials.V3.AuthPassword)
	}
	if updated.Credentials.V3.PrivPassword != "super-secret-priv-pass" {
		t.Fatalf("expected priv password to be preserved, got %q", updated.Credentials.V3.PrivPassword)
	}
}

func TestSNMPProfileHandlerUpdate_ClearsV3SecretsWhenSecurityLevelNoLongerUsesThem(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV3Profile(t, repo)
	h := NewSNMPProfileHandler(repo)

	body := `{"name":"updated-v3","snmp":{"version":"3","username":"readonly","security_level":"noAuthNoPriv"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/snmp-profiles/"+id.String(), strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	updated := repo.profiles[id]
	if updated.Credentials.V3.AuthPassword != "" {
		t.Fatalf("expected auth password to be cleared, got %q", updated.Credentials.V3.AuthPassword)
	}
	if updated.Credentials.V3.PrivPassword != "" {
		t.Fatalf("expected priv password to be cleared, got %q", updated.Credentials.V3.PrivPassword)
	}
}

func TestSNMPProfileHandlerUpdate_PreservesExistingV2cCommunityWhenOmitted(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV2cProfile(t, repo)
	h := NewSNMPProfileHandler(repo)

	body := `{"name":"updated-v2c","snmp":{"version":"2c"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/snmp-profiles/"+id.String(), strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	updated := repo.profiles[id]
	if updated.Credentials.V2c.Community != "public" {
		t.Fatalf("expected v2c community to be preserved, got %q", updated.Credentials.V2c.Community)
	}
}

func TestSNMPProfileHandlerReveal_ReturnsCredentialSecretsWithNoStore(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV3Profile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles/"+id.String()+"/reveal", strings.NewReader(`{"reason":"apply profile to device"}`))
	req = withTestOperator(req)
	req.Header.Set("User-Agent", "theia-test")
	rec := httptest.NewRecorder()

	h.HandleReveal(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("expected Pragma no-cache, got %q", got)
	}

	var resp struct {
		Data snmpProfileResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data.SNMP.AuthPassword != "super-secret-auth-pass" {
		t.Fatalf("expected auth_password reveal, got %q", resp.Data.SNMP.AuthPassword)
	}
	if resp.Data.SNMP.PrivPassword != "super-secret-priv-pass" {
		t.Fatalf("expected priv_password reveal, got %q", resp.Data.SNMP.PrivPassword)
	}
}

func TestSNMPProfileHandlerReveal_RequiresReason(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV2cProfile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles/"+id.String()+"/reveal", strings.NewReader(`{"reason":"   "}`))
	req = withTestOperator(req)
	rec := httptest.NewRecorder()

	h.HandleReveal(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSNMPProfileHandlerReveal_RequiresAuthenticatedOperator(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV2cProfile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles/"+id.String()+"/reveal", strings.NewReader(`{"reason":"apply profile to device"}`))
	rec := httptest.NewRecorder()

	h.HandleReveal(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSNMPProfileHandlerReveal_NotFound(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles/"+uuid.New().String()+"/reveal", strings.NewReader(`{"reason":"apply profile to device"}`))
	req = withTestOperator(req)
	rec := httptest.NewRecorder()

	h.HandleReveal(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSNMPProfileHandlerReveal_RejectsExtraPathSegments(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	id := seedV3Profile(t, repo)
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles/"+id.String()+"/extra/reveal", strings.NewReader(`{"reason":"apply profile to device"}`))
	req = withTestOperator(req)
	rec := httptest.NewRecorder()

	h.HandleReveal(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "super-secret-auth-pass") || strings.Contains(rec.Body.String(), "super-secret-priv-pass") {
		t.Fatalf("invalid reveal path leaked credentials: %s", rec.Body.String())
	}
}

func TestSNMPProfileHandlerGet_IncludesFalseSecretMetadata(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	p := &domain.SNMPProfile{
		Name: "test-v3-no-secrets",
		Credentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV3,
			V3: &domain.SNMPv3Credentials{
				Username:      "readonly",
				SecurityLevel: "noAuthNoPriv",
			},
		},
	}
	if err := repo.Create(p); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	h := NewSNMPProfileHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snmp-profiles/"+p.ID.String(), nil)
	rec := httptest.NewRecorder()

	h.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"community_set":false`) {
		t.Fatalf("expected community_set false metadata in body: %s", body)
	}
	if !strings.Contains(body, `"auth_password_set":false`) {
		t.Fatalf("expected auth_password_set false metadata in body: %s", body)
	}
	if !strings.Contains(body, `"priv_password_set":false`) {
		t.Fatalf("expected priv_password_set false metadata in body: %s", body)
	}
}

// =============================================================================
// D-03: SNMP profile name and description length validation
// =============================================================================

func TestSNMPProfile_NameTooLong_400(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	h := NewSNMPProfileHandler(repo)

	longName := strings.Repeat("n", 256)
	body := fmt.Sprintf(`{"name":"%s","snmp":{"version":"2c","community":"public"}}`, longName)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for name > 255 chars, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "name too long") {
		t.Errorf("expected error about name length, got: %s", rec.Body.String())
	}
}

func TestSNMPProfile_DescriptionTooLong_400(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	h := NewSNMPProfileHandler(repo)

	longDesc := strings.Repeat("d", 256)
	body := fmt.Sprintf(`{"name":"valid-name","description":"%s","snmp":{"version":"2c","community":"public"}}`, longDesc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for description > 255 chars, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "description too long") {
		t.Errorf("expected error about description length, got: %s", rec.Body.String())
	}
}

func TestSNMPProfile_NameExactly255_201(t *testing.T) {
	repo := newMockSNMPProfileRepo()
	h := NewSNMPProfileHandler(repo)

	exactName := strings.Repeat("n", 255)
	body := fmt.Sprintf(`{"name":"%s","snmp":{"version":"2c","community":"public"}}`, exactName)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snmp-profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for name exactly 255 chars, got %d; body=%s", rec.Code, rec.Body.String())
	}
}
