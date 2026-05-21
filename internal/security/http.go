package security

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

type subjectContextKey struct{}

// OperatorSubject describes the authenticated operator associated with a request.
type OperatorSubject struct {
	Name          string
	Authenticated bool
}

// AnonymousSubject is used when local development auth is disabled.
var AnonymousSubject = OperatorSubject{Name: "anonymous", Authenticated: false}

// WithOperatorSubject stores the authenticated operator in a context.
func WithOperatorSubject(ctx context.Context, subject OperatorSubject) context.Context {
	return context.WithValue(ctx, subjectContextKey{}, subject)
}

// OperatorSubjectFromContext returns the authenticated operator stored in ctx.
func OperatorSubjectFromContext(ctx context.Context) OperatorSubject {
	subject, ok := ctx.Value(subjectContextKey{}).(OperatorSubject)
	if !ok || strings.TrimSpace(subject.Name) == "" {
		return AnonymousSubject
	}
	return subject
}

// AuthenticateRequest validates a bearer token or signed operator session.
func AuthenticateRequest(r *http.Request, expectedToken string, sessions *SessionManager) (OperatorSubject, bool) {
	expectedToken = strings.TrimSpace(expectedToken)
	if expectedToken == "" {
		return AnonymousSubject, true
	}

	if subject, ok := authenticateBearer(r, expectedToken); ok {
		return subject, true
	}
	if subject, ok := sessions.SubjectFromRequest(r); ok {
		return subject, true
	}
	return AnonymousSubject, false
}

// AuthenticateLoginToken validates the token submitted to the session endpoint.
func AuthenticateLoginToken(gotToken, expectedToken, operatorName string) (OperatorSubject, bool) {
	expectedToken = strings.TrimSpace(expectedToken)
	if expectedToken == "" || !constantTimeTokenEqual(strings.TrimSpace(gotToken), expectedToken) {
		return AnonymousSubject, false
	}
	subject := sanitizeSubject(operatorName)
	if subject == "" {
		subject = "operator"
	}
	return OperatorSubject{Name: subject, Authenticated: true}, true
}

func authenticateBearer(r *http.Request, expectedToken string) (OperatorSubject, bool) {
	if !constantTimeTokenEqual(bearerToken(r.Header.Get("Authorization")), expectedToken) {
		return AnonymousSubject, false
	}

	subject := sanitizeSubject(r.Header.Get("X-Theia-Operator"))
	if subject == "" {
		subject = "operator"
	}
	return OperatorSubject{Name: subject, Authenticated: true}, true
}

// OriginAllowed checks whether the request Origin is allowed by exact origin or same host.
func OriginAllowed(r *http.Request, allowedOrigins []string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	normalizedOrigin, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	if originMatchesHost(normalizedOrigin, r.Host) {
		return true
	}

	for _, allowed := range allowedOrigins {
		normalizedAllowed, ok := normalizeOrigin(allowed)
		if ok && normalizedAllowed == normalizedOrigin {
			return true
		}
	}
	return false
}

// NormalizedAllowedOrigins returns normalized exact origins, excluding invalid values.
func NormalizedAllowedOrigins(allowedOrigins []string) []string {
	normalized := make([]string, 0, len(allowedOrigins))
	seen := make(map[string]struct{}, len(allowedOrigins))
	for _, allowed := range allowedOrigins {
		next, ok := normalizeOrigin(allowed)
		if !ok {
			continue
		}
		if _, exists := seen[next]; exists {
			continue
		}
		seen[next] = struct{}{}
		normalized = append(normalized, next)
	}
	return normalized
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const prefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(header), prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func constantTimeTokenEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}

func sanitizeSubject(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range value {
		if r > unicode.MaxASCII || r < 0x20 || r == 0x7f {
			continue
		}
		switch r {
		case '"', '\'', '\\':
			continue
		default:
			b.WriteRune(r)
		}
		if b.Len() >= 128 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizeOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "ws", "wss":
	default:
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}

func originMatchesHost(normalizedOrigin, host string) bool {
	parsed, err := url.Parse(normalizedOrigin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, strings.TrimSpace(host))
}
