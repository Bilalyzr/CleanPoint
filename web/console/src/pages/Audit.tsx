import React from "react";
import { api, fmtTime } from "../api";
import { Chip, Banner, PageHead, Skel } from "../components";

// UI-14: actor, action, target, outcome, created. Secrets are never recorded;
// operator-only access policy.
export default function Audit() {
  const [events, setEvents] = React.useState<any[] | null>(null);
  const [error, setError] = React.useState<unknown>(null);
  const [q, setQ] = React.useState("");
  const load = React.useCallback(() => {
    api<{ audit_events: any[] }>("GET", "/audit-events")
      .then((d) => setEvents(d.audit_events))
      .catch(setError);
  }, []);
  React.useEffect(load, [load]);
  if (error)
    return (
      <>
        <PageHead title="Audit log" />
        <Banner tone="error" title="Operator access required">
          {error instanceof Error ? error.message : String(error)}
        </Banner>
      </>
    );
  if (!events)
    return (
      <div aria-busy="true">
        <Skel kind="title" />
        <Skel kind="block" />
        <Skel kind="block" />
      </div>
    );
  const filtered = events.filter(
    (a) =>
      !q ||
      a.action.toLowerCase().includes(q.toLowerCase()) ||
      a.target_id.toLowerCase().includes(q.toLowerCase()) ||
      a.actor.toLowerCase().includes(q.toLowerCase())
  );
  const rejected = events.filter((a) => String(a.outcome).startsWith("rejected")).length;
  return (
    <>
      <PageHead title="Audit log" count={{ n: events.length, label: "events" }}>
        <input type="text" placeholder="filter by action/target/actor…" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Filter audit events" />
        <button className="btn" onClick={load} title="Refresh">⟳</button>
      </PageHead>
      <div className="mini-stats">
        <div className="mini-stat"><b>{events.length}</b><span>recorded actions</span></div>
        <div className="mini-stat"><b className={rejected ? "red" : "green"}>{rejected}</b><span>rejected attempts</span></div>
        <div className="mini-stat"><b>{events.filter((a) => a.action.startsWith("restore")).length}</b><span>restore operations</span></div>
        <div className="mini-stat"><b>{events.filter((a) => a.action.startsWith("simulate")).length}</b><span>simulations started</span></div>
      </div>
      <p className="hint" style={{ marginBottom: 12 }}>State-changing operations only. Secrets are never recorded — actor, action, target, outcome.</p>
      {filtered.length === 0 ? (
        <div className="empty">{events.length === 0 ? "No audit events yet." : "No events match your filter."}</div>
      ) : (
        <table className="tbl">
          <thead>
            <tr>
              <th className="num">#</th>
              <th>Time</th>
              <th>Actor</th>
              <th>Action</th>
              <th>Target</th>
              <th>Outcome</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((a) => (
              <tr key={a.id}>
                <td className="num">{a.id}</td>
                <td className="path">{fmtTime(a.created_at)}</td>
                <td>{a.actor}</td>
                <td className="path">{a.action}</td>
                <td className="path">{a.target_id}</td>
                <td>
                  <Chip
                    kind={String(a.outcome).startsWith("rejected") ? "failed" : "verified"}
                    label={a.outcome}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}
