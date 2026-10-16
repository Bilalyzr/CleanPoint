-- CleanPoint initial schema.
-- Conventions: all timestamps are UTC, layout '2006-01-02T15:04:05.000Z'
-- (fixed width => lexicographic order == chronological order). Sealed
-- snapshots are never mutated; assessment truth lives in separate tables.

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS workloads (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    root_path  TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS file_events (
    id          TEXT PRIMARY KEY,
    workload_id TEXT NOT NULL REFERENCES workloads(id) ON DELETE RESTRICT,
    event_time  TEXT NOT NULL,
    operation   TEXT NOT NULL CHECK (operation IN ('create','modify','delete','rename')),
    path        TEXT NOT NULL,
    dest_path   TEXT,
    old_sha256  TEXT,
    new_sha256  TEXT,
    old_entropy REAL,
    new_entropy REAL,
    old_size    INTEGER,
    new_size    INTEGER,
    actor       TEXT,
    scenario_id TEXT,
    ingested_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_file_events_wl_time ON file_events(workload_id, event_time);

CREATE TABLE IF NOT EXISTS snapshots (
    id            TEXT PRIMARY KEY,
    workload_id   TEXT NOT NULL REFERENCES workloads(id) ON DELETE RESTRICT,
    captured_at   TEXT NOT NULL,
    status        TEXT NOT NULL CHECK (status IN ('creating','sealed','invalid')),
    manifest_hash TEXT,
    file_count    INTEGER NOT NULL DEFAULT 0,
    total_bytes   INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snapshots_wl_time ON snapshots(workload_id, captured_at);

CREATE TABLE IF NOT EXISTS file_objects (
    sha256      TEXT PRIMARY KEY,
    size_bytes  INTEGER NOT NULL,
    storage_path TEXT NOT NULL,
    ref_count   INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS snapshot_files (
    snapshot_id   TEXT NOT NULL REFERENCES snapshots(id) ON DELETE RESTRICT,
    relative_path TEXT NOT NULL,
    sha256        TEXT NOT NULL REFERENCES file_objects(sha256) ON DELETE RESTRICT,
    size_bytes    INTEGER NOT NULL,
    entropy       REAL,
    PRIMARY KEY (snapshot_id, relative_path)
);

CREATE TABLE IF NOT EXISTS analysis_runs (
    id                   TEXT PRIMARY KEY,
    workload_id          TEXT NOT NULL REFERENCES workloads(id) ON DELETE RESTRICT,
    detector_version     TEXT NOT NULL,
    baseline_period      TEXT NOT NULL,
    window_seconds       INTEGER NOT NULL,
    baseline_window_count INTEGER NOT NULL,
    risk_threshold       REAL NOT NULL,
    status               TEXT NOT NULL CHECK (status IN ('running','completed','failed')),
    onset_time           TEXT,
    windows_evaluated    INTEGER NOT NULL DEFAULT 0,
    started_at           TEXT NOT NULL,
    finished_at          TEXT
);
CREATE INDEX IF NOT EXISTS idx_analysis_runs_wl ON analysis_runs(workload_id, started_at);

CREATE TABLE IF NOT EXISTS detection_windows (
    id              TEXT PRIMARY KEY,
    analysis_run_id TEXT NOT NULL REFERENCES analysis_runs(id) ON DELETE RESTRICT,
    workload_id     TEXT NOT NULL REFERENCES workloads(id) ON DELETE RESTRICT,
    starts_at       TEXT NOT NULL,
    ends_at         TEXT NOT NULL,
    feature_json    TEXT NOT NULL,
    risk_score      REAL NOT NULL,
    alert_level     TEXT NOT NULL CHECK (alert_level IN ('normal','suspicious')),
    explanation_json TEXT,
    is_baseline     INTEGER NOT NULL DEFAULT 0,
    event_count     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (analysis_run_id, starts_at)
);
CREATE INDEX IF NOT EXISTS idx_detection_windows_wl ON detection_windows(workload_id, starts_at);

CREATE TABLE IF NOT EXISTS snapshot_assessments (
    snapshot_id        TEXT PRIMARY KEY REFERENCES snapshots(id) ON DELETE RESTRICT,
    analysis_run_id    TEXT NOT NULL REFERENCES analysis_runs(id) ON DELETE RESTRICT,
    integrity_ok       INTEGER NOT NULL,
    coverage_ok        INTEGER NOT NULL,
    exposure_status    TEXT NOT NULL CHECK (exposure_status IN ('unknown','clear','suspect','affected')),
    risk_score         REAL NOT NULL,
    affected_file_count INTEGER NOT NULL DEFAULT 0,
    explanation_json   TEXT NOT NULL,
    evaluated_at       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS recovery_runs (
    id               TEXT PRIMARY KEY,
    workload_id      TEXT NOT NULL REFERENCES workloads(id) ON DELETE RESTRICT,
    snapshot_id      TEXT NOT NULL REFERENCES snapshots(id) ON DELETE RESTRICT,
    status           TEXT NOT NULL CHECK (status IN ('requested','verifying','verified','failed')),
    target_name      TEXT NOT NULL,
    output_path      TEXT NOT NULL,
    total_files      INTEGER NOT NULL DEFAULT 0,
    verified_files   INTEGER NOT NULL DEFAULT 0,
    failed_files     INTEGER NOT NULL DEFAULT 0,
    source_unchanged INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL,
    finished_at      TEXT,
    report_json      TEXT
);
CREATE INDEX IF NOT EXISTS idx_recovery_runs_wl ON recovery_runs(workload_id, created_at);

CREATE TABLE IF NOT EXISTS audit_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    actor      TEXT NOT NULL,
    action     TEXT NOT NULL,
    target_id  TEXT NOT NULL,
    outcome    TEXT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
