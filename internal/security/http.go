package security

// This file defines http security policy helpers and trust-boundary handling.

import (
	"context"
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

// AnonymousSubject is used when a request has no authenticated user.
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

// SecureCookieForRequest returns true when a browser cookie should be marked Secure.
func SecureCookieForRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
