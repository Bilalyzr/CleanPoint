package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cleanpoint/internal/domain"
	"cleanpoint/internal/metrics"
)

// ---------- M2 support: snapshot detail, analysis history, evaluations ----------

// handleSnapshotDetail returns one snapshot with its assessment and manifest
// rows (paginated) — the proposed optional GET /snapshots/{id}.
func (s *Server) handleSnapshotDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	snap, err := s.store.GetSnapshot(id)
	if err != nil {
		mapError(w, r, err)
		return
	}
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		var n int
		if _, err := fmt.Sscanf(l, "%d", &n); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	files, err := s.store.ListSnapshotFiles(id)
	if err != nil {
		mapError(w, r, err)
		return
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	type fileRow struct {
		Path    string   `json:"path"`
		SHA256  string   `json:"sha256"`
		Size    int64    `json:"size_bytes"`
		Entropy *float64 `json:"entropy,omitempty"`
	}
	rows := make([]fileRow, 0, limit)
	for i, p := range paths {
		if i >= limit {
			break
		}
		f := files[p]
		rows = append(rows, fileRow{Path: p, SHA256: f.SHA256, Size: f.Size, Entropy: f.Entropy})
	}
	resp := map[string]any{
		"snapshot":  snap,
		"files":     rows,
		"file_total": len(paths),
	}
	if a, err := s.store.GetSnapshotAssessment(id); err == nil {
		resp["assessment"] = a
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAnalysisRuns returns the analysis history of a workload (proposed
// optional GET): every analysis run with its parameters and outcome.
func (s *Server) handleAnalysisRuns(w http.ResponseWriter, r *http.Request) {
	wl, ok := s.workload(w, r)
	if !ok {
		return
	}
	rows, err := s.store.DB().Query(`
		SELECT id, detector_version, baseline_period, window_seconds, baseline_window_count,
		       risk_threshold, status, onset_time, windows_evaluated, started_at, finished_at
		FROM analysis_runs WHERE workload_id=? ORDER BY started_at DESC`, wl.ID)
	if err != nil {
		mapError(w, r, err)
		return
	}
	defer rows.Close()
	type runRow struct {
		ID                  string   `json:"id"`
		DetectorVersion     string   `json:"detector_version"`
		BaselinePeriod      string   `json:"baseline_period"`
		WindowSeconds       int      `json:"window_seconds"`
		BaselineWindowCount int      `json:"baseline_window_count"`
		RiskThreshold       float64  `json:"risk_threshold"`
		Status              string   `json:"status"`
		OnsetTime           *string  `json:"incident_onset,omitempty"`
		WindowsEvaluated    int      `json:"windows_evaluated"`
		StartedAt           string   `json:"started_at"`
		FinishedAt          *string  `json:"finished_at,omitempty"`
	}
	var out []runRow
	for rows.Next() {
		var rr runRow
		var onset, fin any
		if err := rows.Scan(&rr.ID, &rr.DetectorVersion, &rr.BaselinePeriod, &rr.WindowSeconds,
			&rr.BaselineWindowCount, &rr.RiskThreshold, &rr.Status, &onset,
			&rr.WindowsEvaluated, &rr.StartedAt, &fin); err != nil {
			mapError(w, r, err)
			return
		}
		if v, ok := onset.(string); ok {
			rr.OnsetTime = &v
		}
		if v, ok := fin.(string); ok {
			rr.FinishedAt = &v
		}
		out = append(out, rr)
	}
	if out == nil {
		out = []runRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workload_id": wl.ID, "analysis_runs": out})
}

// handleEvaluations exposes the checked-in evaluation report for the console
// (proposed optional GET /evaluations). Nothing is invented here: it serves
// the measured artifact if present.
func (s *Server) handleEvaluations(w http.ResponseWriter, r *http.Request) {
	dir := os.Getenv("CLEANPOINT_RESULTS_DIR")
	if dir == "" {
		dir = "results"
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "eval-locked.json"
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') || !strings.HasSuffix(name, ".json") {
		badRequest(w, r, "evaluation name must be a plain .json file name")
		return
	}
	data, err := os.ReadFile(filepath.FromSlash(filepath.Join(dir, name)))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false,
			"note": "no evaluation report found; run `cleanpoint evaluate --scenario locked --output results/eval-locked.json`"})
		return
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		mapError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ---------- Simulation Lab: scenario-control jobs (operator-only) ----------

// simScenarioFamilies mirrors simulator/scenarios.py; validated server-side
// so no arbitrary strings reach the shell.
var simScenarioFamilies = map[string]bool{
	"normal_office": true, "benign_archive": true, "benign_bulk_update": true,
	"rapid_destructive": true, "slow_distributed": true, "extension_churn": true,
	"mixed_activity": true, "pre_existing_contamination": true,
}

type simJob struct {
	ID         string         `json:"id"`
	Scenario   string         `json:"scenario"`
	Seed       int            `json:"seed"`
	Status     string         `json:"status"` // running | completed | failed
	Log        []string       `json:"log"`
	Result     map[string]any `json:"result,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  string         `json:"started_at"`
	FinishedAt *string        `json:"finished_at,omitempty"`
}

type simRegistry struct {
	mu   sync.Mutex
	jobs map[string]*simJob
}

var simReg = &simRegistry{jobs: map[string]*simJob{}}

// pythonExe resolves the interpreter used by the deterministic generator.
func pythonExe() string {
	if v := os.Getenv("CLEANPOINT_PYTHON"); v != "" {
		return v
	}
	return "python"
}

// repoRoot locates the repository root (development layout).
func repoRoot() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func (s *Server) handleSimulateCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scenario string `json:"scenario"`
		Seed     int    `json:"seed"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if !simScenarioFamilies[req.Scenario] {
		badRequest(w, r, "unknown scenario family %q", req.Scenario)
		return
	}
	if req.Seed < 1 || req.Seed > 999 {
		badRequest(w, r, "seed must be 1..999")
		return
	}
	simReg.mu.Lock()
	for _, j := range simReg.jobs {
		if j.Status == "running" {
			simReg.mu.Unlock()
			writeError(w, r, http.StatusConflict, "busy",
				"a simulation is already running; wait for it to finish")
			return
		}
	}
	id, err := domain.NewWorkloadID()
	id = "sim-" + strings.TrimPrefix(id, "wl-")
	if err != nil {
		mapError(w, r, err)
		return
	}
	job := &simJob{ID: id, Scenario: req.Scenario, Seed: req.Seed,
		Status: "running", StartedAt: domain.FormatTime(time.Now())}
	simReg.jobs[id] = job
	simReg.mu.Unlock()

	go s.runSimulation(job)
	s.audit(r, role(r), "simulate.start", fmt.Sprintf("%s:%d", req.Scenario, req.Seed), id)
	writeJSON(w, http.StatusAccepted, job)
}

// runSimulation executes the deterministic Python scenario driver against
// this same server (operator token in env only, never logged).
func (s *Server) runSimulation(job *simJob) {
	appendLog := func(line string) {
		simReg.mu.Lock()
		if len(job.Log) < 400 {
			job.Log = append(job.Log, line)
		}
		simReg.mu.Unlock()
	}
	resultPath := filepath.Join(filepath.FromSlash(s.cfg.SandboxRoot), "sim",
		fmt.Sprintf("%s-seed%d-ui.json", job.Scenario, job.Seed))
	cmd := exec.CommandContext(context.Background(), pythonExe(),
		filepath.Join(repoRoot(), "simulator", "scenario_driver.py"),
		"--scenario", job.Scenario,
		"--seed", fmt.Sprint(job.Seed),
		"--api", "http://"+s.cfg.Addr,
		"--result", resultPath)
	cmd.Dir = repoRoot()
	cmd.Env = append(os.Environ(),
		"CLEANPOINT_OPERATOR_TOKEN="+s.cfg.OperatorToken,
		"CLEANPOINT_API=http://"+s.cfg.Addr)
	out, err := cmd.CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			appendLog(strings.TrimSpace(line))
		}
	}
	fin := domain.FormatTime(time.Now())
	simReg.mu.Lock()
	job.FinishedAt = &fin
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		metrics.Inc("simulation_total", "outcome", "failed")
	} else {
		job.Status = "completed"
		if data, rerr := os.ReadFile(resultPath); rerr == nil {
			res := map[string]any{}
			if json.Unmarshal(data, &res) == nil {
				job.Result = res
			}
		}
		metrics.Inc("simulation_total", "outcome", "completed")
	}
	simReg.mu.Unlock()
}

func (s *Server) handleSimulationGet(w http.ResponseWriter, r *http.Request) {
	simReg.mu.Lock()
	defer simReg.mu.Unlock()
	job, ok := simReg.jobs[r.PathValue("id")]
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "unknown simulation job")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleSimulationList(w http.ResponseWriter, r *http.Request) {
	simReg.mu.Lock()
	defer simReg.mu.Unlock()
	out := make([]*simJob, 0, len(simReg.jobs))
	for _, j := range simReg.jobs {
		out = append(out, j)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].StartedAt > out[k].StartedAt })
	if len(out) > 20 {
		out = out[:20]
	}
	writeJSON(w, http.StatusOK, map[string]any{"simulations": out})
}
