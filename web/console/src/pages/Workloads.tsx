import React from "react";
import { useNavigate } from "react-router-dom";
import { api, Workload, Snapshot, fmtTime } from "../api";
import { Chip, Modal, Loading, ErrorBanner } from "../components";
import { Session } from "../App";

// UI-03: search, created time, last snapshot, status; Create Workload
// (operator). Roots are always server-generated inside the allowlist.
export default function Workloads() {
  const nav = useNavigate();
  const { role } = React.useContext(Session);
  const [rows, setRows] = React.useState<{ wl: Workload; snaps: Snapshot[] }[] | null>(null);
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
        const out = [];
        for (const wl of workloads) {
          const s = await api<{ snapshots: Snapshot[] }>("GET", `/workloads/${wl.id}/snapshots`).catch(() => ({ snapshots: [] }));
          out.push({ wl, snaps: s.snapshots });
        }
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

  if (error) return <ErrorBanner error={error} />;
  if (!rows) return <Loading />;

  const filtered = rows.filter((r) => r.wl.name.toLowerCase().includes(q.toLowerCase()));
  const lastSnap = (r: { snaps: Snapshot[] }) => r.snaps[0];

  return (
    <>
      <h2 className="page">
        Workloads
        {role === "operator" && (
          <button className="btn primary" style={{ marginLeft: "auto" }} onClick={() => setCreating(true)}>
            + Register workload
          </button>
        )}
      </h2>
      <p className="hint">
        Workload roots are sandbox-allowlisted and server-generated — the client never supplies an unrestricted destination.
      </p>
      <input
        type="text"
        placeholder="search by name…"
        value={q}
        onChange={(e) => setQ(e.target.value)}
        style={{ maxWidth: 320, marginBottom: 14 }}
        aria-label="Search workloads"
      />
      {filtered.length === 0 ? (
        <div className="empty">No workloads match.</div>
      ) : (
        <table className="tbl">
          <thead>
            <tr>
              <th>Name</th>
              <th>ID</th>
              <th>Created</th>
              <th>Last snapshot</th>
              <th>Newest status</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((r) => (
              <tr
                key={r.wl.id}
                className="click"
                tabIndex={0}
                onClick={() => nav(`/workload/${r.wl.id}/overview`)}
                onKeyDown={(e) => e.key === "Enter" && nav(`/workload/${r.wl.id}/overview`)}
              >
                <td><b>{r.wl.name}</b></td>
                <td className="path">{r.wl.id}</td>
                <td>{fmtTime(r.wl.created_at)}</td>
                <td>{lastSnap(r) ? fmtTime(lastSnap(r)!.captured_at) : "—"}</td>
                <td>{lastSnap(r)?.assessment ? <Chip kind={lastSnap(r)!.assessment!.exposure_status} /> : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
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
