#!/usr/bin/env python3
"""Guided CleanPoint demonstration (the PRD section 18.4 narrative).

  1. Periodic snapshots with benign office activity.
  2. Harmless attack-like mass rewrite: show risk signals; high entropy alone
     is not proof.
  3. Newest / integrity-only selectors versus CleanPoint.
  4. Candidate exclusion, conservative onset, recovery gap.
  5. Operator-confirmed isolated restore of a strictly eligible snapshot.
  6. Per-file verification + source-unchanged proof.
  7. A case with no defensible candidate: CleanPoint refuses to recommend.

Requires a running server: cleanpoint serve
"""

import argparse
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

from scenario_driver import load_token, run_scenario  # noqa: E402


def hr(title):
    print()
    print("=" * 78)
    print(title)
    print("=" * 78)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", default=os.environ.get("CLEANPOINT_API", "http://127.0.0.1:8080"))
    args = ap.parse_args()
    token = load_token(args.api)

    hr("CLEANPOINT — local ransomware-recovery analysis demonstration")
    print("Defensive simulation prototype. No live malware. No production guarantees.")

    hr("SCENE 1 — normal office activity (baseline + periodic snapshots)")
    r1 = run_scenario("normal_office", 1, args.api, token, verbose=True)
    print(json.dumps({k: r1[k] for k in
                      ("selector_status", "recommended_snapshot_id", "suspicious_windows",
                       "onset", "restore")}, indent=2))
    print("-> Expected: no suspicious windows, no onset, newest snapshot eligible, restore verified.")

    hr("SCENE 2 — rapid destructive-like rewrite (harmless synthetic)")
    r2 = run_scenario("rapid_destructive", 1, args.api, token, verbose=True)
    print("detector onset (conservative):", r2["onset"], "| truth attack start:", r2["attack_start"])
    print("CleanPoint recommendation:", r2["selector_status"],
          r2["recommended_snapshot_id"] or "(abstained)")
    print("newest-snapshot baseline would pick:", r2["baseline_newest"])
    print("integrity-only baseline would pick:", r2["baseline_integrity_newest"])
    print("restore:", json.dumps(r2["restore"]))
    print("-> Expected: attack detected; newest snapshots excluded; an OLDER clean")
    print("   snapshot is recommended; both naive baselines would have restored")
    print("   contaminated data; every restored file verifies; source unchanged.")

    hr("SCENE 3 — pre-existing contamination (abstention case)")
    r3 = run_scenario("pre_existing_contamination", 1, args.api, token, verbose=True)
    print("selector status:", r3["selector_status"])
    print("reasons sample:", json.dumps(r3["candidates"][:2], indent=2))
    print("-> Expected: every snapshot shows encryption-like content; CleanPoint")
    print("   deliberately refuses to recommend any recovery point.")

    hr("SUMMARY")
    rows = [
        ("normal_office", r1),
        ("rapid_destructive", r2),
        ("pre_existing_contamination", r3),
    ]
    for name, r in rows:
        if r["selector_status"] == "candidate_available":
            verdict = "recommended %s contaminated=%s" % (
                r["recommended_snapshot_id"], r["recommended_contaminated"])
        else:
            verdict = "ABSTAINED (correct: %s)" % r.get("abstention_correct")
        print("%-28s %s" % (name, verdict))
    print()
    print("Truth labels were held separately and never exposed to the detector.")


if __name__ == "__main__":
    main()
