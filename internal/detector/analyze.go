package detector

import (
	"fmt"
	"time"

	"cleanpoint/internal/domain"
	"cleanpoint/internal/metrics"
	"cleanpoint/internal/snapshots"
	"cleanpoint/internal/storage"
)

// Verifier abstracts snapshot integrity verification for the analysis run.
type Verifier interface {
	Verify(snapshotID string) (snapshots.IntegrityReport, error)
}

// RunParams are the caller-controllable analysis inputs (with conservative
// floors: the baseline must contain at least the configured window count).
type RunParams struct {
	WindowSeconds       int
	BaselineWindowCount int
	DetectorVersion     string
	RiskThreshold       float64
}

// Result summarizes a completed analysis for API serialization.
type Result struct {
	Run           domain.AnalysisRun
	Windows       []domain.DetectionWindow
	Onset         *string
	Assessments   []AssessedSnapshot
	AffectedFiles []string
	Notes         []string
}

// Run executes the full detection pipeline for one workload and persists all
// outputs. The detector sees ONLY ingested events and sealed snapshot
// metadata — never scenario ground truth.
func Run(store *storage.Store, verifier Verifier, workload domain.Workload, params RunParams) (*Result, error) {
	started := time.Now()
	runID, err := domain.NewAnalysisRunID()
	if err != nil {
		return nil, err
	}
	events, err := store.ListEvents(workload.ID, 0)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("workload has no events to analyze")
	}

	wf := ExtractFeatures(events, params.WindowSeconds)
	if len(wf) == 0 {
		return nil, fmt.Errorf("no windows extracted")
	}
	baselineCount := params.BaselineWindowCount
	if baselineCount > len(wf) {
		baselineCount = len(wf)
	}
	ApplyExtensionNovelty(wf, baselineCount)

	baselineVectors := make([]FeatureVector, 0, baselineCount)
	for i := 0; i < baselineCount; i++ {
		baselineVectors = append(baselineVectors, wf[i].Vector)
	}
	baseline := TrainBaseline(baselineVectors)

	scored := make([]ScoreResult, len(wf))
	for i := range wf {
		if i < baselineCount {
			scored[i] = ScoreResult{}
			continue
		}
		scored[i] = Score(wf[i].Vector, baseline)
	}

	analysis := &Analysis{
		Windows:        wf,
		Scored:         scored,
		BaselineCount:  baselineCount,
		RiskThreshold:  params.RiskThreshold,
		EventsByWindow: map[string][]domain.FileEvent{},
		LoadFiles: func(snapshotID string) (map[string]float64, error) {
			rows, err := store.ListSnapshotFiles(snapshotID)
			if err != nil {
				return nil, err
			}
			out := make(map[string]float64, len(rows))
			for p, r := range rows {
				e := 0.0
				if r.Entropy != nil {
					e = *r.Entropy
				}
				out[p] = e
			}
			return out, nil
		},
	}
	for _, e := range events {
		w := WindowOfEvent(e, params.WindowSeconds)
		analysis.EventsByWindow[w] = append(analysis.EventsByWindow[w], e)
	}

	baselinePeriod := fmt.Sprintf("%s..%s", wf[0].StartsAt, wf[baselineCount-1].EndsAt)
	run := domain.AnalysisRun{
		ID:                  runID,
		WorkloadID:          workload.ID,
		DetectorVersion:     params.DetectorVersion,
		BaselinePeriod:      baselinePeriod,
		WindowSeconds:       params.WindowSeconds,
		BaselineWindowCount: baselineCount,
		RiskThreshold:       params.RiskThreshold,
		Status:              "running",
		StartedAt:           domain.FormatTime(started),
	}
	if err := store.InsertAnalysisRun(run); err != nil {
		return nil, err
	}

	evaluated := 0
	for i, w := range wf {
		isBase := i < baselineCount
		alert := domain.AlertNormal
		risk := 0.0
		if !isBase {
			risk = scored[i].Risk
			if risk >= params.RiskThreshold {
				alert = domain.AlertSuspicious
			}
			evaluated++
		}
		winID, err := domain.NewWindowID()
		if err != nil {
			return nil, err
		}
		dw := domain.DetectionWindow{
			ID:            winID,
			AnalysisRunID: runID,
			WorkloadID:    workload.ID,
			StartsAt:      w.StartsAt,
			EndsAt:        w.EndsAt,
			Features:      w.Vector.Values,
			RiskScore:     risk,
			AlertLevel:    alert,
			Contributions: scored[i].Contributions,
			IsBaseline:    isBase,
			EventCount:    w.Vector.Events,
		}
		if err := store.InsertDetectionWindow(dw, storage.EncodeFeatureJSON(w.Vector.Values), storage.EncodeContribJSON(scored[i].Contributions)); err != nil {
			return nil, err
		}
	}

	onset := analysis.EarliestOnset()

	snapList, err := store.ListSnapshots(workload.ID)
	if err != nil {
		return nil, err
	}
	assessed := analysis.AssessSnapshots(snapList)

	var affectedFiles []string
	for i := range assessed {
		rep, err := verifier.Verify(assessed[i].Snapshot.ID)
		if err != nil {
			return nil, fmt.Errorf("verify snapshot %s: %w", assessed[i].Snapshot.ID, err)
		}
		assessed[i].IntegrityOK = rep.ManifestOK && len(rep.Problems) == 0
		if !assessed[i].IntegrityOK {
			assessed[i].Explanation = append(assessed[i].Explanation,
				fmt.Sprintf("Integrity verification failed: %d problem(s), e.g. %v", len(rep.Problems), firstOr(rep.Problems, "unknown")))
			metrics.Inc("integrity_failures_total", "", "")
		}
		for _, f := range assessed[i].AffectedFiles {
			affectedFiles = appendUnique(affectedFiles, f)
		}
		a := domain.SnapshotAssessment{
			SnapshotID:        assessed[i].Snapshot.ID,
			AnalysisRunID:     runID,
			IntegrityOK:       assessed[i].IntegrityOK,
			CoverageOK:        assessed[i].CoverageOK,
			ExposureStatus:    assessed[i].ExposureStatus,
			RiskScore:         assessed[i].RiskScore,
			AffectedFileCount: len(assessed[i].AffectedFiles),
			Explanation:       assessed[i].Explanation,
			EvaluatedAt:       domain.FormatTime(time.Now()),
		}
		if err := store.UpsertSnapshotAssessment(a, storage.EncodeStringList(a.Explanation)); err != nil {
			return nil, err
		}
	}

	if err := store.FinishAnalysisRun(runID, "completed", onset, evaluated); err != nil {
		return nil, err
	}
	run.Status = "completed"
	run.OnsetTime = onset
	run.WindowsEvaluated = evaluated
	fin := domain.FormatTime(time.Now())
	run.FinishedAt = fin

	var windowsOut []domain.DetectionWindow
	for i, w := range wf {
		outID, _ := domain.NewWindowID()
		dw := domain.DetectionWindow{
			ID: outID, AnalysisRunID: runID, WorkloadID: workload.ID,
			StartsAt: w.StartsAt, EndsAt: w.EndsAt,
			Features: w.Vector.Values, EventCount: w.Vector.Events,
			IsBaseline: i < baselineCount,
		}
		if i >= baselineCount {
			dw.RiskScore = scored[i].Risk
			dw.AlertLevel = domain.AlertNormal
			if scored[i].Risk >= params.RiskThreshold {
				dw.AlertLevel = domain.AlertSuspicious
			}
			dw.Contributions = scored[i].Contributions
		}
		windowsOut = append(windowsOut, dw)
	}

	var notes []string
	if len(wf) <= baselineCount {
		notes = append(notes, "Insufficient history: every window is baseline; no window could be certified, exposure is unknown for all snapshots.")
	}
	if bad, why := analysis.BaselineContaminated(); bad {
		notes = append(notes, "Baseline contamination suspected: "+why+". All snapshot assessments set to unknown; selector will abstain.")
	}
	if onset != nil {
		notes = append(notes, "Onset is the earliest suspicious window start (conservative lower bound), not a proven infection time.")
	}
	metrics.Observe("analyze_duration_seconds", time.Since(started))

	return &Result{Run: run, Windows: windowsOut, Onset: onset, Assessments: assessed,
		AffectedFiles: affectedFiles, Notes: notes}, nil
}

func firstOr(xs []string, def string) string {
	if len(xs) == 0 {
		return def
	}
	return xs[0]
}
