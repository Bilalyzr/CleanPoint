"""CleanPoint scenario families (deterministic, standard library only).

Eight mandatory synthetic families from the source specification:

  normal_office            benign; steady low-level activity
  benign_archive           benign; a few high-entropy archives appear
  benign_bulk_update       benign; large but ordinary software update burst
  rapid_destructive        attack-like; fast mass encryption/rewrite
  slow_distributed         attack-like; cumulative low-rate changes
  extension_churn          attack-like; mass renames to random extensions
  mixed_activity           both benign burst and destructive activity
  pre_existing_contamination  contamination predates the first snapshot (abstain)

Everything here is harmless synthetic data. No real malware is ever used.

Timeline model: N windows of 300 simulated seconds. The first baseline_windows
windows form the assumed-normal training period. Snapshot points are chosen
every snapshot_every windows. Attack windows are per-family.
"""

BASE_EPOCH = "2026-01-05T09:00:00"  # a Monday morning, UTC
WINDOW_SECONDS = 300
TOTAL_WINDOWS = 40
SNAPSHOT_EVERY = 3  # snapshot every 3 windows = 15 minutes
BASELINE_WINDOWS = 24
FILES = 120

# family -> parameters
FAMILIES = {
    "normal_office": {
        "benign": True,
        "attack_windows": [],
    },
    "benign_archive": {
        "benign": True,
        "attack_windows": [28],
        "archive_files": 15,  # 15/120 = 12.5% novel-ext high-entropy files
    },
    "benign_bulk_update": {
        "benign": True,
        "attack_windows": [28, 29],
        "bulk_fraction": 0.60,
    },
    "rapid_destructive": {
        "benign": False,
        "attack_windows": [27],
        "rewrite_fraction": 0.90,
        "novel_ext": "lockd",
        "inflate": 1.35,
    },
    "slow_distributed": {
        "benign": False,
        "attack_windows": list(range(26, 39)),
        "per_window_fraction": 0.10,
        "novel_ext": "crypt",
    },
    "extension_churn": {
        "benign": False,
        "attack_windows": list(range(27, 34)),
        "rename_fraction": 0.30,
    },
    "mixed_activity": {
        "benign": False,
        "attack_windows": [30],
        "benign_burst_windows": [27, 28],
        "rewrite_fraction": 0.50,
        "novel_ext": "lockd",
        "inflate": 1.3,
    },
    "pre_existing_contamination": {
        "benign": False,
        # mass rewrite in window 0: contamination predates EVERY snapshot
        # point (first capture is window 3). Normal activity continues on
        # the encrypted-like files; the destructive burst sits INSIDE the
        # would-be normal baseline, so the detector must void certification.
        "attack_windows": [0],
        "rewrite_fraction": 0.90,
        "novel_ext": "locked",
        "inflate": 1.4,
    },
}


def snapshot_points():
    """Window indices at which the driver captures snapshots.

    The first capture is at window 3 so every scenario has a little live
    history behind its initial snapshot.
    """
    return list(range(3, TOTAL_WINDOWS, SNAPSHOT_EVERY))


def family(name):
    if name not in FAMILIES:
        raise SystemExit(
            "unknown scenario %r; choose one of: %s" % (name, ", ".join(sorted(FAMILIES)))
        )
    return FAMILIES[name]
