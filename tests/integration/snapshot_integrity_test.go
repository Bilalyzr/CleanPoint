// Package integration covers snapshot integrity, event ingestion, API auth
// and end-to-end scenario outcomes.
package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cleanpoint/internal/storage"
	tt "cleanpoint/tests/harness"
)

// ---------- Snapshot integrity (10+ cases) ----------

func captureBaselineSnapshot(t *testing.T, e *tt.T, name string, files int) (wl, snapID, root string) {
	wl, root = e.RegisterWorkload(name)
	evs := []map[string]any{}
	for i := 0; i < files; i++ {
		evs = append(evs, map[string]any{
			"id": name + "-e" + string(rune('a'+i%26)) + "-" + strings.Repeat("x", i/26),
			"timestamp": tt.TS(30 + i), "operation": "create",
			"path": "f" + string(rune('a'+i%26)) + strings.Repeat("y", i/26) + ".txt",
			"new_size": 2048,
		})
	}
	e.IngestEvents(wl, evs)
	for i := 0; i < files; i++ {
		e.WriteFile(root, "f"+string(rune('a'+i%26))+strings.Repeat("y", i/26)+".txt",
			strings.Repeat("v", 2048))
	}
	snap := e.Capture(wl, tt.TS(300))
	return wl, snap["id"].(string), root
}

func TestSnapshotIntegrity(t *testing.T) {
	t.Run("seal_basics", func(t *testing.T) {
		e := tt.New(t)
		wl, snapID, _ := captureBaselineSnapshot(t, e, "int-basic", 3)
		_, list := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/snapshots", nil)
		snaps := list["snapshots"].([]any)
		if len(snaps) != 1 {
			t.Fatalf("expected 1 snapshot, got %d", len(snaps))
		}
		s := snaps[0].(map[string]any)
		if s["status"] != "sealed" {
			t.Errorf("status = %v, want sealed", s["status"])
		}
		if s["file_count"].(float64) != 3 {
			t.Errorf("file_count = %v, want 3", s["file_count"])
		}
		mh := s["manifest_hash"].(string)
		if len(mh) != 64 {
			t.Errorf("manifest hash length = %d", len(mh))
		}
		if snapID == "" {
			t.Error("empty snapshot id")
		}
	})

	t.Run("object_dedup_same_content", func(t *testing.T) {
		// identical content across snapshots shares one object (content addressing)
		e := tt.New(t)
		wl, root := e.RegisterWorkload("int-dedup")
		e.IngestEvents(wl, []map[string]any{{
			"id": "d1", "timestamp": tt.TS(30), "operation": "create",
			"path": "a.txt", "new_size": 1024}})
		e.WriteFile(root, "a.txt", strings.Repeat("z", 1024))
		e.Capture(wl, tt.TS(290))
		e.Capture(wl, tt.TS(590))
		// count rows in file_objects
		var n int
		if err := e.Store.DB().QueryRow(`SELECT COUNT(1) FROM file_objects`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("expected 1 object for identical content, got %d", n)
		}
	})

	t.Run("corrupted_object_detected", func(t *testing.T) {
		// T03: tamper with an object's bytes; analyze must mark integrity failed
		e := tt.New(t)
		wl, _, _ := captureBaselineSnapshot(t, e, "int-corrupt", 2)
		var objPath string
		err := e.Store.DB().QueryRow(`SELECT storage_path FROM file_objects LIMIT 1`).Scan(&objPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.FromSlash(objPath), []byte("tampered bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
		st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/analyze", map[string]any{})
		if st != 200 {
			t.Fatalf("analyze: %d %v", st, out)
		}
		_, list := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/snapshots", nil)
		s := list["snapshots"].([]any)[0].(map[string]any)
		a := s["assessment"].(map[string]any)
		if a["integrity_ok"].(bool) {
			t.Error("tampered object still reported integrity_ok")
		}
	})

	t.Run("missing_object_detected", func(t *testing.T) {
		// T03b: delete an object file entirely
		e := tt.New(t)
		wl, _, _ := captureBaselineSnapshot(t, e, "int-missing", 1)
		var objPath string
		if err := e.Store.DB().QueryRow(`SELECT storage_path FROM file_objects LIMIT 1`).Scan(&objPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.FromSlash(objPath)); err != nil {
			t.Fatal(err)
		}
		e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/analyze", map[string]any{})
		_, list := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/snapshots", nil)
		s := list["snapshots"].([]any)[0].(map[string]any)
		if s["assessment"].(map[string]any)["integrity_ok"].(bool) {
			t.Error("missing object still integrity_ok")
		}
	})

	t.Run("tampered_manifest_detected", func(t *testing.T) {
		// T04: alter manifest rows (e.g. size) so the recomputed hash differs
		e := tt.New(t)
		wl, _, _ := captureBaselineSnapshot(t, e, "int-manifest", 2)
		if _, err := e.Store.DB().Exec(`UPDATE snapshot_files SET size_bytes = size_bytes + 1`); err != nil {
			t.Fatal(err)
		}
		e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/analyze", map[string]any{})
		_, list := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/snapshots", nil)
		s := list["snapshots"].([]any)[0].(map[string]any)
		if s["assessment"].(map[string]any)["integrity_ok"].(bool) {
			t.Error("tampered manifest still integrity_ok")
		}
	})

	t.Run("sealed_content_not_mutable_via_api", func(t *testing.T) {
		// there is no API to mutate or delete a sealed snapshot
		e := tt.New(t)
		wl, snapID, _ := captureBaselineSnapshot(t, e, "int-immutable", 1)
		for _, attempt := range []struct{ method, path string }{
			{"DELETE", "/api/v1/workloads/" + wl + "/snapshots/" + snapID},
			{"PUT", "/api/v1/workloads/" + wl + "/snapshots/" + snapID},
			{"PATCH", "/api/v1/workloads/" + wl + "/snapshots/" + snapID},
		} {
			st, _ := e.Client("operator", attempt.method, attempt.path, map[string]any{"status": "invalid"})
			if st < 400 {
				t.Errorf("%s %s mutated sealed snapshot (status %d)", attempt.method, attempt.path, st)
			}
		}
		_, list := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/snapshots", nil)
		if list["snapshots"].([]any)[0].(map[string]any)["status"] != "sealed" {
			t.Error("sealed status changed")
		}
	})

	t.Run("capture_with_missing_root_returns_422", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("int-noroot")
		e.IngestEvents(wl, []map[string]any{{
			"id": "nr1", "timestamp": tt.TS(30), "operation": "create",
			"path": "a.txt", "new_size": 10}})
		if err := os.RemoveAll(filepath.Join(filepath.FromSlash(e.RootsDir()), "int-noroot")); err != nil {
			t.Fatal(err)
		}
		st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/snapshots", map[string]any{})
		if st != 422 || out["code"] != "root_unavailable" {
			t.Errorf("missing root capture: %d %v", st, out)
		}
		// no snapshot row may claim sealed
		var n int
		_ = e.Store.DB().QueryRow(`SELECT COUNT(1) FROM snapshots WHERE status='sealed'`).Scan(&n)
		if n != 0 {
			t.Errorf("%d sealed snapshots despite missing root", n)
		}
	})

	t.Run("capture_without_events_rejected", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("int-noevents")
		e.WriteFile(filepath.Join(e.RootsDir(), "int-noevents"), "a.txt", "x")
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/snapshots", map[string]any{})
		if st == 201 {
			t.Error("snapshot without telemetry accepted")
		}
	})

	t.Run("manifest_hash_is_content_addressed", func(t *testing.T) {
		// same file bytes + same paths across two workloads -> same manifest hash
		e := tt.New(t)
		_, _, _ = captureBaselineSnapshot(t, e, "int-ca-1", 2)
		_, _, _ = captureBaselineSnapshot(t, e, "int-ca-2", 2)
		var hashes []string
		rows, err := e.Store.DB().Query(`SELECT manifest_hash FROM snapshots ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var h string
			_ = rows.Scan(&h)
			hashes = append(hashes, h)
		}
		if len(hashes) != 2 || hashes[0] != hashes[1] {
			t.Errorf("identical trees produced different manifests: %v", hashes)
		}
	})

	t.Run("object_store_layout_fans_out", func(t *testing.T) {
		e := tt.New(t)
		_, _, root := captureBaselineSnapshot(t, e, "int-fanout", 1)
		_ = root
		var paths []string
		rows, err := e.Store.DB().Query(`SELECT storage_path FROM file_objects`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			_ = rows.Scan(&p)
			paths = append(paths, p)
		}
		for _, p := range paths {
			rel := strings.TrimPrefix(p, e.Cfg.ObjectsDir()+"/")
			parts := strings.Split(rel, "/")
			if len(parts) != 3 || len(parts[0]) != 2 || len(parts[1]) != 2 {
				t.Errorf("object not fanned out: %s", p)
			}
		}
	})

	t.Run("sha_roundtrip", func(t *testing.T) {
		content := strings.Repeat("cleanpoint", 200)
		sum := sha256.Sum256([]byte(content))
		var stored string
		e := tt.New(t)
		wl, root := e.RegisterWorkload("int-sha")
		e.IngestEvents(wl, []map[string]any{{
			"id": "s1", "timestamp": tt.TS(30), "operation": "create",
			"path": "x.bin", "new_size": len(content)}})
		e.WriteFile(root, "x.bin", content)
		e.Capture(wl, tt.TS(200))
		if err := e.Store.DB().QueryRow(`SELECT sha256 FROM snapshot_files LIMIT 1`).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != hex.EncodeToString(sum[:]) {
			t.Errorf("stored sha %s != computed %s", stored, hex.EncodeToString(sum[:]))
		}
	})
}

// ---------- Event ingestion (8+ cases) ----------

func TestEventIngestion(t *testing.T) {
	t.Run("valid_batch_accepted", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-ok")
		st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{
			"events": []map[string]any{
				{"id": "e1", "timestamp": "2026-01-05T09:00:10Z", "operation": "create", "path": "a.txt"},
				{"id": "e2", "timestamp": "2026-01-05T09:01:10Z", "operation": "modify", "path": "a.txt"},
			}})
		if st != 200 || out["ingested"].(float64) != 2 {
			t.Errorf("ingest: %d %v", st, out)
		}
	})

	t.Run("idempotent_same_event", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-dup")
		ev := map[string]any{"events": []map[string]any{
			{"id": "dup1", "timestamp": tt.TS(10), "operation": "create", "path": "a.txt"}}}
		e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", ev)
		st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", ev)
		if st != 200 || out["ingested"].(float64) != 0 || out["duplicates"].(float64) != 1 {
			t.Errorf("duplicate handling: %d %v", st, out)
		}
	})

	t.Run("conflicting_same_id_rejected", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-conflict")
		e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{
			"events": []map[string]any{{"id": "c1", "timestamp": tt.TS(10), "operation": "create", "path": "a.txt"}}})
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{
			"events": []map[string]any{{"id": "c1", "timestamp": tt.TS(10), "operation": "delete", "path": "a.txt"}}})
		if st != 409 {
			t.Errorf("conflicting payload under same id: expected 409, got %d", st)
		}
	})

	t.Run("out_of_order_accepted", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-ooo")
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{
			"events": []map[string]any{
				{"id": "o2", "timestamp": tt.TS(500), "operation": "modify", "path": "a.txt"},
				{"id": "o1", "timestamp": tt.TS(100), "operation": "create", "path": "a.txt"},
			}})
		if st != 200 {
			t.Errorf("out-of-order batch rejected: %d", st)
		}
	})

	t.Run("invalid_operation_rejected", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-badop")
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{
			"events": []map[string]any{{"id": "x1", "timestamp": tt.TS(10), "operation": "chmod", "path": "a.txt"}}})
		if st != 400 {
			t.Errorf("invalid operation: expected 400, got %d", st)
		}
	})

	t.Run("invalid_timestamp_rejected", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-badts")
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{
			"events": []map[string]any{{"id": "x2", "timestamp": "not-a-time", "operation": "create", "path": "a.txt"}}})
		if st != 400 {
			t.Errorf("invalid timestamp: expected 400, got %d", st)
		}
	})

	t.Run("bad_hash_format_rejected", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-badhash")
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{
			"events": []map[string]any{{"id": "x3", "timestamp": tt.TS(10), "operation": "create",
				"path": "a.txt", "new_sha256": "zzz"}}})
		if st != 400 {
			t.Errorf("bad sha: expected 400, got %d", st)
		}
	})

	t.Run("empty_batch_rejected_and_unknown_workload_404", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-empty")
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{"events": []map[string]any{}})
		if st != 400 {
			t.Errorf("empty batch: expected 400, got %d", st)
		}
		st, _ = e.Client("operator", "POST", "/api/v1/workloads/wl-nonexistent/events", map[string]any{
			"events": []map[string]any{{"id": "x", "timestamp": tt.TS(1), "operation": "create", "path": "a"}}})
		if st != 404 {
			t.Errorf("unknown workload: expected 404, got %d", st)
		}
	})

	t.Run("entropy_bounds_enforced", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-ent")
		st, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{
			"events": []map[string]any{{"id": "x4", "timestamp": tt.TS(10), "operation": "create",
				"path": "a.txt", "new_entropy": 9.5}}})
		if st != 400 {
			t.Errorf("entropy 9.5 accepted (%d)", st)
		}
	})

	t.Run("events_listed_chronologically", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("ing-list")
		e.IngestEvents(wl, []map[string]any{
			{"id": "l2", "timestamp": tt.TS(500), "operation": "modify", "path": "a.txt"},
			{"id": "l1", "timestamp": tt.TS(100), "operation": "create", "path": "a.txt"},
		})
		_, out := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/events", nil)
		evs := out["events"].([]any)
		if evs[0].(map[string]any)["id"] != "l1" {
			t.Errorf("events not chronological: %v", evs)
		}
	})
}

var _ = storage.ErrNotFound
