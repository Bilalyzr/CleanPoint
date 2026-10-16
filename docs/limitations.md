# CleanPoint — Limitations and Honest Positioning

**CleanPoint is a local, defensively designed ransomware-recovery analysis
prototype built for a defensive security portfolio. It is not a production
data-protection product and must not be presented as one.**

## What it is

- A simulator: mock files, synthetic events, seeded "encryption-like" bytes
  (uniform random data — never real malware).
- An analysis tool: explainable anomaly evidence, conservative snapshot
  eligibility, verified isolated restores.
- A demonstration that "newest backup with valid hashes" is an unsafe
  recovery policy when contamination can predate detection.

## What it is NOT

- No live malware, no malware analysis, no exploit development.
- No production backup integration, no host restoration, no automatic
  rollback of real systems.
- No enterprise security controls: static local bearer tokens, no TLS, no
  rotation, no multi-tenancy; the API binds to loopback by design.
- Not WORM storage: sealed snapshots are logically immutable via API policy
  only; a local administrator can edit SQLite or the object store.
- Not a guarantee: integrity proves consistency, **not** absence of
  compromise. A valid SHA-256 means bytes are unchanged, not that they are
  clean.

## Detection honesty

- `risk_index` is a relative anomaly score in [0, 1], not a probability.
- The detector sees only ingested events and snapshot manifests. It never
  reads the hidden evaluation truth.
- `incident_onset` is a conservative lower bound (the earliest suspicious
  window start), not a proven infection time.
- Behaviorally "affected files" are metadata-supported flags, not proven
  ransomware encryption.
- Recall/precision figures are measured on the synthetic corpus only.

## Recovery honesty

- The selector may abstain (`no_safe_candidate`). That is a feature: when no
  snapshot can be defended, refusing to recommend is the correct answer.
- Restores go only to server-generated directories under the local sandbox
  and are verified file-by-file; the sealed source is re-verified to prove
  non-mutation.
- Resource caps (5,000 files / 100 MiB per workload) are synchronous MVP
  limits, not scaling claims.

## Benchmark honesty

- Every performance number in `results/` is an actual measurement with
  sample counts, host and Go version attached.
- Hardware-specific; re-run before citing.

## If you extend it

- Keep the truth files out of `internal/`.
- Never relax the path validator "just for the demo".
- Keep the UI read-only until the core is accepted (phase discipline).
- Re-run `make evaluate` after any detector change; never tune on the locked
  split.
