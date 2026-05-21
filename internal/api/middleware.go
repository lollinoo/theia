package api

import (
	"log"
	"net/http"
	"time"

	"github.com/lollinoo/theia/internal/security"
)

// SecurityConfig controls HTTP authentication and browser origin policy.
type SecurityConfig struct {
	OperatorToken  string
	AllowedOrigins []string
	Sessions       *security.SessionManager
}

// JSONContentType sets the Content-Type header to application/json on all responses.
func JSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// RequestLogger logs each request with method, path, status code, and duration.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

// CORS applies the default same-origin browser policy.
func CORS(next http.Handler) http.Handler {
	return CORSWithConfig(SecurityConfig{})(next)
}

// CORSWithConfig echoes exact configured origins and same-host origins.
func CORSWithConfig(config SecurityConfig) func(http.Handler) http.Handler {
	allowedOrigins := security.NormalizedAllowedOrigins(config.AllowedOrigins)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin := r.Header.Get("Origin"); origin != "" && security.OriginAllowed(r, allowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Theia-Operator")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OriginGuard rejects browser requests from origins outside the configured allowlist.
func OriginGuard(config SecurityConfig) func(http.Handler) http.Handler {
	allowedOrigins := security.NormalizedAllowedOrigins(config.AllowedOrigins)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Origin") != "" && !security.OriginAllowed(r, allowedOrigins) {
				writeError(w, http.StatusForbidden, "origin not allowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// OperatorAuth requires the configured operator bearer token when one is set.
func OperatorAuth(config SecurityConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextRequest, ok := AuthenticateOperatorRequest(w, r, config)
			if !ok {
				return
			}
			next.ServeHTTP(w, nextRequest)
		})
	}
}

// AuthenticateOperatorRequest validates one request and returns it with operator context.
func AuthenticateOperatorRequest(w http.ResponseWriter, r *http.Request, config SecurityConfig) (*http.Request, bool) {
	subject, ok := security.AuthenticateRequest(r, config.OperatorToken, config.Sessions)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="theia"`)
		writeError(w, http.StatusUnauthorized, "authentication required")
		return r, false
	}
	if subject.Authenticated {
		return r.WithContext(security.WithOperatorSubject(r.Context(), subject)), true
	}
	return r, true
}

// OperatorSubjectFromRequest returns the authenticated operator for audit logs.
func OperatorSubjectFromRequest(r *http.Request) security.OperatorSubject {
	return security.OperatorSubjectFromContext(r.Context())
}

func requireAuthenticatedOperator(w http.ResponseWriter, r *http.Request, action string) (security.OperatorSubject, bool) {
	subject := OperatorSubjectFromRequest(r)
	if subject.Authenticated {
		return subject, true
	}
	writeError(w, http.StatusForbidden, action+" requires an authenticated operator")
	return subject, false
}

func applyMiddleware(next http.Handler, config SecurityConfig, includeJSON bool, bodyLimit int64) http.Handler {
	handler := next
	if includeJSON {
		handler = JSONContentType(handler)
	}
	if bodyLimit > 0 {
		handler = MaxBodySize(bodyLimit)(handler)
	}
	handler = OperatorAuth(config)(handler)
	handler = OriginGuard(config)(handler)
	handler = RequestLogger(handler)
	handler = CORSWithConfig(config)(handler)
	return handler
}

func applyPublicMiddleware(next http.Handler, config SecurityConfig, includeJSON bool, bodyLimit int64) http.Handler {
	handler := next
	if includeJSON {
		handler = JSONContentType(handler)
	}
	if bodyLimit > 0 {
		handler = MaxBodySize(bodyLimit)(handler)
	}
	handler = OriginGuard(config)(handler)
	handler = RequestLogger(handler)
	handler = CORSWithConfig(config)(handler)
	return handler
}

// MaxBodySize limits the size of request bodies to prevent memory exhaustion.
// When the limit is exceeded, subsequent reads return an error that triggers HTTP 413.
func MaxBodySize(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
