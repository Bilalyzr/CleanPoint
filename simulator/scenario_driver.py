#!/usr/bin/env python3
"""Drive one scenario end-to-end against the CleanPoint API.

Phases: register workload (sandbox-allowlisted root) -> for each snapshot
point: rebuild tree through the phase's event index, ingest the phase events,
capture a snapshot -> analyze -> fetch candidates -> (if available) restore
the recommended snapshot with explicit confirmation -> verify report.

Ground truth (truth.json) stays LOCAL: it is only used here to compute the
evaluation verdicts, never sent to or read by the detector.

Usage:
  python scenario_driver.py --scenario rapid_destructive --seed 1 \
      --api http://127.0.0.1:8080 [--result out.json] [--no-restore]
"""

import argparse
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
sys.path.insert(0, HERE)

import generate  # noqa: E402


def http_json(method, url, token, body=None, expect=(200, 201, 409)):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", "Bearer " + token)
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            payload = json.loads(resp.read().decode() or "{}")
            return resp.status, payload
    except urllib.error.HTTPError as e:
        body_txt = e.read().decode()
        try:
            payload = json.loads(body_txt or "{}")
        except json.JSONDecodeError:
            payload = {"raw": body_txt}
        return e.code, payload


def load_token(api):
    tok = os.environ.get("CLEANPOINT_OPERATOR_TOKEN")
    if tok:
        return tok
    sandbox = os.environ.get("CLEANPOINT_SANDBOX_ROOT", "sandbox")
    for cand in (os.path.join(ROOT, sandbox), os.path.join(os.getcwd(), sandbox), sandbox):
        path = os.path.join(cand, ".tokens")
        if os.path.isfile(path):
            for line in open(path):
                if line.startswith("operator="):
                    return line.split("=", 1)[1].strip()
    raise SystemExit("operator token not found; set CLEANPOINT_OPERATOR_TOKEN or run the server once")


def run_scenario(scenario, seed, api, token, out_base=None, do_restore=True, verbose=False):
    if out_base is None:
        sandbox = os.environ.get("CLEANPOINT_SANDBOX_ROOT", os.path.join(os.getcwd(), "sandbox"))
        out_base = os.path.join(os.path.abspath(sandbox), "sim", "%s-seed%d" % (scenario, seed))
    manifest_dir = os.path.join(out_base, "manifest")
    root = os.path.join(out_base, "root")

    # 1) deterministic manifest
    subprocess.run([sys.executable, os.path.join(HERE, "generate.py"),
                    "--scenario", scenario, "--seed", str(seed), "--out", manifest_dir],
                   check=True, capture_output=True)
    events = json.load(open(os.path.join(manifest_dir, "events.json")))
    truth = json.load(open(os.path.join(manifest_dir, "truth.json")))
    meta = json.load(open(os.path.join(manifest_dir, "meta.json")))

    # 2) register workload (root must live under the server's allowlisted
    #    sandbox/roots tree; we mirror that layout locally)
    sandbox = os.environ.get("CLEANPOINT_SANDBOX_ROOT", os.path.join(os.getcwd(), "sandbox"))
    roots_dir = os.path.join(os.path.abspath(sandbox), "roots")
    wl_root = os.path.join(roots_dir, "%s-seed%d" % (scenario, seed))
    # workload names allow only [a-z0-9-]; scenario families use underscores
    wl_name = ("%s-seed%d" % (scenario, seed)).replace("_", "-")[:64]

    # probe: tolerate re-runs by reusing an existing workload with same root
    status, wl = http_json("POST", api + "/api/v1/workloads", token,
                           {"name": wl_name, "root_path": wl_root.replace(os.sep, "/")})
    if status == 409:
        # name exists from a previous run: find it
        _, listing = http_json("GET", api + "/api/v1/workloads", token)
        wl = next(w for w in listing["workloads"] if w["name"] == wl_name)
    elif status >= 300:
        raise SystemExit("workload registration failed: %s %s" % (status, wl))

    # 3) phases — captured_at is the phase's logical window end (the
    # simulated clock), which matters when a scenario goes quiet afterwards.
    import datetime as dt
    import scenarios as scns
    base = dt.datetime.strptime(scns.BASE_EPOCH, "%Y-%m-%dT%H:%M:%S").replace(
        tzinfo=dt.timezone.utc)
    prev_idx = -1
    snapshots = []
    for point in meta["snapshot_points"]:
        through = point["through_index"]
        window = point["window"]
        captured_at = (base + dt.timedelta(
            seconds=(window + 1) * meta["window_seconds"])
        ).strftime("%Y-%m-%dT%H:%M:%S.000Z")
        phase_events = events[prev_idx + 1:through + 1]
        prev_idx = through
        # rebuild tree deterministically to this point
        subprocess.run([sys.executable, os.path.join(HERE, "generate.py"),
                        "--scenario", scenario, "--seed", str(seed),
                        "--root", wl_root, "--through", str(through)],
                       check=True, capture_output=True)
        if phase_events:
            st, resp = http_json("POST", api + "/api/v1/workloads/%s/events" % wl["id"],
                                 token, {"events": phase_events})
            if st >= 300:
                raise SystemExit("event ingest failed at through=%d: %s %s" % (through, st, resp))
        st, snap = http_json("POST", api + "/api/v1/workloads/%s/snapshots" % wl["id"],
                             token, {"captured_at": captured_at})
        if st >= 300:
            raise SystemExit("snapshot capture failed at through=%d: %s %s" % (through, st, snap))
        snapshots.append(snap)
        if verbose:
            print("  window %2d -> snapshot %s (%d files, %s)"
                  % (point["window"], snap["id"], snap["file_count"], snap["captured_at"]))

    # 4) analyze
    st, findings = http_json("POST", api + "/api/v1/workloads/%s/analyze" % wl["id"], token, {})
    if st >= 300:
        raise SystemExit("analyze failed: %s %s" % (st, findings))

    # 5) candidates
    st, cand = http_json("GET", api + "/api/v1/workloads/%s/recovery-candidates" % wl["id"], token)
    if st >= 300:
        raise SystemExit("candidates failed: %s %s" % (st, cand))

    # truth verdicts for every snapshot
    attack_start = truth["attack_start"]
    def contaminated(snap):
        return bool(attack_start) and snap["captured_at"] >= attack_start
    snap_truth = {s["id"]: contaminated(s) for s in snapshots}

    result = {
        "workload_id": wl["id"],
        "scenario": scenario,
        "seed": seed,
        "benign": truth["benign"],
        "attack_start": attack_start,
        "onset": findings.get("incident_onset"),
        "windows_evaluated": findings.get("windows_evaluated"),
        "suspicious_windows": sum(1 for w in findings.get("windows", [])
                                  if w.get("alert_level") == "suspicious"),
        "total_windows": len(findings.get("windows", [])),
        "selector_status": cand["status"],
        "recommended_snapshot_id": cand.get("recommended_snapshot_id"),
        "candidates": [
            {"snapshot_id": c["snapshot_id"], "eligible": c["eligible"],
             "exposure": c["exposure_status"], "integrity": c["integrity_ok"],
             "captured_at": c["captured_at"], "reasons": c["reasons"][:1]}
            for c in cand.get("candidates", [])
        ],
        "snapshot_truth": snap_truth,
    }

    # verdicts
    rec = result["recommended_snapshot_id"]
    if result["selector_status"] == "candidate_available":
        result["recommended_contaminated"] = snap_truth.get(rec, None)
        # newest clean snapshot by truth (selection success reference)
        clean = [s for s in snapshots if not snap_truth[s["id"]]]
        newest_clean = max((s["captured_at"] for s in clean), default=None)
        rec_snap = next((c for c in cand["candidates"] if c["snapshot_id"] == rec), None)
        result["selection_is_newest_clean"] = bool(
            newest_clean and rec_snap and rec_snap["captured_at"] == newest_clean)
    else:
        result["recommended_contaminated"] = None
        result["selection_is_newest_clean"] = None
        result["abstained"] = True
    # Was abstention defensible? True when abstained AND no truth-clean
    # snapshot was actually certifiable (i.e. every clean snapshot lacked
    # baseline coverage or was otherwise ineligible).
    if result.get("abstained"):
        clean_ids = [sid for sid, bad in snap_truth.items() if not bad]
        certifiable_clean = any(
            c["eligible"] for c in cand.get("candidates", [])
            if c["snapshot_id"] in clean_ids)
        result["abstention_correct"] = not certifiable_clean
    else:
        result["abstention_correct"] = None

    # 6) baseline selectors on the same evidence (newest sealed / integrity-only)
    sealed = sorted(snapshots, key=lambda s: s["captured_at"])
    result["baseline_newest"] = {"snapshot_id": sealed[-1]["id"],
                                 "contaminated": snap_truth[sealed[-1]["id"]]} if sealed else None
    def integ_ok(s):
        row = next(c for c in cand["candidates"] if c["snapshot_id"] == s["id"])
        return row["integrity_ok"]
    integ = [s for s in sealed if integ_ok(s)]
    result["baseline_integrity_newest"] = (
        {"snapshot_id": integ[-1]["id"], "contaminated": snap_truth[integ[-1]["id"]]}
        if integ else None)

    # 7) operator-confirmed restore (only for candidate_available unless forced)
    result["restore"] = None
    if do_restore and rec:
        target = ("%s-seed%d" % (scenario, seed)).replace("_", "-")[:40]
        st, rep = http_json("POST", api + "/api/v1/workloads/%s/restore" % wl["id"], token,
                            {"snapshot_id": rec, "target_name": target,
                             "confirm": True})
        result["restore"] = {"status": rep.get("status"), "http": st,
                             "verified_files": rep.get("verified_files"),
                             "failed_files": rep.get("failed_files"),
                             "source_unchanged": rep.get("source_unchanged"),
                             "output_dir": rep.get("output_dir")}
    return result


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--scenario", required=True)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--api", default=os.environ.get("CLEANPOINT_API", "http://127.0.0.1:8080"))
    ap.add_argument("--result", help="write result JSON here")
    ap.add_argument("--no-restore", action="store_true")
    ap.add_argument("-v", "--verbose", action="store_true")
    args = ap.parse_args()

    token = load_token(args.api)
    t0 = time.time()
    res = run_scenario(args.scenario, args.seed, args.api, token,
                       do_restore=not args.no_restore, verbose=args.verbose)
    res["duration_s"] = round(time.time() - t0, 1)
    print(json.dumps(res, indent=2))
    if args.result:
        os.makedirs(os.path.dirname(os.path.abspath(args.result)), exist_ok=True)
        json.dump(res, open(args.result, "w"), indent=2)


if __name__ == "__main__":
    main()
