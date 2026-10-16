// Command cli is the cleanpoint CLI: serve, simulate, evaluate, benchmark
// and demo. Scenario generation and evaluation delegate to the deterministic
// Python simulator (single source of truth for fixtures); benchmarks are
// native Go so they measure the API itself.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		serveCmd(os.Args[2:])
	case "simulate":
		simulateCmd(os.Args[2:])
	case "evaluate":
		evaluateCmd(os.Args[2:])
	case "benchmark":
		benchmarkCmd(os.Args[2:])
	case "demo":
		demoCmd(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`cleanpoint — local ransomware-recovery analysis simulator (defensive prototype)

Usage:
  cleanpoint serve                                  run the local API server
  cleanpoint simulate --scenario <name> --seed <n>  generate a scenario and drive the API
  cleanpoint evaluate --scenario <folder> --json    run the evaluation harness
  cleanpoint benchmark snapshots --files N --file-size-kb K --output <file>
  cleanpoint benchmark restore  --files N --file-size-kb K --output <file>
  cleanpoint demo                                   full guided demonstration

Scenarios: normal_office, benign_archive, benign_bulk_update, rapid_destructive,
           slow_distributed, extension_churn, mixed_activity, pre_existing_contamination
`)
}

func serveCmd(args []string) {
	// Reuse the server command in-process.
	flag.CommandLine = flag.NewFlagSet("serve", flag.ExitOnError)
	// The server main is small; exec the same binary path is overkill —
	// import-free delegation keeps one process.
	serverMain()
}

func pythonCmd() string {
	if v := os.Getenv("CLEANPOINT_PYTHON"); v != "" {
		return v
	}
	return "python"
}

func repoRoot() string {
	exe, err := os.Executable()
	if err == nil {
		// Development layout: repo/cmd/cli/cli.exe
		if d := filepath.Dir(filepath.Dir(exe)); strings.HasSuffix(filepath.ToSlash(d), "/cmd") {
			return filepath.Dir(d)
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func simulateCmd(args []string) {
	fs := flag.NewFlagSet("simulate", flag.ExitOnError)
	scenario := fs.String("scenario", "normal_office", "scenario family")
	seed := fs.Int("seed", 1, "deterministic seed")
	api := fs.String("api", envOr("CLEANPOINT_API", "http://127.0.0.1:8080"), "API base URL")
	fs.Parse(args)
	runPython(repoFile("simulator", "scenario_driver.py"),
		"--scenario", *scenario, "--seed", fmt.Sprint(*seed), "--api", *api)
}

func evaluateCmd(args []string) {
	fs := flag.NewFlagSet("evaluate", flag.ExitOnError)
	scenario := fs.String("scenario", "", "scenario folder or 'all'")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	api := fs.String("api", envOr("CLEANPOINT_API", "http://127.0.0.1:8080"), "API base URL")
	out := fs.String("output", "", "write JSON report to file")
	fs.Parse(args)
	argz := []string{"--api", *api}
	if *scenario != "" {
		argz = append(argz, "--scenario", *scenario)
	}
	if *asJSON {
		argz = append(argz, "--json")
	}
	if *out != "" {
		argz = append(argz, "--output", *out)
	}
	runPython(repoFile("benchmarks", "evaluate.py"), argz...)
}

func demoCmd(args []string) {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	api := fs.String("api", envOr("CLEANPOINT_API", "http://127.0.0.1:8080"), "API base URL")
	fs.Parse(args)
	runPython(repoFile("simulator", "demo.py"), "--api", *api)
}

func benchmarkCmd(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cleanpoint benchmark snapshots|restore --files N --file-size-kb K --output FILE")
		os.Exit(2)
	}
	kind := args[0]
	fs := flag.NewFlagSet("benchmark", flag.ExitOnError)
	files := fs.Int("files", 1000, "number of mock files")
	fileSizeKB := fs.Int("file-size-kb", 32, "size of each file in KiB")
	output := fs.String("output", "", "output JSON file")
	api := fs.String("api", envOr("CLEANPOINT_API", "http://127.0.0.1:8080"), "API base URL")
	warmup := fs.Int("warmup", 0, "warmup iterations")
	iterations := fs.Int("iterations", 3, "measured iterations")
	fs.Parse(args[1:])
	if *files <= 0 || *iterations <= 0 {
		fmt.Fprintln(os.Stderr, "--files and --iterations must be positive")
		os.Exit(2)
	}
	if kind != "snapshots" && kind != "restore" {
		fmt.Fprintln(os.Stderr, "benchmark kind must be snapshots or restore")
		os.Exit(2)
	}
	if err := runBenchmark(kind, *api, *files, *fileSizeKB, *iterations, *warmup, *output); err != nil {
		fmt.Fprintln(os.Stderr, "benchmark failed:", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func repoFile(parts ...string) string {
	return filepath.Join(append([]string{repoRoot()}, parts...)...)
}

func runPython(script string, args ...string) {
	cmd := exec.Command(pythonCmd(), append([]string{script}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "python %s failed: %v\n", script, err)
		os.Exit(1)
	}
}

// ---- native benchmark ----

type benchSample struct {
	Iteration int     `json:"iteration"`
	DurationMS float64 `json:"duration_ms"`
}

type benchReport struct {
	Kind         string        `json:"kind"`
	Files        int           `json:"files"`
	FileSizeKB   int           `json:"file_size_kb"`
	TotalMiB     float64       `json:"total_mib"`
	Iterations   int           `json:"iterations"`
	Samples      []benchSample `json:"samples"`
	P50MS        float64       `json:"p50_ms"`
	P95MS        float64       `json:"p95_ms"`
	MeanMS       float64       `json:"mean_ms"`
	ThroughputMBPerSec float64 `json:"throughput_mb_per_sec"`
	FilesPerSec  float64       `json:"files_per_sec"`
	Hardware     string        `json:"hardware"`
	GoVersion    string        `json:"go_version"`
	Timestamp    string        `json:"timestamp"`
	APIURL       string        `json:"api_url"`
}

func runBenchmark(kind, apiURL string, files, fileSizeKB, iterations, warmup int, output string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	token, err := operatorToken()
	if err != nil {
		return err
	}
	hw, _ := os.Hostname()

	// Unique workload for this benchmark run.
	name := fmt.Sprintf("bench-%s-%d-%d-%d", kind, files, fileSizeKB, time.Now().Unix())
	// name regex caps at 64 chars and lowercase/digits/dash: ok.
	var wl struct {
		ID       string `json:"id"`
		RootPath string `json:"root_path"`
	}
	if err := apiJSON(client, apiURL, "POST", "/api/v1/workloads", token,
		map[string]any{"name": name}, &wl); err != nil {
		return fmt.Errorf("register workload: %w", err)
	}
	defer func() {
		// Bench workloads are disposable data; leave them for the operator
		// (no delete API by design).
		_ = hw
	}()

	// Write mock files into the workload root.
	root := filepath.FromSlash(wl.RootPath)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	buf := make([]byte, fileSizeKB*1024)
	for i := range buf {
		buf[i] = byte(i*31 + 7)
	}
	totalBytes := int64(files * fileSizeKB * 1024)
	for i := 0; i < files; i++ {
		rel := fmt.Sprintf("data/file-%04d.bin", i)
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, buf, 0o644); err != nil {
			return err
		}
	}
	// Ingest one create event per file so the workload has telemetry.
	// Timestamps span 26 windows so a baseline (24) + evaluated windows
	// exist and the workload is certifiable.
	const batchSize = 500
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spanSeconds := int64(26 * 300)
	evs := make([]map[string]any, 0, batchSize)
	for i := 0; i < files; i++ {
		rel := fmt.Sprintf("data/file-%04d.bin", i)
		off := int64(i) * spanSeconds / int64(files)
		evs = append(evs, map[string]any{
			"id":        fmt.Sprintf("%s-ev-%05d", name, i),
			"timestamp": base.Add(time.Duration(off) * time.Second).Format("2006-01-02T15:04:05.000Z"),
			"operation": "create",
			"path":      rel,
			"new_size":  int64(fileSizeKB * 1024),
			"actor":     "bench",
		})
		if len(evs) == batchSize || i == files-1 {
			if err := apiJSON(client, apiURL, "POST", "/api/v1/workloads/"+wl.ID+"/events", token,
				map[string]any{"events": evs}, nil); err != nil {
				return fmt.Errorf("ingest events: %w", err)
			}
			evs = evs[:0]
		}
	}


	sample := func() (float64, string, error) {
		start := time.Now()
		var snap struct {
			ID string `json:"id"`
		}
		if err := apiJSON(client, apiURL, "POST", "/api/v1/workloads/"+wl.ID+"/snapshots", token,
			map[string]any{}, &snap); err != nil {
			return 0, "", err
		}
		if kind == "snapshots" {
			return float64(time.Since(start).Milliseconds()), snap.ID, nil
		}
		// The restore path requires a completed analysis covering the fresh
		// snapshot before eligibility can be certified.
		if err := apiJSON(client, apiURL, "POST", "/api/v1/workloads/"+wl.ID+"/analyze", token,
			map[string]any{}, nil); err != nil {
			return 0, "", fmt.Errorf("analyze: %w", err)
		}
		// restore benchmark: time the restore of the fresh snapshot
		startRestore := time.Now()
		var rep struct {
			Status string `json:"status"`
		}
		if err := apiJSON(client, apiURL, "POST", "/api/v1/workloads/"+wl.ID+"/restore", token,
			map[string]any{"snapshot_id": snap.ID, "target_name": "bench-" + name, "confirm": true}, &rep); err != nil {
			return 0, "", err
		}
		if rep.Status != "verified" {
			return 0, "", fmt.Errorf("restore did not verify: %s", rep.Status)
		}
		return float64(time.Since(startRestore).Milliseconds()), snap.ID, nil
	}

	for i := 0; i < warmup; i++ {
		if _, _, err := sample(); err != nil {
			return err
		}
	}
	rep := benchReport{
		Kind: kind, Files: files, FileSizeKB: fileSizeKB,
		TotalMiB:   float64(totalBytes) / (1024 * 1024),
		Iterations: iterations, Hardware: hw, GoVersion: goVersion(),
		Timestamp: time.Now().UTC().Format(time.RFC3339), APIURL: apiURL,
	}
	for i := 0; i < iterations; i++ {
		d, _, err := sample()
		if err != nil {
			return err
		}
		rep.Samples = append(rep.Samples, benchSample{Iteration: i + 1, DurationMS: d})
	}
	rep.computeStats()
	b, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(b))
	if output != "" {
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(output, b, 0o644); err != nil {
			return err
		}
		fmt.Printf("report written to %s\n", output)
	}
	return nil
}

func (r *benchReport) computeStats() {
	n := len(r.Samples)
	if n == 0 {
		return
	}
	ds := make([]float64, n)
	sum := 0.0
	for i, s := range r.Samples {
		ds[i] = s.DurationMS
		sum += s.DurationMS
	}
	sortFloats(ds)
	r.MeanMS = round(sum / float64(n))
	r.P50MS = round(pct(ds, 50))
	r.P95MS = round(pct(ds, 95))
	if r.MeanMS > 0 {
		r.ThroughputMBPerSec = round(r.TotalMiB / (r.MeanMS / 1000))
		r.FilesPerSec = round(float64(r.Files) / (r.MeanMS / 1000))
	}
}

func sortFloats(xs []float64) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

func pct(sorted []float64, p int) float64 {
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func round(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

// apiJSON performs a token-authenticated JSON call; out may be nil.
func apiJSON(client *http.Client, base, method, path, token string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s -> %d: %s", method, path, resp.StatusCode, truncate(string(data), 400))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func operatorToken() (string, error) {
	if t := os.Getenv("CLEANPOINT_OPERATOR_TOKEN"); t != "" {
		return t, nil
	}
	sandbox := envOr("CLEANPOINT_SANDBOX_ROOT", "sandbox")
	data, err := os.ReadFile(filepath.Join(filepath.FromSlash(sandbox), ".tokens"))
	if err != nil {
		return "", fmt.Errorf("no operator token (set CLEANPOINT_OPERATOR_TOKEN or run the server once to create %s/.tokens): %w", sandbox, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "operator="); ok {
			return strings.TrimSpace(v), nil
		}
	}
	return "", fmt.Errorf("operator token not found in %s/.tokens", sandbox)
}

func goVersion() string { return runtimeVersion() }
