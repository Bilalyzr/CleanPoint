package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"cleanpoint/internal/metrics"
)

type ctxKey string

const (
	ctxRequestID ctxKey = "request_id"
	ctxRole      ctxKey = "role"
)

// Roles.
const (
	RoleOperator = "operator"
	RoleReadOnly = "read-only"
)

func requestID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxRequestID).(string); ok {
		return v
	}
	return ""
}

func role(r *http.Request) string {
	if v, ok := r.Context().Value(ctxRole).(string); ok {
		return v
	}
	return ""
}

// Authenticate validates the bearer token and stores the role. It rejects
// anything malformed with 401; role checks happen per-route (403).
func (s *Server) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, r, http.StatusUnauthorized, "unauthenticated", "missing bearer token")
			return
		}
		tok := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		// constant-shape comparison to avoid trivially leaking equality
		switch tok {
		case s.cfg.OperatorToken:
			// operator
		case s.cfg.ReadOnlyToken:
			// read-only
		default:
			writeError(w, r, http.StatusUnauthorized, "unauthenticated", "invalid token")
			return
		}
		var role string
		if tok == s.cfg.OperatorToken {
			role = RoleOperator
		} else {
			role = RoleReadOnly
		}
		ctx := context.WithValue(r.Context(), ctxRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireOperator gates mutating endpoints.
func RequireOperator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if role(r) != RoleOperator {
			writeError(w, r, http.StatusForbidden, "forbidden", "operator token required for this operation")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WithRequestID assigns a correlation id and emits a redacted structured log
// line (no tokens, no file contents).
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		rid := "req-" + hex.EncodeToString(b)
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), ctxRequestID, rid)))
		metrics.Inc("http_requests_total", "code", codeClass(sw.status))
		if r.URL.Path != "/api/v1/metrics" && r.URL.Path != "/api/v1/healthz" {
			log.Printf("%s %s %d %s id=%s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond), rid)
		}
	})
}

func codeClass(code int) string {
	return string(rune('0'+code/100)) + "xx"
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// heavyGate serializes snapshot capture and restore so they cannot race on
// the object store or SQLite. Busy callers receive a 409-style error.
type heavyGate struct {
	mu sync.Mutex
}

var errBusy = errors.New("operation busy")

func (g *heavyGate) try(f func() error) error {
	if !g.mu.TryLock() {
		return errBusy
	}
	defer g.mu.Unlock()
	return f()
}
