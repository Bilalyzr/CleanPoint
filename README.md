# CleanPoint

**A local, defensively designed ransomware-recovery analysis prototype.**
CleanPoint models a snapshot-protected filesystem, replays *safe synthetic*
file-activity scenarios, detects ransomware-like anomalies with an explainable
rules-based detector, recommends the **newest defensible recovery point** — or
explicitly refuses to recommend one — and performs an operator-confirmed,
verified restore into an isolated sandbox directory.

> ⚠️ **This is not a production data-protection product.** No live malware is
> used anywhere; "encrypted" content is seeded pseudo-random bytes. A valid
> hash proves integrity, **not** absence of compromise. See
> [docs/limitations.md](docs/limitations.md).

## Why it exists

After a destructive incident, the newest backup may already contain modified
or compromised data. File-extension matching and checksum validity alone
cannot prove recovery cleanliness. CleanPoint demonstrates a conservative,
explainable alternative: **safety before recency** — the selector may abstain
(`no_safe_candidate`) when no snapshot can be defended, and it never silently
falls back to the newest backup.

## Quick start (one command)

```bash
make build && make demo      # or: scripts/bootstrap.sh && scripts/demo.sh
```

### Landing page + web console

`http://127.0.0.1:8099/` serves a public landing page (what the project is,
how it works, measured results, live API status). The operator console lives
at **`http://127.0.0.1:8099/app/`** — after `demo.sh` (or `make serve`),
open it and paste the operator or read-only token from `sandbox/.tokens`. The console covers: overview
dashboard, workload detail (events / snapshot timeline / detection risk
chart / recovery candidates), the operator restore-approval flow with
per-file verification report, audit log and settings/limitations — with the
LOCAL SIMULATION badge and labelled status chips everywhere (never color
alone). It is the PRD's M1 "static assets served from Go" console (no build
toolchain); the React/TypeScript M2 console remains optional future work.

This starts the local API, then runs the guided three-scene demonstration:

1. **Normal office activity** — periodic snapshots, no anomalies, newest
   snapshot eligible, restore verified.
2. **Rapid destructive-like rewrite** — risk signals fire, post-onset
   snapshots are excluded *despite valid hashes*, an older clean point is
   recommended and restored with per-file hash proof; the naive
   newest/integrity-only selectors would both have restored contaminated
   data.
3. **Pre-existing contamination** — every snapshot is tainted before
   certification was possible: CleanPoint **abstains** and explains why.

## Repository layout

```
cmd/{server,cli,simulator}/       Go binaries
internal/{api,config,domain,storage,snapshots,detector,selector,restore,security}/
migrations/001_init.sql           SQLite schema (WAL, FKs)
simulator/                        deterministic Python generator + 8 scenario families
benchmarks/                       evaluation harness, latency/throughput probes, baselines
fixtures/                         generated deterministic event fixtures
tests/{integration,security,recovery}/   the mandated automated test matrix
api/openapi.yaml                  REST contract
docs/                             architecture, threat model, detection, evaluation, limitations
scripts/                          bootstrap / test / demo / benchmark
Dockerfile  compose.yaml  Makefile  go.mod  go.sum  requirements.txt
```

## The 8 scenario families

`normal_office` · `benign_archive` (high entropy, benign) ·
`benign_bulk_update` · `rapid_destructive` · `slow_distributed` ·
`extension_churn` · `mixed_activity` · `pre_existing_contamination` (abstain)

All are deterministic per `(family, seed)`: identical bytes, identical event
logs, identical outcomes. The generator reproduces any file tree state
phase-by-phase so snapshots always match the event log exactly.

## Detection in one paragraph

Events are bucketed into aligned 300-second windows. The first 24 windows
form an assumed-normal baseline; later windows are scored against robust
median/MAD statistics with fixed `rules-v1` weights (modified ratio 0.25,
rename rate 0.20, entropy-shift ratio 0.25, extension novelty 0.15, delete
rate 0.10, size-change ratio 0.05). A window is suspicious when its relative
risk index crosses the calibrated threshold (0.35). Snapshots are classified
`clear / suspect / affected / unknown` from window evidence **plus** two
conservative extras: a manifest content check (mass novel-extension
high-entropy files ⇒ affected) and a baseline contamination check
(destructive burst inside the training period ⇒ certification void). Full
formulas and every threshold: [docs/detection.md](docs/detection.md).

## Recovery selection rules

A snapshot is eligible only when it is **sealed**, **integrity-verified**
(manifest + every object re-hashed), **exposure clear**, **baseline
coverage sufficient**, and captured **strictly before** the earliest
plausible onset. Among eligible snapshots the newest wins (ties: greater
verified coverage, then id). If nothing qualifies:
`no_safe_candidate` with reasons — never a fallback to newest.

Restore requires the **operator token** plus `confirm: true`; the
destination is always a server-generated directory under
`sandbox/restored/`; every copied file is re-hashed before an atomic rename,
and the sealed source is re-verified afterwards to prove non-mutation.

## API & CLI

REST contract: [api/openapi.yaml](api/openapi.yaml) (base `/api/v1`,
operator/read-only bearer tokens, uniform error objects). Highlights:

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/workloads` | register sandbox workload (operator) |
| POST | `/workloads/{id}/events` | ingest idempotent event batches |
| POST | `/workloads/{id}/snapshots` | capture & seal snapshot |
| POST | `/workloads/{id}/analyze` | run rules-v1 detector |
| GET | `/workloads/{id}/recovery-candidates` | rank or abstain |
| POST | `/workloads/{id}/restore` | confirmed sandbox restore |
| GET | `/recovery-runs/{id}` | verification report |
| GET | `/healthz`, `/metrics` | health; restricted Prometheus metrics |

```bash
cleanpoint serve                                  # local API server
cleanpoint simulate --scenario rapid_destructive --seed 1
cleanpoint evaluate --scenario locked --seeds 9,10 --json --output results/eval-locked.json
cleanpoint benchmark snapshots --files 1000 --file-size-kb 32 --output results/snapshot-1000.json
cleanpoint benchmark restore  --files 1000 --file-size-kb 32 --output results/restore-1000.json
cleanpoint demo                                   # guided demonstration
```

Tokens: set `CLEANPOINT_OPERATOR_TOKEN`/`CLEANPOINT_READONLY_TOKEN`, or let
the server generate them once into `sandbox/.tokens` (never logged).

## Tests — 104 automated cases (minimum required: 92)

| Suite | Required | Shipped |
|---|---|---|
| Path & filesystem security | 12 | 14 |
| Snapshot integrity | 10 | 11 |
| Event ingestion | 8 | 10 |
| Feature extraction | 12 | 13 |
| Detection | 12 | 14 |
| Recovery eligibility | 12 | 13 |
| Restore correctness | 10 | 11 |
| API authentication | 8 | 11 |
| End-to-end scenarios | 8 | 8 |
| **Total** | **92** | **104** |

```bash
scripts/test.sh        # go vet + full suite + generator determinism check
```

## Measured results

All numbers below are real measurements produced by this repository on the
development machine (see `results/` for samples, seeds, hardware and
methodology). Regenerate with `make evaluate` / `make benchmarks`.
Nothing here is a production statistic.

<!-- MEASURED_RESULTS -->
### Locked-corpus evaluation (seeds 9–10, all 8 families, 2026-10-16)

`results/eval-locked.json` — machine `Luna`, Go 1.26.1, Python 3.11:

| Metric | Target | Measured |
|---|---|---|
| Unsafe snapshot selections | **0 (release-blocking)** | **0** / 14 selections |
| Recovery selection success | — | 8/8 (100 %) |
| Known-attack recall | ≥ 0.90 (dev target) | 8/8 (1.00) |
| Benign-window alert rate | ≤ 0.05 | 2/240 (0.0083) |
| Pre-existing-contamination abstention | must abstain | 2/2 correct |
| Verified restores | 100 % verified | 14/14 verified, source unchanged in all |
| Naive "newest" selector | (comparison) | contaminated in 10/16 scenarios |
| Naive "integrity-only" selector | (comparison) | contaminated in 10/16 scenarios |
| Detection delay | — | 2.1–4.8 min (window-quantized) |

### Performance benchmarks (1000 files × 32 KiB, 3 iterations)

| Operation | p50 | mean | Throughput |
|---|---|---|---|
| Snapshot capture (`results/snapshot-1000.json`) | 1,196 ms* | 6,593 ms | 151.7 files/s |
| Verified restore (`results/restore-1000.json`) | 10,451 ms | 10,736 ms | 93.1 files/s |
| GET recovery-candidates (`results/latency.json`) | ~11 ms | ~11 ms | — |
| GET recovery-candidates @ concurrency 20 (`results/throughput.json`) | — | — | 36.6 req/s |

\* first iteration includes object-store warm-up (31.25 MiB written once;
subsequent captures dedupe content-addressed objects). Restore re-verifies
every object hash before and after copying — the safety margin is the cost.

## Security posture (summary)

- Distinct operator / read-only bearer tokens; mutations are operator-only.
- Path validator rejects traversal, absolute, drive, device, symlink,
  backslash and null-byte inputs (`422 unsafe_path`); denial paths leave
  outside directories untouched.
- Sealed snapshots have **no** update/delete API (logical immutability —
  not WORM).
- Restores only under server-generated sandbox directories; source
  non-mutation proven after every run.
- Ground truth lives only in scenario folders; the detector cannot read it.
- Full analysis: [docs/threat-model.md](docs/threat-model.md).

## Documentation

- [Architecture](docs/architecture.md) — components, boundaries, topology
- [Detection design](docs/detection.md) — every formula and threshold
- [Evaluation](docs/evaluation.md) — splits, metrics, measured results
- [Threat model](docs/threat-model.md) — in/out of scope, mitigations
- [Limitations](docs/limitations.md) — honest positioning

## Status

Phase **M0** (14-day MVP) complete: local Go + SQLite core, Python
simulation/evaluation, ≥92-case test matrix, benchmarks, one-command demo.
**M1 shipped**: the embedded operator web console (static assets served from
Go, same-origin, token-gated) — M0 acceptance had already passed (0 unsafe
selections on the locked corpus). The richer React/TypeScript M2 console
(investigation timeline, scenario control) remains optional future work.
`web/` holds the console sources (`index.html`, `app.js`, `styles.css`,
embedded via `go:embed`).
