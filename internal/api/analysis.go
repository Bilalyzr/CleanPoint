package api

import (
	"cleanpoint/internal/config"
	"cleanpoint/internal/detector"
	"cleanpoint/internal/domain"
	"cleanpoint/internal/restore"
)

// restoreReport aliases the restore package report for handler responses.
type restoreReport = restore.Report

// analysisResult mirrors detector.Result minus assessments detail.
type analysisResult struct {
	Run           domain.AnalysisRun
	Windows       []domain.DetectionWindow
	Onset         *string
	AffectedFiles []string
}

// detectorParams applies safe defaults and floors to caller input.
func detectorParams(cfg *config.Config, windowSeconds, baselineWindowCount int, detectorVersion string) detector.RunParams {
	p := detector.RunParams{
		WindowSeconds:       cfg.WindowSeconds,
		BaselineWindowCount: cfg.BaselineWindows,
		DetectorVersion:     cfg.DetectorVersion,
		RiskThreshold:       cfg.RiskThreshold,
	}
	if windowSeconds >= 60 {
		p.WindowSeconds = windowSeconds
	}
	if baselineWindowCount >= 1 {
		p.BaselineWindowCount = baselineWindowCount
	}
	if detectorVersion != "" {
		p.DetectorVersion = detectorVersion
	}
	return p
}

// runAnalysis invokes the detector pipeline with this server's snapshot
// verifier.
func runAnalysis(s *Server, wl domain.Workload, params detector.RunParams) (*detector.Result, error) {
	return detector.Run(s.store, s.snaps, wl, params)
}

// analysisResponse shapes the /analyze and /findings payloads.
func analysisResponse(res *analysisResult, affected []string) map[string]any {
	if res.Windows == nil {
		res.Windows = []domain.DetectionWindow{}
	}
	if affected == nil {
		affected = []string{}
	}
	var note string
	if res.Run.WindowsEvaluated == 0 {
		note = "Insufficient normal history: every window belongs to the baseline, so no snapshot can be certified (exposure unknown; selector will abstain)."
	}
	return map[string]any{
		"analysis_run_id":   res.Run.ID,
		"workload_id":       res.Run.WorkloadID,
		"detector_version":  res.Run.DetectorVersion,
		"risk_threshold":    res.Run.RiskThreshold,
		"baseline_period":   res.Run.BaselinePeriod,
		"baseline_windows":  res.Run.BaselineWindowCount,
		"window_seconds":    res.Run.WindowSeconds,
		"windows_evaluated": res.Run.WindowsEvaluated,
		"incident_onset":    res.Run.OnsetTime,
		"windows":           res.Windows,
		"affected_files":    affected,
		"note":              note,
		"warning":           "risk_index is a relative anomaly score (0..1), not a probability; integrity proves consistency, not absence of compromise.",
	}
}
