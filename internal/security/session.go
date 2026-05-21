package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	// OperatorSessionCookieName is the HttpOnly browser session cookie.
	OperatorSessionCookieName = "theia_operator_session"
	defaultSessionTTL         = 12 * time.Hour
)

// SessionManager creates and verifies stateless signed operator sessions.
type SessionManager struct {
	signingKey []byte
	now        func() time.Time
	ttl        time.Duration
}

type sessionClaims struct {
	Subject   string `json:"sub"`
	ExpiresAt int64  `json:"exp"`
}

// NewSessionManager creates a stateless session signer from an operator token.
func NewSessionManager(operatorToken string) *SessionManager {
	return &SessionManager{
		signingKey: []byte(strings.TrimSpace(operatorToken)),
		now:        time.Now,
		ttl:        defaultSessionTTL,
	}
}

// CreateCookie returns a signed session cookie for subject.
func (m *SessionManager) CreateCookie(subject string, secure bool) (*http.Cookie, time.Time, bool) {
	if m == nil || len(m.signingKey) == 0 {
		return nil, time.Time{}, false
	}
	subject = sanitizeSubject(subject)
	if subject == "" {
		subject = "operator"
	}
	expiresAt := m.now().UTC().Add(m.ttl)
	claims := sessionClaims{Subject: subject, ExpiresAt: expiresAt.Unix()}
	raw, err := json.Marshal(claims)
	if err != nil {
		return nil, time.Time{}, false
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	signature := m.sign(payload)
	return &http.Cookie{
		Name:     OperatorSessionCookieName,
		Value:    payload + "." + signature,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(m.ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	}, expiresAt, true
}

// ClearCookie returns a cookie that removes the operator session.
func ClearCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     OperatorSessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	}
}

// SubjectFromRequest verifies the request session cookie.
func (m *SessionManager) SubjectFromRequest(r *http.Request) (OperatorSubject, bool) {
	if m == nil || len(m.signingKey) == 0 {
		return AnonymousSubject, false
	}
	cookie, err := r.Cookie(OperatorSessionCookieName)
	if err != nil {
		return AnonymousSubject, false
	}
	return m.subjectFromValue(cookie.Value)
}

func (m *SessionManager) subjectFromValue(value string) (OperatorSubject, bool) {
	payload, signature, ok := strings.Cut(strings.TrimSpace(value), ".")
	if !ok || payload == "" || signature == "" {
		return AnonymousSubject, false
	}
	if !constantTimeTokenEqual(signature, m.sign(payload)) {
		return AnonymousSubject, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return AnonymousSubject, false
	}
	var claims sessionClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return AnonymousSubject, false
	}
	if claims.ExpiresAt <= m.now().Unix() {
		return AnonymousSubject, false
	}
	subject := sanitizeSubject(claims.Subject)
	if subject == "" {
		return AnonymousSubject, false
	}
	return OperatorSubject{Name: subject, Authenticated: true}, true
}

func (m *SessionManager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.signingKey)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// SecureCookieForRequest returns true when a browser cookie should be marked Secure.
func SecureCookieForRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
