package recoverytest

import (
	"math"
	"testing"

	"cleanpoint/internal/detector"
	"cleanpoint/internal/domain"
	tt "cleanpoint/tests/harness"
)

// ---------- Detection scoring (12+ cases) ----------

func vec(mods, rename, ent, novel, del, size float64) detector.FeatureVector {
	return detector.FeatureVector{
		Values: map[string]float64{
			detector.FModifiedRatio: mods,
			detector.FRenameRate:    rename,
			detector.FEntropyShift:  ent,
			detector.FExtNovelty:    novel,
			detector.FDeleteRate:    del,
			detector.FSizeChange:    size,
		},
		Missing: map[string]bool{},
	}
}

func TestDetectionScoring(t *testing.T) {
	t.Run("median_even_and_odd", func(t *testing.T) {
		if got := detector.Median([]float64{1, 3, 5}); got != 3 {
			t.Errorf("median odd = %v", got)
		}
		if got := detector.Median([]float64{1, 2, 3, 4}); got != 2.5 {
			t.Errorf("median even = %v", got)
		}
	})

	t.Run("mad_computed_from_median", func(t *testing.T) {
		// values 1..5: median 3, abs devs 2,1,0,1,2 -> MAD 1
		if got := detector.Mad([]float64{1, 2, 3, 4, 5}); got != 1 {
			t.Errorf("mad = %v, want 1", got)
		}
	})

	t.Run("baseline_center_and_scale", func(t *testing.T) {
		vs := []detector.FeatureVector{}
		for i := 0; i < 24; i++ {
			vs = append(vs, vec(0.1, 0.2, 0, 0, 0.2, 0))
		}
		b := detector.TrainBaseline(vs)
		if b.Center[detector.FModifiedRatio] != 0.1 {
			t.Errorf("center = %v", b.Center[detector.FModifiedRatio])
		}
		// constant series -> MAD 0 -> scale clamps to epsilon
		if b.Scale[detector.FModifiedRatio] != 0.01 {
			t.Errorf("scale = %v, want epsilon 0.01", b.Scale[detector.FModifiedRatio])
		}
	})

	t.Run("scale_uses_1_4826_mad", func(t *testing.T) {
		vs := []detector.FeatureVector{}
		vals := []float64{0.1, 0.2, 0.3, 0.4}
		for _, v := range append(vals, vals...) {
			vs = append(vs, vec(v, 0, 0, 0, 0, 0))
		}
		b := detector.TrainBaseline(vs)
		want := 1.4826 * detector.Mad(col(vs, detector.FModifiedRatio))
		if math.Abs(b.Scale[detector.FModifiedRatio]-want) > 1e-12 {
			t.Errorf("scale = %v, want %v", b.Scale[detector.FModifiedRatio], want)
		}
	})

	t.Run("normal_window_scores_zero", func(t *testing.T) {
		vs := []detector.FeatureVector{}
		for i := 0; i < 24; i++ {
			vs = append(vs, vec(0.1, 0.2, 0.05, 0, 0.2, 0))
		}
		b := detector.TrainBaseline(vs)
		s := detector.Score(vec(0.1, 0.2, 0.05, 0, 0.2, 0), b)
		if s.Risk != 0 {
			t.Errorf("normal window risk = %v, want 0", s.Risk)
		}
	})

	t.Run("signal_capped_at_one", func(t *testing.T) {
		b := detector.Baseline{
			Center: map[string]float64{detector.FModifiedRatio: 0.1,
				detector.FRenameRate: 0.2, detector.FEntropyShift: 0.05,
				detector.FExtNovelty: 0, detector.FDeleteRate: 0.2, detector.FSizeChange: 0},
			Scale: map[string]float64{detector.FModifiedRatio: 0.01,
				detector.FRenameRate: 0.5, detector.FEntropyShift: 0.01,
				detector.FExtNovelty: 0.01, detector.FDeleteRate: 0.5, detector.FSizeChange: 0.01},
		}
		s := detector.Score(vec(1.0, 50, 1.0, 1.0, 30, 1.0), b)
		// every signal saturates -> risk = sum of weights = 1.0
		if s.Risk > 1.0 || math.Abs(s.Risk-1.0) > 1e-9 {
			t.Errorf("saturated risk = %v, want 1.0", s.Risk)
		}
	})

	t.Run("z_score_formula", func(t *testing.T) {
		b := detector.Baseline{
			Center: map[string]float64{detector.FModifiedRatio: 0.10},
			Scale:  map[string]float64{detector.FModifiedRatio: 0.05},
		}
		for k := range b.Center {
			b.Scale[k] = 0.05
		}
		b.Center = fullCenter()
		s := detector.Score(vec(0.35, 0, 0, 0, 0, 0), b)
		var zc float64
		for _, c := range s.Contributions {
			if c.Feature == detector.FModifiedRatio {
				zc = c.ZScore
			}
		}
		if math.Abs(zc-5.0) > 1e-9 {
			t.Errorf("z = %v, want 5.0", zc)
		}
	})

	t.Run("missing_feature_not_scored_and_lowers_coverage", func(t *testing.T) {
		b := detector.Baseline{Center: fullCenter(), Scale: fullScale()}
		v := vec(1, 1, 1, 1, 1, 1)
		v.Missing[detector.FEntropyShift] = true
		v.Missing[detector.FSizeChange] = true
		s := detector.Score(v, b)
		if s.Coverage != 4.0/6.0 {
			t.Errorf("coverage = %v, want 4/6", s.Coverage)
		}
		maxPossible := 1.0 - detector.Weights[detector.FEntropyShift] - detector.Weights[detector.FSizeChange]
		if s.Risk > maxPossible+1e-9 {
			t.Errorf("risk %v exceeds reduced maximum %v", s.Risk, maxPossible)
		}
	})

	t.Run("weights_per_source_spec", func(t *testing.T) {
		want := map[string]float64{
			"modified_file_ratio": 0.25, "rename_rate": 0.20, "entropy_shift_ratio": 0.25,
			"extension_novelty": 0.15, "delete_rate": 0.10, "size_change_ratio": 0.05,
		}
		for k, v := range want {
			if detector.Weights[k] != v {
				t.Errorf("weight %s = %v, want %v", k, detector.Weights[k], v)
			}
		}
	})

	t.Run("explanations_name_actual_features", func(t *testing.T) {
		b := detector.Baseline{Center: fullCenter(), Scale: fullScale()}
		s := detector.Score(vec(0.9, 30, 0.9, 0.8, 10, 0.6), b)
		ex := detector.Explain(vec(0.9, 30, 0.9, 0.8, 10, 0.6), s)
		joined := ""
		for _, line := range ex {
			joined += line + " "
		}
		if len(ex) == 0 {
			t.Fatal("no explanations produced")
		}
		for _, frag := range []string{"Modification ratio", "entropy increase"} {
			if !contains(joined, frag) {
				t.Errorf("explanation missing %q: %s", frag, joined)
			}
		}
	})

	t.Run("earliest_onset_is_first_suspicious_window", func(t *testing.T) {
		an := &detector.Analysis{BaselineCount: 2, RiskThreshold: 0.5,
			Windows:  winList("09:00", "09:05", "09:10", "09:15"),
			Scored:   []detector.ScoreResult{{}, {}, {Risk: 0.9}, {Risk: 0.7}}}
		on := an.EarliestOnset()
		if on == nil || *on != "2026-01-05T09:10:00.000Z" {
			t.Errorf("onset = %v", on)
		}
	})

	t.Run("no_onset_when_quiet", func(t *testing.T) {
		an := &detector.Analysis{BaselineCount: 2, RiskThreshold: 0.5,
			Windows: winList("09:00", "09:05", "09:10"),
			Scored:  []detector.ScoreResult{{}, {}, {Risk: 0.1}}}
		if an.EarliestOnset() != nil {
			t.Error("onset should be nil")
		}
	})

	t.Run("baseline_contamination_detection", func(t *testing.T) {
		an := &detector.Analysis{BaselineCount: 3, RiskThreshold: 0.35,
			Windows: winList("09:00", "09:05", "09:10", "09:15")}
		an.Windows[1].Vector = vec(0.9, 0, 1.0, 0, 0, 0)
		bad, why := an.BaselineContaminated()
		if !bad || !contains(why, "rewrite") {
			t.Errorf("baseline contamination missed: %v %q", bad, why)
		}
	})

	t.Run("rename_storm_flags_baseline", func(t *testing.T) {
		an := &detector.Analysis{BaselineCount: 1, RiskThreshold: 0.35,
			Windows: winList("09:00", "09:05")}
		an.Windows[0].Vector = vec(0.3, 25, 0, 0, 0, 0)
		bad, _ := an.BaselineContaminated()
		if !bad {
			t.Error("rename storm in baseline not flagged")
		}
	})

	t.Run("content_check_mass_novel_high_entropy", func(t *testing.T) {
		files := map[string]float64{}
		for i := 0; i < 10; i++ {
			files[mustPath("docs/f", i, ".txt")] = 4.2
		}
		// ref snapshot has only .txt; candidate has 40% .lockd at entropy 7.9
		ref := map[string]float64{}
		for k := range files {
			ref[k] = 4.2
		}
		cand := map[string]float64{}
		for k, v := range files {
			cand[k] = v
		}
		for i := 10; i < 14; i++ {
			cand[mustPath("docs/f", i, ".lockd")] = 7.9
		}
		// 4/14 = 28.6% < 30%: below threshold
		an := &detector.Analysis{LoadFiles: func(string) (map[string]float64, error) { return cand, nil }}
		hit, ev := an.ContentEvidence(cand, extSet(ref))
		if hit {
			t.Errorf("28.6%% novel content flagged: %+v", ev)
		}
		for i := 14; i < 16; i++ {
			cand[mustPath("docs/f", i, ".lockd")] = 7.9
		}
		// 6/16 = 37.5% >= 30%: flagged
		hit, _ = an.ContentEvidence(cand, extSet(ref))
		if !hit {
			t.Error("37.5% novel high-entropy content not flagged")
		}
	})

	t.Run("content_check_low_entropy_churn_not_affected", func(t *testing.T) {
		ref := map[string]float64{"a.txt": 4.2, "b.txt": 4.2}
		cand := map[string]float64{"a.zz1": 4.2, "b.zz2": 4.2, "c.txt": 4.2}
		an := &detector.Analysis{}
		hit, _ := an.ContentEvidence(cand, extSet(ref))
		if hit {
			t.Error("low-entropy churn wrongly classified affected")
		}
	})
}

func fullCenter() map[string]float64 {
	return map[string]float64{
		detector.FModifiedRatio: 0.1, detector.FRenameRate: 0.2, detector.FEntropyShift: 0.05,
		detector.FExtNovelty: 0, detector.FDeleteRate: 0.2, detector.FSizeChange: 0,
	}
}

func fullScale() map[string]float64 {
	return map[string]float64{
		detector.FModifiedRatio: 0.05, detector.FRenameRate: 0.5, detector.FEntropyShift: 0.05,
		detector.FExtNovelty: 0.05, detector.FDeleteRate: 0.5, detector.FSizeChange: 0.05,
	}
}

func col(vs []detector.FeatureVector, f string) []float64 {
	out := make([]float64, len(vs))
	for i, v := range vs {
		out[i] = v.Values[f]
	}
	return out
}

func winList(times ...string) []detector.WindowFeatures {
	var epoch = "2026-01-05T"
	out := []detector.WindowFeatures{}
	for _, tm := range times {
		out = append(out, detector.WindowFeatures{StartsAt: epoch + tm + ":00.000Z"})
	}
	// note: winList below patches to full timestamps
	for i := range out {
		t := out[i].StartsAt
		if len(t) == len("2026-01-05T09:00:00.000Z") {
			out[i].EndsAt = t
		}
	}
	return out
}

func mustPath(prefix string, i int, ext string) string {
	return prefix + string(rune('a'+i)) + ext
}

func extSet(files map[string]float64) map[string]bool {
	out := map[string]bool{}
	for p := range files {
		for i := len(p) - 1; i >= 0; i-- {
			if p[i] == '.' {
				out[p[i+1:]] = true
				break
			}
		}
	}
	return out
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

var _ = domain.AlertNormal
var _ = tt.TS
