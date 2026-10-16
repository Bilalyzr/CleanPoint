// Package recoverytest covers feature extraction, detection scoring,
// recovery eligibility and restore correctness (the PRD test matrix rows
// 4-7).
package recoverytest

import (
	"fmt"
	"math"
	"testing"

	"cleanpoint/internal/detector"
	"cleanpoint/internal/domain"
	tt "cleanpoint/tests/harness"
)

func ev(id, ts, op, path string, oldE, newE *float64, oldS, newS *int64) domain.FileEvent {
	e := domain.FileEvent{ID: id, EventTime: ts, Operation: op, Path: path,
		OldEntropy: oldE, NewEntropy: newE, OldSize: oldS, NewSize: newS}
	return e
}

func f64ptr(v float64) *float64 { return &v }
func i64ptr(v int64) *int64     { return &v }

func TestFeatureExtraction(t *testing.T) {
	t.Run("modified_ratio_uses_files_at_window_start", func(t *testing.T) {
		// 10 files exist after creates in w0; w1 modifies 5 of them
		var evs []domain.FileEvent
		for i := 0; i < 10; i++ {
			evs = append(evs, ev(fmt.Sprintf("c%d", i), tt.TS(10+i), "create",
				fmt.Sprintf("f%d.txt", i), nil, f64ptr(4.2), nil, i64ptr(1500)))
		}
		for i := 0; i < 5; i++ {
			evs = append(evs, ev(fmt.Sprintf("m%d", i), tt.TS(400+i), "modify",
				fmt.Sprintf("f%d.txt", i), f64ptr(4.2), f64ptr(4.3), i64ptr(1500), i64ptr(1600)))
		}
		ws := detector.ExtractFeatures(evs, 300)
		if len(ws) != 2 { // windows 09:00..09:05 and 09:05..09:10
			t.Fatalf("windows = %d, want 2", len(ws))
		}
		w1 := ws[1]
		if got := w1.Vector.Values[detector.FModifiedRatio]; math.Abs(got-0.5) > 1e-6 {
			t.Errorf("modified ratio = %v, want 0.5 (5 of 10 at start)", got)
		}
	})

	t.Run("rename_rate_per_minute", func(t *testing.T) {
		var evs []domain.FileEvent
		for i := 0; i < 10; i++ {
			evs = append(evs, ev(fmt.Sprintf("c%d", i), tt.TS(10+i), "create",
				fmt.Sprintf("f%d.txt", i), nil, f64ptr(4.2), nil, i64ptr(1500)))
		}
		for i := 0; i < 10; i++ {
			e := ev(fmt.Sprintf("r%d", i), tt.TS(400+i), "rename",
				fmt.Sprintf("f%d.txt", i), f64ptr(4.2), nil, i64ptr(1500), nil)
			d := fmt.Sprintf("f%d.ren", i)
			e.DestPath = &d
			evs = append(evs, e)
		}
		ws := detector.ExtractFeatures(evs, 300)
		if got := ws[1].Vector.Values[detector.FRenameRate]; math.Abs(got-2.0) > 1e-6 {
			t.Errorf("rename rate = %v, want 2.0/min", got)
		}
	})

	t.Run("delete_rate_per_minute", func(t *testing.T) {
		evs := []domain.FileEvent{
			ev("c1", tt.TS(10), "create", "a.txt", nil, f64ptr(4.2), nil, i64ptr(1500)),
			ev("c2", tt.TS(11), "create", "b.txt", nil, f64ptr(4.2), nil, i64ptr(1500)),
			ev("d1", tt.TS(400), "delete", "a.txt", f64ptr(4.2), nil, i64ptr(1500), nil),
			ev("d2", tt.TS(401), "delete", "b.txt", f64ptr(4.2), nil, i64ptr(1500), nil),
		}
		ws := detector.ExtractFeatures(evs, 300)
		if got := ws[1].Vector.Values[detector.FDeleteRate]; math.Abs(got-0.4) > 1e-6 {
			t.Errorf("delete rate = %v, want 0.4/min", got)
		}
	})

	t.Run("entropy_shift_ratio_eligible_only", func(t *testing.T) {
		// 4 modifies: 3 with a >=1.5 bit jump, 1 small; creates excluded
		var evs []domain.FileEvent
		for i := 0; i < 4; i++ {
			evs = append(evs, ev(fmt.Sprintf("m%d", i), tt.TS(400+i), "modify",
				fmt.Sprintf("f%d.txt", i), f64ptr(4.0), f64ptr(7.9), i64ptr(2048), i64ptr(2048)))
		}
		evs = append(evs, ev("m9", tt.TS(410), "modify", "f9.txt", f64ptr(4.0), f64ptr(4.1), i64ptr(2048), i64ptr(2048)))
		evs = append(evs, ev("c9", tt.TS(411), "create", "new.txt", nil, f64ptr(7.9), nil, i64ptr(2048)))
		ws := detector.ExtractFeatures(evs, 300)
		w := ws[0]
		if w.Vector.Missing[detector.FEntropyShift] {
			t.Fatal("entropy shift marked missing despite eligible events")
		}
		// 5 eligible modifies (4 with a big jump, 1 small); the create is not eligible
		if got := w.Vector.Values[detector.FEntropyShift]; math.Abs(got-0.8) > 1e-6 {
			t.Errorf("entropy shift ratio = %v, want 0.8 (4 of 5)", got)
		}
	})

	t.Run("entropy_missing_when_no_eligible", func(t *testing.T) {
		evs := []domain.FileEvent{
			ev("c1", tt.TS(10), "create", "a.txt", nil, nil, nil, i64ptr(10)),
		}
		ws := detector.ExtractFeatures(evs, 300)
		if !ws[0].Vector.Missing[detector.FEntropyShift] {
			t.Error("entropy shift should be missing without eligible events")
		}
	})

	t.Run("small_files_not_eligible_for_entropy", func(t *testing.T) {
		evs := []domain.FileEvent{
			ev("m1", tt.TS(10), "modify", "a.txt", f64ptr(4.0), f64ptr(7.9), i64ptr(100), i64ptr(100)),
		}
		ws := detector.ExtractFeatures(evs, 300)
		if !ws[0].Vector.Missing[detector.FEntropyShift] {
			t.Error("entropy shift should be missing for small files")
		}
	})

	t.Run("size_change_ratio", func(t *testing.T) {
		var evs []domain.FileEvent
		// 2 big shifts, 2 small
		for i := 0; i < 2; i++ {
			evs = append(evs, ev(fmt.Sprintf("big%d", i), tt.TS(400+i), "modify",
				fmt.Sprintf("b%d.txt", i), f64ptr(4.2), f64ptr(4.3), i64ptr(1000), i64ptr(3000)))
		}
		for i := 0; i < 2; i++ {
			evs = append(evs, ev(fmt.Sprintf("sml%d", i), tt.TS(410+i), "modify",
				fmt.Sprintf("s%d.txt", i), f64ptr(4.2), f64ptr(4.3), i64ptr(1000), i64ptr(1100)))
		}
		ws := detector.ExtractFeatures(evs, 300)
		if got := ws[0].Vector.Values[detector.FSizeChange]; math.Abs(got-0.5) > 1e-6 {
			t.Errorf("size change ratio = %v, want 0.5", got)
		}
	})

	t.Run("extension_novelty_vs_baseline", func(t *testing.T) {
		var evs []domain.FileEvent
		// w0 baseline writes .txt; w1 writes .txt and .lockd
		evs = append(evs, ev("b1", tt.TS(10), "create", "a.txt", nil, f64ptr(4.2), nil, i64ptr(1500)))
		evs = append(evs, ev("b2", tt.TS(20), "create", "b.txt", nil, f64ptr(4.2), nil, i64ptr(1500)))
		evs = append(evs, ev("n1", tt.TS(400), "create", "c.lockd", nil, f64ptr(7.9), nil, i64ptr(1500)))
		evs = append(evs, ev("n2", tt.TS(410), "create", "d.txt", nil, f64ptr(4.2), nil, i64ptr(1500)))
		ws := detector.ExtractFeatures(evs, 300)
		detector.ApplyExtensionNovelty(ws, 1)
		if got := ws[1].Vector.Values[detector.FExtNovelty]; math.Abs(got-0.5) > 1e-6 {
			t.Errorf("extension novelty = %v, want 0.5", got)
		}
	})

	t.Run("windows_are_aligned_and_non_overlapping", func(t *testing.T) {
		evs := []domain.FileEvent{
			ev("a", tt.TS(5), "create", "a.txt", nil, f64ptr(4.2), nil, i64ptr(1500)),
			ev("b", tt.TS(700), "modify", "a.txt", f64ptr(4.2), f64ptr(4.3), i64ptr(1500), i64ptr(1500)),
		}
		ws := detector.ExtractFeatures(evs, 300)
		if ws[0].StartsAt != "2026-01-05T09:00:00.000Z" {
			t.Errorf("window 0 start = %s", ws[0].StartsAt)
		}
		if ws[0].EndsAt != "2026-01-05T09:05:00.000Z" {
			t.Errorf("window 0 end = %s", ws[0].EndsAt)
		}
	})

	t.Run("empty_windows_counted_as_baseline", func(t *testing.T) {
		evs := []domain.FileEvent{
			ev("a", tt.TS(5), "create", "a.txt", nil, f64ptr(4.2), nil, i64ptr(1500)),
			ev("b", tt.TS(2000), "modify", "a.txt", f64ptr(4.2), f64ptr(4.3), i64ptr(1500), i64ptr(1500)),
		}
		ws := detector.ExtractFeatures(evs, 300)
		if len(ws) < 6 {
			t.Errorf("gap windows missing: %d", len(ws))
		}
		for i := 1; i < len(ws)-1; i++ {
			if ws[i].Vector.Events != 0 {
				t.Errorf("window %d should be empty", i)
			}
		}
	})

	t.Run("files_at_window_start_respects_deletes_and_renames", func(t *testing.T) {
		evs := []domain.FileEvent{
			ev("c1", tt.TS(10), "create", "a.txt", nil, f64ptr(4.2), nil, i64ptr(1500)),
			ev("c2", tt.TS(11), "create", "b.txt", nil, f64ptr(4.2), nil, i64ptr(1500)),
			ev("d1", tt.TS(12), "delete", "a.txt", f64ptr(4.2), nil, i64ptr(1500), nil),
			ev("m1", tt.TS(400), "modify", "b.txt", f64ptr(4.2), f64ptr(4.3), i64ptr(1500), i64ptr(1500)),
		}
		ws := detector.ExtractFeatures(evs, 300)
		// 1 file at w1 start -> modified ratio = 1/1
		if got := ws[1].Vector.Values[detector.FModifiedRatio]; math.Abs(got-1.0) > 1e-6 {
			t.Errorf("modified ratio = %v, want 1.0", got)
		}
	})

	t.Run("write_concentration_informational", func(t *testing.T) {
		evs := []domain.FileEvent{
			ev("c1", tt.TS(10), "create", "a.txt", nil, f64ptr(4.2), nil, i64ptr(1500)),
			ev("c2", tt.TS(11), "create", "b.txt", nil, f64ptr(4.2), nil, i64ptr(1500)),
			ev("c3", tt.TS(12), "create", "c.txt", nil, f64ptr(4.2), nil, i64ptr(1500)),
		}
		for i := range evs {
			a := "office-user"
			evs[i].Actor = &a
		}
		ws := detector.ExtractFeatures(evs, 300)
		if got := ws[0].Vector.Values[detector.FWriteConcen]; got <= 0.99 {
			t.Errorf("write concentration = %v, want 1.0 for single actor", got)
		}
		if detector.Weights[detector.FWriteConcen] != 0 {
			t.Error("write concentration must carry zero weight")
		}
	})

	t.Run("weights_sum_to_one", func(t *testing.T) {
		sum := 0.0
		for _, w := range detector.Weights {
			sum += w
		}
		if math.Abs(sum-1.0) > 1e-9 {
			t.Errorf("rules-v1 weights sum to %v, want 1.0", sum)
		}
	})
}
