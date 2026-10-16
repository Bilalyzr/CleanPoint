# CleanPoint — Evaluation Design and Measured Results

## Split discipline

Scenario families × seeds are grouped into three disjoint splits
(`simulator/seeds.json`), never interleaved windows from one incident:

| Split | Seeds | Purpose |
|---|---|---|
| development | 1–6 | building, debugging, threshold exploration |
| validation | 7–8 | threshold calibration (the shipped 0.35 default) |
| locked | 9–10 | final release-blocking measurement, untouched during tuning |

Hidden truth (`truth.json`: attack start/end, compromised paths) lives only
in scenario folders and is read exclusively by `benchmarks/evaluate.py`.

## Selector comparison

Three selectors run on identical evidence:

- **newest** — newest sealed snapshot, ignoring all evidence;
- **integrity-only** — newest integrity-valid snapshot (hashes OK is enough);
- **CleanPoint** — this system (may abstain).

Results are reported separately per metric; they are never merged into a
single accuracy figure.

## Metrics

- `unsafe_selection_rate` — contaminated recommendations ÷ scenarios with a
  selection. **Release-blocking target: 0 on the locked corpus.**
- `selection_success` — newest-eligible-clean candidate chosen ÷ attack
  scenarios where a clean candidate exists.
- `known_attack_recall` — detected onsets ÷ attack scenarios (development
  target ≥ 0.90).
- `benign_alert_rate` — flagged windows ÷ benign windows (held-out target
  ≤ 0.05).
- `abstention correctness` on `pre_existing_contamination` (all snapshots
  contaminated ⇒ must abstain; a clean-but-uncertifiable snapshot does not
  make abstention wrong).
- `restore_integrity` — every performed restore verified with source
  unchanged.
- `detection_delay_minutes` — estimated onset minus true attack start.

## Measured results (this repository, 2026-10-16)

Machine: `Luna` (local Windows 11 dev box), Go 1.26.1, Python 3.11. Command:

```
cleanpoint evaluate --scenario locked --seeds 9,10 --json --output results/eval-locked.json
```

Locked corpus (16 scenarios = 8 families × seeds {9, 10}):

| Metric | Target | Measured |
|---|---|---|
| unsafe_selection_rate | **0 (release-blocking)** | **0.0** (0 / 14 selections) |
| selection_success_rate | — | 1.00 (8/8) |
| known_attack_recall | ≥ 0.90 development target | 1.00 (8/8) |
| benign_alert_rate | ≤ 0.05 held-out target | 0.0083 (2 / 240 benign windows) |
| pre_existing abstention correct | must abstain | 2 / 2 |
| restore_integrity | 100 % verified | 14 / 14 verified, all sources unchanged |
| naive "newest" unsafe | comparison | 10 / 16 scenarios |
| naive "integrity-only" unsafe | comparison | 10 / 16 scenarios |
| detection_delay_minutes | — | [2.12, 2.23, 2.37, 3.32, 3.45, 4.08, 4.43, 4.80] |

Threshold calibration evidence (validation seeds 7–8, `results/eval-validation.json`):
at threshold 0.35 the benign-window alert rate was 0.42 % (1 of 240 benign
windows flagged; the highest-scoring benign window reached 0.40, from the
bulk-update burst families) and known-attack recall was 100 % (8/8) with 0
unsafe selections — both inside the acceptance targets (≤ 5 % benign alerts,
0 unsafe). The threshold was frozen after this run and before the locked
corpus was executed. Margin note: benign bursts can score up to ~0.40, so a
threshold above that would flag fewer benign bursts but risks recall; 0.35
kept recall perfect at the cost of a ~0.4–0.8 % benign flag rate.

Performance (`make benchmarks`, 1000 files × 32 KiB, 3 iterations):

- Snapshot capture: p50 1,196 ms / mean 6,593 ms; 151.7 files/s; 4.74 MB/s
  (first iteration includes the 31.25 MiB object-store write; later captures
  dedupe content-addressed objects).
- Verified restore: p50 10,451 ms / mean 10,736 ms; 93.1 files/s; 2.91 MB/s
  (includes full pre- and post-restore hash verification).
- GET /recovery-candidates: p50 ≈ 11 ms, p95 ≈ 28 ms (20 samples).
- Throughput of GET /recovery-candidates: 6.8 / 27.4 / 33.8 / 36.6 req/s at
  concurrency 1 / 5 / 10 / 20 (5 s windows, single-process sync server).

*(Regenerate with `make evaluate` after any detector change; treat the
checked-in files as the release evidence. Never tune on the locked split.)*

## Benchmark methodology

- `cleanpoint benchmark snapshots|restore --files N --file-size-kb K
  --iterations I --output results/<name>.json`: registers a disposable
  workload, writes N mock files, ingests telemetry, then times API snapshot
  capture (and, for `restore`, the verified restore of that snapshot).
  Reports p50/p95/mean, throughput MB/s and files/s, Go version, host and
  timestamp. Samples and hardware accompany every number.
- `benchmarks/latency.py` — p50/p95 for candidate selection, findings,
  capture and restore on an existing workload.
- `benchmarks/throughput.sh` — GET `/recovery-candidates` request rate at
  concurrency 1/5/10/20.

## Known limitations of the evaluation

- The corpus is **synthetic**; results are corpus-specific evidence, not
  production statistics.
- Detection delay is window-quantized (±5 minutes).
- Scenario families were designed alongside the detector; an independent
  corpus would be stronger evidence.
