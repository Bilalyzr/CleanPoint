package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cleanpoint/internal/domain"
	"cleanpoint/internal/metrics"
	"cleanpoint/internal/security"
)

const maxBodyBytes = 32 << 20 // 32 MiB request cap

func nowString() string { return domain.FormatTime(time.Now()) }

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		badRequest(w, r, "cannot read request body")
		return false
	}
	if len(body) > maxBodyBytes {
		writeError(w, r, http.StatusRequestEntityTooLarge, "limit_exceeded", "request body too large")
		return false
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		badRequest(w, r, "empty request body")
		return false
	}
	if err := json.Unmarshal(body, v); err != nil {
		badRequest(w, r, "invalid JSON: %v", err)
		return false
	}
	return true
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func (s *Server) handleCreateWorkload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		RootPath string `json:"root_path,omitempty"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if !nameRe.MatchString(req.Name) {
		badRequest(w, r, "name must match ^[a-z0-9][a-z0-9-]{0,63}$")
		return
	}
	id, err := domain.NewWorkloadID()
	if err != nil {
		mapError(w, r, err)
		return
	}

	root := req.RootPath
	if root == "" {
		root = filepath.ToSlash(filepath.Join(filepath.FromSlash(s.cfg.RootsDir()), req.Name+"-"+strings.TrimPrefix(id, "wl-")))
	}
	rootAbs, err := filepath.Abs(filepath.FromSlash(root))
	if err != nil {
		mapError(w, r, err)
		return
	}
	root = filepath.ToSlash(rootAbs)
	if err := security.ValidateRootInside(s.cfg.RootsDir(), root); err != nil {
		mapError(w, r, err)
		return
	}
	// The root may already exist (simulator pre-created it) but must not be a
	// symlink and must not contain symlinked components.
	if err := security.CheckNoSymlinks(rootAbs, false); err != nil {
		mapError(w, r, err)
		return
	}
	if fi, err := os.Lstat(rootAbs); err == nil && !fi.IsDir() {
		writeError(w, r, http.StatusUnprocessableEntity, "unsafe_path", "root exists and is not a directory")
		return
	}
	if err := os.MkdirAll(rootAbs, 0o755); err != nil {
		mapError(w, r, err)
		return
	}

	wl := domain.Workload{ID: id, Name: req.Name, RootPath: root, CreatedAt: nowString()}
	if err := s.store.CreateWorkload(wl); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, r, http.StatusConflict, "conflict", "workload name already registered")
			return
		}
		mapError(w, r, err)
		return
	}
	s.audit(r, RoleOperator, "workload.create", wl.ID, "created")
	writeJSON(w, http.StatusCreated, wl)
}

func (s *Server) handleListWorkloads(w http.ResponseWriter, r *http.Request) {
	wls, err := s.store.ListWorkloads()
	if err != nil {
		mapError(w, r, err)
		return
	}
	if wls == nil {
		wls = []domain.Workload{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workloads": wls, "count": len(wls)})
}

type eventPayload struct {
	ID         string   `json:"id"`
	Timestamp  string   `json:"timestamp"`
	Operation  string   `json:"operation"`
	Path       string   `json:"path"`
	DestPath   *string  `json:"dest_path,omitempty"`
	OldSHA256  *string  `json:"old_sha256,omitempty"`
	NewSHA256  *string  `json:"new_sha256,omitempty"`
	OldEntropy *float64 `json:"old_entropy,omitempty"`
	NewEntropy *float64 `json:"new_entropy,omitempty"`
	OldSize    *int64   `json:"old_size,omitempty"`
	NewSize    *int64   `json:"new_size,omitempty"`
	Actor      *string  `json:"actor,omitempty"`
	ScenarioID *string  `json:"scenario_id,omitempty"`
}

var shaRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

func validateEvent(w http.ResponseWriter, r *http.Request, wlID string, e eventPayload) (domain.FileEvent, bool) {
	out := domain.FileEvent{WorkloadID: wlID}
	if e.ID == "" || len(e.ID) > 128 {
		badRequest(w, r, "event id must be 1..128 characters")
		return out, false
	}
	out.ID = e.ID
	ts, err := domain.NormalizeTime(e.Timestamp)
	if err != nil {
		badRequest(w, r, "event %s: %v", e.ID, err)
		return out, false
	}
	out.EventTime = ts
	if !domain.ValidOperations[e.Operation] {
		badRequest(w, r, "event %s: operation must be create|modify|delete|rename", e.ID)
		return out, false
	}
	out.Operation = e.Operation
	if err := security.ValidateRelativePath(e.Path); err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "unsafe_path",
			fmt.Sprintf("event %s path rejected: %v", e.ID, err))
		return out, false
	}
	out.Path = e.Path
	if e.DestPath != nil {
		if err := security.ValidateRelativePath(*e.DestPath); err != nil {
			writeError(w, r, http.StatusUnprocessableEntity, "unsafe_path",
				fmt.Sprintf("event %s dest_path rejected: %v", e.ID, err))
			return out, false
		}
		out.DestPath = e.DestPath
	}
	for name, v := range map[string]*string{"old_sha256": e.OldSHA256, "new_sha256": e.NewSHA256} {
		if v != nil && !shaRe.MatchString(*v) {
			badRequest(w, r, "event %s: %s must be 64 hex characters", e.ID, name)
			return out, false
		}
	}
	out.OldSHA256, out.NewSHA256 = e.OldSHA256, e.NewSHA256
	for name, v := range map[string]*float64{"old_entropy": e.OldEntropy, "new_entropy": e.NewEntropy} {
		if v != nil && (*v < 0 || *v > 8) {
			badRequest(w, r, "event %s: %s must be within 0..8 bits/byte", e.ID, name)
			return out, false
		}
	}
	out.OldEntropy, out.NewEntropy = e.OldEntropy, e.NewEntropy
	for name, v := range map[string]*int64{"old_size": e.OldSize, "new_size": e.NewSize} {
		if v != nil && *v < 0 {
			badRequest(w, r, "event %s: %s must be >= 0", e.ID, name)
			return out, false
		}
	}
	out.OldSize, out.NewSize = e.OldSize, e.NewSize
	out.Actor, out.ScenarioID = e.Actor, e.ScenarioID
	if e.ScenarioID != nil && len(*e.ScenarioID) > 64 {
		badRequest(w, r, "event %s: scenario_id too long", e.ID)
		return out, false
	}
	return out, true
}

func (s *Server) handleIngestEvents(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	var req struct {
		Events []eventPayload `json:"events"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Events) == 0 {
		badRequest(w, r, "events array must not be empty")
		return
	}
	if len(req.Events) > s.cfg.MaxFiles {
		writeError(w, r, http.StatusRequestEntityTooLarge, "limit_exceeded",
			fmt.Sprintf("batch exceeds %d events", s.cfg.MaxFiles))
		return
	}
	ingested, duplicates := 0, 0
	for _, ep := range req.Events {
		ev, ok := validateEvent(w, r, wl.ID, ep)
		if !ok {
			return
		}
		ev.IngestedAt = nowString()
		ins, err := s.store.IngestEvent(ev)
		if err != nil {
			mapError(w, r, err)
			return
		}
		if ins {
			ingested++
		} else {
			duplicates++
		}
	}
	metrics.Inc("events_ingested_total", "outcome", "accepted")
	s.audit(r, role(r), "events.ingest", wl.ID, fmt.Sprintf("accepted=%d duplicates=%d", ingested, duplicates))
	writeJSON(w, http.StatusOK, map[string]any{
		"workload_id": wl.ID, "ingested": ingested, "duplicates": duplicates, "received": len(req.Events),
	})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	limit := 1000
	if l := r.URL.Query().Get("limit"); l != "" {
		var n int
		if _, err := fmt.Sscanf(l, "%d", &n); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	events, err := s.store.ListEvents(wl.ID, limit)
	if err != nil {
		mapError(w, r, err)
		return
	}
	if events == nil {
		events = []domain.FileEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workload_id": wl.ID, "events": events, "count": len(events)})
}

func (s *Server) workload(w http.ResponseWriter, r *http.Request) (domain.Workload, bool) {
	id := r.PathValue("id")
	wl, err := s.store.GetWorkload(id)
	if err != nil {
		mapError(w, r, err)
		return domain.Workload{}, false
	}
	return wl, true
}

func (s *Server) handleCaptureSnapshot(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	var req struct {
		CapturedAt string `json:"captured_at,omitempty"`
	}
	// Body optional for this endpoint.
	if r.ContentLength > 0 {
		if !decodeBody(w, r, &req) {
			return
		}
	}
	var snap domain.Snapshot
	err := s.gate.try(func() error {
		var err error
		snap, err = s.snaps.Capture(wl, req.CapturedAt)
		return err
	})
	if err != nil {
		mapError(w, r, err)
		return
	}
	s.audit(r, role(r), "snapshot.capture", snap.ID, "sealed")
	writeJSON(w, http.StatusCreated, snap)
}

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	list, err := s.store.ListSnapshots(wl.ID)
	if err != nil {
		mapError(w, r, err)
		return
	}
	if list == nil {
		list = []domain.Snapshot{}
	}
	// Enrich with current assessments when present.
	type snapWithAssess struct {
		domain.Snapshot
		Assessment *domain.SnapshotAssessment `json:"assessment,omitempty"`
	}
	out := make([]snapWithAssess, 0, len(list))
	for _, sn := range list {
		row := snapWithAssess{Snapshot: sn}
		if a, err := s.store.GetSnapshotAssessment(sn.ID); err == nil {
			row.Assessment = &a
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"workload_id": wl.ID, "snapshots": out, "count": len(out)})
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	var req struct {
		WindowSeconds       int     `json:"window_seconds,omitempty"`
		BaselineWindowCount int     `json:"baseline_window_count,omitempty"`
		DetectorVersion     string  `json:"detector_version,omitempty"`
	}
	if r.ContentLength > 0 {
		if !decodeBody(w, r, &req) {
			return
		}
	}
	params := detectorParams(s.cfg, req.WindowSeconds, req.BaselineWindowCount, req.DetectorVersion)
	res, err := runAnalysis(s, wl, params)
	if err != nil {
		mapError(w, r, err)
		return
	}
	s.audit(r, role(r), "workload.analyze", wl.ID, res.Run.ID)
	ar := &analysisResult{Run: res.Run, Windows: res.Windows, Onset: res.Run.OnsetTime, AffectedFiles: res.AffectedFiles}
	writeJSON(w, http.StatusOK, analysisResponse(ar, res.AffectedFiles))
}

func (s *Server) handleFindings(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	run, err := s.store.LatestAnalysisRun(wl.ID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found",
			"no completed analysis for workload; run POST /workloads/{id}/analyze first")
		return
	}
	windows, err := s.store.ListDetectionWindows(run.ID)
	if err != nil {
		mapError(w, r, err)
		return
	}
	// affected files: recompute from suspicious windows' events.
	events, err := s.store.ListEvents(wl.ID, 0)
	if err != nil {
		mapError(w, r, err)
		return
	}
	var affected []string
	seen := map[string]bool{}
	for _, win := range windows {
		if win.AlertLevel != domain.AlertSuspicious {
			continue
		}
		for _, e := range events {
			if e.EventTime >= win.StartsAt && e.EventTime < win.EndsAt && !seen[e.Path] {
				seen[e.Path] = true
				affected = append(affected, e.Path)
			}
		}
	}
	res := &analysisResult{Run: run, Windows: windows, Onset: run.OnsetTime, AffectedFiles: affected}
	writeJSON(w, http.StatusOK, analysisResponse(res, affected))
}

func (s *Server) handleCandidates(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	rep, err := s.sel.Candidates(wl)
	if err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	var req struct {
		SnapshotID string `json:"snapshot_id"`
		TargetName string `json:"target_name"`
		Confirm    bool   `json:"confirm"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	var rep *restoreReport
	err := s.gate.try(func() error {
		var err error
		rep, err = s.restorer.Execute(wl, req.SnapshotID, req.TargetName, req.Confirm)
		return err
	})
	if err != nil {
		s.audit(r, role(r), "restore.request", req.SnapshotID, "rejected: "+err.Error())
		mapError(w, r, err)
		return
	}
	s.audit(r, role(r), "restore.execute", rep.RunID, rep.Status)
	status := http.StatusCreated
	if rep.Status == "failed" {
		status = http.StatusConflict
	}
	writeJSON(w, status, rep)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetRecoveryRun(r.PathValue("id"))
	if err != nil {
		mapError(w, r, err)
		return
	}
	// Include the stored per-file report (proposed in PRD §6.3: per-file
	// verification GET) so the console can render outcomes after navigation.
	type runWithReport struct {
		domain.RecoveryRun
		Report map[string]any `json:"report,omitempty"`
	}
	out := runWithReport{RecoveryRun: run}
	var raw string
	if err := s.store.DB().QueryRow(`SELECT report_json FROM recovery_runs WHERE id=?`, run.ID).Scan(&raw); err == nil && raw != "" {
		rep := map[string]any{}
		if json.Unmarshal([]byte(raw), &rep) == nil {
			out.Report = rep
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	runs, err := s.store.ListRecoveryRuns(wl.ID)
	if err != nil {
		mapError(w, r, err)
		return
	}
	if runs == nil {
		runs = []domain.RecoveryRun{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workload_id": wl.ID, "recovery_runs": runs})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.ListAuditEvents(200)
	if err != nil {
		mapError(w, r, err)
		return
	}
	if events == nil {
		events = []domain.AuditEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit_events": events, "count": len(events)})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":             "LOCAL SIMULATION — not a production data-protection product",
		"detector_version": s.cfg.DetectorVersion,
		"window_seconds":   s.cfg.WindowSeconds,
		"baseline_windows": s.cfg.BaselineWindows,
		"risk_threshold":   s.cfg.RiskThreshold,
		"max_files":        s.cfg.MaxFiles,
		"max_bytes":        s.cfg.MaxBytes,
		"limits": []string{
			"No live malware is ever executed, packaged or downloaded.",
			"Restores go only to server-generated directories under the local sandbox.",
			"Sealed snapshots are logically immutable via API policy; this is not WORM storage.",
			"A valid hash proves integrity, NOT absence of compromise.",
			"The detector never reads hidden evaluation ground truth.",
		},
		"cli": []string{
			"cleanpoint serve", "cleanpoint simulate --scenario <name> --seed <n>",
			"cleanpoint evaluate --scenario <folder> --json",
			"cleanpoint benchmark snapshots --files 1000 --file-size-kb 32 --output <file>",
			"cleanpoint benchmark restore --files 1000 --file-size-kb 32 --output <file>",
		},
	})
}

func (s *Server) audit(r *http.Request, actor, action, target, outcome string) {
	_ = s.store.InsertAuditEvent(actor, action, target, outcome, requestID(r))
}
