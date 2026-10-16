// Package detector implements 300-second non-overlapping window feature
// extraction, the robust normal baseline (median/MAD) and the explainable
// rules-v1 risk scoring. It never reads scenario ground truth: the only
// inputs are ingested events and sealed snapshot metadata.
package detector

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cleanpoint/internal/domain"
)

// Feature names (rules-v1 six fixed weights + optional concentration).
const (
	FModifiedRatio  = "modified_file_ratio"
	FRenameRate     = "rename_rate"
	FEntropyShift   = "entropy_shift_ratio"
	FExtNovelty     = "extension_novelty"
	FDeleteRate     = "delete_rate"
	FSizeChange     = "size_change_ratio"
	FWriteConcen    = "write_concentration" // informational, weight 0
)

// Weights per the source specification (rules-v1).
var Weights = map[string]float64{
	FModifiedRatio: 0.25,
	FRenameRate:    0.20,
	FEntropyShift:  0.25,
	FExtNovelty:    0.15,
	FDeleteRate:    0.10,
	FSizeChange:    0.05,
	FWriteConcen:   0.0,
}

// Minimum effect sizes that make an individual file "large entropy increase"
// (bits/byte) or "unusually large size shift" (relative delta). These are
// detection definitions documented in docs/detection.md, calibrated so benign
// text editing stays below and synthetic encryption-like rewrites land above.
const (
	EntropyIncreaseBits = 1.5
	SizeShiftRatio      = 0.5
	MinEligibleBytes    = 512
)

// epsilon per feature keeps scale from collapsing to zero on quiet baselines.
var Epsilon = map[string]float64{
	FModifiedRatio: 0.01,
	FRenameRate:    0.5,
	FEntropyShift:  0.01,
	FExtNovelty:    0.01,
	FDeleteRate:    0.5,
	FSizeChange:    0.01,
	FWriteConcen:   0.01,
}

// FeatureVector carries raw values plus availability flags. A feature marked
// missing contributes no signal and lowers coverage — its weight is never
// redistributed silently.
type FeatureVector struct {
	Values   map[string]float64
	Missing  map[string]bool
	Events   int
	Writes   int
	Eligible int

	// exts is the set of file extensions written in this window (used for
	// novelty against the baseline; never serialized).
	exts map[string]bool
}

func ext(p string) string {
	i := strings.LastIndexByte(p, '.')
	if i < 0 || i == len(p)-1 {
		return ""
	}
	return strings.ToLower(p[i+1:])
}

// windowBounds returns aligned non-overlapping [start,end) windows covering
// [min,max], anchored to Unix epoch multiples of windowSeconds for
// determinism.
func windowBounds(minT, maxT time.Time, windowSeconds int) [] [2]time.Time {
	w := time.Duration(windowSeconds) * time.Second
	start := minT.Truncate(w)
	if start.After(minT) {
		start = start.Add(-w)
	}
	var out [][2]time.Time
	for t := start; !t.After(maxT); t = t.Add(w) {
		out = append(out, [2]time.Time{t, t.Add(w)})
	}
	return out
}

// fileState replays events to answer "how many files exist at time X".
type fileState struct {
	exists map[string]bool
}

func replayState(events []domain.FileEvent) (starts []string, states []map[string]bool) {
	st := map[string]bool{}
	states = make([]map[string]bool, 0, len(events))
	starts = make([]string, 0, len(events))
	for _, e := range events {
		switch e.Operation {
		case "create", "modify":
			st[e.Path] = true
		case "delete":
			delete(st, e.Path)
		case "rename":
			delete(st, e.Path)
			if e.DestPath != nil {
				st[*e.DestPath] = true
			}
		}
		// snapshot copy of state AFTER this event; window start counts are
		// derived by looking at the last event before the boundary.
		cp := make(map[string]bool, len(st))
		for k, v := range st {
			cp[k] = v
		}
		starts = append(starts, e.EventTime)
		states = append(states, cp)
	}
	return
}

func countAt(states []map[string]bool, starts []string, boundary string) int {
	idx := -1
	for i, s := range starts {
		if s < boundary {
			idx = i
		} else {
			break
		}
	}
	if idx < 0 {
		return 0
	}
	return len(states[idx])
}

// ExtractFeatures computes the feature vector for every window covering the
// ingested events. baselineExtensions receives the union of extensions seen
// across baseline windows after a first pass.
func ExtractFeatures(events []domain.FileEvent, windowSeconds int) []WindowFeatures {
	if len(events) == 0 {
		return nil
	}
	sorted := make([]domain.FileEvent, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].EventTime < sorted[j].EventTime })

	minT, _ := time.Parse(domain.TimeLayout, sorted[0].EventTime)
	maxT, _ := time.Parse(domain.TimeLayout, sorted[len(sorted)-1].EventTime)
	bounds := windowBounds(minT, maxT, windowSeconds)
	starts, states := replayState(sorted)

	out := make([]WindowFeatures, 0, len(bounds))
	for _, b := range bounds {
		startStr := domain.FormatTime(b[0])
		endStr := domain.FormatTime(b[1])
		var inWindow []domain.FileEvent
		for _, e := range sorted {
			if e.EventTime >= startStr && e.EventTime < endStr {
				inWindow = append(inWindow, e)
			}
		}
		fv := computeWindow(inWindow, countAt(states, starts, startStr))
		out = append(out, WindowFeatures{
			StartsAt: startStr,
			EndsAt:   endStr,
			Vector:   fv,
		})
	}
	return out
}

func computeWindow(evts []domain.FileEvent, filesAtStart int) FeatureVector {
	v := FeatureVector{Values: map[string]float64{}, Missing: map[string]bool{}}
	minutes := 5.0
	modified := map[string]bool{}
	renames, deletes := 0, 0
	eligible, largeEnt := 0, 0
	sizeEligible, sizeLarge := 0, 0
	extSeen := map[string]bool{}
	actorCounts := map[string]int{}
	writes := 0
	var writeExts []string

	for _, e := range evts {
		switch e.Operation {
		case "create", "modify":
			modified[e.Path] = true
			writes++
			writeExts = append(writeExts, ext(e.Path))
			if e.Actor != nil {
				actorCounts[*e.Actor]++
			}
		case "rename":
			renames++
			if e.DestPath != nil {
				modified[*e.DestPath] = true
				writes++
				writeExts = append(writeExts, ext(*e.DestPath))
			}
			if e.Actor != nil {
				actorCounts[*e.Actor]++
			}
		case "delete":
			deletes++
		}
		// entropy eligibility: modify/create of a file with known old+new
		// entropy and non-trivial new size.
		if (e.Operation == "modify" || e.Operation == "create") &&
			e.OldEntropy != nil && e.NewEntropy != nil && e.NewSize != nil && *e.NewSize >= MinEligibleBytes {
			eligible++
			if *e.NewEntropy-*e.OldEntropy >= EntropyIncreaseBits {
				largeEnt++
			}
		}
		if e.Operation == "modify" && e.OldSize != nil && e.NewSize != nil && *e.OldSize > 0 {
			sizeEligible++
			delta := float64(*e.NewSize-*e.OldSize)
			if delta < 0 {
				delta = -delta
			}
			if delta/float64(*e.OldSize) >= SizeShiftRatio {
				sizeLarge++
			}
		}
	}
	for _, x := range writeExts {
		extSeen[x] = true
	}

	denom := filesAtStart
	if denom < 1 {
		denom = 1
	}
	v.Values[FModifiedRatio] = round4(float64(len(modified)) / float64(denom))
	v.Values[FRenameRate] = round4(float64(renames) / minutes)
	v.Values[FDeleteRate] = round4(float64(deletes) / minutes)

	if eligible == 0 {
		v.Missing[FEntropyShift] = true
		v.Values[FEntropyShift] = 0
	} else {
		v.Values[FEntropyShift] = round4(float64(largeEnt) / float64(eligible))
	}
	if sizeEligible == 0 {
		v.Missing[FSizeChange] = true
		v.Values[FSizeChange] = 0
	} else {
		v.Values[FSizeChange] = round4(float64(sizeLarge) / float64(sizeEligible))
	}
	if writes == 0 {
		v.Values[FExtNovelty] = 0
		v.Missing[FExtNovelty] = false
	} else {
		v.Values[FExtNovelty] = 0 // refined against baseline in a second pass
	}
	// write concentration: fraction of writes by busiest known actor.
	totalKnown := 0
	busiest := 0
	for _, c := range actorCounts {
		totalKnown += c
		if c > busiest {
			busiest = c
		}
	}
	if totalKnown == 0 {
		v.Values[FWriteConcen] = 0
		v.Missing[FWriteConcen] = true
	} else {
		v.Values[FWriteConcen] = round4(float64(busiest) / float64(totalKnown))
	}
	v.Events = len(evts)
	v.Writes = writes
	v.Eligible = eligible
	v.exts = extSeen
	return v
}

// WindowFeatures is a raw window plus its feature vector.
type WindowFeatures struct {
	StartsAt string
	EndsAt   string
	Vector   FeatureVector
	// NovelExtensions is populated by ApplyExtensionNovelty.
	NovelWrites int
	TotalWritesExt int
}

// ApplyExtensionNovelty computes extension novelty against the union of
// extensions observed across baseline windows. Called once after baseline
// windows are identified.
func ApplyExtensionNovelty(windows []WindowFeatures, baselineCount int) {
	base := map[string]bool{}
	for i := 0; i < baselineCount && i < len(windows); i++ {
		for e := range windows[i].Vector.exts {
			base[e] = true
		}
	}
	for i := range windows {
		w := &windows[i]
		total := 0
		novel := 0
		for e := range w.Vector.exts {
			total++
			if !base[e] {
				novel++
			}
		}
		if total == 0 {
			w.Vector.Values[FExtNovelty] = 0
		} else {
			w.Vector.Values[FExtNovelty] = round4(float64(novel) / float64(total))
		}
		w.NovelWrites = novel
		w.TotalWritesExt = total
	}
}

func round4(f float64) float64 { return float64(int(f*10000+0.5)) / 10000 }

func fmtDur(d time.Duration) string { return fmt.Sprintf("%.0fs", d.Seconds()) }
