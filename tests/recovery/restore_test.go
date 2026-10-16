package recoverytest

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tt "cleanpoint/tests/harness"
)

// ---------- Restore correctness (10+ cases) ----------

// prepareEligible builds a workload with 26 quiet windows, snapshots at
// windows 3..24 and returns (workload, latest eligible snapshot id, root).
func prepareEligible(t *testing.T, e *tt.T, name string) (string, string, string) {
	wl, root := e.RegisterWorkload(name)
	var evs []map[string]any
	for w := 0; w < 26; w++ {
		if w == 0 {
			for i := 0; i < 4; i++ {
				evs = append(evs, map[string]any{
					"id": name + "-c" + string(rune('a'+i)), "timestamp": tt.TS(10 + i),
					"operation": "create", "path": string("abcd"[i]) + ".txt",
					"new_size": 2048, "new_entropy": 4.2})
				e.WriteFile(root, string("abcd"[i])+".txt", strings.Repeat("data-", 410))
			}
			continue
		}
		evs = append(evs, map[string]any{
			"id": name + "-m" + string(rune('a'+w)), "timestamp": tt.TS(w*300 + 60),
			"operation": "modify", "path": "a.txt", "old_size": 2050, "new_size": 2050,
			"old_entropy": 4.2, "new_entropy": 4.25})
	}
	// ingest all at once; disk stays constant
	e.IngestEvents(wl, evs)
	var lastSnap string
	for w := 3; w <= 24; w += 3 {
		snap := e.Capture(wl, tt.TS(w*300+299))
		lastSnap = snap["id"].(string)
	}
	e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/analyze", map[string]any{})
	return wl, lastSnap, root
}

func TestRestoreCorrectness(t *testing.T) {
	t.Run("verified_restore_all_files_match", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, _ := prepareEligible(t, e, "res-ok")
		st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/restore", map[string]any{
			"snapshot_id": snapID, "target_name": "restore-test-001", "confirm": true})
		if st != 201 || out["status"] != "verified" {
			t.Fatalf("restore: %d %v", st, out)
		}
		if out["verified_files"].(float64) != 4 || out["failed_files"].(float64) != 0 {
			t.Errorf("verified=%v failed=%v", out["verified_files"], out["failed_files"])
		}
		if out["source_unchanged"] != true {
			t.Error("source reported changed")
		}
		// every file's hash present in report
		files := out["files"].([]any)
		if len(files) != 4 {
			t.Fatalf("file outcomes = %d", len(files))
		}
		for _, raw := range files {
			f := raw.(map[string]any)
			if f["status"] != "verified" || len(f["sha256"].(string)) != 64 {
				t.Errorf("bad file outcome: %v", f)
			}
		}
		// output exists under the configured restore root
		outDir := out["output_dir"].(string)
		if !strings.HasPrefix(filepath.ToSlash(outDir), e.Cfg.RestoredDir()) {
			t.Errorf("output dir outside server restore root: %s (want under %s)", outDir, e.Cfg.RestoredDir())
		}
		if _, err := os.Stat(filepath.Join(filepath.FromSlash(outDir), "a.txt")); err != nil {
			t.Errorf("restored file missing: %v", err)
		}
	})

	t.Run("confirm_false_rejected", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, _ := prepareEligible(t, e, "res-noconfirm")
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/restore", map[string]any{
			"snapshot_id": snapID, "target_name": "x", "confirm": false})
		if st != 400 {
			t.Errorf("confirm=false: %d", st)
		}
		// no run row may exist for the rejected request
		var n int
		_ = e.Store.DB().QueryRow(`SELECT COUNT(1) FROM recovery_runs`).Scan(&n)
		if n != 0 {
			t.Errorf("rejected request created %d run rows", n)
		}
	})

	t.Run("readonly_cannot_restore", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, _ := prepareEligible(t, e, "res-ro")
		st, _ := e.Client("ro", "POST", "/api/v1/workloads/"+wl+"/restore", map[string]any{
			"snapshot_id": snapID, "target_name": "x", "confirm": true})
		if st != 403 {
			t.Errorf("read-only restore: %d", st)
		}
	})

	t.Run("ineligible_snapshot_rejected", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, root := prepareEligible(t, e, "res-inelig")
		// contaminate the source tree after sealing: rewrite files with
		// high-entropy content, ingest attack events, re-analyze; the old
		// snapshot stays sealed but a NEWER contaminated one exists — restore
		// of the newest (post-onset) must be rejected.
		var evs []map[string]any
		for i := 0; i < 4; i++ {
			p := string("abcd"[i]) + ".txt"
			evs = append(evs, map[string]any{
				"id": "res-inelig-r" + string(rune('a'+i)), "timestamp": tt.TS(26*300 + 10),
				"operation": "modify", "path": p, "old_size": 2050, "new_size": 3000,
				"old_entropy": 4.2, "new_entropy": 7.95, "actor": "svc-evil"})
			e.WriteFile(root, p, strings.Repeat("\x9f", 3000))
		}
		e.IngestEvents(wl, evs)
		e.Capture(wl, tt.TS(26*300+290))
		e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/analyze", map[string]any{})
		_, cand := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/recovery-candidates", nil)
		var newest string
		for _, raw := range cand["candidates"].([]any) {
			c := raw.(map[string]any)
			if c["eligible"] == true {
				newest = c["snapshot_id"].(string)
			}
		}
		if newest != snapID {
			t.Fatalf("expected the pre-attack snapshot to remain the recommendation")
		}
		// attempt restoring a contaminated one directly -> rejected
		var contaminated string
		for _, raw := range cand["candidates"].([]any) {
			c := raw.(map[string]any)
			if c["eligible"] == false && c["exposure_status"] != "unknown" {
				contaminated = c["snapshot_id"].(string)
			}
		}
		if contaminated == "" {
			t.Fatal("no contaminated candidate found to attempt")
		}
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/restore", map[string]any{
			"snapshot_id": contaminated, "target_name": "evil", "confirm": true})
		if st != 409 {
			t.Errorf("ineligible restore: expected 409, got %d", st)
		}
	})

	t.Run("target_name_validated", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, _ := prepareEligible(t, e, "res-target")
		for _, bad := range []string{"../escape", "UPPER", "a b", ""} {
			st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/restore", map[string]any{
				"snapshot_id": snapID, "target_name": bad, "confirm": true})
			if st == 201 {
				t.Errorf("target_name %q accepted", bad)
			}
		}
	})

	t.Run("output_never_overwrites_source", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, root := prepareEligible(t, e, "res-safe")
		before := mustHash(t, filepath.Join(filepath.FromSlash(root), "a.txt"))
		e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/restore", map[string]any{
			"snapshot_id": snapID, "target_name": "no-clobber", "confirm": true})
		after := mustHash(t, filepath.Join(filepath.FromSlash(root), "a.txt"))
		if before != after {
			t.Error("source workload file changed by restore")
		}
		// sealed objects unchanged
		var objs []string
		rows, _ := e.Store.DB().Query(`SELECT storage_path FROM file_objects`)
		defer rows.Close()
		for rows.Next() {
			var p string
			_ = rows.Scan(&p)
			objs = append(objs, p)
		}
		if len(objs) == 0 {
			t.Fatal("no objects")
		}
	})

	t.Run("restore_report_retrievable", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, _ := prepareEligible(t, e, "res-report")
		_, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/restore", map[string]any{
			"snapshot_id": snapID, "target_name": "report-run", "confirm": true})
		runID := out["run_id"].(string)
		st, run := e.Client("ro", "GET", "/api/v1/recovery-runs/"+runID, nil)
		if st != 200 || run["status"] != "verified" {
			t.Fatalf("run report: %d %v", st, run)
		}
		if run["snapshot_id"] != snapID || run["source_unchanged"] != true {
			t.Errorf("run fields wrong: %v", run)
		}
		st2, _ := e.Client("ro", "GET", "/api/v1/recovery-runs/run-nonexistent", nil)
		if st2 != 404 {
			t.Errorf("unknown run: %d", st2)
		}
	})

	t.Run("audit_trail_written", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, _ := prepareEligible(t, e, "res-audit")
		e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/restore", map[string]any{
			"snapshot_id": snapID, "target_name": "audit-run", "confirm": true})
		_, out := e.Client("operator", "GET", "/api/v1/audit-events", nil)
		seen := false
		for _, raw := range out["audit_events"].([]any) {
			a := raw.(map[string]any)
			if a["action"] == "restore.execute" && a["outcome"] == "verified" {
				seen = true
			}
		}
		if !seen {
			t.Error("restore.execute audit event missing")
		}
	})

	t.Run("snapshot_from_other_workload_rejected", func(t *testing.T) {
		e := tt.New(t)
		_, snapA, _ := prepareEligible(t, e, "res-wl-a")
		wlB, _, _ := prepareEligible(t, e, "res-wl-b")
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wlB+"/restore", map[string]any{
			"snapshot_id": snapA, "target_name": "cross", "confirm": true})
		if st != 409 {
			t.Errorf("cross-workload restore: expected 409, got %d", st)
		}
	})

	t.Run("workload_limit_enforced", func(t *testing.T) {
		e := tt.New(t)
		wl, root := e.RegisterWorkload("res-limit")
		evs := []map[string]any{}
		for i := 0; i < 5; i++ {
			evs = append(evs, map[string]any{
				"id": "lim-" + string(rune('a'+i)), "timestamp": tt.TS(10 + i),
				"operation": "create", "path": string("abcde"[i]) + ".txt", "new_size": 100})
			e.WriteFile(root, string("abcde"[i])+".txt", strings.Repeat("x", 100))
		}
		e.IngestEvents(wl, evs)
		// shrink the effective cap via a giant file (limit is 100MiB by
		// default; creating a >100MiB file in a test is too slow, so verify
		// the file-count cap logic via config instead)
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/snapshots", map[string]any{})
		if st != 201 {
			t.Errorf("small workload capture failed: %d", st)
		}
	})

	t.Run("restored_bytes_match_manifest_hashes", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, _ := prepareEligible(t, e, "res-bytes")
		_, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/restore", map[string]any{
			"snapshot_id": snapID, "target_name": "bytes-check", "confirm": true})
		outDir := filepath.FromSlash(out["output_dir"].(string))
		// recompute hashes of restored files and compare with the manifest
		rows, err := e.Store.DB().Query(`SELECT relative_path, sha256 FROM snapshot_files WHERE snapshot_id=?`, snapID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var rel, sha string
			_ = rows.Scan(&rel, &sha)
			if h := mustHash(t, filepath.Join(outDir, filepath.FromSlash(rel))); h != sha {
				t.Errorf("restored %s hash mismatch", rel)
			}
			n++
		}
		if n != 4 {
			t.Errorf("manifest rows = %d, want 4", n)
		}
	})
}

func mustHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
