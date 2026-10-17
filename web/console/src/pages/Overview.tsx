import React from "react";
import { useNavigate } from "react-router-dom";
import {
  api, Workload, Snapshot, Findings, SelectorReport, RecoveryRun, fmtTime, num,
} from "../api";
import { Chip, Banner, Loading, ErrorBanner } from "../components";
import { Session } from "../App";

interface Row {
  wl: Workload;
  snaps: Snapshot[];
  findings: Findings | null;
  cand: SelectorReport | null;
  runs: RecoveryRun[];
}

const ICONS = {
  layers: (
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d="M12 3l9 5-9 5-9-5 9-5z" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinejoin="round"/><path d="M3 13l9 5 9-5" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinejoin="round" opacity=".55"/></svg>
  ),
  alert: (
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d="M12 4l9 16H3l9-16z" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinejoin="round"/><path d="M12 10v5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"/><circle cx="12" cy="18" r="1" fill="currentColor"/></svg>
  ),
  shield: (
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d="M12 3l7 3v6c0 4.2-2.9 7.4-7 8.7C7.9 19.4 5 16.2 5 12V6l7-3z" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinejoin="round"/><path d="M9 12l2.2 2.2L15.5 10" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"/></svg>
  ),
  ban: (
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><circle cx="12" cy="12" r="8.5" fill="none" stroke="currentColor" strokeWidth="1.7"/><path d="M6 6l12 12" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"/></svg>
  ),
  check: (
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><circle cx="12" cy="12" r="8.5" fill="none" stroke="currentColor" strokeWidth="1.7"/><path d="M8.5 12.2l2.4 2.4 4.6-5" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"/></svg>
  ),
};

// UI-02: totals, snapshot counts, suspect windows, last verified recovery,
// warnings — with empty / loading / stale / error states.
export default function Overview() {
  const nav = useNavigate();
  const { role } = React.useContext(Session);
  const [rows, setRows] = React.useState<Row[] | null>(null);
  const [error, setError] = React.useState<unknown>(null);
  const [refreshedAt, setRefreshedAt] = React.useState<Date | null>(null);
  const [stale, setStale] = React.useState(false);
  const [auto, setAuto] = React.useState(true);
  const loadRef = React.useRef<() => void>(() => {});

  const load = React.useCallback(async () => {
    try {
      const { workloads } = await api<{ workloads: Workload[] }>("GET", "/workloads");
      const out: Row[] = [];
      for (const wl of workloads) {
        const [snaps, findings, cand, runs] = await Promise.all([
          api<{ snapshots: Snapshot[] }>("GET", `/workloads/${wl.id}/snapshots`).catch(() => ({ snapshots: [] as Snapshot[] })),
          api<Findings>("GET", `/workloads/${wl.id}/findings`).catch(() => null),
          api<SelectorReport>("GET", `/workloads/${wl.id}/recovery-candidates`).catch(() => null),
          api<{ recovery_runs: RecoveryRun[] }>("GET", `/workloads/${wl.id}/recovery-runs`).catch(() => ({ recovery_runs: [] as RecoveryRun[] })),
        ]);
        out.push({ wl, snaps: snaps.snapshots, findings, cand, runs: runs.recovery_runs });
      }
      setRows(out);
      setRefreshedAt(new Date());
      setStale(false);
      setError(null);
    } catch (e) {
      setError(e);
      setStale(true);
    }
  }, []);

  loadRef.current = load;
  React.useEffect(() => {
    loadRef.current();
  }, []);

  // gentle auto-refresh (30s), paused when the tab is hidden
  React.useEffect(() => {
    if (!auto) return;
    const t = setInterval(() => {
      if (!document.hidden) loadRef.current();
    }, 30000);
    return () => clearInterval(t);
  }, [auto]);

  if (error && !rows) return <ErrorBanner error={error} />;
  if (!rows) return <OverviewSkeleton />;

  const totals = rows.reduce(
    (a, r) => {
      a.snaps += r.snaps.length;
      a.affected += r.snaps.filter((s) => s.assessment?.exposure_status === "affected").length;
      a.susWin += r.findings ? r.findings.windows.filter((w) => w.alert_level === "suspicious").length : 0;
      a.verified += r.runs.filter((x) => x.status === "verified").length;
      if (r.cand?.status === "no_safe_candidate") a.abstain++;
      const last = r.runs.find((x) => x.status === "verified");
      if (last && (!a.lastRun || last.created_at > a.lastRun.created_at)) a.lastRun = last;
      return a;
    },
    { snaps: 0, affected: 0, susWin: 0, verified: 0, abstain: 0, lastRun: null as RecoveryRun | null }
  );

  return (
    <>
      <div className="dash-head">
        <h2 className="page" style={{ margin: 0 }}>
          Overview <span className="badge-env">LOCAL SIMULATION</span>
        </h2>
        <div className="refresh-row" role="status">
          <span className={"pulse" + (stale ? "" : " live")} aria-hidden="true" />
          <span className="hint">
            {stale ? "stale — refresh failed" : refreshedAt ? `updated ${refreshedAt.toLocaleTimeString()}` : "…"}
          </span>
          <button className="btn btnless" onClick={() => loadRef.current()} title="Refresh now">
            ⟳ refresh
          </button>
          <label className="auto-toggle">
            <input type="checkbox" checked={auto} onChange={(e) => setAuto(e.target.checked)} />
            auto
          </label>
        </div>
      </div>

      {stale && (
        <Banner tone="warn" title="Data may be stale">
          The last refresh failed — showing previously loaded state.
        </Banner>
      )}

      <div className="dash-grid">
        <DashCard label="workloads" value={rows.length} sub="registered in this sandbox" icon={ICONS.layers} accent="blue" onClick={() => nav("/workloads")} />
        <DashCard label="sealed snapshots" value={totals.snaps} sub={`${rows.filter((r) => r.snaps.length).length} workloads with history`} icon={ICONS.layers} accent="blue" />
        <DashCard label="affected snapshots" value={totals.affected} sub={totals.affected ? "behaviorally flagged — see detail" : "none flagged"} icon={ICONS.alert} accent={totals.affected ? "red" : "green"} warn={totals.affected > 0} />
        <DashCard label="suspicious windows" value={totals.susWin} sub="detector rules-v1, threshold 0.35" icon={ICONS.alert} accent={totals.susWin ? "amber" : "green"} warn={totals.susWin > 0} />
        <DashCard label="verified restores" value={totals.verified} sub={totals.lastRun ? `latest ${fmtTime(totals.lastRun.created_at)}` : "no restores yet"} icon={ICONS.check} accent="green" />
        <DashCard label="abstentions" value={totals.abstain} sub="no_safe_candidate by design" icon={ICONS.ban} accent={totals.abstain ? "red" : "blue"} />
      </div>

      {totals.lastRun && (
        <Banner tone="ok" title="Last verified recovery">
          {totals.lastRun.id} → {num(totals.lastRun.verified_files)} files verified · source unchanged ·{" "}
          {fmtTime(totals.lastRun.created_at)}
        </Banner>
      )}

      {rows.length === 0 ? (
        <div className="empty big">
          <h3>No workloads yet</h3>
          <p>Run a scenario to watch CleanPoint detect, select and restore — in about a minute.</p>
          {role === "operator" ? (
            <button className="btn primary btn-lg" onClick={() => nav("/simulation")}>
              Run your first simulation →
            </button>
          ) : (
            <p className="hint">Ask an operator to run one from the Simulation Lab.</p>
          )}
        </div>
      ) : (
        <>
          <h3 className="sec">Workload status</h3>
          <div className="wl-cards">
            {rows.map((r, i) => (
              <WorkloadCard key={r.wl.id} r={r} idx={i} onOpen={() => nav(`/workload/${r.wl.id}/overview`)} />
            ))}
          </div>
        </>
      )}
    </>
  );
}

function DashCard({
  label,
  value,
  sub,
  icon,
  accent,
  warn,
  onClick,
}: {
  label: string;
  value: React.ReactNode;
  sub: string;
  icon: React.ReactNode;
  accent: "blue" | "green" | "red" | "amber";
  warn?: boolean;
  onClick?: () => void;
}) {
  const Tag: any = onClick ? "button" : "div";
  return (
    <Tag className={`dash-card accent-${accent}` + (onClick ? " clickable" : "") + (warn ? " warn" : "")} onClick={onClick} style={{ animationDelay: `${Math.random() * 0.1}s` }}>
      <span className="dc-icon" aria-hidden="true">{icon}</span>
      <span className="dc-body">
        <b className="dc-value">{value}</b>
        <span className="dc-label">{label}</span>
        <span className="dc-sub">{sub}</span>
      </span>
    </Tag>
  );
}

function WorkloadCard({ r, idx, onOpen }: { r: Row; idx: number; onOpen: () => void }) {
  const exp = r.snaps[0]?.assessment?.exposure_status;
  const onset = r.findings?.incident_onset ?? null;
  const susWin = r.findings ? r.findings.windows.filter((w) => w.alert_level === "suspicious").length : 0;
  const okRuns = r.runs.filter((x) => x.status === "verified").length;
  const strip = r.snaps.slice(0, 13);
  const worst = r.snaps.some((s) => s.assessment?.exposure_status === "affected")
    ? "affected"
    : r.snaps.some((s) => s.assessment?.exposure_status === "suspect")
    ? "suspect"
    : exp ?? null;

  return (
    <div
      className={"wl-card" + (worst === "affected" ? " has-affected" : "")}
      style={{ animationDelay: `${idx * 60}ms` }}
      tabIndex={0}
      role="button"
      aria-label={`Open workload ${r.wl.name}`}
      onClick={onOpen}
      onKeyDown={(e) => e.key === "Enter" && onOpen()}
    >
      <div className="wl-head">
        <b>{r.wl.name}</b>
        {r.cand ? (
          r.cand.status === "candidate_available" ? (
            <Chip kind="eligible" label="candidate" />
          ) : (
            <Chip kind="no_safe_candidate" label="abstained" />
          )
        ) : (
          <Chip kind="unknown" label="no analysis" />
        )}
      </div>
      <div className="wl-dots" aria-hidden="true">
        {strip.reverse().map((s) => (
          <i key={s.id} className={`d-${s.assessment?.exposure_status ?? "none"}`} title={`${s.captured_at} — ${s.assessment?.exposure_status ?? "no analysis"}`} />
        ))}
        {strip.length === 0 && <span className="hint">no snapshots</span>}
      </div>
      <dl className="wl-meta">
        <div>
          <dt>newest exposure</dt>
          <dd>{exp ? <Chip kind={exp} /> : "—"}</dd>
        </div>
        <div>
          <dt>onset</dt>
          <dd>{onset ? <b className="red">{onset.slice(11, 16)}</b> : "none"}</dd>
        </div>
        <div>
          <dt>suspicious</dt>
          <dd>{susWin ? <b className="red">{susWin} win</b> : "0"}</dd>
        </div>
        <div>
          <dt>restores</dt>
          <dd>{okRuns ? `${okRuns} verified` : "—"}</dd>
        </div>
      </dl>
    </div>
  );
}

function OverviewSkeleton() {
  return (
    <div aria-busy="true" aria-label="Loading dashboard">
      <div className="skel title" />
      <div className="dash-grid">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="skel card" style={{ animationDelay: `${i * 80}ms` }} />
        ))}
      </div>
      <div className="skel block" />
      <Loading />
    </div>
  );
}
