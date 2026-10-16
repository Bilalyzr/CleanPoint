package integration

import (
	"strings"
	"testing"

	tt "cleanpoint/tests/harness"
)

// ---------- API authentication (8+ cases) ----------

func TestAPIAuthentication(t *testing.T) {
	t.Run("no_token_401", func(t *testing.T) {
		e := tt.New(t)
		st, out := e.Client("none", "GET", "/api/v1/workloads", nil)
		if st != 401 || out["code"] != "unauthenticated" {
			t.Errorf("no token: %d %v", st, out)
		}
	})

	t.Run("invalid_token_401", func(t *testing.T) {
		e := tt.New(t)
		st, body := e.Raw("GET", "/api/v1/workloads", "", map[string]string{
			"Authorization": "Bearer definitely-wrong-token"})
		if st != 401 || !strings.Contains(body, "unauthenticated") {
			t.Errorf("bad token: %d %s", st, body)
		}
	})

	t.Run("readonly_cannot_mutate", func(t *testing.T) {
		e := tt.New(t)
		st, out := e.Client("ro", "POST", "/api/v1/workloads", map[string]any{
			"name": "ro-wl", "root_path": e.NewRoot("ro-wl")})
		if st != 403 || out["code"] != "forbidden" {
			t.Errorf("read-only mutate: %d %v", st, out)
		}
	})

	t.Run("readonly_cannot_ingest_or_capture_or_restore", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("auth-ro")
		checks := []struct{ method, path string }{
			{"POST", "/api/v1/workloads/" + wl + "/events"},
			{"POST", "/api/v1/workloads/" + wl + "/snapshots"},
			{"POST", "/api/v1/workloads/" + wl + "/analyze"},
			{"POST", "/api/v1/workloads/" + wl + "/restore"},
		}
		for _, c := range checks {
			st, _ := e.Client("ro", c.method, c.path, map[string]any{})
			if st != 403 {
				t.Errorf("read-only %s %s: expected 403, got %d", c.method, c.path, st)
			}
		}
	})

	t.Run("readonly_can_read", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("auth-read")
		for _, path := range []string{
			"/api/v1/workloads",
			"/api/v1/workloads/" + wl + "/snapshots",
			"/api/v1/workloads/" + wl + "/events",
			"/api/v1/workloads/" + wl + "/recovery-candidates",
		} {
			st, _ := e.Client("ro", "GET", path, nil)
			if st == 401 || st == 403 {
				t.Errorf("read-only GET %s rejected: %d", path, st)
			}
		}
	})

	t.Run("operator_can_mutate", func(t *testing.T) {
		e := tt.New(t)
		st, _ := e.Client("operator", "POST", "/api/v1/workloads", map[string]any{
			"name": "auth-op", "root_path": e.NewRoot("auth-op")})
		if st != 201 {
			t.Errorf("operator create workload: %d", st)
		}
	})

	t.Run("healthz_open_locally", func(t *testing.T) {
		e := tt.New(t)
		st, _ := e.Client("none", "GET", "/api/v1/healthz", nil)
		if st != 200 {
			t.Errorf("healthz: %d", st)
		}
	})

	t.Run("metrics_requires_token", func(t *testing.T) {
		e := tt.New(t)
		st, _ := e.Client("none", "GET", "/api/v1/metrics", nil)
		if st != 401 {
			t.Errorf("anonymous metrics: %d", st)
		}
		st2, body := e.Raw("GET", "/api/v1/metrics", "", map[string]string{
			"Authorization": "Bearer " + tt.ReadOnlyToken})
		if st2 != 200 || !strings.Contains(body, "http_requests_total") {
			t.Errorf("read-only metrics: %d %.80s", st2, body)
		}
	})

	t.Run("audit_operator_only", func(t *testing.T) {
		e := tt.New(t)
		st, _ := e.Client("ro", "GET", "/api/v1/audit-events", nil)
		if st != 403 {
			t.Errorf("read-only audit access: %d", st)
		}
		st2, _ := e.Client("operator", "GET", "/api/v1/audit-events", nil)
		if st2 != 200 {
			t.Errorf("operator audit access: %d", st2)
		}
	})

	t.Run("unknown_route_structured_404", func(t *testing.T) {
		e := tt.New(t)
		st, out := e.Client("ro", "GET", "/api/v1/definitely-not-a-route", nil)
		if st != 404 || out["code"] != "not_found" {
			t.Errorf("unknown route: %d %v", st, out)
		}
	})

	t.Run("error_object_shape", func(t *testing.T) {
		e := tt.New(t)
		_, out := e.Client("none", "GET", "/api/v1/workloads", nil)
		for _, k := range []string{"code", "message", "request_id"} {
			if _, ok := out[k]; !ok {
				t.Errorf("error object missing %q: %v", k, out)
			}
		}
	})
}
