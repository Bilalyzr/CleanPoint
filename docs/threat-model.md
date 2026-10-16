# CleanPoint — Threat Model and Security Architecture

CleanPoint is a defensive cyber-resilience analysis simulator. This document
defines what the system defends against, what it explicitly does not, and how
each in-scope threat is mitigated.

## Safety first

- **No live malware.** The simulator produces harmless seeded pseudo-random
  bytes to mimic encryption-like content. Nothing is executed, packaged,
  downloaded or analyzed from real malware samples.
- **No production restores.** Restores copy synthetic mock files into
  server-generated directories. CleanPoint never touches a real production
  backup system or performs host-level restoration.

## In scope (and mitigations)

| Threat | Mitigation | Verified by |
|---|---|---|
| Path traversal (`../`, nested) | Relative-path validator rejects `..`, `.`, empty components; workload roots must resolve inside the allowlisted `<sandbox>/roots` | security suite cases 01–02, 10 |
| Absolute / drive / UNC paths | Rejected at validation (leading `/`, `X:`) | cases 03–04 |
| Backslash smuggling (Windows separators) | Backslashes rejected outright | case 05 |
| Reserved device names (`CON`, `NUL`, `COM1..9`) | Rejected per component | case 06 |
| Symlink escape (T02) | Snapshot capture walks with `WalkDir`, rejects any non-regular file; registration checks components with `Lstat`; restore writes via validated joins; denial leaves the target untouched | case 11 |
| Unauthorized mutation | Distinct operator/read-only bearer tokens; every state-changing route requires the operator role (403 otherwise) | API auth suite |
| Corrupted snapshots (T03/T04) | Manifest hash recomputed from persisted rows; every object re-hashed during analysis and again immediately before restore | snapshot integrity suite |
| Internally valid but contaminated snapshot (T05) | Behavioral onset gate + content-level check + baseline contamination check; a valid hash alone never implies clean | e2e scenarios |
| Accidental source overwrite | Restore reads objects, writes only into `<sandbox>/restored/<run>`; post-restore verification proves the sealed source unchanged | restore suite |
| No clean snapshot (T06) | Selector returns `no_safe_candidate` with reasons; UI must not fall back to newest | e2e pre-existing scenario |
| Benign high entropy (T07) | Entropy shift must be paired with mass modification; benign archives stay under content fraction thresholds | e2e benign archive |
| Missing telemetry (T09) | Features without evidence are marked missing (coverage drops); insufficient baseline yields `unknown` and abstention | detection suite |
| Analysis/list concurrency (T10) | Single-writer SQLite; a heavy-operation gate serializes capture/restore (409 `busy`) | design + integration tests |
| Token leakage | Tokens are never logged; audit events record actor roles, actions and outcomes only | code inspection |
| Hidden-truth contamination of the detector | Truth files live only in scenario folders, consumed exclusively by the evaluation harness | repo layout + e2e tests |

## Out of scope

- A privileged local administrator tampering with the database, object store
  or binary (all trust anchors are local).
- Real ransomware prevention, detection guarantees beyond the synthetic
  corpus, or disaster-recovery assurance.
- Malware that produces no observable file-activity change.
- Multi-tenant or network-exposed deployment (the API binds to loopback;
  TLS/CORS/network policy are intentionally absent).

## Honest limitations of the security architecture

- Logical immutability of sealed snapshots is API policy, not WORM storage.
- Bearer tokens are static local credentials; no rotation, MFA or rate
  limiting is provided.
- The audit log records actions but cannot defend against local DB edits.
- `422`/`400` distinctions follow the documented convention; nothing here is
  a compliance certification.
