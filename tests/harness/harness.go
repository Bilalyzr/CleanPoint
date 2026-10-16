// Package harness provides shared test infrastructure: an isolated sandbox
// with deterministic tokens, a booted in-process server and HTTP helpers.
package harness

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cleanpoint/internal/api"
	"cleanpoint/internal/config"
	"cleanpoint/internal/domain"
	"cleanpoint/internal/storage"
)

const (
	OperatorToken = "test-operator-token-0001"
	ReadOnlyToken = "test-readonly-token-0002"
)

// T is a fully wired test environment.
type T struct {
	Sandbox string
	Server  *httptest.Server
	Cfg     *config.Config
	Store   *storage.Store
	t       *testing.T
}

// New creates a fresh sandbox + server per test (isolated DB and roots).
func New(t *testing.T) *T {
	t.Helper()
	sandbox := t.TempDir()
	t.Setenv("CLEANPOINT_SANDBOX_ROOT", filepath.ToSlash(sandbox))
	t.Setenv("CLEANPOINT_OPERATOR_TOKEN", OperatorToken)
	t.Setenv("CLEANPOINT_READONLY_TOKEN", ReadOnlyToken)
	t.Setenv("CLEANPOINT_DB_PATH", filepath.Join(sandbox, "test.db"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := storage.NewStore(db)
	srv := api.NewServer(cfg, store)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &T{Sandbox: sandbox, Server: ts, Cfg: cfg, Store: store, t: t}
}

// RootsDir returns the allowlisted workload-roots directory.
func (e *T) RootsDir() string { return e.Cfg.RootsDir() }

// NewRoot creates an empty workload root inside the allowlisted tree.
func (e *T) NewRoot(name string) string {
	p := filepath.Join(filepath.FromSlash(e.RootsDir()), name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		e.t.Fatalf("newroot: %v", err)
	}
	return filepath.ToSlash(p)
}

// WriteFile writes a file inside dir at the slash-relative path.
func (e *T) WriteFile(dir, rel, content string) {
	e.t.Helper()
	p := filepath.Join(filepath.FromSlash(dir), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatalf("writefile mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatalf("writefile: %v", err)
	}
}

// Client performs a token-authenticated JSON request. role: "operator" or "ro".
func (e *T) Client(role, method, path string, body any) (int, map[string]any) {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.Server.URL+path, rdr)
	if err != nil {
		e.t.Fatalf("request: %v", err)
	}
	switch role {
	case "operator":
		req.Header.Set("Authorization", "Bearer "+OperatorToken)
	case "ro":
		req.Header.Set("Authorization", "Bearer "+ReadOnlyToken)
	case "none":
	default:
		e.t.Fatalf("unknown role %q", role)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &out); err != nil {
			e.t.Fatalf("non-JSON response (%s %s): %s", method, path, truncate(string(data), 300))
		}
	}
	return resp.StatusCode, out
}

// Raw performs an unauthenticated request returning raw status/body.
func (e *T) Raw(method, path string, body string, hdr map[string]string) (int, string) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.Server.URL+path, rdr)
	if err != nil {
		e.t.Fatalf("request: %v", err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// RegisterWorkload creates a workload with a fresh root and returns its id.
func (e *T) RegisterWorkload(name string) (id, root string) {
	e.t.Helper()
	root = e.NewRoot(name)
	st, out := e.Client("operator", "POST", "/api/v1/workloads", map[string]any{
		"name": name, "root_path": root,
	})
	if st != 201 {
		e.t.Fatalf("register workload: %d %v", st, out)
	}
	return out["id"].(string), root
}

// IngestEvents posts events as the operator.
func (e *T) IngestEvents(wl string, events []map[string]any) int {
	e.t.Helper()
	st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events",
		map[string]any{"events": events})
	if st >= 300 {
		e.t.Fatalf("ingest: %d %v", st, out)
	}
	return st
}

// Capture takes a snapshot (capturedAt may be empty for the event clock).
func (e *T) Capture(wl, capturedAt string) map[string]any {
	e.t.Helper()
	body := map[string]any{}
	if capturedAt != "" {
		body["captured_at"] = capturedAt
	}
	st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/snapshots", body)
	if st != 201 {
		e.t.Fatalf("capture: %d %v", st, out)
	}
	return out
}

// TS renders a fixed-layout timestamp offset from the epoch base by seconds.
func TS(seconds int) string {
	base := int64(1767603600) // 2026-01-05T09:00:00Z
	tm := time.Unix(base+int64(seconds), 0).UTC()
	return domain.FormatTime(tm)
}
