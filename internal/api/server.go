package api

import (
	"net/http"

	"cleanpoint/internal/config"
	"cleanpoint/internal/metrics"
	"cleanpoint/internal/restore"
	"cleanpoint/internal/selector"
	"cleanpoint/internal/snapshots"
	"cleanpoint/internal/storage"
)

// Server wires configuration, storage and services into HTTP handlers.
type Server struct {
	cfg      *config.Config
	store    *storage.Store
	snaps    *snapshots.Manager
	sel      *selector.Selector
	restorer *restore.Restorer
	gate     heavyGate
	mux      *http.ServeMux
}

func NewServer(cfg *config.Config, store *storage.Store) *Server {
	s := &Server{cfg: cfg, store: store}
	s.snaps = snapshots.NewManager(cfg, store)
	s.sel = selector.New(cfg, store)
	s.restorer = restore.New(cfg, store, s.snaps, s.sel)
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return WithRequestID(s.mux)
}

func (s *Server) routes() {
	m := s.mux
	auth := s.Authenticate

	// Health is intentionally unauthenticated but only meant for loopback.
	m.HandleFunc("GET /api/v1/healthz", s.handleHealth)

	// Metrics are restricted: any valid token (read-only reviewers may view
	// metrics; anonymous access is denied).
	m.Handle("GET /api/v1/metrics", auth(metrics.Handler()))

	// Workloads
	m.Handle("POST /api/v1/workloads", auth(RequireOperator(http.HandlerFunc(s.handleCreateWorkload))))
	m.Handle("GET /api/v1/workloads", auth(http.HandlerFunc(s.handleListWorkloads)))

	// Events
	m.Handle("POST /api/v1/workloads/{id}/events", auth(RequireOperator(http.HandlerFunc(s.handleIngestEvents))))
	m.Handle("GET /api/v1/workloads/{id}/events", auth(http.HandlerFunc(s.handleListEvents)))

	// Snapshots
	m.Handle("POST /api/v1/workloads/{id}/snapshots", auth(RequireOperator(http.HandlerFunc(s.handleCaptureSnapshot))))
	m.Handle("GET /api/v1/workloads/{id}/snapshots", auth(http.HandlerFunc(s.handleListSnapshots)))

	// Analysis
	m.Handle("POST /api/v1/workloads/{id}/analyze", auth(RequireOperator(http.HandlerFunc(s.handleAnalyze))))
	m.Handle("GET /api/v1/workloads/{id}/findings", auth(http.HandlerFunc(s.handleFindings)))
	m.Handle("GET /api/v1/workloads/{id}/recovery-candidates", auth(http.HandlerFunc(s.handleCandidates)))

	// Restore
	m.Handle("POST /api/v1/workloads/{id}/restore", auth(RequireOperator(http.HandlerFunc(s.handleRestore))))
	m.Handle("GET /api/v1/recovery-runs/{id}", auth(http.HandlerFunc(s.handleGetRun)))
	m.Handle("GET /api/v1/workloads/{id}/recovery-runs", auth(http.HandlerFunc(s.handleListRuns)))

	// Audit (operator-only per proposed policy) and settings
	m.Handle("GET /api/v1/audit-events", auth(RequireOperator(http.HandlerFunc(s.handleAudit))))
	m.Handle("GET /api/v1/config", auth(http.HandlerFunc(s.handleConfig)))

	// Public landing page at "/", operator console under /app/ (M1 static
	// assets, same-origin).
	m.HandleFunc("GET /{$}", s.landingPage())
	m.Handle("GET /app", http.RedirectHandler("/app/", http.StatusMovedPermanently))
	m.Handle("GET /app/{path...}", s.webUI())
	m.Handle("GET /landing/{path...}", s.webUI())

	// Fallback: unknown paths return structured 404 rather than plain text.
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusNotFound, "not_found", "unknown route")
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   nowString(),
		"mode":   "local-simulation",
	})
}
