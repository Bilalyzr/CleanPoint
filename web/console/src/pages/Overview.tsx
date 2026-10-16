import React from "react";
import { useNavigate } from "react-router-dom";
import { api, Workload, Snapshot, Findings, SelectorReport, RecoveryRun, fmtTime, num } from "../api";
import { Stat, Chip, Banner, Loading, ErrorBanner } from "../components";

interface Row {
  wl: Workload;
  snaps: Snapshot[];
  findings: Findings | null;
  cand: SelectorReport | null;
  runs: RecoveryRun[];
}

// UI-02: totals, snapshot counts, suspect windows, last verified recovery,
// warnings; empty/processing/error states per workload.
export default function Overview() {
  const nav = useNavigate();
  const [rows, setRows] = React.useState<Row[] | null>(null);
  const [error, setError] = React.useState<unknown>(null);

  React.useEffect(() => {
    let alive = true;
    (async () => {
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
          out.push({ wl, snaps: snaps.snapshots, findings, cand: cand as any, runs: (runs as any).recovery_runs });
        }
        if (alive) setRows(out);
      } catch (e) {
        if (alive) setError(e);
      }
    })();
    return () => {
      alive = false;
    };
  }, []);

  if (error) return <ErrorBanner error={error} />;
  if (!rows) return <Loading />;

  const totals = rows.reduce(
    (a, r) => {
      a.snaps += r.snaps.length;
      a.affected += r.snaps.filter((s) => s.assessment?.exposure_status === "affected").length;
      a.suspect += r.snaps.filter((s) => s.assessment?.exposure_status === "suspect").length;
      a.susWin += r.findings ? r.findings.windows.filter((w) => w.alert_level === "suspicious").length : 0;
      a.verified += r.runs.filter((x) => x.status === "verified").length;
      if (r.cand?.status === "no_safe_candidate") a.abstain++;
      const last = r.runs.find((x) => x.status === "verified");
      if (last && (!a.lastRun || last.created_at > a.lastRun.created_at)) a.lastRun = last;
      return a;
    },
    { snaps: 0, affected: 0, suspect: 0, susWin: 0, verified: 0, abstain: 0, lastRun: null as RecoveryRun | null }
  );

  return (
    <>
      <h2 className="page">
        Overview <span className="badge-env">LOCAL SIMULATION</span>
      </h2>
      <div className="cards">
        <Stat n={rows.length} label="workloads" />
        <Stat n={totals.snaps} label="sealed snapshots" />
        <Stat n={totals.affected} label="affected snapshots" warn={totals.affected > 0} />
        <Stat n={totals.suspect} label="suspect snapshots" />
        <Stat n={totals.susWin} label="suspicious windows" warn={totals.susWin > 0} />
        <Stat n={totals.verified} label="verified restores" />
        <Stat n={totals.abstain} label="abstentions (no safe candidate)" />
      </div>
      {totals.lastRun && (
        <Banner tone="ok" title="Last verified recovery">
          {totals.lastRun.id} → {num(totals.lastRun.verified_files)} files verified, source unchanged,{" "}
          {fmtTime(totals.lastRun.created_at)}
        </Banner>
      )}
      {rows.length === 0 ? (
        <div className="empty">
          No workloads yet. Run the Simulation Lab, or <code>scripts/demo.sh</code> from a terminal.
        </div>
      ) : (
        <table className="tbl">
          <thead>
            <tr>
              <th>Workload</th>
              <th className="num">Snapshots</th>
              <th>Newest exposure</th>
              <th>Incident onset</th>
              <th className="num">Suspicious windows</th>
              <th>Selector</th>
              <th>Restores</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => {
              const exp = r.snaps[0]?.assessment?.exposure_status;
              const onset = r.findings?.incident_onset;
              const susWin = r.findings ? r.findings.windows.filter((w) => w.alert_level === "suspicious").length : 0;
              const ok = r.runs.filter((x) => x.status === "verified").length;
              return (
                <tr key={r.wl.id} className="click" onClick={() => nav(`/workload/${r.wl.id}/overview`)} tabIndex={0}
                    onKeyDown={(e) => e.key === "Enter" && nav(`/workload/${r.wl.id}/overview`)}>
                  <td>
                    <b>{r.wl.name}</b>
                    <div className="path">{r.wl.id}</div>
                  </td>
                  <td className="num">{r.snaps.length}</td>
                  <td>{exp ? <Chip kind={exp} /> : <Chip kind="unknown" label="no analysis" />}</td>
                  <td>{onset ? <b className="red">{fmtTime(onset)}</b> : "no onset"}</td>
                  <td className="num">{susWin}</td>
                  <td>
                    {r.cand ? (
                      r.cand.status === "candidate_available" ? (
                        <Chip kind="eligible" label="candidate" />
                      ) : (
                        <Chip kind="no_safe_candidate" label="abstained" />
                      )
                    ) : (
                      "—"
                    )}
                  </td>
                  <td>{ok ? <Chip kind="verified" label={`verified ×${ok}`} /> : "—"}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </>
  );
}
