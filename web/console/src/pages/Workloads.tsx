import React from "react";
import { useNavigate } from "react-router-dom";
import { api, Workload, Snapshot, SelectorReport, fmtTime } from "../api";
import { Chip, Modal, Empty, PageHead, Skel } from "../components";
import { Session } from "../App";

interface Row {
  wl: Workload;
  snaps: Snapshot[];
  cand: SelectorReport | null;
}

// UI-03: search, created time, last snapshot, status; Create Workload.
// Roots are always server-generated inside the sandbox allowlist.
export default function Workloads() {
  const nav = useNavigate();
  const { role } = React.useContext(Session);
  const [rows, setRows] = React.useState<Row[] | null>(null);
  const [error, setError] = React.useState<unknown>(null);
  const [q, setQ] = React.useState("");
  const [creating, setCreating] = React.useState(false);
  const [name, setName] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [formErr, setFormErr] = React.useState("");

  const load = React.useCallback(() => {
    (async () => {
      try {
        const { workloads } = await api<{ workloads: Workload[] }>("GET", "/workloads");
        const out = await Promise.all(
          workloads.map(async (wl) => {
            const [snaps, cand] = await Promise.all([
              api<{ snapshots: Snapshot[] }>("GET", `/workloads/${wl.id}/snapshots`).catch(() => ({ snapshots: [] as Snapshot[] })),
              api<SelectorReport>("GET", `/workloads/${wl.id}/recovery-candidates`).catch(() => null),
            ]);
            return { wl, snaps: snaps.snapshots, cand };
          })
        );
        setRows(out);
        setError(null);
      } catch (e) {
        setError(e);
      }
    })();
  }, []);
  React.useEffect(load, [load]);

  const create = async () => {
    setBusy(true);
    setFormErr("");
    try {
      const wl = await api<Workload>("POST", "/workloads", { name: name.trim() });
      setCreating(false);
      nav(`/workload/${wl.id}/overview`);
    } catch (e) {
      setFormErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  if (error) return <PageHead title="Workloads">{null}</PageHead>;
  if (!rows)
    return (
      <div aria-busy="true">
        <Skel kind="title" />
        <div className="wl-cards">
          <Skel kind="card" n={3} />
        </div>
      </div>
    );

  const filtered = rows.filter((r) => r.wl.name.toLowerCase().includes(q.toLowerCase()));
  const withSnaps = rows.filter((r) => r.snaps.length).length;
  const abstained = rows.filter((r) => r.cand?.status === "no_safe_candidate").length;

  return (
    <>
      <PageHead
        title="Workloads"
        count={{ n: rows.length, label: rows.length === 1 ? "registered" : "registered" }}
      >
        <input
          type="text"
          placeholder="search by name…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          aria-label="Search workloads"
          style={{ maxWidth: 260 }}
        />
        <button className="btn" onClick={load} title="Refresh">⟳</button>
        {role === "operator" && (
          <button className="btn primary" onClick={() => setCreating(true)}>
            + Register workload
          </button>
        )}
      </PageHead>

      <div className="mini-stats">
        <div className="mini-stat"><b>{rows.length}</b><span>workloads</span></div>
        <div className="mini-stat"><b>{withSnaps}</b><span>with snapshot history</span></div>
        <div className="mini-stat"><b>{rows.length - abstained}</b><span>with a defensible candidate</span></div>
        <div className="mini-stat"><b className={abstained ? "red" : undefined}>{abstained}</b><span>abstained (no safe candidate)</span></div>
      </div>

      <p className="hint" style={{ marginBottom: 12 }}>
        Workload roots are sandbox-allowlisted and server-generated — the client never supplies an unrestricted destination.
      </p>

      {filtered.length === 0 ? (
        <Empty>
          {rows.length === 0 ? (
            <>
              No workloads yet.{" "}
              {role === "operator" ? (
                <>
                  Register one above, or{" "}
                  <button className="btn btnless" onClick={() => nav("/simulation")}>
                    run a simulation →
                  </button>
                </>
              ) : (
                "Ask an operator to run one from the Simulation Lab."
              )}
            </>
          ) : (
            "No workloads match your search."
          )}
        </Empty>
      ) : (
        <>
          <div className="wl-cards">
            {filtered.map((r, i) => (
              <div
                key={r.wl.id}
                className="wl-card"
                style={{ animationDelay: `${i * 60}ms` }}
                tabIndex={0}
                role="button"
                aria-label={`Open workload ${r.wl.name}`}
                onClick={() => nav(`/workload/${r.wl.id}/overview`)}
                onKeyDown={(e) => e.key === "Enter" && nav(`/workload/${r.wl.id}/overview`)}
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
                  {r.snaps.slice(0, 13).map((s) => (
                    <i key={s.id} className={`d-${s.assessment?.exposure_status ?? "none"}`}
                       title={`${s.captured_at} — ${s.assessment?.exposure_status ?? "no analysis"}`} />
                  ))}
                  {r.snaps.length === 0 && <span className="hint">no snapshots</span>}
                </div>
                <dl className="wl-meta">
                  <div>
                    <dt>created</dt>
                    <dd>{fmtTime(r.wl.created_at).slice(0, 16)}</dd>
                  </div>
                  <div>
                    <dt>snapshots</dt>
                    <dd>{r.snaps.length}</dd>
                  </div>
                  <div>
                    <dt>last capture</dt>
                    <dd>{r.snaps[0] ? fmtTime(r.snaps[0].captured_at).slice(11, 16) : "—"}</dd>
                  </div>
                  <div>
                    <dt>newest</dt>
                    <dd>{r.snaps[0]?.assessment ? <Chip kind={r.snaps[0].assessment.exposure_status} /> : "—"}</dd>
                  </div>
                </dl>
              </div>
            ))}
          </div>
        </>
      )}

      {creating && (
        <Modal title="Register workload" onClose={() => setCreating(false)}>
          <p className="hint">
            The server creates the root under its allowlisted sandbox — never an arbitrary host path.
          </p>
          <label className="fld" htmlFor="wlname">Name (lowercase, digits, dashes)</label>
          <input id="wlname" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. demo-batch-01" autoFocus />
          <p className="login-error" role="alert">{formErr}</p>
          <div className="actions">
            <button className="btn" onClick={() => setCreating(false)}>Cancel</button>
            <button className="btn primary" onClick={create} disabled={busy || !name.trim()}>
              {busy ? "Registering…" : "Register"}
            </button>
          </div>
        </Modal>
      )}
    </>
  );
}
