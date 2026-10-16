// Package metrics keeps restricted Prometheus-format counters shared across
// packages. Counters only ever increase; gauges are simple last-write values.
package metrics

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

type counters struct {
	mu     sync.Mutex
	values map[string]float64
	order  []string
	// duration accumulators: name -> [count, sumSeconds]
	durations map[string][2]float64
	durOrder  []string
}

var global = &counters{
	values:    map[string]float64{},
	durations: map[string][2]float64{},
}

// Inc increments a labeled counter such as
// Inc("snapshot_capture_total", "outcome", "sealed").
func Inc(name string, labelName, labelValue string) {
	global.mu.Lock()
	defer global.mu.Unlock()
	key := name
	if labelName != "" {
		key = name + "{" + labelName + `="` + labelValue + `"}`
	}
	if _, seen := global.values[key]; !seen {
		global.order = append(global.order, key)
	}
	global.values[key]++
}

// Observe records a duration for p50/p95-style aggregates.
func Observe(name string, d time.Duration) {
	global.mu.Lock()
	defer global.mu.Unlock()
	acc, seen := global.durations[name]
	if !seen {
		global.durOrder = append(global.durOrder, name)
	}
	acc[0]++
	acc[1] += d.Seconds()
	global.durations[name] = acc
}

// Render produces the Prometheus text exposition.
func Render() string {
	global.mu.Lock()
	defer global.mu.Unlock()
	var b strings.Builder
	for _, k := range global.order {
		fmt.Fprintf(&b, "%s %v\n", sanitize(k), num(global.values[k]))
	}
	for _, name := range global.durOrder {
		acc := global.durations[name]
		count, sum := acc[0], acc[1]
		fmt.Fprintf(&b, "%s_count %v\n%s_sum %.6f\n", sanitize(name), num(count), sanitize(name), sum)
	}
	host, _ := os.Hostname()
	fmt.Fprintf(&b, "cleanpoint_info{version=\"1.0.0\",go=%q,host=%q} 1\n", runtime.Version(), host)
	return b.String()
}

func num(f float64) string {
	return fmt.Sprintf("%g", f)
}

func sanitize(k string) string {
	k = strings.ReplaceAll(k, `"`, `'`)
	return k
}

// Handler serves metrics (callers must apply auth restrictions).
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, Render())
	})
}
