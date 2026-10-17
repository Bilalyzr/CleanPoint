import React from "react";
import { useNavigate } from "react-router-dom";
import { api, SimJob, fmtTime } from "../api";
import { Chip, Banner, KV } from "../components";
import { Session } from "../App";

const FAMILIES: { id: string; label: string; note: string; tone: "good" | "warn" | "bad"; glyph: string }[] = [
  { id: "normal_office", label: "Normal office", note: "steady benign activity — expect no onset", tone: "good", glyph: "◐" },
  { id: "benign_archive", label: "Benign archive", note: "high-entropy .7z files — must NOT be flagged", tone: "good", glyph: "▣" },
  { id: "benign_bulk_update", label: "Bulk update", note: "mass benign rewrite — safe under threshold", tone: "good", glyph: "▤" },
  { id: "rapid_destructive", label: "Rapid destructive", note: "fast mass rewrite — older clean point", tone: "bad", glyph: "⚡" },
  { id: "slow_distributed", label: "Slow distributed", note: "low-rate changes — cumulative detection", tone: "bad", glyph: "≋" },
  { id: "extension_churn", label: "Extension churn", note: "mass renames — not signature-based", tone: "bad", glyph: "⟳" },
  { id: "mixed_activity", label: "Mixed activity", note: "burst + attack — onset must skip the burst", tone: "warn", glyph: "◑" },
  { id: "pre_existing_contamination", label: "Pre-existing", note: "predates every snapshot — must ABSTAIN", tone: "warn", glyph: "⊘" },
];

const PHASES = [
  { id: "gen", label: "Generate", hint: "deterministic files + events + hidden truth" },
  { id: "seal", label: "Seal ×13", hint: "ingest phases, capture snapshots" },
  { id: "analyze", label: "Analyze", hint: "rules-v1 windows + baseline" },
  { id: "select", label: "Select", hint: "rank or abstain" },
  { id: "restore", label: "Restore", hint: "verified isolated copy" },
];

// UI-05 redesigned as the control room: family tiles, seed stepper, live
// run panel with phase stepper and streaming driver log.
export default function Simulation() {
  const nav = useNavigate();
  const { role } = React.useContext(Session);
  const [family, setFamily] = React.useState("rapid_destructive");
  const [seed, setSeed] = React.useState(1);
  const [job, setJob] = React.useState<SimJob | null>(null);
  const [history, setHistory] = React.useState<SimJob[]>([]);
  const [err, setErr] = React.useState("");
  const isOperator = role === "operator";
  const logRef = React.useRef<HTMLPreElement>(null);

  const loadHistory = React.useCallback(() => {
    api<{ simulations: SimJob[] }>("GET", "/simulations")
      .then((d) => setHistory(d.simulations ?? []))
      .catch(() => {});
  }, []);
  React.useEffect(loadHistory, [loadHistory]);

  React.useEffect(() => {
    if (!job || job.status !== "running") return;
    const t = setInterval(async () => {
      try {
        const j = await api<SimJob>("GET", `/simulations/${job.id}`);
        setJob(j);
        if (j.status !== "running") loadHistory();
      } catch {
        /* keep polling */
      }
    }, 1500);
    return () => clearInterval(t);
  }, [job, loadHistory]);

  React.useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [job?.log.length]);

  const run = async () => {
    setErr("");
    try {
      setJob(await api<SimJob>("POST", "/simulate", { scenario: family, seed }));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  const phaseState = (idx: number): string => {
    if (!job) return "";
    const done = phaseDone(job, job.log ?? []);
    if (done > idx) return "done";
    if (done === idx && job.status === "running") return "active";
    return "";
  };

  const sel = FAMILIES.find((f) => f.id === family)!;
  const result = job?.result;

  return (
    <>
      <div className="page-head">
        <h2 className="page" style={{ margin: 0 }}>Simulation Lab</h2>
        <span className="count-pill">8 families · deterministic</span>
        <span className="spacer" />
        <span className="badge-env">SYNTHETIC ONLY — NO LIVE MALWARE</span>
      </div>

      <div className="sim-layout">
        <div className="sim-families" role="radiogroup" aria-label="Scenario family">
          {FAMILIES.map((f) => (
            <button
              key={f.id}
              role="radio"
              aria-checked={family === f.id}
              className={`fam-tile tone-${f.tone}` + (family === f.id ? " selected" : "")}
              onClick={() => setFamily(f.id)}
              disabled={!isOperator}
            >
              <span className="ft-glyph" aria-hidden="true">{f.glyph}</span>
              <span className="ft-body">
                <b>{f.label}</b>
                <span>{f.note}</span>
              </span>
            </button>
          ))}
        </div>

        <div className="sim-launch card">
          <h3 className="sec" style={{ marginTop: 0 }}>Launch</h3>
          <p className="hint" style={{ marginBottom: 14 }}>
            Everything is harmless seeded data. A run takes ~45–60 s: registers a fresh workload,
            seals 13 snapshots across a simulated timeline, analyzes, then restores if a candidate exists.
          </p>
          <label className="fld" htmlFor="seedStep">Seed — {sel.label}</label>
          <div className="seed-stepper">
            <button className="btn" onClick={() => setSeed((s) => Math.max(1, s - 1))} aria-label="Decrease seed" disabled={!isOperator}>−</button>
            <input id="seedStep" type="number" min={1} max={999} value={seed}
              onChange={(e) => setSeed(Math.max(1, Math.min(999, Number(e.target.value) || 1)))} disabled={!isOperator} />
            <button className="btn" onClick={() => setSeed((s) => Math.min(999, s + 1))} aria-label="Increase seed" disabled={!isOperator}>+</button>
          </div>
          <p className="hint" style={{ marginTop: 8 }}>
            Same family + seed ⇒ identical bytes, events and outcome — every time.
          </p>
          <button className="btn primary btn-lg run-btn" onClick={run} disabled={!isOperator || job?.status === "running"}>
            {job?.status === "running" ? "Simulation running…" : "Run scenario →"}
          </button>
          {!isOperator && <p className="hint" style={{ marginTop: 10 }}>Read-only role: ask an operator to launch runs.</p>}
          <p className="login-error" role="alert">{err}</p>
        </div>
      </div>

      {job && (
        <div className={"run-panel" + (job.status === "running" ? " live" : "")}>
          <div className="run-head">
            <span className={"pulse" + (job.status === "running" ? " live" : "")} aria-hidden="true" />
            <b>{job.status === "running" ? "Running" : job.status === "completed" ? "Completed" : "Failed"}</b>
            <span className="mono-id">{sel.label} · seed {job.seed} · {job.id}</span>
            <span className="spacer" />
            {job.status !== "running" && (
              <button className="btn" onClick={() => setJob(null)}>Clear</button>
            )}
          </div>

          <div className="stepper" aria-hidden="true">
            {PHASES.map((p, i) => (
              <div key={p.id} className={`step ${phaseState(i)}`} title={p.hint}>
                {p.label}
              </div>
            ))}
          </div>

          {job.status === "failed" && (
            <Banner tone="error" title="Simulation failed">{job.error}</Banner>
          )}

          {result && (
            <div className="run-result">
              <div className="rr-chips">
                <Chip kind={result.selector_status === "candidate_available" ? "eligible" : "no_safe_candidate"} label={String(result.selector_status)} />
                {result.recommended_contaminated === true && <Chip kind="failed" label="unsafe pick" />}
                {result.recommended_contaminated === false && <Chip kind="verified" label="clean pick" />}
                {result.abstention_correct === true && <Chip kind="verified" label="abstention correct" />}
                {result.restore && <Chip kind={result.restore.status === "verified" ? "verified" : "failed"} label={`restore ${result.restore.status} · ${result.restore.verified_files} files`} />}
              </div>
              <KV
                rows={[
                  ["Workload", <span className="path">{String(result.workload_id ?? "—")}</span>],
                  ["Onset (detector) vs truth (hidden)", `${result.onset ?? "none"} vs ${result.attack_start ?? "none"}`],
                  ["Source unchanged", result.restore ? String(result.restore.source_unchanged) : "—"],
                ]}
              />
              {result.workload_id && (
                <button className="btn" onClick={() => nav(`/workload/${result.workload_id}/overview`)}>
                  Open workload →
                </button>
              )}
            </div>
          )}

          <details open={job.status === "running"}>
            <summary>Driver log ({(job.log ?? []).length} lines)</summary>
            <pre className="term runlog" ref={logRef}>{(job.log ?? []).join("\n")}</pre>
          </details>
        </div>
      )}

      <h3 className="sec">Recent simulations</h3>
      {history.length === 0 ? (
        <div className="empty">No simulations started from this console yet.</div>
      ) : (
        <table className="tbl">
          <thead>
            <tr>
              <th>Job</th>
              <th>Scenario</th>
              <th className="num">Seed</th>
              <th>Status</th>
              <th>Started</th>
            </tr>
          </thead>
          <tbody>
            {history.map((j) => (
              <tr key={j.id}>
                <td className="path">{j.id}</td>
                <td>{FAMILIES.find((f) => f.id === j.scenario)?.label ?? j.scenario}</td>
                <td className="num">{j.seed}</td>
                <td><Chip kind={j.status === "completed" ? "verified" : j.status === "failed" ? "failed" : "warn"} label={j.status} /></td>
                <td>{fmtTime(j.started_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

function phaseDone(job: SimJob, log: string[]): number {
  if (job.status === "completed") return PHASES.length;
  if (log.some((l) => l.includes("restore") || l.includes("candidates"))) return 4;
  if (log.some((l) => l.includes("analyze"))) return 3;
  if (log.some((l) => /window (1\d|[2-3]\d)/.test(l))) return 2;
  if (log.some((l) => l.includes("window"))) return 1;
  return 0;
}
