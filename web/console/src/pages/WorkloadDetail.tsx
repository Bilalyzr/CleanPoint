import React from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  api, Workload, Snapshot, Findings, SelectorReport, FileEvent, RestoreReport,
  Assessment, fmtTime, fmtBytes, num,
} from "../api";
import { Chip, Stat, Banner, Modal, Tabs, KV, Loading, ErrorBanner } from "../components";
import { RiskChart, SnapshotTimeline, ActivitySpark, GapBar, maxGap } from "../charts";
import { Session } from "../App";

// Covers UI-04 (detail + actions), UI-06 (events), UI-07/08 (snapshots +
// detail), UI-09 (findings), UI-10 (candidates), UI-11/12 (restore + report).

interface SnapshotDetail {
  snapshot: Snapshot;
  assessment?: Assessment;
  files: { path: string; sha256: string; size_bytes: number; entropy?: number }[];
  file_total: number;
}

export default function WorkloadDetail() {
  const { id = "", tab = "overview" } = useParams();
  const nav = useNavigate();
  const { role } = React.useContext(Session);
  const [wl, setWl] = React.useState<Workload | null>(null);
  const [snaps, setSnaps] = React.useState<Snapshot[]>([]);
  const [findings, setFindings] = React.useState<Findings | null>(null);
  const [findingsErr, setFindingsErr] = React.useState<string | null>(null);
  const [cand, setCand] = React.useState<SelectorReport | null>(null);
  const [runs, setRuns] = React.useState<any[]>([]);
  const [error, setError] = React.useState<unknown>(null);
  const [busy, setBusy] = React.useState("");
  const [toast, setToast] = React.useState("");
  const [detail, setDetail] = React.useState<SnapshotDetail | null>(null);
  const [restoreSnap, setRestoreSnap] = React.useState<string | null>(null);
  const [report, setReport] = React.useState<RestoreReport | null>(null);

  const refresh = React.useCallback(async () => {
    try {
      const { workloads } = await api<{ workloads: Workload[] }>("GET", "/workloads");
      const me = workloads.find((w) => w.id === id);
      setWl(me ?? null);
      const s = await api<{ snapshots: Snapshot[] }>("GET", `/workloads/${id}/snapshots`);
      setSnaps(s.snapshots);
      setFindings(null);
      setFindingsErr(null);
      try {
        setFindings(await api<Findings>("GET", `/workloads/${id}/findings`));
      } catch (e) {
        setFindingsErr(e instanceof Error ? e.message : "no analysis yet");
      }
      setCand(await api<SelectorReport>("GET", `/workloads/${id}/recovery-candidates`).catch(() => null));
      const rr = await api<{ recovery_runs: any[] }>("GET", `/workloads/${id}/recovery-runs`).catch(() => ({ recovery_runs: [] }));
      setRuns(rr.recovery_runs);
      setError(null);
    } catch (e) {
      setError(e);
    }
  }, [id]);

  React.useEffect(() => {
    refresh();
  }, [refresh]);

  const act = async (label: string, fn: () => Promise<any>) => {
    setBusy(label);
    setToast("");
    try {
      await fn();
      setToast(`${label} complete`);
      await refresh();
    } catch (e) {
      setToast(`${label} failed: ${e instanceof Error ? e.message : e}`);
    } finally {
      setBusy("");
    }
  };

  if (error) return <ErrorBanner error={error} />;
  if (!wl) return <Loading />;
  const isOperator = role === "operator";
  const tabs = [
    { id: "overview", label: "Overview" },
    { id: "events", label: "Events" },
    { id: "snapshots", label: "Snapshots" },
    { id: "findings", label: "Findings" },
    { id: "recovery", label: "Recovery" },
  ];

  return (
    <>
      <div className="crumbs">
        <a href="#/workloads">Workloads</a> / {wl.name}
      </div>
      <h2 className="page">
        {wl.name} <Chip kind="unknown" label={wl.id} />
        {isOperator && (
          <>
            <button
              className="btn"
              style={{ marginLeft: "auto" }}
              disabled={!!busy}
              onClick={() => act("Snapshot capture", () => api("POST", `/workloads/${id}/snapshots`, {}))}
            >
              {busy === "Snapshot capture" ? "Capturing…" : "Capture snapshot"}
            </button>
            <button
              className="btn primary"
              disabled={!!busy}
              onClick={() => act("Analysis", () => api("POST", `/workloads/${id}/analyze`, {}))}
            >
              {busy === "Analysis" ? "Analyzing…" : "Analyze"}
            </button>
          </>
        )}
      </h2>
      <Tabs tabs={tabs} active={tab} onSelect={(t) => nav(`/workload/${id}/${t}`)} />
      {toast && <Banner tone={toast.includes("failed") ? "error" : "ok"}>{toast}</Banner>}

      {tab === "overview" && (
        <WorkloadOverview
          snaps={snaps}
          findings={findings}
          runs={runs}
          onOpenSnap={async (sid) => setDetail(await fetchDetail(sid))}
        />
      )}
      {tab === "events" && <EventsTab id={id} />}
      {tab === "snapshots" && (
        <SnapshotsTab
          snaps={snaps}
          onOpen={async (sid) => setDetail(await fetchDetail(sid))}
        />
      )}
      {tab === "findings" &&
        (findings ? (
          <FindingsTab findings={findings} />
        ) : (
          <Banner tone="warn" title="No analysis yet">
            {findingsErr ?? "Run Analyze (operator) first — the detector refuses to guess without evidence."}
          </Banner>
        ))}
      {tab === "recovery" && cand && (
        <RecoveryTab
          wlId={id}
          cand={cand}
          isOperator={isOperator}
          onRestore={(sid) => setRestoreSnap(sid)}
        />
      )}
      {tab === "recovery" && !cand && (
        <Banner tone="warn" title="No analysis yet">
          The selector ranks candidates only after an analysis run.
        </Banner>
      )}

      {detail && (
        <Modal title={`Snapshot ${detail.snapshot.id}`} onClose={() => setDetail(null)} wide>
          <SnapshotDetailBody d={detail} />
        </Modal>
      )}
      {restoreSnap && (
        <RestoreDialog
          wlId={id}
          snapId={restoreSnap}
          onClose={() => setRestoreSnap(null)}
          onDone={(rep) => {
            setRestoreSnap(null);
            setReport(rep);
            refresh();
          }}
        />
      )}
      {report && (
        <Modal title={`Recovery run report — ${report.run_id}`} onClose={() => setReport(null)} wide>
          <RunReportBody rep={report} />
        </Modal>
      )}
      <section aria-label="Recovery runs">
        <h3 className="sec">Recovery runs</h3>
        {runs.length === 0 ? (
          <div className="empty">No restores performed for this workload.</div>
        ) : (
          <table className="tbl">
            <thead>
              <tr>
                <th>Run</th>
                <th>Snapshot</th>
                <th>Status</th>
                <th className="num">Verified</th>
                <th className="num">Failed</th>
                <th>Source</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => (
                <tr key={r.id}>
                  <td className="path">{r.id}</td>
                  <td className="path">{r.snapshot_id}</td>
                  <td><Chip kind={r.status} /></td>
                  <td className="num">{num(r.verified_files)}</td>
                  <td className="num">{num(r.failed_files)}</td>
                  <td>{r.source_unchanged ? <Chip kind="verified" label="unchanged" /> : "—"}</td>
                  <td>{fmtTime(r.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}

async function fetchDetail(sid: string): Promise<SnapshotDetail> {
  return api<SnapshotDetail>("GET", `/snapshots/${sid}`);
}

/* ---------- overview tab (UI-04) ---------- */
function WorkloadOverview({
  snaps,
  findings,
  runs,
  onOpenSnap,
}: {
  snaps: Snapshot[];
  findings: Findings | null;
  runs: any[];
  onOpenSnap: (id: string) => void;
}) {
  const latest = snaps[0];
  const susWin = findings ? findings.windows.filter((w) => w.alert_level === "suspicious").length : null;
  return (
    <>
      <div className="cards">
        <Stat n={snaps.length} label="snapshots" />
        <Stat n={latest ? latest.file_count : 0} label="files in newest snapshot" />
        <Stat n={latest ? fmtBytes(latest.total_bytes) : "—"} label="newest snapshot size" />
        <Stat n={susWin ?? "—"} label="suspicious windows" warn={(susWin ?? 0) > 0} />
        <Stat n={runs.filter((r) => r.status === "verified").length} label="verified restores" />
      </div>
      <div className="card">
        <h3 className="sec" style={{ marginTop: 0 }}>Latest snapshot</h3>
        {latest ? (
          <>
            <KV
              rows={[
                ["Snapshot", <span className="path">{latest.id}</span>],
                ["Captured", fmtTime(latest.captured_at)],
                ["Status", <Chip kind={latest.status} />],
                ["Manifest SHA-256", <span className="path">{latest.manifest_hash ?? "—"}</span>],
                [
                  "Assessment",
                  latest.assessment ? (
                    <>
                      <Chip kind={latest.assessment.exposure_status} />{" "}
                      integrity {latest.assessment.integrity_ok ? "OK" : "FAILED"}
                    </>
                  ) : (
                    "not analyzed yet"
                  ),
                ],
              ]}
            />
            <p className="hint" style={{ marginTop: 8 }}>
              <button className="btn btnless" onClick={() => onOpenSnap(latest.id)}>
                View manifest contents →
              </button>
            </p>
          </>
        ) : (
          <div className="empty">No snapshots. Capture one (operator) or run a simulation.</div>
        )}
      </div>
      {findings && (
        <div className="card">
          <h3 className="sec">Activity trend</h3>
          <ActivitySpark windows={findings.windows} />
          <p className="hint">Events per {findings.window_seconds}s window; red = suspicious window.</p>
        </div>
      )}
    </>
  );
}

/* ---------- events tab (UI-06) ---------- */
function EventsTab({ id }: { id: string }) {
  const [events, setEvents] = React.useState<FileEvent[] | null>(null);
  const [op, setOp] = React.useState("");
  const [q, setQ] = React.useState("");
  React.useEffect(() => {
    api<{ events: FileEvent[] }>("GET", `/workloads/${id}/events?limit=1000`)
      .then((d) => setEvents(d.events))
      .catch(() => setEvents([]));
  }, [id]);
  if (!events) return <Loading />;
  const rows = events.filter(
    (e) => (!op || e.operation === op) && (!q || e.path.toLowerCase().includes(q.toLowerCase()) || (e.dest_path ?? "").toLowerCase().includes(q.toLowerCase()))
  );
  return (
    <>
      <p className="hint">Ingested file actions (most recent {Math.min(events.length, 1000)}). Long paths wrap, full values retrievable via the API.</p>
      <div className="filters">
        <select value={op} onChange={(e) => setOp(e.target.value)} aria-label="Filter by operation">
          <option value="">all operations</option>
          {["create", "modify", "delete", "rename"].map((o) => (
            <option key={o} value={o}>{o}</option>
          ))}
        </select>
        <input type="text" placeholder="filter by path…" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Filter by path" />
      </div>
      <table className="tbl">
        <thead>
          <tr>
            <th>Time</th>
            <th>Operation</th>
            <th>Path</th>
            <th>Destination</th>
            <th className="num">Entropy old→new</th>
            <th className="num">New size</th>
            <th>Actor</th>
          </tr>
        </thead>
        <tbody>
          {rows.slice(0, 400).map((e) => (
            <tr key={e.id}>
              <td className="path">{fmtTime(e.timestamp)}</td>
              <td><Chip kind={e.operation === "delete" ? "failed" : e.operation === "create" ? "verified" : "unknown"} label={e.operation} /></td>
              <td className="path">{e.path}</td>
              <td className="path">{e.dest_path ?? "—"}</td>
              <td className="num">
                {e.old_entropy != null ? e.old_entropy.toFixed(2) : "—"} → {e.new_entropy != null ? e.new_entropy.toFixed(2) : "—"}
              </td>
              <td className="num">{num(e.new_size)}</td>
              <td>{e.actor ?? "—"}</td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td colSpan={7}><div className="empty">No events match.</div></td>
            </tr>
          )}
        </tbody>
      </table>
    </>
  );
}

/* ---------- snapshots tab (UI-07 + UI-08) ---------- */
function SnapshotsTab({ snaps, onOpen }: { snaps: Snapshot[]; onOpen: (id: string) => void }) {
  return (
    <>
      <SnapshotTimeline snapshots={snaps} onSelect={(s) => onOpen(s.id)} />
      <p className="hint">
        Chronological snapshots. A <b>valid hash does not mean clean</b> — exposure comes from behavioral + content analysis.
      </p>
      <table className="tbl">
        <thead>
          <tr>
            <th>Captured</th>
            <th>Snapshot</th>
            <th>Status</th>
            <th>Integrity</th>
            <th>Exposure</th>
            <th className="num">Files</th>
            <th className="num">Size</th>
          </tr>
        </thead>
        <tbody>
          {snaps.map((s) => (
            <tr key={s.id} className="click" tabIndex={0} onClick={() => onOpen(s.id)} onKeyDown={(e) => e.key === "Enter" && onOpen(s.id)}>
              <td className="path">{fmtTime(s.captured_at)}</td>
              <td className="path">{s.id}</td>
              <td><Chip kind={s.status} /></td>
              <td>{s.assessment ? <Chip kind={s.assessment.integrity_ok ? "verified" : "failed"} label={s.assessment.integrity_ok ? "OK" : "FAILED"} /> : "—"}</td>
              <td>{s.assessment ? <Chip kind={s.assessment.exposure_status} /> : <Chip kind="unknown" label="—" />}</td>
              <td className="num">{num(s.file_count)}</td>
              <td className="num">{fmtBytes(s.total_bytes)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

function SnapshotDetailBody({ d }: { d: SnapshotDetail }) {
  const s = d.snapshot;
  const a = d.assessment;
  return (
    <>
      <KV
        rows={[
          ["Captured at", fmtTime(s.captured_at)],
          ["Status", <Chip kind={s.status} />],
          ["Files / size", `${num(s.file_count)} files · ${fmtBytes(s.total_bytes)}`],
          ["Manifest SHA-256", <span className="path">{s.manifest_hash ?? "—"}</span>],
          ["Integrity", a ? <Chip kind={a.integrity_ok ? "verified" : "failed"} label={a.integrity_ok ? "verified" : "failed"} /> : "—"],
          ["Exposure", a ? <Chip kind={a.exposure_status} /> : "not analyzed"],
          ["Risk score (max ended window)", a ? a.risk_score.toFixed(3) : "—"],
          ["Affected files", a ? num(a.affected_file_count) : "—"],
        ]}
      />
      {a?.explanation?.length ? (
        <>
          <h3 className="sec">Why this classification</h3>
          <ul className="explain">
            {a.explanation.map((x, i) => (
              <li key={i}>{x}</li>
            ))}
          </ul>
        </>
      ) : null}
      <h3 className="sec">Manifest contents ({num(d.file_total)}{d.file_total > d.files.length ? `, first ${d.files.length}` : ""})</h3>
      <table className="tbl">
        <thead>
          <tr>
            <th>Path</th>
            <th className="num">Bytes</th>
            <th className="num">Entropy</th>
            <th>SHA-256</th>
          </tr>
        </thead>
        <tbody>
          {d.files.map((f) => (
            <tr key={f.path}>
              <td className="path">{f.path}</td>
              <td className="num">{num(f.size_bytes)}</td>
              <td className="num">{f.entropy != null ? f.entropy.toFixed(2) : "—"}</td>
              <td className="path">{f.sha256.slice(0, 16)}…</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="hint">Sealed snapshots expose no mutation or delete controls.</p>
    </>
  );
}

/* ---------- findings tab (UI-09) ---------- */
function FindingsTab({ findings }: { findings: Findings }) {
  const sus = findings.windows.filter((w) => w.alert_level === "suspicious");
  return (
    <>
      <Banner tone="info" title={`Detector ${findings.detector_version} · threshold ${findings.risk_threshold}`}>
        risk_index is a <b>relative anomaly score (0..1), not a probability</b>. Onset is the earliest
        suspicious window start — a conservative lower bound, not a proven infection time.
      </Banner>
      {findings.incident_onset && (
        <Banner tone="error" title={`Estimated earliest plausible onset: ${fmtTime(findings.incident_onset)}`}>
          Snapshots captured at/after this time are not eligible for recovery.
        </Banner>
      )}
      <div className="chart-wrap">
        <RiskChart
          windows={findings.windows}
          threshold={findings.risk_threshold}
          onset={findings.incident_onset}
          baselineCount={findings.baseline_windows}
        />
        <p className="hint">
          Each point is one {findings.window_seconds}s window (click a point for details). Shaded region =
          assumed-normal baseline ({findings.baseline_windows} windows). Click dots for contributions.
        </p>
      </div>
      <h3 className="sec">Suspicious windows ({sus.length})</h3>
      {sus.length === 0 ? (
        <div className="empty">No suspicious windows in the evaluated period.</div>
      ) : (
        <table className="tbl">
          <thead>
            <tr>
              <th>Window start</th>
              <th className="num">Risk</th>
              <th>Contributing signals</th>
              <th className="num">Events</th>
            </tr>
          </thead>
          <tbody>
            {sus.map((w) => (
              <tr key={w.id}>
                <td className="path">{fmtTime(w.starts_at)}</td>
                <td className="num red"><b>{w.risk_score.toFixed(3)}</b></td>
                <td>
                  {(w.signal_contributions ?? []).filter((c) => c.signal >= 0.2).map((c) => (
                    <span key={c.feature} className="mini-chip">{c.feature} ×{c.signal.toFixed(2)}</span>
                  ))}
                </td>
                <td className="num">{w.event_count}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <h3 className="sec">Behaviorally affected files ({findings.affected_files.length})</h3>
      {findings.affected_files.length === 0 ? (
        <div className="empty">No affected files flagged.</div>
      ) : (
        <>
          <p className="hint">Flagged from event metadata in suspicious windows — behavioral evidence, <b>not</b> proven encryption.</p>
          <ul className="filelist">
            {findings.affected_files.map((p) => (
              <li key={p}>{p}</li>
            ))}
          </ul>
        </>
      )}
      <details>
        <summary>All windows (raw)</summary>
        <table className="tbl" style={{ marginTop: 10 }}>
          <thead>
            <tr>
              <th>Start</th>
              <th>Baseline?</th>
              <th className="num">Events</th>
              <th className="num">Risk</th>
              <th>Alert</th>
            </tr>
          </thead>
          <tbody>
            {findings.windows.map((w) => (
              <tr key={w.id}>
                <td className="path">{fmtTime(w.starts_at)}</td>
                <td>{w.is_baseline ? "yes" : ""}</td>
                <td className="num">{w.event_count}</td>
                <td className="num">{w.is_baseline ? "—" : w.risk_score.toFixed(3)}</td>
                <td>{w.is_baseline ? "—" : <Chip kind={w.alert_level} />}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </>
  );
}

/* ---------- recovery tab (UI-10) ---------- */
function RecoveryTab({
  wlId,
  cand,
  isOperator,
  onRestore,
}: {
  wlId: string;
  cand: SelectorReport;
  isOperator: boolean;
  onRestore: (sid: string) => void;
}) {
  void wlId;
  const avail = cand.status === "candidate_available";
  const rec = cand.candidates.find((c) => c.snapshot_id === cand.recommended_snapshot_id);
  return (
    <>
      {avail && rec ? (
        <Banner tone="ok" title={`Recommendation: ${rec.snapshot_id}`}>
          newest defensible recovery point · requires operator approval · recovery gap{" "}
          {rec.data_loss_minutes.toFixed(1)} min
        </Banner>
      ) : (
        <Banner tone="error" title="no_safe_candidate — CleanPoint refuses to recommend a restore point">
          The UI never falls back to the newest backup. Reasons:
          <ul className="explain">
            {(cand.reasons ?? ["no defensible candidate"]).map((r, i) => (
              <li key={i}>{r}</li>
            ))}
          </ul>
        </Banner>
      )}
      <table className="tbl">
        <thead>
          <tr>
            <th className="num">Rank</th>
            <th>Captured</th>
            <th>Snapshot</th>
            <th>Integrity</th>
            <th>Exposure</th>
            <th>Eligibility</th>
            <th>Recovery gap</th>
            <th>Reasons</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {cand.candidates.map((c, i) => (
            <tr key={c.snapshot_id} className={c.eligible ? "row-good" : ""}>
              <td className="num">{i + 1}</td>
              <td className="path">{fmtTime(c.captured_at)}</td>
              <td className="path">{c.snapshot_id}</td>
              <td><Chip kind={c.integrity_ok ? "verified" : "failed"} label={c.integrity_ok ? "OK" : "FAILED"} /></td>
              <td><Chip kind={c.exposure_status} /></td>
              <td><Chip kind={c.eligible ? "eligible" : "ineligible"} /></td>
              <td><GapBar minutes={c.data_loss_minutes} max={maxGap(cand.candidates)} /></td>
              <td className="reasons">{c.reasons.slice(0, 2).map((r, j) => <div key={j}>{r}</div>)}</td>
              <td>
                {c.eligible && isOperator && (
                  <button className="btn primary" onClick={() => onRestore(c.snapshot_id)}>
                    Restore…
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="hint">
        Ranked newest-first. Eligibility = sealed + integrity verified + exposure clear + sufficient
        baseline coverage + captured strictly before the earliest plausible onset. Recovery gap is an
        RPO proxy, not business loss.
      </p>
    </>
  );
}

/* ---------- restore dialog (UI-11) + report (UI-12) ---------- */
function RestoreDialog({
  wlId,
  snapId,
  onClose,
  onDone,
}: {
  wlId: string;
  snapId: string;
  onClose: () => void;
  onDone: (rep: RestoreReport) => void;
}) {
  const [target, setTarget] = React.useState(`recovery-${snapId.slice(5, 13)}`);
  const [confirm, setConfirm] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [err, setErr] = React.useState("");
  const go = async () => {
    setBusy(true);
    setErr("");
    try {
      const rep = await api<RestoreReport>("POST", `/workloads/${wlId}/restore`, {
        snapshot_id: snapId,
        target_name: target.trim(),
        confirm: true,
      });
      onDone(rep);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setBusy(false);
    }
  };
  return (
    <Modal title="Restore confirmation" onClose={onClose}>
      <Banner tone="warn" title="Isolated sandbox restore — explicit operator approval">
        Copies the snapshot into a server-generated directory under <code>sandbox/restored/</code>.
        The protected source is never modified. Eligibility and object hashes are re-verified at restore time.
      </Banner>
      <KV
        rows={[
          ["Snapshot", <span className="path">{snapId}</span>],
          ["Known caveats", "Integrity proves consistency, not absence of compromise. Every action is audit-logged."],
        ]}
      />
      <label className="fld" htmlFor="target">Target name (label for the run directory, [a-z0-9-])</label>
      <input id="target" value={target} onChange={(e) => setTarget(e.target.value)} pattern="[a-z0-9][a-z0-9-]{0,63}" />
      <div className="checkline">
        <input type="checkbox" id="confirm" checked={confirm} onChange={(e) => setConfirm(e.target.checked)} />
        <label htmlFor="confirm">
          I am the operator and I approve this restore of <b>{snapId}</b>.
        </label>
      </div>
      <p className="login-error" role="alert">{err}</p>
      <div className="actions">
        <button className="btn" onClick={onClose}>Cancel</button>
        <button className="btn danger" onClick={go} disabled={!confirm || busy}>
          {busy ? "Restoring & verifying…" : "Execute restore"}
        </button>
      </div>
    </Modal>
  );
}

function RunReportBody({ rep }: { rep: RestoreReport }) {
  return (
    <>
      <Banner tone={rep.status === "verified" ? "ok" : "error"}
        title={`${rep.status.toUpperCase()} — ${num(rep.verified_files)}/${num(rep.total_files)} files verified ${rep.source_unchanged ? "· source snapshot unchanged" : "· SOURCE CHANGE DETECTED"}`}>
        Manifest match is separate from synthetic ground truth (evaluated offline, never by the detector).
      </Banner>
      <KV
        rows={[
          ["Output directory", <span className="path">{rep.output_dir}</span>],
          ["Manifest SHA-256", <span className="path">{rep.manifest_hash}</span>],
          ["Started / finished", `${fmtTime(rep.started_at)} → ${fmtTime(rep.finished_at)}`],
          ...(rep.reasons?.length ? ([["Reasons", rep.reasons.join("; ")]] as [string, React.ReactNode][]) : []),
        ]}
      />
      <details>
        <summary>Per-file verification ({rep.files.length})</summary>
        <table className="tbl" style={{ marginTop: 8 }}>
          <thead>
            <tr>
              <th>Path</th>
              <th>Status</th>
              <th className="num">Bytes</th>
              <th>SHA-256</th>
            </tr>
          </thead>
          <tbody>
            {rep.files.map((f) => (
              <tr key={f.path}>
                <td className="path">{f.path}</td>
                <td><Chip kind={f.status === "verified" ? "verified" : "failed"} label={f.status} /></td>
                <td className="num">{num(f.size_bytes)}</td>
                <td className="path">{f.sha256 ? f.sha256.slice(0, 16) + "…" : f.reason ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </>
  );
}
