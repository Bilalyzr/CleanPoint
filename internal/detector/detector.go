package detector

import (
	"fmt"
	"strings"
	"time"

	"cleanpoint/internal/domain"
)

// ScoreResult is the rules-v1 outcome for one evaluated window.
type ScoreResult struct {
	Risk          float64
	Contributions []domain.SignalContribution
	Coverage      float64 // fraction of the six features with evidence
	SignalsHit    []string
}

// Score implements the source formula:
//
//	z_i     = (current_i - center_i) / scale_i
//	signal_i = min(1, max(0, z_i)/5)
//	risk    = sum(weight_i * signal_i)  over features with evidence only
//
// The risk index is a RELATIVE score in [0,1], never a probability.
func Score(v FeatureVector, b Baseline) ScoreResult {
	res := ScoreResult{}
	present := 0
	for _, f := range []string{FModifiedRatio, FRenameRate, FEntropyShift, FExtNovelty, FDeleteRate, FSizeChange} {
		w := Weights[f]
		if v.Missing[f] {
			res.Contributions = append(res.Contributions, domain.SignalContribution{
				Feature: f, Weight: w, Signal: 0, Value: v.Values[f], Center: b.Center[f], Missing: true,
			})
			continue
		}
		present++
		cur := v.Values[f]
		scale := b.Scale[f]
		if scale <= 0 {
			scale = Epsilon[f]
		}
		z := (cur - b.Center[f]) / scale
		sig := z / 5
		if sig < 0 {
			sig = 0
		}
		if sig > 1 {
			sig = 1
		}
		res.Risk += w * sig
		res.Contributions = append(res.Contributions, domain.SignalContribution{
			Feature: f, Weight: w, Signal: round4(sig), Value: cur, Center: b.Center[f], ZScore: round4(z),
		})
		if sig >= 0.2 {
			res.SignalsHit = append(res.SignalsHit, f)
		}
	}
	res.Coverage = float64(present) / 6
	res.Risk = round4(res.Risk)
	return res
}

// Explain produces human-readable evidence lines for a suspicious window.
func Explain(v FeatureVector, s ScoreResult) []string {
	var out []string
	for _, c := range s.Contributions {
		if c.Missing || c.Signal < 0.2 {
			continue
		}
		switch c.Feature {
		case FModifiedRatio:
			out = append(out, fmt.Sprintf("Modification ratio %.2f exceeds the normal median %.2f.", c.Value, c.Center))
		case FRenameRate:
			out = append(out, fmt.Sprintf("Rename rate %.1f/min is far above normal %.1f/min.", c.Value, c.Center))
		case FEntropyShift:
			out = append(out, fmt.Sprintf("%.0f%% of changed files show a large entropy increase (≥%.1f bits/byte).", c.Value*100, EntropyIncreaseBits))
		case FExtNovelty:
			out = append(out, fmt.Sprintf("%.0f%% of written extensions were never seen in the normal baseline.", c.Value*100))
		case FDeleteRate:
			out = append(out, fmt.Sprintf("Delete rate %.1f/min is far above normal %.1f/min.", c.Value, c.Center))
		case FSizeChange:
			out = append(out, fmt.Sprintf("%.0f%% of modified files changed size by ≥%d%%.", c.Value*100, int(SizeShiftRatio*100)))
		}
	}
	if s.Coverage < 1 {
		out = append(out, fmt.Sprintf("Coverage %.0f%%: some signals lacked evidence and contributed nothing.", s.Coverage*100))
	}
	return out
}

// AssessedSnapshot is the per-snapshot classification result.
type AssessedSnapshot struct {
	Snapshot        domain.Snapshot
	IntegrityOK     bool
	CoverageOK      bool
	ExposureStatus  string
	RiskScore       float64
	AffectedFiles   []string
	Explanation     []string
}

// Analysis carries everything a run computed: windows, scores, baseline size,
// threshold and events per window.
type Analysis struct {
	Windows        []WindowFeatures
	Scored         []ScoreResult
	BaselineCount  int
	RiskThreshold  float64
	EventsByWindow map[string][]domain.FileEvent
	// LoadFiles returns the sealed manifest rows (path -> entropy) of a
	// snapshot for the content-level compromise check.
	LoadFiles func(snapshotID string) (map[string]float64, error)
}

// Content-compromise thresholds: a snapshot whose manifest shows a mass of
// files with extensions absent from the earliest snapshot AND near-random
// entropy is classified affected even when telemetry looks quiet (e.g.
// contamination that predates the evaluated windows). Benign archives stay
// far below the fraction threshold. See docs/detection.md.
const (
	ContentNovelFraction = 0.30
	ContentEntropyFloor  = 7.0
)

type contentEvidence struct {
	NovelFraction  float64
	NovelMedianEnt float64
	NovelCount     int
}

// ContentEvidence exposes the content-level compromise check for evaluation
// tooling and tests: it compares a manifest (path -> entropy) against the
// reference extension set.
func (an *Analysis) ContentEvidence(files map[string]float64, refExts map[string]bool) (bool, contentEvidence) {
	return an.contentCheck(files, refExts)
}

// contentCheck compares a snapshot's manifest against the reference (earliest)
// snapshot extension set.
func (an *Analysis) contentCheck(files map[string]float64, refExts map[string]bool) (bool, contentEvidence) {
	if len(refExts) == 0 || len(files) == 0 {
		return false, contentEvidence{}
	}
	var novelEnts []float64
	novel, total := 0, 0
	for p, ent := range files {
		total++
		e := strings.ToLower(ext(p))
		if !refExts[e] {
			novel++
			novelEnts = append(novelEnts, ent)
		}
	}
	ev := contentEvidence{NovelFraction: float64(novel) / float64(total), NovelCount: novel}
	if novel > 0 {
		ev.NovelMedianEnt = Median(novelEnts)
	}
	if ev.NovelFraction >= ContentNovelFraction && ev.NovelMedianEnt >= ContentEntropyFloor {
		return true, ev
	}
	return false, ev
}

// refExtensions returns the extension set of the earliest sealed snapshot.
func (an *Analysis) refExtensions(snapshots []domain.Snapshot) map[string]bool {
	exts := map[string]bool{}
	if an.LoadFiles == nil {
		return exts
	}
	// snapshots arrive newest-first; walk to the oldest sealed
	var oldest *domain.Snapshot
	for i := range snapshots {
		if snapshots[i].Status != domain.SnapshotSealed {
			continue
		}
		if oldest == nil || snapshots[i].CapturedAt < oldest.CapturedAt {
			oldest = &snapshots[i]
		}
	}
	if oldest == nil {
		return exts
	}
	files, err := an.LoadFiles(oldest.ID)
	if err != nil {
		return exts
	}
	for p := range files {
		exts[strings.ToLower(ext(p))] = true
	}
	return exts
}

// AssessSnapshots performs exposure assessment given a prepared Analysis.
func (an *Analysis) AssessSnapshots(snapshots []domain.Snapshot) []AssessedSnapshot {
	refExts := an.refExtensions(snapshots)
	var out []AssessedSnapshot
	// A contaminated training period voids certification outright: even
	// pre-onset snapshots cannot be defended because "normal" was never
	// actually normal.
	if bad, why := an.BaselineContaminated(); bad {
		for _, snap := range snapshots {
			if snap.Status != domain.SnapshotSealed {
				continue
			}
			out = append(out, AssessedSnapshot{
				Snapshot: snap, ExposureStatus: domain.ExposureUnknown,
				Explanation: []string{
					"Normal baseline itself contains a destructive burst: " + why + ". " +
						"Certification impossible; exposure unknown for every snapshot."},
			})
		}
		return out
	}
	for _, snap := range snapshots {
		if snap.Status != domain.SnapshotSealed {
			continue
		}
		a := AssessedSnapshot{Snapshot: snap, ExposureStatus: domain.ExposureUnknown}
		coverageOK := len(an.Windows) > an.BaselineCount
		for i := 0; i < an.BaselineCount && i < len(an.Windows); i++ {
			if an.Windows[i].EndsAt > snap.CapturedAt {
				coverageOK = false
				break
			}
		}
		if !coverageOK {
			a.CoverageOK = false
			a.Explanation = append(a.Explanation, "Insufficient completed normal-baseline windows before capture; exposure is unknown.")
			out = append(out, a)
			continue
		}
		a.CoverageOK = true

		suspicious, affected := false, false
		maxRisk := 0.0
		for i, w := range an.Windows {
			if w.EndsAt > snap.CapturedAt {
				break
			}
			if i < an.BaselineCount {
				continue
			}
			s := an.Scored[i]
			if s.Risk > maxRisk {
				maxRisk = s.Risk
			}
			if s.Risk >= an.RiskThreshold {
				suspicious = true
				for _, c := range s.Contributions {
					if c.Signal >= 0.5 && (c.Feature == FEntropyShift || c.Feature == FDeleteRate || c.Feature == FRenameRate) {
						affected = true
					}
				}
				for _, e := range an.EventsByWindow[w.StartsAt] {
					a.AffectedFiles = appendUnique(a.AffectedFiles, e.Path)
				}
			}
		}
		a.RiskScore = round4(maxRisk)

		// Content-level evidence from the sealed manifest, independent of
		// window telemetry.
		contentAffected := false
		if an.LoadFiles != nil {
			if files, err := an.LoadFiles(snap.ID); err == nil && len(files) > 0 {
				hit, ev := an.contentCheck(files, refExts)
				if hit {
					contentAffected = true
					a.AffectedFiles = nil
					for p := range files {
						if !refExts[strings.ToLower(ext(p))] {
							a.AffectedFiles = appendUnique(a.AffectedFiles, p)
						}
					}
					a.Explanation = append(a.Explanation, fmt.Sprintf(
						"Manifest content: %d/%d files (%.0f%%) use extensions absent from the earliest snapshot with median entropy %.2f bits/byte — mass encryption-like rewrite.",
						ev.NovelCount, len(files), ev.NovelFraction*100, ev.NovelMedianEnt))
				}
			}
		}

		switch {
		case contentAffected:
			a.ExposureStatus = domain.ExposureAffected
		case suspicious && affected:
			a.ExposureStatus = domain.ExposureAffected
			a.Explanation = append(a.Explanation, "Suspicious windows before capture include destructive signals (entropy shift, mass renames or deletes).")
		case suspicious:
			a.ExposureStatus = domain.ExposureSuspect
			a.Explanation = append(a.Explanation, "Suspicious windows completed before capture although no destructive signal fired.")
		default:
			a.ExposureStatus = domain.ExposureClear
			a.Explanation = append(a.Explanation, "No suspicious window completed before capture; baseline coverage sufficient.")
		}
		out = append(out, a)
	}
	return out
}

// EarliestOnset returns the start of the earliest evaluated suspicious window
// (the conservative earliest plausible onset), or nil when nothing fired.
func (an *Analysis) EarliestOnset() *string {
	for i, w := range an.Windows {
		if i < an.BaselineCount {
			continue
		}
		if an.Scored[i].Risk >= an.RiskThreshold {
			s := w.StartsAt
			return &s
		}
	}
	return nil
}

// BaselineContaminated reports whether the assumed-normal training period
// itself contains a destructive burst (mass entropy shift + mass rewrite, a
// rename storm or a delete storm). When it does, no snapshot can be
// certified: every assessment becomes unknown and the selector abstains.
func (an *Analysis) BaselineContaminated() (bool, string) {
	for i := 0; i < an.BaselineCount && i < len(an.Windows); i++ {
		v := an.Windows[i].Vector
		if !v.Missing[FEntropyShift] &&
			v.Values[FEntropyShift] >= 0.5 && v.Values[FModifiedRatio] >= 0.30 {
			return true, "baseline window " + an.Windows[i].StartsAt +
				" shows a mass high-entropy rewrite (>=50% entropy shift on >=30% of files)"
		}
		if v.Values[FRenameRate] >= 20 {
			return true, "baseline window " + an.Windows[i].StartsAt +
				" shows a rename storm (>=20 renames/min)"
		}
		if v.Values[FDeleteRate] >= 10 {
			return true, "baseline window " + an.Windows[i].StartsAt +
				" shows a delete storm (>=10 deletes/min)"
		}
	}
	return false, ""
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// WindowOfEvent reports which window an event belongs to (debug/testing aid).
func WindowOfEvent(e domain.FileEvent, windowSeconds int) string {
	t, _ := time.Parse(domain.TimeLayout, e.EventTime)
	w := time.Duration(windowSeconds) * time.Second
	return domain.FormatTime(t.Truncate(w))
}
