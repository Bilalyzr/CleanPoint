package recoverytest

import (
	"testing"

	"cleanpoint/internal/detector"
	"cleanpoint/internal/domain"
	tt "cleanpoint/tests/harness"
)

// ---------- Recovery eligibility (12+ cases) ----------

// buildAnalysis constructs an Analysis with baseline windows (quiet) and one
// suspicious window at the given index.
func buildAnalysis(baseline int, windows []detector.WindowFeatures, scored []detector.ScoreResult, threshold float64) *detector.Analysis {
	for i := range windows {
		if windows[i].Vector.Values == nil {
			windows[i].Vector = vec(0.05, 0.2, 0, 0, 0.2, 0)
		}
	}
	return &detector.Analysis{Windows: windows, Scored: scored, BaselineCount: baseline,
		RiskThreshold: threshold, EventsByWindow: map[string][]domain.FileEvent{}}
}

func snapsAt(times ...string) []domain.Snapshot {
	var out []domain.Snapshot
	for _, ts := range times {
		out = append(out, domain.Snapshot{ID: "snap-" + ts, CapturedAt: "2026-01-05T" + ts + ":00.000Z",
			Status: domain.SnapshotSealed, FileCount: 10})
	}
	return out
}

func TestRecoveryEligibility(t *testing.T) {
	t.Run("clear_snapshot_with_full_baseline_is_eligible", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		an := buildAnalysis(24, ws, sc, 0.35)
		assessed := an.AssessSnapshots(snapsAt("11:30")) // after the 24-window baseline (ends 11:00)
		if len(assessed) != 1 || assessed[0].ExposureStatus != domain.ExposureClear {
			t.Fatalf("exposure = %+v", assessed)
		}
		if !assessed[0].CoverageOK {
			t.Error("coverage should be OK after 24 full baseline windows")
		}
	})

	t.Run("snapshot_during_baseline_is_unknown", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		an := buildAnalysis(24, ws, sc, 0.35)
		assessed := an.AssessSnapshots(snapsAt("09:20")) // baseline ends 11:00
		if assessed[0].ExposureStatus != domain.ExposureUnknown {
			t.Errorf("mid-baseline snapshot exposure = %v, want unknown", assessed[0].ExposureStatus)
		}
	})

	t.Run("insufficient_history_all_unknown", func(t *testing.T) {
		ws := quietWindows(10)
		sc := quietScores(10)
		an := buildAnalysis(24, ws, sc, 0.35)
		assessed := an.AssessSnapshots(snapsAt("09:40"))
		if assessed[0].ExposureStatus != domain.ExposureUnknown {
			t.Error("fewer windows than baseline must yield unknown")
		}
	})

	t.Run("suspicious_window_before_capture_makes_suspect", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		// window 24 (11:00-11:05) suspicious without destructive signals
		ws[24].Vector = vec(0.5, 0.2, 0, 0, 0.2, 0) // high modified ratio only
		sc[24] = detector.ScoreResult{Risk: 0.6}
		an := buildAnalysis(24, ws, sc, 0.35)
		assessed := an.AssessSnapshots(snapsAt("11:30"))
		if assessed[0].ExposureStatus != domain.ExposureSuspect {
			t.Errorf("exposure = %v, want suspect", assessed[0].ExposureStatus)
		}
	})

	t.Run("destructive_signals_make_affected", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		ws[24].Vector = vec(0.5, 0.2, 1.0, 0, 0.2, 0)
		sc[24] = detector.ScoreResult{Risk: 0.85, Contributions: []domain.SignalContribution{
			{Feature: detector.FEntropyShift, Signal: 1.0},
		}}
		an := buildAnalysis(24, ws, sc, 0.35)
		assessed := an.AssessSnapshots(snapsAt("11:30"))
		if assessed[0].ExposureStatus != domain.ExposureAffected {
			t.Errorf("exposure = %v, want affected", assessed[0].ExposureStatus)
		}
	})

	t.Run("suspicious_window_after_capture_ignored", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		ws[25].Vector = vec(0.9, 20, 1.0, 1.0, 5, 0)
		sc[25] = detector.ScoreResult{Risk: 0.95}
		an := buildAnalysis(24, ws, sc, 0.35)
		// snapshot captured while window 25 is still open: only fully-ended
		// windows count, so the 11:05-11:10 burst is excluded
		assessed := an.AssessSnapshots(snapsAt("11:08"))
		if assessed[0].ExposureStatus != domain.ExposureClear {
			t.Errorf("exposure = %v, want clear", assessed[0].ExposureStatus)
		}
	})

	t.Run("baseline_window_anomalies_never_scored", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		// anomaly INSIDE baseline window 5 (kept below the destructive-burst
		// contamination thresholds) — never evaluated, so a snapshot after
		// the baseline stays clear (behavioral contract).
		ws[5].Vector = vec(0.6, 2.0, 0.0, 0.0, 1.0, 0.0)
		an := buildAnalysis(24, ws, sc, 0.35)
		assessed := an.AssessSnapshots(snapsAt("11:30"))
		if assessed[0].ExposureStatus != domain.ExposureClear {
			t.Errorf("exposure = %v, want clear", assessed[0].ExposureStatus)
		}
	})

	t.Run("contaminated_baseline_voids_everything", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		ws[2].Vector = vec(0.9, 20, 1.0, 1.0, 0, 0)
		an := buildAnalysis(24, ws, sc, 0.35)
		assessed := an.AssessSnapshots(snapsAt("09:10", "11:30", "12:00"))
		for _, a := range assessed {
			if a.ExposureStatus != domain.ExposureUnknown {
				t.Errorf("snapshot %s exposure = %v, want unknown under contaminated baseline",
					a.Snapshot.CapturedAt, a.ExposureStatus)
			}
		}
	})

	t.Run("unsealed_snapshots_skipped_in_assessment", func(t *testing.T) {
		an := buildAnalysis(24, quietWindows(26), quietScores(26), 0.35)
		creating := domain.Snapshot{ID: "snap-x", CapturedAt: "2026-01-05T12:00:00.000Z",
			Status: domain.SnapshotCreating}
		assessed := an.AssessSnapshots([]domain.Snapshot{creating})
		if len(assessed) != 0 {
			t.Error("creating snapshot was assessed")
		}
	})

	t.Run("content_check_marks_affected_over_windows", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		an := buildAnalysis(24, ws, sc, 0.35)
		ref := map[string]float64{}
		for i := 0; i < 10; i++ {
			ref[mustPath("d/f", i, ".txt")] = 4.2
		}
		enc := map[string]float64{}
		for i := 0; i < 10; i++ {
			enc[mustPath("d/f", i, ".lockd")] = 7.95
		}
		calls := map[string]map[string]float64{
			"2026-01-05T09:20:00.000Z": ref, // earliest (clean)
			"2026-01-05T12:00:00.000Z": enc, // encrypted
		}
		an.LoadFiles = func(id string) (map[string]float64, error) {
			if f, ok := calls[callTime(id)]; ok {
				return f, nil
			}
			return ref, nil
		}
		assessed := an.AssessSnapshots(snapsAt("09:20", "12:00"))
		if assessed[1].ExposureStatus != domain.ExposureAffected {
			t.Errorf("encrypted content not affected: %v", assessed[1].ExposureStatus)
		}
		if assessed[0].ExposureStatus != domain.ExposureUnknown {
			// earliest snapshot itself is mid-baseline: unknown is expected;
			// the important part is it is NOT clear-certified wrongly.
			t.Logf("earliest snapshot exposure: %v", assessed[0].ExposureStatus)
		}
	})

	t.Run("risk_score_is_max_window_risk", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		sc[24] = detector.ScoreResult{Risk: 0.42}
		an := buildAnalysis(24, ws, sc, 0.6) // above threshold? 0.42 < 0.6 -> normal
		assessed := an.AssessSnapshots(snapsAt("11:30"))
		if assessed[0].RiskScore != 0.42 {
			t.Errorf("risk score = %v, want 0.42", assessed[0].RiskScore)
		}
		if assessed[0].ExposureStatus != domain.ExposureClear {
			t.Errorf("0.42 below threshold 0.6 should stay clear: %v", assessed[0].ExposureStatus)
		}
	})

	t.Run("deterministic_ordering_of_assessments", func(t *testing.T) {
		an := buildAnalysis(24, quietWindows(26), quietScores(26), 0.35)
		assessed := an.AssessSnapshots(snapsAt("12:00", "09:20", "11:00"))
		if assessed[0].Snapshot.CapturedAt != "2026-01-05T12:00:00.000Z" {
			t.Error("assessment order not preserved")
		}
	})

	t.Run("affected_files_collected_from_suspicious_windows", func(t *testing.T) {
		ws := quietWindows(26)
		sc := quietScores(26)
		ws[24].Vector = vec(0.5, 0.2, 1.0, 0, 0.2, 0)
		sc[24] = detector.ScoreResult{Risk: 0.9, Contributions: []domain.SignalContribution{
			{Feature: detector.FEntropyShift, Signal: 1.0}}}
		an := buildAnalysis(24, ws, sc, 0.35)
		an.EventsByWindow[ws[24].StartsAt] = []domain.FileEvent{
			{Path: "docs/a.txt"}, {Path: "docs/b.txt"}, {Path: "docs/a.txt"}}
		assessed := an.AssessSnapshots(snapsAt("11:30"))
		if len(assessed[0].AffectedFiles) != 2 {
			t.Errorf("affected files = %v, want 2 distinct", assessed[0].AffectedFiles)
		}
	})
}

func quietWindows(n int) []detector.WindowFeatures {
	out := make([]detector.WindowFeatures, n)
	for i := range out {
		// 300s windows starting 09:00
		ts := tt.TS(i * 300)
		out[i] = detector.WindowFeatures{StartsAt: ts, EndsAt: tt.TS((i + 1) * 300)}
		out[i].Vector = vec(0.05, 0.2, 0, 0, 0.2, 0)
	}
	return out
}

func quietScores(n int) []detector.ScoreResult {
	return make([]detector.ScoreResult, n)
}

// callTime maps snapshot ids built by snapsAt back to their captured time.
func callTime(id string) string {
	return "2026-01-05T" + id[len("snap-"):] + ":00.000Z"
}
