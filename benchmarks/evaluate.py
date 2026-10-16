#!/usr/bin/env python3
"""CleanPoint evaluation harness.

Runs scenario families against a live API across the seed split
(development / validation / locked, from simulator/seeds.json), computes the
PRD metrics, and compares three selectors:

  newest           newest sealed snapshot (ignores evidence)
  integrity-only   newest integrity-valid snapshot
  cleanpoint       this system (may abstain)

Metrics (never merged into a single accuracy figure):
  unsafe_selection_rate   contaminated recommendations / scenarios with a selection
  selection_success       newest-eligible-clean chosen / scenarios where one exists
  benign_alert_rate       flagged benign windows / benign windows
  abstention_rate         abstentions / scenarios
  detection_delay_min     onset - true attack start (minutes, attack scenarios)

Ground truth lives in each scenario folder's truth.json and is used ONLY by
this harness, never by the detector.

Usage:
  python evaluate.py --api http://127.0.0.1:8080 --scenario all --json --output results/eval.json
"""

import argparse
import csv
import datetime as dt
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
sys.path.insert(0, os.path.join(ROOT, "simulator"))

from scenario_driver import load_token, run_scenario  # noqa: E402

FAMILIES = [
    "normal_office", "benign_archive", "benign_bulk_update",
    "rapid_destructive", "slow_distributed", "extension_churn",
    "mixed_activity", "pre_existing_contamination",
]


def parse_ts(s):
    return dt.datetime.strptime(s, "%Y-%m-%dT%H:%M:%S.%fZ").replace(tzinfo=dt.timezone.utc) \
        if s else None


def evaluate_split(name, families, seeds, api, token, do_restore=True):
    results = []
    for fam in families:
        for seed in seeds:
            r = run_scenario(fam, seed, api, token, do_restore=do_restore)
            results.append(r)
            rec_bad = r.get("recommended_contaminated")
            print("  [%s] %-28s seed=%-2d status=%-18s rec_contaminated=%s abstain_ok=%s" % (
                name, fam, seed, r["selector_status"], rec_bad, r.get("abstention_correct")))
    return results


def metrics(results):
    m = {}
    with_selection = [r for r in results if r["selector_status"] == "candidate_available"]
    contaminated_selections = [r for r in with_selection if r["recommended_contaminated"]]
    m["scenarios"] = len(results)
    m["scenarios_with_selection"] = len(with_selection)
    m["unsafe_selections"] = len(contaminated_selections)
    m["unsafe_selection_rate"] = round(len(contaminated_selections) / len(with_selection), 4) \
        if with_selection else None

    # selection success: attack scenarios where a clean snapshot exists
    success_den, success_num = 0, 0
    for r in results:
        if r["benign"]:
            continue
        clean = [sid for sid, bad in r["snapshot_truth"].items() if not bad]
        if r["selector_status"] == "candidate_available" and clean:
            success_den += 1
            if r.get("selection_is_newest_clean"):
                success_num += 1
    m["selection_success_num"] = success_num
    m["selection_success_den"] = success_den
    m["selection_success_rate"] = round(success_num / success_den, 4) if success_den else None

    # benign alert rate over benign windows: benign scenarios only
    benign = [r for r in results if r["benign"]]
    total_windows = sum(r["total_windows"] for r in benign) or 1
    flagged = sum(r["suspicious_windows"] for r in benign)
    m["benign_windows"] = total_windows
    m["benign_flagged_windows"] = flagged
    m["benign_alert_rate"] = round(flagged / total_windows, 4)

    # known-attack recall: attack scenarios where onset was detected
    attack = [r for r in results if not r["benign"] and r["scenario"] != "pre_existing_contamination"]
    detected = [r for r in attack if r.get("onset")]
    m["attack_scenarios"] = len(attack)
    m["attack_detected"] = len(detected)
    m["known_attack_recall"] = round(len(detected) / len(attack), 4) if attack else None

    delays = []
    for r in detected:
        on, at = parse_ts(r["onset"]), parse_ts(r["attack_start"])
        if on and at:
            delays.append(round((at - on).total_seconds() / 60, 2))
    m["detection_delay_minutes"] = delays

    # abstention correctness on the pre-existing family
    pre = [r for r in results if r["scenario"] == "pre_existing_contamination"]
    m["pre_existing_abstained"] = sum(1 for r in pre if r["selector_status"] == "no_safe_candidate")
    m["pre_existing_total"] = len(pre)
    m["pre_existing_abstention_correct"] = sum(
        1 for r in pre if r.get("abstention_correct")) if pre else None

    # baseline selector comparison over the same runs
    base_unsafe = {"newest": 0, "integrity_only": 0}
    base_den = 0
    for r in results:
        if r["selector_status"] == "candidate_available" or r.get("baseline_newest"):
            base_den += 1
            if r.get("baseline_newest") and r["baseline_newest"]["contaminated"]:
                base_unsafe["newest"] += 1
            if r.get("baseline_integrity_newest") and r["baseline_integrity_newest"]["contaminated"]:
                base_unsafe["integrity_only"] += 1
    m["baseline_selector_unsafe"] = base_unsafe
    m["baseline_selector_den"] = base_den

    # restore integrity across every performed restore
    restores = [r["restore"] for r in results if r.get("restore")]
    verified = [x for x in restores if x and x.get("status") == "verified"]
    m["restores_performed"] = len(restores)
    m["restores_verified"] = len(verified)
    m["restore_integrity_ok"] = (len(verified) == len(restores)) if restores else None
    m["all_restores_source_unchanged"] = all(
        x.get("source_unchanged") for x in restores) if restores else None
    return m


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--api", default=os.environ.get("CLEANPOINT_API", "http://127.0.0.1:8080"))
    ap.add_argument("--scenario", default="all",
                    help="family name, 'all', or a split name (development/validation/locked)")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--output", help="write JSON report to this path")
    ap.add_argument("--seeds", help="comma-separated seed override")
    ap.add_argument("--no-restore", action="store_true")
    args = ap.parse_args()

    token = load_token(args.api)
    split = json.load(open(os.path.join(ROOT, "simulator", "seeds.json")))

    if args.scenario == "all":
        families = FAMILIES
    elif args.scenario in ("development", "validation", "locked"):
        families = FAMILIES
        seeds = split[args.scenario]
        args._seeds = seeds
    else:
        families = [args.scenario]

    if args.seeds:
        seeds = [int(s) for s in args.seeds.split(",")]
    elif args.scenario in ("development", "validation", "locked"):
        seeds = split[args.scenario]
    else:
        seeds = split["development"]

    print("CleanPoint evaluation — families=%s seeds=%s" % (len(families), seeds))
    results = evaluate_split("eval", families, seeds, args.api, token,
                             do_restore=not args.no_restore)
    m = metrics(results)

    m["methodology"] = {
        "splits": split,
        "families": families,
        "seeds_used": seeds,
        "note": "Synthetic scenarios only. Metrics are measured on this corpus, "
                "not production statistics. Baseline selectors and CleanPoint are "
                "reported separately and never merged into one accuracy figure.",
    }
    if args.json:
        print(json.dumps(m, indent=2))
    else:
        for k, v in m.items():
            if k != "methodology":
                print("%-32s %s" % (k, v))
    if args.output:
        os.makedirs(os.path.dirname(os.path.abspath(args.output)), exist_ok=True)
        with open(args.output, "w") as f:
            json.dump({"metrics": m, "results": [
                {k: v for k, v in r.items() if k != "candidates"} for r in results]}, f, indent=2)
        csv_path = os.path.splitext(args.output)[0] + ".csv"
        with open(csv_path, "w", newline="") as f:
            w = csv.writer(f)
            w.writerow(["scenario", "seed", "benign", "selector_status",
                        "recommended_contaminated", "abstention_correct",
                        "selection_is_newest_clean", "onset", "attack_start"])
            for r in results:
                w.writerow([r["scenario"], r["seed"], r["benign"], r["selector_status"],
                            r.get("recommended_contaminated"), r.get("abstention_correct"),
                            r.get("selection_is_newest_clean"), r.get("onset"), r.get("attack_start")])
        print("report: %s (+ %s)" % (args.output, csv_path))


if __name__ == "__main__":
    main()
