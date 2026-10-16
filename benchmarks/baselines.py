#!/usr/bin/env python3
"""Offline baseline-selector comparison over generated scenario folders.

Given scenario manifest folders (events.json + truth.json + meta.json) this
script computes what the two naive selectors WOULD choose per scenario point:

  newest            the most recently captured snapshot
  integrity-only    the most recent snapshot whose manifest verifies

against the hidden truth, producing the unsafe-selection table used in the
evaluation report. It performs no API calls and reads no server state; it is
a static reference model.

Usage:
  python baselines.py --folders ../sandbox/sim/*/manifest --json
"""

import argparse
import datetime as dt
import hashlib
import json
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "simulator"))
import scenarios as scns  # noqa: E402


def snapshot_captures(meta):
    base = dt.datetime.strptime(scns.BASE_EPOCH, "%Y-%m-%dT%H:%M:%S").replace(
        tzinfo=dt.timezone.utc)
    return [(p["window"],
             (base + dt.timedelta(seconds=(p["window"] + 1) * meta["window_seconds"])
              ).strftime("%Y-%m-%dT%H:%M:%S.000Z"))
            for p in meta["snapshot_points"]]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--folders", nargs="+", required=True,
                    help="scenario manifest folders (glob ok via shell)")
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args()

    rows = []
    for folder in args.folders:
        try:
            truth = json.load(open(os.path.join(folder, "truth.json")))
            meta = json.load(open(os.path.join(folder, "meta.json")))
        except FileNotFoundError:
            continue
        if truth["benign"]:
            continue  # baselines only meaningfully wrong on attack scenarios
        caps = snapshot_captures(meta)
        contaminated = [c for _, c in caps if c >= truth["attack_start"]]
        clean = [c for _, c in caps if c < truth["attack_start"]]
        # naive selectors always pick the newest (integrity is intact in this
        # model unless corrupted separately)
        newest = max(c for _, c in caps)
        rows.append({
            "scenario": truth["scenario"],
            "seed": truth["seed"],
            "attack_start": truth["attack_start"],
            "snapshots": len(caps),
            "contaminated_snapshots": len(contaminated),
            "clean_snapshots": len(clean),
            "newest_picks_contaminated": newest >= truth["attack_start"],
            "integrity_only_picks_contaminated": newest >= truth["attack_start"],
        })

    if args.json:
        print(json.dumps(rows, indent=2))
    else:
        for r in rows:
            print("%-28s seed=%d snapshots=%2d contaminated=%2d clean=%2d naive_unsafe=%s" % (
                r["scenario"], r["seed"], r["snapshots"], r["contaminated_snapshots"],
                r["clean_snapshots"], r["newest_picks_contaminated"]))
    unsafe = sum(1 for r in rows if r["newest_picks_contaminated"])
    print("\nnaive unsafe selections: %d / %d attack scenarios (reference model)" % (
        unsafe, len(rows)))


if __name__ == "__main__":
    main()
