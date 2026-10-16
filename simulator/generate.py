#!/usr/bin/env python3
"""CleanPoint deterministic scenario generator (standard library only).

Two modes:

  manifest mode (default)
      Writes <out>/events.json, <out>/truth.json, <out>/meta.json describing
      the complete simulated timeline for a (scenario, seed) pair. Hidden
      ground truth lives ONLY in truth.json and is never sent to the API.

  apply mode (--root DIR --through IDX)
      Rebuilds the mock file tree deterministically to the state right after
      event index IDX (inclusive) under DIR. Identical (scenario, seed, IDX)
      always produces identical bytes, so phase-by-phase snapshots match the
      event log exactly.

Content model: every file version's bytes derive from a stable hash of
(seed, origin_path, version, kind), where origin_path tracks renames so a
renamed file keeps byte-identical content. No malware is ever involved; all
"encrypted" content is uniform random bytes from a seeded PRNG.
"""

import argparse
import datetime as dt
import hashlib
import json
import math
import os
import random
import shutil
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import scenarios  # noqa: E402

WORDS = (
    "report budget invoice client meeting draft memo policy vendor audit "
    "quarter summary index asset license backup server network printer "
    "payroll hires schedule ticket cache config roster shift plan target "
).split()

TEXT_EXTS = ["txt", "md", "csv", "log", "py", "json", "html", "cfg", "xml", "ini"]
BIN_EXTS = ["jpg", "png", "pdf"]
DOC_NAMES = [
    "notes", "summary", "invoice", "report", "memo", "plan", "roster",
    "budget", "audit", "draft", "contract", "agenda", "ledger", "spec",
]


def stable_seed(*parts):
    h = hashlib.sha256("|".join(str(p) for p in parts).encode()).digest()
    return int.from_bytes(h[:8], "big")


def content_bytes(seed, origin, version, kind, size):
    """Deterministic bytes for one file version.

    kind "text" produces word-like content; every other kind ("bin",
    "encrypted") produces the SAME opaque uniform layout from one PRNG
    stream, so replay never needs to disambiguate the two from entropy.
    """
    rng = random.Random(stable_seed(seed, origin, version, "opaque" if kind != "text" else "text"))
    if kind == "text":
        buf = []
        total = 0
        while total < size:
            buf.append(rng.choice(WORDS))
            total += len(rng.choice(WORDS)) + 1
        data = " ".join(buf).encode()[:size]
    else:
        chunk = bytes(rng.getrandbits(8) for _ in range(min(size, 8192)))
        data = (chunk * (size // max(1, len(chunk)) + 1))[:size]
    return data


def entropy_of(data):
    """Shannon entropy over up to 4 x 4KiB chunks (mirrors the Go sampler)."""
    n = len(data)
    if n == 0:
        return 0.0
    c = 4096
    if n <= c:
        chunks = [data]
    elif n <= 4 * c:
        chunks = [data[:c]]
    else:
        chunks = [data[:c], data[n // 4:n // 4 + c], data[n // 2:n // 2 + c], data[n - c:]]
    counts = [0] * 256
    total = 0
    for ch in chunks:
        for b in ch:
            counts[b] += 1
        total += len(ch)
    h = 0.0
    for k in counts:
        if k:
            p = k / total
            h -= p * math.log2(p)
    return h


class FileVer:
    __slots__ = ("kind", "version", "size", "entropy", "sha", "ext", "origin")

    def __init__(self, kind, version, size, entropy, sha, ext, origin):
        self.kind = kind
        self.version = version
        self.size = size
        self.entropy = entropy
        self.sha = sha
        self.ext = ext
        self.origin = origin

    def carried(self, ext=None):
        """Same content under a new path (rename)."""
        return FileVer(self.kind, self.version, self.size, self.entropy,
                       self.sha, ext or self.ext, self.origin)


def new_version(seed, path, kind, version, size_hint=None):
    rng = random.Random(stable_seed(seed, path, version, "meta"))
    if size_hint is None:
        if kind == "text":
            size_hint = rng.randint(1200, 6000)
        elif kind == "bin":
            size_hint = rng.randint(2048, 12000)
        else:
            size_hint = rng.randint(2400, 9000)
    data = content_bytes(seed, path, version, kind, size_hint)
    ext = path.rsplit(".", 1)[-1] if "." in path else "txt"
    return FileVer(kind, version, size_hint, round(entropy_of(data), 2),
                   hashlib.sha256(data).hexdigest(), ext, path)


def build_timeline(scenario, seed):
    """Return (events, truth) for one deterministic scenario run."""
    fam = scenarios.family(scenario)
    rng = random.Random(stable_seed(seed, scenario, "timeline"))
    base = dt.datetime.strptime(scenarios.BASE_EPOCH, "%Y-%m-%dT%H:%M:%S").replace(
        tzinfo=dt.timezone.utc)

    registry = {}
    events = []
    seq = 0
    last_t = None

    def emit(window, op, path, old=None, new=None, dest=None, actor="office-user"):
        nonlocal seq, last_t
        trng = random.Random(stable_seed(seed, scenario, window, seq, op, path))
        t = base + dt.timedelta(seconds=window * scenarios.WINDOW_SECONDS
                                + trng.randint(3, 295))
        # causally linked pairs (rename -> rewrite of the destination) must
        # stay ordered after the final time sort, so never emit backwards.
        if last_t is not None and t <= last_t:
            t = last_t + dt.timedelta(seconds=1)
        last_t = t
        seq += 1
        ev = {
            "id": "%s-s%d-%05d" % (scenario, seed, seq),
            "timestamp": t.strftime("%Y-%m-%dT%H:%M:%S.000Z"),
            "operation": op,
            "path": path,
            "actor": actor,
            "scenario_id": "%s:%d" % (scenario, seed),
        }
        if old is not None:
            ev["old_sha256"] = old.sha
            ev["old_entropy"] = old.entropy
            ev["old_size"] = old.size
        if new is not None:
            ev["new_sha256"] = new.sha
            ev["new_entropy"] = new.entropy
            ev["new_size"] = new.size
        if dest is not None:
            ev["dest_path"] = dest
        events.append((t, ev))
        return t

    def kind_of(path, entropy):
        if entropy >= 6.8:
            return "encrypted"
        ext = path.rsplit(".", 1)[-1] if "." in path else ""
        return "bin" if ext in BIN_EXTS else "text"

    # ---- initial population (window 0) ----
    for i in range(scenarios.FILES):
        prng = random.Random(stable_seed(seed, "init", i))
        ext = prng.choice(TEXT_EXTS + BIN_EXTS)
        kind = "bin" if ext in BIN_EXTS else "text"
        folder = prng.choice(["docs", "src", "media", "data"])
        name = prng.choice(DOC_NAMES) if kind == "text" else "asset"
        path = "%s/%s-%03d.%s" % (folder, name, i, ext)
        fv = new_version(seed, path, kind, 0)
        registry[path] = fv
        emit(0, "create", path, new=fv)

    def normal_activity(window):
        paths = sorted(registry)
        for _ in range(rng.randint(3, 7)):
            if not paths:
                break
            p = rng.choice(paths)
            fv = registry[p]
            nv = new_version(seed, p, fv.kind, fv.version + 1)
            emit(window, "modify", p, old=fv, new=nv)
            registry[p] = nv
        if rng.random() < 0.25 and paths:
            p = rng.choice(paths)
            fv = registry.pop(p)
            folder, name = p.rsplit("/", 1)
            np = folder + "/arch-" + name
            emit(window, "rename", p, old=fv, dest=np)
            registry[np] = fv.carried()
        if rng.random() < 0.2:
            ext = rng.choice(TEXT_EXTS)
            folder = rng.choice(["docs", "src"])
            np = "%s/%s-new%02d.%s" % (folder, rng.choice(DOC_NAMES), window, ext)
            nv = new_version(seed, np, "text", 0)
            emit(window, "create", np, new=nv)
            registry[np] = nv
        if rng.random() < 0.15 and len(registry) > 40:
            p = rng.choice(sorted(registry))
            fv = registry.pop(p)
            emit(window, "delete", p, old=fv)

    attack_times = []
    compromised = set()

    def attack_rewrite(window, fraction, novel_ext, inflate, actor="svc-backup"):
        victims = sorted(registry)
        rng2 = random.Random(stable_seed(seed, scenario, window, "attack"))
        rng2.shuffle(victims)
        n = int(len(victims) * fraction)
        for p in victims[:n]:
            fv = registry.pop(p)
            folder, base_name = p.rsplit("/", 1)
            stem = base_name.rsplit(".", 1)[0]
            np = "%s/%s.id-%s.%s" % (folder, stem, scenario[:3].upper(), novel_ext)
            enc = new_version(seed, np, "encrypted", fv.version + 1,
                              size_hint=int(fv.size * inflate) + 512)
            emit(window, "rename", p, old=fv, dest=np, actor=actor)
            attack_times.append(emit(window, "modify", np, old=fv, new=enc, actor=actor))
            registry[np] = enc
            compromised.add(np)
        for p in victims[n:n + max(1, len(victims) // 20)]:
            fv = registry.pop(p)
            attack_times.append(emit(window, "delete", p, old=fv, actor=actor))

    def slow_rewrite(window, fraction, novel_ext):
        victims = sorted(registry)
        rng2 = random.Random(stable_seed(seed, scenario, window, "slow"))
        rng2.shuffle(victims)
        n = max(1, int(len(victims) * fraction))
        actors = ["host-7", "host-12", "host-23", "host-31"]
        for i, p in enumerate(victims[:n]):
            fv = registry.pop(p)
            folder, base_name = p.rsplit("/", 1)
            stem = base_name.rsplit(".", 1)[0]
            np = "%s/%s.%s" % (folder, stem, novel_ext)
            enc = new_version(seed, np, "encrypted", fv.version + 1,
                              size_hint=int(fv.size * 1.2) + 256)
            actor = actors[i % len(actors)]
            emit(window, "rename", p, old=fv, dest=np, actor=actor)
            attack_times.append(emit(window, "modify", np, old=fv, new=enc, actor=actor))
            registry[np] = enc
            compromised.add(np)

    def ext_churn(window, fraction):
        victims = sorted(registry)
        rng2 = random.Random(stable_seed(seed, scenario, window, "churn"))
        rng2.shuffle(victims)
        n = int(len(victims) * fraction)
        for p in victims[:n]:
            fv = registry.pop(p)
            folder, base_name = p.rsplit("/", 1)
            stem = base_name.rsplit(".", 1)[0]
            junk = hashlib.sha1(("%s%s%d" % (seed, p, window)).encode()).hexdigest()[:5]
            np = "%s/%s.%szz" % (folder, stem, junk)
            attack_times.append(emit(window, "rename", p, old=fv, dest=np,
                                     actor="svc-indexer"))
            registry[np] = fv.carried(ext=junk + "zz")
            compromised.add(np)

    quiet_after = fam.get("quiet_after_attack", False)
    attacked = False

    for window in range(scenarios.TOTAL_WINDOWS):
        if quiet_after and attacked:
            continue
        if window in fam.get("benign_burst_windows", []):
            victims = sorted(registry)
            rng2 = random.Random(stable_seed(seed, scenario, window, "burst"))
            rng2.shuffle(victims)
            for p in victims[:int(len(victims) * 0.20)]:
                fv = registry[p]
                nv = new_version(seed, p, fv.kind, fv.version + 1,
                                 size_hint=fv.size)
                emit(window, "modify", p, old=fv, new=nv, actor="build-agent")
                registry[p] = nv
            continue
        if "archive_files" in fam and window in fam["attack_windows"]:
            srcs = sorted(registry)
            for i in range(fam["archive_files"]):
                p = srcs[(i * 7) % len(srcs)]
                folder = p.rsplit("/", 1)[0]
                np = "%s/backup-%02d-%02d.7z" % (folder, window, i)
                arc = new_version(seed, np, "encrypted", 0,
                                  size_hint=rng.randint(90000, 140000))
                emit(window, "create", np, new=arc)
                registry[np] = arc
            continue
        if "bulk_fraction" in fam and window in fam["attack_windows"]:
            victims = sorted(registry)
            rng2 = random.Random(stable_seed(seed, scenario, window, "bulk"))
            rng2.shuffle(victims)
            for p in victims[:int(len(victims) * fam["bulk_fraction"])]:
                fv = registry[p]
                nv = new_version(seed, p, fv.kind, fv.version + 1,
                                 size_hint=int(fv.size * rng2.uniform(0.95, 1.1)))
                emit(window, "modify", p, old=fv, new=nv, actor="update-agent")
                registry[p] = nv
            continue
        if window in fam["attack_windows"]:
            if "rewrite_fraction" in fam:
                attack_rewrite(window, fam["rewrite_fraction"], fam["novel_ext"],
                               fam.get("inflate", 1.3))
                attacked = True
            elif "per_window_fraction" in fam:
                slow_rewrite(window, fam["per_window_fraction"], fam["novel_ext"])
            elif "rename_fraction" in fam:
                ext_churn(window, fam["rename_fraction"])
            continue
        normal_activity(window)

    events.sort(key=lambda e: (e[0], e[1]["id"]))
    flat = [e[1] for e in events]
    truth = {
        "scenario": scenario,
        "seed": seed,
        "benign": fam["benign"],
        "attack_start": min(attack_times).strftime("%Y-%m-%dT%H:%M:%S.000Z") if attack_times else None,
        "attack_end": max(attack_times).strftime("%Y-%m-%dT%H:%M:%S.000Z") if attack_times else None,
        "compromised_paths": sorted(compromised),
    }
    return flat, truth


def apply_through(scenario, seed, out_root, through_idx):
    """Rebuild the mock tree to the state right after event #through_idx."""
    flat, _ = build_timeline(scenario, seed)
    root = os.path.abspath(out_root)
    if os.path.isdir(root):
        shutil.rmtree(root)
    os.makedirs(root, exist_ok=True)

    # Replay: path -> (origin, version, size) plus the last recorded
    # new_entropy, from which text-vs-opaque is recovered at write time.
    meta = {}
    entropy_hint = {}
    for i, ev in enumerate(flat):
        if i > through_idx:
            break
        op = ev["operation"]
        p = ev["path"]
        if op in ("create", "modify"):
            if "new_sha256" not in ev:
                continue
            prev = meta.get(p)
            # Generation rule (mirrors build_timeline): creates start at
            # version 0; every modify continues the version chain of the
            # registry entry (renames carry version counts forward), and
            # content is always generated under the CURRENT path.
            if op == "create" or prev is None:
                ver, origin = 0, p
            else:
                ver, origin = prev[1] + 1, p
            meta[p] = (origin, ver, ev["new_size"])
            entropy_hint[p] = ev.get("new_entropy") or 0
        elif op == "rename":
            m = meta.pop(p, None)
            hint = entropy_hint.pop(p, 0)
            if m is not None and "dest_path" in ev:
                meta[ev["dest_path"]] = m
                entropy_hint[ev["dest_path"]] = hint
        elif op == "delete":
            meta.pop(p, None)

    written = 0
    for path in sorted(meta):
        origin, ver, size = meta[path]
        # text vs opaque is recoverable from the recorded entropy
        # (word-like text peaks near 4.2 bits/byte; opaque content near 8).
        kind = "text" if entropy_hint[path] < 6.0 else "opaque"
        data = content_bytes(seed, origin, ver, kind, size)
        dest = os.path.join(root, *path.split("/"))
        os.makedirs(os.path.dirname(dest), exist_ok=True)
        with open(dest, "wb") as f:
            f.write(data)
        written += 1
    return written


def main():
    ap = argparse.ArgumentParser(description="CleanPoint deterministic generator")
    ap.add_argument("--scenario", required=True)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--out", help="manifest output directory")
    ap.add_argument("--root", help="apply mode: rebuild tree under this root")
    ap.add_argument("--through", type=int, help="apply mode: event index bound")
    args = ap.parse_args()

    if args.root is not None:
        if args.through is None:
            ap.error("--through is required with --root")
        n = apply_through(args.scenario, args.seed, args.root, args.through)
        print(json.dumps({"written_files": n, "through": args.through}))
        return
    if not args.out:
        ap.error("--out is required in manifest mode")
    os.makedirs(args.out, exist_ok=True)
    flat, truth = build_timeline(args.scenario, args.seed)

    base = dt.datetime.strptime(scenarios.BASE_EPOCH, "%Y-%m-%dT%H:%M:%S").replace(
        tzinfo=dt.timezone.utc)

    def window_end(w):
        return base + dt.timedelta(seconds=(w + 1) * scenarios.WINDOW_SECONDS)

    bounds = []
    for w in scenarios.snapshot_points():
        end = window_end(w)
        idx = len(flat) - 1
        for i, ev in enumerate(flat):
            t = dt.datetime.strptime(ev["timestamp"], "%Y-%m-%dT%H:%M:%S.000Z").replace(
                tzinfo=dt.timezone.utc)
            if t >= end:
                idx = max(i - 1, 0)
                break
        bounds.append({"window": w, "through_index": idx})

    meta = {
        "scenario": args.scenario,
        "seed": args.seed,
        "total_events": len(flat),
        "window_seconds": scenarios.WINDOW_SECONDS,
        "total_windows": scenarios.TOTAL_WINDOWS,
        "baseline_windows": scenarios.BASELINE_WINDOWS,
        "snapshot_points": bounds,
    }
    with open(os.path.join(args.out, "events.json"), "w") as f:
        json.dump(flat, f, indent=1)
    with open(os.path.join(args.out, "truth.json"), "w") as f:
        json.dump(truth, f, indent=1)
    with open(os.path.join(args.out, "meta.json"), "w") as f:
        json.dump(meta, f, indent=1)
    print(json.dumps({"events": len(flat), "out": args.out,
                      "attack_start": truth["attack_start"]}))


if __name__ == "__main__":
    main()
