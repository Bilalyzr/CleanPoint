package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tt "cleanpoint/tests/harness"
)

// ---------- M2 console assets & support API routes ----------

func TestM2EndpointsAndWebUI(t *testing.T) {
	t.Run("landing_page_and_assets_served", func(t *testing.T) {
		e := tt.New(t)
		st, html := e.Raw("GET", "/", "", nil)
		if st != 200 || !strings.Contains(html, "CleanPoint") {
			t.Fatalf("landing page: status=%d len=%d", st, len(html))
		}
		stCSS, css := e.Raw("GET", "/landing/landing.css", "", nil)
		if stCSS != 200 || len(css) == 0 {
			t.Errorf("landing.css: status=%d len=%d", stCSS, len(css))
		}
		stJS, js := e.Raw("GET", "/landing/landing.js", "", nil)
		if stJS != 200 || len(js) == 0 {
			t.Errorf("landing.js: status=%d len=%d", stJS, len(js))
		}
		stBad, bad := e.Raw("GET", "/landing/not-allowed.js", "", nil)
		if stBad != 404 || !strings.Contains(bad, "not_found") {
			t.Errorf("unlisted landing asset: status=%d body=%s", stBad, bad)
		}
	})

	t.Run("console_spa_and_deep_link_fallback", func(t *testing.T) {
		e := tt.New(t)
		st, html := e.Raw("GET", "/app/", "", nil)
		if st != 200 || !strings.Contains(html, `id="root"`) {
			t.Fatalf("console /app/: status=%d body=%.120s", st, html)
		}
		stDeep, deepHTML := e.Raw("GET", "/app/workload/wl-demo/findings", "", nil)
		if stDeep != 200 || !strings.Contains(deepHTML, `id="root"`) {
			t.Errorf("SPA deep-link fallback: status=%d body=%.120s", stDeep, deepHTML)
		}
	})

	t.Run("snapshot_detail_manifest_and_limit", func(t *testing.T) {
		e := tt.New(t)
		wl, root := e.RegisterWorkload("m2-snap-detail")
		e.WriteFile(root, "docs/a.txt", "alpha content")
		e.WriteFile(root, "docs/b.txt", "bravo content")
		e.WriteFile(root, "docs/c.txt", "charlie content")
		e.IngestEvents(wl, []map[string]any{
			{"id": "ev-1", "timestamp": tt.TS(60), "operation": "create", "path": "docs/a.txt"},
		})
		snap := e.Capture(wl, tt.TS(120))
		sid := snap["id"].(string)

		st, out := e.Client("ro", "GET", "/api/v1/snapshots/"+sid+"?limit=2", nil)
		if st != 200 {
			t.Fatalf("snapshot detail: %d %v", st, out)
		}
		if int(out["file_total"].(float64)) != 3 {
			t.Errorf("expected file_total=3, got %v", out["file_total"])
		}
		files := out["files"].([]any)
		if len(files) != 2 {
			t.Errorf("expected limit=2 files, got %d", len(files))
		}

		st404, _ := e.Client("ro", "GET", "/api/v1/snapshots/snap-does-not-exist", nil)
		if st404 != 404 {
			t.Errorf("unknown snapshot expected 404, got %d", st404)
		}
	})

	t.Run("analysis_runs_history", func(t *testing.T) {
		e := tt.New(t)
		wl, root := e.RegisterWorkload("m2-analysis-runs")
		e.WriteFile(root, "report.txt", "clean baseline report")
		e.IngestEvents(wl, []map[string]any{
			{"id": "ar-1", "timestamp": tt.TS(0), "operation": "create", "path": "report.txt"},
			{"id": "ar-2", "timestamp": tt.TS(25 * 300), "operation": "modify", "path": "report.txt"},
		})

		st0, out0 := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/analysis-runs", nil)
		if st0 != 200 || len(out0["analysis_runs"].([]any)) != 0 {
			t.Fatalf("before analyze: %d %v", st0, out0)
		}

		stAn, _ := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/analyze", map[string]any{})
		if stAn != 200 {
			t.Fatalf("analyze failed: %d", stAn)
		}

		st1, out1 := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/analysis-runs", nil)
		runs := out1["analysis_runs"].([]any)
		if st1 != 200 || len(runs) != 1 {
			t.Fatalf("after analyze: %d %v", st1, out1)
		}
	})

	t.Run("evaluations_endpoint_and_traversal_guard", func(t *testing.T) {
		e := tt.New(t)
		resDir := t.TempDir()
		t.Setenv("CLEANPOINT_RESULTS_DIR", resDir)

		stMissing, outMissing := e.Client("ro", "GET", "/api/v1/evaluations", nil)
		if stMissing != 200 || outMissing["available"] != false {
			t.Fatalf("missing eval report: %d %v", stMissing, outMissing)
		}

		if err := os.WriteFile(filepath.Join(resDir, "eval-locked.json"),
			[]byte(`{"metrics":{"unsafe_selections":0,"scenarios":16}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		stOK, outOK := e.Client("ro", "GET", "/api/v1/evaluations", nil)
		if stOK != 200 || outOK["metrics"] == nil {
			t.Fatalf("valid eval report: %d %v", stOK, outOK)
		}

		for _, bad := range []string{"../secret.json", `..\secret.json`, "notes.txt"} {
			stBad, _ := e.Client("ro", "GET", "/api/v1/evaluations?name="+bad, nil)
			if stBad != 400 {
				t.Errorf("name=%q expected 400, got %d", bad, stBad)
			}
		}
	})

	t.Run("simulation_rbac_and_input_validation", func(t *testing.T) {
		e := tt.New(t)
		if st, _ := e.Client("ro", "POST", "/api/v1/simulate", map[string]any{"scenario": "normal_office", "seed": 1}); st != 403 {
			t.Errorf("read-only POST /simulate expected 403, got %d", st)
		}
		if st, _ := e.Client("ro", "GET", "/api/v1/simulations", nil); st != 403 {
			t.Errorf("read-only GET /simulations expected 403, got %d", st)
		}
		if st, _ := e.Client("ro", "GET", "/api/v1/simulations/sim-unknown", nil); st != 403 {
			t.Errorf("read-only GET /simulations/{id} expected 403, got %d", st)
		}
		if st, _ := e.Client("operator", "POST", "/api/v1/simulate", map[string]any{"scenario": "not_a_family", "seed": 1}); st != 400 {
			t.Errorf("invalid family expected 400, got %d", st)
		}
		if st, _ := e.Client("operator", "POST", "/api/v1/simulate", map[string]any{"scenario": "normal_office", "seed": 0}); st != 400 {
			t.Errorf("seed=0 expected 400, got %d", st)
		}
		stList, outList := e.Client("operator", "GET", "/api/v1/simulations", nil)
		if stList != 200 || outList["simulations"] == nil {
			t.Errorf("GET /simulations: %d %v", stList, outList)
		}
		if st, _ := e.Client("operator", "GET", "/api/v1/simulations/sim-does-not-exist", nil); st != 404 {
			t.Errorf("unknown simulation job expected 404, got %d", st)
		}
	})
}
