package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

type requestCSRFProvider struct {
	*fakeAPIAuthProvider
	checked          *service.AuthenticatedUser
	reused, fallback int
}

func (p *requestCSRFProvider) ValidateAuthenticatedCSRF(ctx context.Context, user *service.AuthenticatedUser, token, csrf string) error {
	p.reused++
	p.checked = user
	return p.fakeAPIAuthProvider.ValidateCSRF(ctx, token, csrf)
}
func (p *requestCSRFProvider) ValidateCSRF(ctx context.Context, token, csrf string) error {
	p.fallback++
	return p.fakeAPIAuthProvider.ValidateCSRF(ctx, token, csrf)
}

func TestProtectedMutationReusesAuthenticatedSessionForCSRF(t *testing.T) {
	base := newFakeAPIAuthProvider()
	user := testAPIUser("operator", false, domain.PermissionSettingsUpdate)
	base.setSession(testSessionToken, testCSRFToken, user)
	provider := &requestCSRFProvider{fakeAPIAuthProvider: base}
	called := false
	handler := UserAuth(provider)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) }))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/test.key", nil)
	req.AddCookie(&http.Cookie{Name: authSessionCookieName, Value: testSessionToken})
	addCSRFCookieAndHeader(req, testCSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if !called || response.Code != http.StatusNoContent || provider.reused != 1 || provider.fallback != 0 || provider.checked != base.usersByToken[testSessionToken] {
		t.Fatalf("response=%d reuse=%d fallback=%d body=%s", response.Code, provider.reused, provider.fallback, response.Body.String())
	}
	// Logout and other callers without a checked request session retain their
	// independent lookup rather than trusting a header or cookie alone.
	if !validateRequestCSRF(httptest.NewRecorder(), req, provider, testSessionToken) || provider.fallback != 1 {
		t.Fatal("unauthenticated request skipped session lookup")
	}
}
