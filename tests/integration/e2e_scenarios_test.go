package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tt "cleanpoint/tests/harness"
)

// e2e timelines reproduce each mandatory scenario family's shape compactly:
// 24+ baseline windows then family-specific activity, snapshots every 3
// windows (15 simulated minutes), one analysis, selector outcome compared to
// the family's expectation. Ground truth lives only in this file's
// expectations, never in the detector.

const textBody = "quarterly report budget invoice client meeting draft memo policy vendor audit "
const encPrefix = "cleanpoint-synthetic-opaque-content-not-malware-"

func textFile(n int) string  { return strings.Repeat(textBody, 20)[:1200+n] }
func encFile(n int) string {
	// deterministic near-uniform bytes (synthetic, harmless)
	out := make([]byte, 0, 1800)
	seed := uint32(n*2654435761 + 7)
	for i := 0; i < 1800; i++ {
		seed = seed*1664525 + 1013904223
		out = append(out, byte(seed>>24))
	}
	return encPrefix + string(out)
}

type phase struct {
	window int
	events []map[string]any
	disk   func(root string)
}

func write(root, rel, body string) {
	p := filepath.Join(filepath.FromSlash(root), filepath.FromSlash(rel))
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(body), 0o644)
}
func remove(root, rel string) { _ = os.Remove(filepath.Join(filepath.FromSlash(root), filepath.FromSlash(rel))) }

const evTextEnt = 4.2
const evEncEnt = 7.95

func createEv(id string, w int, path string, size int, ent float64) map[string]any {
	return map[string]any{"id": id, "timestamp": ts(w, 10), "operation": "create",
		"path": path, "new_size": size, "new_entropy": ent, "actor": "office-user"}
}
func modifyEv(id string, w int, path string, oldSize, newSize int, oldEnt, newEnt float64) map[string]any {
	return map[string]any{"id": id, "timestamp": ts(w, 30), "operation": "modify",
		"path": path, "old_size": oldSize, "new_size": newSize,
		"old_entropy": oldEnt, "new_entropy": newEnt, "actor": "office-user"}
}
func renameEv(id string, w int, from, to string, size int, ent float64) map[string]any {
	return map[string]any{"id": id, "timestamp": ts(w, 50), "operation": "rename",
		"path": from, "dest_path": to, "old_size": size, "old_entropy": ent, "actor": "svc-backup"}
}
func deleteEv(id string, w int, path string, size int, ent float64) map[string]any {
	return map[string]any{"id": id, "timestamp": ts(w, 80), "operation": "delete",
		"path": path, "old_size": size, "old_entropy": ent, "actor": "svc-backup"}
}

// ts renders a timestamp inside window w (300s windows from the same epoch
// base as the harness TS helper).
func ts(w, offset int) string { return tt.TS(w*300 + offset) }

// basePhases builds the initial population + 24 normal baseline windows.
func basePhases(prefix string, files int, totalWindows int) []phase {
	var phases []phase
	// window 0: initial creates
	p0 := phase{window: 0}
	for i := 0; i < files; i++ {
		p := fmt.Sprintf("%s/f%02d.txt", folderFor(i), i)
		p0.events = append(p0.events, createEv(fmt.Sprintf("%s-c%02d", prefix, i), 0, p, 1200+i, evTextEnt))
	}
	root0 := fmt.Sprintf("%s", "")
	_ = root0
	p0.disk = func(root string) {
		for i := 0; i < files; i++ {
			write(root, fmt.Sprintf("%s/f%02d.txt", folderFor(i), i), textFile(i))
		}
	}
	phases = append(phases, p0)
	for w := 1; w < totalWindows; w++ {
		ph := phase{window: w}
		i := w % files
		p := fmt.Sprintf("%s/f%02d.txt", folderFor(i), i)
		ph.events = append(ph.events, modifyEv(fmt.Sprintf("%s-m%02d", prefix, w), w, p, 1200+i, 1250+i, evTextEnt, evTextEnt))
		phases = append(phases, ph)
	}
	return phases
}

func folderFor(i int) string {
	return []string{"docs", "src", "media"}[i%3]
}

// runFamily drives the timeline: per window ingest events, apply disk state,
// capture every 3 windows, then analyze.
func runFamily(t *testing.T, e *tt.T, name string, phases []phase) (string, map[string]any, map[string]any) {
	wl, root := e.RegisterWorkload(name)
	byWindow := map[int]*phase{}
	for i := range phases {
		byWindow[phases[i].window] = &phases[i]
	}
	maxWin := 0
	for w := range byWindow {
		if w > maxWin {
			maxWin = w
		}
	}
	var snapshotTimes []string
	for w := 0; w <= maxWin; w++ {
		if ph := byWindow[w]; ph != nil {
			if len(ph.events) > 0 {
				e.IngestEvents(wl, ph.events)
			}
			if ph.disk != nil {
				ph.disk(root)
			}
		}
		if w%3 == 0 && w >= 3 {
			snap := e.Capture(wl, ts(w, 299))
			snapshotTimes = append(snapshotTimes, snap["captured_at"].(string))
		}
	}
	st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/analyze", map[string]any{})
	if st != 200 {
		t.Fatalf("analyze: %d %v", st, out)
	}
	_, cand := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/recovery-candidates", nil)
	return wl, out, cand
}

func candidateOf(t *testing.T, cand map[string]any) map[string]any {
	t.Helper()
	if cand["status"] != "candidate_available" {
		return nil
	}
	id := cand["recommended_snapshot_id"].(string)
	for _, raw := range cand["candidates"].([]any) {
		c := raw.(map[string]any)
		if c["snapshot_id"] == id {
			return c
		}
	}
	return nil
}

func TestEndToEndScenarios(t *testing.T) {
	const W = 30 // windows 0..29, baseline 24, activity 25+

	t.Run("normal_office_all_clear", func(t *testing.T) {
		e := tt.New(t)
		_, findings, cand := runFamily(t, e, "e2e-normal", basePhases("nm", 12, W))
		if findings["incident_onset"] != nil {
			t.Errorf("false onset in normal scenario: %v", findings["incident_onset"])
		}
		if cand["status"] != "candidate_available" {
			t.Fatalf("normal scenario should recommend newest: %v", cand["status"])
		}
		c := candidateOf(t, cand)
		if c["exposure_status"] != "clear" {
			t.Errorf("newest snapshot not clear: %v", c["exposure_status"])
		}
	})

	t.Run("benign_archive_not_flagged_as_attack", func(t *testing.T) {
		e := tt.New(t)
		phases := basePhases("ar", 12, W)
		ph := phase{window: 27}
		for i := 0; i < 2; i++ {
			p := fmt.Sprintf("docs/backup-%02d.7z", i)
			ph.events = append(ph.events, createEv(fmt.Sprintf("ar-z%d", i), 27, p, 40000, 7.9))
		}
		ph.disk = func(root string) {
			for i := 0; i < 2; i++ {
				write(root, fmt.Sprintf("docs/backup-%02d.7z", i), encFile(50+i))
			}
		}
		phases[27] = ph
		_, findings, cand := runFamily(t, e, "e2e-archive", phases)
		_ = findings
		if cand["status"] != "candidate_available" {
			t.Fatalf("benign archive must not cause abstention: %v", cand["status"])
		}
		if rec := candidateOf(t, cand); rec == nil || rec["eligible"] != true {
			t.Errorf("no eligible candidate after benign archives: %v", cand["status"])
		}
	})

	t.Run("benign_bulk_update_safe", func(t *testing.T) {
		e := tt.New(t)
		phases := basePhases("bu", 12, W)
		ph := phase{window: 27}
		for i := 0; i < 8; i++ {
			p := fmt.Sprintf("%s/f%02d.txt", folderFor(i), i)
			ph.events = append(ph.events, modifyEv(fmt.Sprintf("bu-b%d", i), 27, p, 1200+i, 1100+i, evTextEnt, evTextEnt))
		}
		ph.disk = func(root string) {
			for i := 0; i < 8; i++ {
				write(root, fmt.Sprintf("%s/f%02d.txt", folderFor(i), i), textFile(i)+" updated")
			}
		}
		phases[27] = ph
		_, _, cand := runFamily(t, e, "e2e-bulk", phases)
		if cand["status"] != "candidate_available" {
			t.Fatalf("benign bulk update caused abstention: %v", cand["status"])
		}
	})

	t.Run("rapid_destructive_older_clean_candidate", func(t *testing.T) {
		e := tt.New(t)
		phases := basePhases("rd", 12, W)
		ph := phase{window: 27}
		// mass rewrite: rename + encrypt-like modify + one delete
		for i := 0; i < 11; i++ {
			from := fmt.Sprintf("%s/f%02d.txt", folderFor(i), i)
			to := fmt.Sprintf("%s/f%02d.id-RAP.lockd", folderFor(i), i)
			ph.events = append(ph.events, renameEv(fmt.Sprintf("rd-r%02d", i), 27, from, to, 1200+i, evTextEnt))
			ph.events = append(ph.events, modifyEv(fmt.Sprintf("rd-w%02d", i), 27, to, 1200+i, 1900+i, evTextEnt, evEncEnt))
		}
		ph.events = append(ph.events, deleteEv("rd-d00", 27, fmt.Sprintf("%s/f11.txt", folderFor(11)), 1211, evTextEnt))
		ph.disk = func(root string) {
			for i := 0; i < 11; i++ {
				remove(root, fmt.Sprintf("%s/f%02d.txt", folderFor(i), i))
				write(root, fmt.Sprintf("%s/f%02d.id-RAP.lockd", folderFor(i), i), encFile(i))
			}
			remove(root, fmt.Sprintf("%s/f11.txt", folderFor(11)))
		}
		phases[27] = ph
		_, findings, cand := runFamily(t, e, "e2e-rapid", phases)
		if findings["incident_onset"] == nil {
			t.Fatal("attack not detected")
		}
		if cand["status"] != "candidate_available" {
			t.Fatalf("expected an older clean candidate: %v", cand["status"])
		}
		rec := candidateOf(t, cand)
		if rec["captured_at"].(string) >= "2026-01-05T11:15" { // window 27 start
			t.Errorf("recommended snapshot captured after onset: %v", rec["captured_at"])
		}
		// every post-onset snapshot must be ineligible
		for _, raw := range cand["candidates"].([]any) {
			c := raw.(map[string]any)
			if c["captured_at"].(string) >= "2026-01-05T11:15" && c["eligible"] == true {
				t.Errorf("post-onset snapshot eligible: %v", c["snapshot_id"])
			}
		}
	})

	t.Run("slow_distributed_cumulative_detection", func(t *testing.T) {
		e := tt.New(t)
		phases := basePhases("sl", 12, W)
		for w := 25; w < W; w++ {
			i := w - 25
			from := fmt.Sprintf("%s/f%02d.txt", folderFor(i), i)
			to := fmt.Sprintf("%s/f%02d.crypt", folderFor(i), i)
			ph := phase{window: w, events: []map[string]any{
				renameEv(fmt.Sprintf("sl-r%d", w), w, from, to, 1200+i, evTextEnt),
				modifyEv(fmt.Sprintf("sl-w%d", w), w, to, 1200+i, 1600+i, evTextEnt, evEncEnt),
			}, disk: func(root string) {}}
			phases[w] = ph
		}
		// apply cumulative disk state progressively
		for w := 25; w < W; w++ {
			i := w - 25
			from := fmt.Sprintf("%s/f%02d.txt", folderFor(i), i)
			to := fmt.Sprintf("%s/f%02d.crypt", folderFor(i), i)
			phases[w].disk = func(root string) {
				remove(root, from)
				write(root, to, encFile(i))
			}
		}
		_, findings, cand := runFamily(t, e, "e2e-slow", phases)
		if findings["incident_onset"] == nil {
			t.Fatal("slow attack not detected")
		}
		if cand["status"] != "candidate_available" {
			t.Fatalf("slow attack should still leave an older candidate: %v", cand["status"])
		}
	})

	t.Run("extension_churn_detected_not_signature_dependent", func(t *testing.T) {
		e := tt.New(t)
		phases := basePhases("ch", 12, W)
		for w := 25; w < 29; w++ {
			var evs []map[string]any
			for i := 0; i < 4; i++ {
				idx := (w-25)*4 + i
				if idx >= 12 {
					break
				}
				from := fmt.Sprintf("%s/f%02d.txt", folderFor(idx), idx)
				to := fmt.Sprintf("%s/f%02d.zz%02d", folderFor(idx), idx, w)
				evs = append(evs, renameEv(fmt.Sprintf("ch-r-%d-%d", w, i), w, from, to, 1200+idx, evTextEnt))
			}
			phases[w] = phase{window: w, events: evs}
		}
		// cumulative disk renames (content preserved)
		apply := 0
		for w := 25; w < 29; w++ {
			snapshot := apply
			phases[w].disk = func(root string) {
				for idx := 0; idx < snapshot; idx++ {
					from := fmt.Sprintf("%s/f%02d.txt", folderFor(idx), idx)
					if idx < 12 {
						// find the last rename target for this idx
						w2 := 25 + idx/4
						to := fmt.Sprintf("%s/f%02d.zz%02d", folderFor(idx), idx, w2)
						data, err := os.ReadFile(filepath.Join(filepath.FromSlash(root), filepath.FromSlash(from)))
						if err == nil {
							remove(root, from)
							write(root, to, string(data))
						}
					}
				}
			}
			apply += 4
		}
		_, findings, cand := runFamily(t, e, "e2e-churn", phases)
		if findings["incident_onset"] == nil {
			t.Fatal("extension churn not detected")
		}
		if cand["status"] != "candidate_available" {
			t.Fatalf("churn scenario expected older candidate: %v", cand["status"])
		}
	})

	t.Run("mixed_activity_onset_after_benign_burst", func(t *testing.T) {
		e := tt.New(t)
		phases := basePhases("mx", 12, W)
		// benign build burst at 26
		burst := phase{window: 26}
		for i := 0; i < 3; i++ {
			p := fmt.Sprintf("%s/f%02d.txt", folderFor(i), i)
			burst.events = append(burst.events, modifyEv(fmt.Sprintf("mx-b%d", i), 26, p, 1200+i, 1200+i, evTextEnt, evTextEnt))
		}
		phases[26] = burst
		// destructive rewrite at 28
		ph := phase{window: 28}
		for i := 0; i < 6; i++ {
			from := fmt.Sprintf("%s/f%02d.txt", folderFor(i), i)
			to := fmt.Sprintf("%s/f%02d.id-MIX.lockd", folderFor(i), i)
			ph.events = append(ph.events, renameEv(fmt.Sprintf("mx-r%d", i), 28, from, to, 1200+i, evTextEnt))
			ph.events = append(ph.events, modifyEv(fmt.Sprintf("mx-w%d", i), 28, to, 1200+i, 1800+i, evTextEnt, evEncEnt))
		}
		ph.disk = func(root string) {
			for i := 0; i < 6; i++ {
				remove(root, fmt.Sprintf("%s/f%02d.txt", folderFor(i), i))
				write(root, fmt.Sprintf("%s/f%02d.id-MIX.lockd", folderFor(i), i), encFile(i))
			}
		}
		phases[28] = ph
		_, findings, cand := runFamily(t, e, "e2e-mixed", phases)
		if findings["incident_onset"] == nil {
			t.Fatal("mixed attack not detected")
		}
		if cand["status"] != "candidate_available" {
			t.Fatalf("mixed scenario expected candidate: %v", cand["status"])
		}
		rec := candidateOf(t, cand)
		// snapshot at window 27 (before onset at 28) must be recommended
		if rec["captured_at"].(string) >= "2026-01-05T11:45" {
			t.Errorf("expected pre-onset snapshot, got %v", rec["captured_at"])
		}
	})

	t.Run("pre_existing_contamination_abstains", func(t *testing.T) {
		e := tt.New(t)
		phases := basePhases("px", 12, W)
		// window 0 gains a mass rewrite right after the initial creates: the
		// destructive burst sits inside the would-be baseline.
		extra := []map[string]any{}
		for i := 0; i < 11; i++ {
			from := fmt.Sprintf("%s/f%02d.txt", folderFor(i), i)
			to := fmt.Sprintf("%s/f%02d.id-PRE.locked", folderFor(i), i)
			extra = append(extra, renameEv(fmt.Sprintf("px-r%02d", i), 0, from, to, 1200+i, evTextEnt))
			extra = append(extra, modifyEv(fmt.Sprintf("px-w%02d", i), 0, to, 1200+i, 1900+i, evTextEnt, evEncEnt))
		}
		phases[0].events = append(phases[0].events, extra...)
		baseDisk := phases[0].disk
		phases[0].disk = func(root string) {
			baseDisk(root)
			for i := 0; i < 11; i++ {
				remove(root, fmt.Sprintf("%s/f%02d.txt", folderFor(i), i))
				write(root, fmt.Sprintf("%s/f%02d.id-PRE.locked", folderFor(i), i), encFile(i))
			}
		}
		_, findings, cand := runFamily(t, e, "e2e-preexisting", phases)
		if cand["status"] != "no_safe_candidate" {
			t.Fatalf("pre-existing contamination must abstain, got %v", cand["status"])
		}
		if cand["recommended_snapshot_id"] != nil {
			t.Errorf("abstention must have null recommendation, got %v", cand["recommended_snapshot_id"])
		}
		if len(cand["reasons"].([]any)) == 0 {
			t.Error("abstention without reasons")
		}
		_ = findings
	})
}
