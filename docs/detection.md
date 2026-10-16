# CleanPoint — Detection and Selection Design

This document defines every formula, threshold and convention used by the
`rules-v1` detector and the conservative recovery selector. All definitions
are implementation contracts: the automated tests pin them.

## 1. Windows

- Non-overlapping windows of `window_seconds` (default **300 s**), aligned to
  Unix-epoch multiples of the window size (deterministic across runs).
- Windows span the full event range including **empty** quiet windows (a
  quiet office is normal evidence).
- Anchoring means window boundaries never drift between analyses.

## 2. Features (per window)

| Feature | Definition | Weight |
|---|---|---|
| `modified_file_ratio` | distinct paths touched by create/modify/rename-destination ÷ files present at window start (min 1) | 0.25 |
| `rename_rate` | renames per minute (window length ÷ 60) | 0.20 |
| `entropy_shift_ratio` | eligible changed files with entropy increase ≥ **1.5 bits/byte** ÷ eligible changed files | 0.25 |
| `extension_novelty` | fraction of written extensions never seen in baseline windows | 0.15 |
| `delete_rate` | deletes per minute | 0.10 |
| `size_change_ratio` | modified files with ≥ **50 %** size change ÷ modified files with known sizes | 0.05 |
| `write_concentration` | fraction of writes by the busiest known actor | 0 (informational) |

Eligibility for the entropy feature: create/modify events carrying **both**
old and new entropy with new size ≥ **512 bytes** (sampling noise below that
is meaningless). Entropy itself is Shannon bits/byte over up to **4 × 4 KiB
chunks** (start, quarter, half, end), identical in the Go capture path and
the Python generator.

**Missing features are never imputed as evidence.** A feature without data is
flagged missing, contributes no signal, and lowers the window's coverage.
Weights are not redistributed.

## 3. Baseline

- The first `baseline_window_count` windows (default **24**) form the
  assumed-normal training period. This is a documented convention: the
  workload's early history is presumed benign. Scenarios violating it are
  caught by the baseline-contamination check (§5).
- Robust statistics per feature:
  - `center_i = median(normal_i)`
  - `scale_i = max(1.4826 × MAD(normal_i), epsilon_i)` with per-feature
    epsilon (0.01 for ratios, 0.5 for per-minute rates) so quiet baselines
    cannot produce zero scales.

## 4. Scoring (rules-v1)

```
z_i      = (current_i - center_i) / scale_i
signal_i = min(1, max(0, z_i) / 5)
risk     = Σ weight_i × signal_i        (over features with evidence only)
```

- `risk_index` ∈ [0, 1] is a **relative anomaly score, not a probability**.
- A window is `suspicious` when `risk_index >= threshold`
  (default **0.35**).
- The threshold was selected on the **validation seed split only** (seeds
  7–8; see `docs/evaluation.md`) and never on the locked corpus:
  normal/benign windows scored ≤ ~0.25 while attack windows scored
  ≥ ~0.45 across validation scenarios, placing 0.35 with margin on both
  sides.

## 5. Snapshot exposure classification

For each **sealed** snapshot, in priority order:

1. **Baseline contamination check** — if any baseline window itself contains
   a destructive burst (entropy-shift ≥ 0.5 on ≥ 30 % of files, or ≥ 20
   renames/min, or ≥ 10 deletes/min), certification is void: every snapshot
   is `unknown`. This catches contamination that predates the evaluated
   windows (the `pre_existing_contamination` family).
2. **Coverage** — all baseline windows must have ended before the snapshot
   was captured, and at least one evaluated window must exist. Otherwise
   `unknown`.
3. **Window evidence** — if any fully-ended suspicious window precedes the
   capture: `affected` when destructive signals (entropy shift, mass renames
   or deletes) fired, else `suspect`.
4. **Content check** — regardless of window telemetry: if ≥ **30 %** of the
   manifest's files use extensions absent from the earliest sealed snapshot
   AND those files' median entropy ≥ **7.0** bits/byte, the snapshot is
   `affected` (mass encryption-like rewrite is visible in the content
   itself).
5. Otherwise `clear`.

Benign archives stay far below the 30 % content threshold (a few `.7z`
files among many) and benign bulk updates keep their extensions and text
entropy, so both remain `clear`.

## 6. Onset

`incident_onset` = start time of the **earliest** suspicious evaluated window
(conservative lower bound). It is an *estimate with uncertainty*, never a
proven infection time.

## 7. Selector

Snapshots are inspected newest-first. A snapshot is **eligible** only when
ALL hold:

1. status `sealed`;
2. manifest + every object hash freshly verified (`integrity_ok`);
3. baseline coverage sufficient (`coverage_ok`);
4. exposure `clear`;
5. captured **strictly before** `incident_onset` (when an onset exists).

Ties break deterministically: newer capture time, then greater verified file
count, then larger snapshot id. If nothing qualifies, the selector returns
`no_safe_candidate` with `recommended_snapshot_id = null` and deduplicated
reasons. **It never falls back to the newest snapshot.**

`data_loss_minutes` (RPO proxy) = minutes between the candidate's capture
time and the onset (or the newest event time when no onset exists). It is a
recovery-gap estimate, not a business-loss measurement.

## 8. What would change the design

- Slow, low-and-slow attacks that stay under every window threshold would
  evade detection; the `slow_distributed` family pins the current floor.
- Actor attribution (`write_concentration`) is computed but unscored; wiring
  it in would need labeled multi-actor scenarios.
- The 30 %/7.0 content thresholds are calibrated to the synthetic families;
  real deployments would need validation against their own file mixes.
