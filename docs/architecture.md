# CleanPoint — Architecture

CleanPoint is a **local, defensively designed ransomware-recovery analysis
prototype**. It models a filesystem protected by periodic snapshots, ingests
safe synthetic file events, detects ransomware-like anomalies, recommends the
latest *defensible* recovery point (or explicitly abstains), and performs an
operator-confirmed, verified restore into an isolated sandbox directory.

It is **not** a production data-protection product. No live malware is ever
used; all "encrypted" content is seeded pseudo-random bytes.

## Logical diagram

```
Python seeded scenario generator (simulator/)
        |  mock files + deterministic event log + hidden truth
        v
Go HTTP API (cmd/server, internal/api)  ── bearer-token auth (operator / read-only)
        |
   SQLite (WAL, foreign_keys=ON)
   workloads / file_events / snapshots / file_objects / snapshot_files
   analysis_runs / detection_windows / snapshot_assessments
   recovery_runs / audit_events
        |
   event ingestion ──> snapshot capture ──> feature extraction ──> detection (rules-v1)
        |                                                            |
   content-addressed SHA-256 object store <── manifest verification  |
        |__________________________________________________________-> recovery selector
                                                                       |  explicit approval
                                                                       |  OR no_safe_candidate
                                                                       v
                                                         isolated restore verifier
                                                                       |
                                                         report + hidden-truth evaluator
```

## Component responsibilities

| Module | Input | Output / invariant |
|---|---|---|
| `simulator/generate.py` | seed + scenario profile | Harmless mock files, deterministic event log, hidden truth |
| `simulator/scenario_driver.py` | manifest + API | Phase-by-phase ingestion, snapshots, analysis, restore, verdicts |
| `internal/api` | HTTP requests | Validated, authenticated, audited calls; uniform error objects |
| `internal/snapshots` | Workload fileset | Content-addressed objects, canonical manifest, sealed snapshot |
| `internal/detector` | Events + snapshot metadata | Windows, features, risk score, explanations, exposure |
| `internal/selector` | Assessments + integrity | Ranked eligible candidates OR abstention |
| `internal/restore` | Approved candidate | Isolated output, per-file hash proof, source-unchanged evidence |
| `benchmarks/evaluate.py` | Hidden truth + API results | Measured recall, unsafe-selection and benign-alert metrics |

## Trust and dependency boundaries

- **API clients are untrusted.** Tokens, ids, timestamps, paths and sizes are
  validated at the boundary; unsafe paths yield `422 unsafe_path`.
- **Ground truth is isolated.** `truth.json` exists only in scenario folders
  and is read exclusively by the evaluation harness; the detector never sees
  it (verified by test `e2e` scenarios and code inspection).
- **Objects are logically immutable via API policy** — no update/delete
  endpoint exists for sealed content. This is not WORM storage; a privileged
  local administrator is out of scope.
- **Restore destinations are always server-generated** under
  `<sandbox>/restored/`, disconnected from the protected roots.
- **SQLite + object writes are transactional/atomic**: objects are written to
  temp files and renamed; snapshots only become `sealed` after all objects and
  the manifest verification succeed.

## Deployment topology (M0)

One local process (`cmd/server`), one SQLite database, restricted local
directories for objects and restores, Python 3 standard-library scripts for
simulation and benchmarking. The API binds to loopback by default; exposing
it further is explicitly unsupported. Optional `compose.yaml` wraps the same
single process — no broker, model server or cluster.

## Detection pipeline (short form)

1. Events are bucketed into aligned 300-second non-overlapping windows.
2. The first `BASELINE_WINDOWS` (default 24) windows form the assumed-normal
   training period (a documented convention; see `docs/detection.md`).
3. Each later window is scored against a robust median/MAD baseline with the
   fixed `rules-v1` weights; a window is suspicious when
   `risk_index >= threshold` (0.35, calibrated on the validation split).
4. Snapshots are classified `clear / suspect / affected / unknown`, using
   window evidence **plus** a manifest content check and a baseline
   contamination check.
5. The selector walks snapshots newest-first and recommends the newest one
   that is sealed, integrity-verified, exposure-clear and captured strictly
   before the earliest plausible onset — or abstains.

See `docs/detection.md` for every formula, threshold and its rationale.
