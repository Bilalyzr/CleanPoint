#!/usr/bin/env python3
"""Latency micro-benchmarks against a live CleanPoint API.

Measures p50/p95 for: candidate selection, findings fetch, snapshot capture
and restore on an existing prepared workload. Reports hardware + sample
counts alongside numbers (no invented results).

Usage:
  python latency.py --api http://127.0.0.1:8080 --workload wl-xxxx [--samples 20]
"""

import argparse
import json
import os
import platform
import statistics
import sys
import time
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
sys.path.insert(0, os.path.join(ROOT, "simulator"))

from scenario_driver import http_json, load_token  # noqa: E402


def pct(xs, p):
    xs = sorted(xs)
    idx = min(len(xs) - 1, int(round(p / 100 * (len(xs) - 1))))
    return xs[idx]


def bench(name, fn, samples):
    ds = []
    for _ in range(samples):
        t0 = time.perf_counter()
        ok = fn()
        if not ok:
            return {"name": name, "error": True}
        ds.append((time.perf_counter() - t0) * 1000)
    return {
        "name": name, "samples": len(ds),
        "p50_ms": round(pct(ds, 50), 2), "p95_ms": round(pct(ds, 95), 2),
        "mean_ms": round(statistics.fmean(ds), 2), "max_ms": round(max(ds), 2),
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", default=os.environ.get("CLEANPOINT_API", "http://127.0.0.1:8080"))
    ap.add_argument("--workload", required=True)
    ap.add_argument("--samples", type=int, default=20)
    ap.add_argument("--output")
    args = ap.parse_args()
    token = load_token(args.api)

    # find newest eligible snapshot for restore benchmarking
    st, cand = http_json("GET", args.api + "/api/v1/workloads/%s/recovery-candidates"
                         % args.workload, token)
    if st >= 300:
        sys.exit("no candidates for workload: %s" % cand)
    rec = cand.get("recommended_snapshot_id")

    results = [
        bench("recovery_candidates", lambda: http_json(
            "GET", args.api + "/api/v1/workloads/%s/recovery-candidates" % args.workload,
            token)[0] == 200, args.samples),
        bench("findings", lambda: http_json(
            "GET", args.api + "/api/v1/workloads/%s/findings" % args.workload,
            token)[0] == 200, args.samples),
        bench("snapshot_capture", lambda: http_json(
            "POST", args.api + "/api/v1/workloads/%s/snapshots" % args.workload,
            token, {})[0] == 201, max(3, args.samples // 4)),
    ]
    if rec:
        n = [0]

        def do_restore():
            n[0] += 1
            stx, rep = http_json("POST", args.api + "/api/v1/workloads/%s/restore"
                                 % args.workload, token,
                                 {"snapshot_id": rec, "target_name": "latency-%03d" % n[0],
                                  "confirm": True})
            return stx in (201, 409) and rep.get("status") in ("verified",)
        results.append(bench("restore_verified", do_restore, max(3, args.samples // 4)))

    out = {
        "api": args.api,
        "workload": args.workload,
        "hardware": platform.node() + " / " + platform.machine() + " / " +
                    platform.platform(),
        "python": platform.python_version(),
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "results": results,
    }
    print(json.dumps(out, indent=2))
    if args.output:
        os.makedirs(os.path.dirname(os.path.abspath(args.output)), exist_ok=True)
        json.dump(out, open(args.output, "w"), indent=2)


if __name__ == "__main__":
    main()
