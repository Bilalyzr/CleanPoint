import React from "react";
import { api, fmtTime } from "../api";
import { Chip, Loading, Banner } from "../components";

// UI-14: actor, action, target, outcome, created. Secrets are never recorded;
// operator-only access policy.
export default function Audit() {
  const [events, setEvents] = React.useState<any[] | null>(null);
  const [error, setError] = React.useState<unknown>(null);
  React.useEffect(() => {
    api<{ audit_events: any[] }>("GET", "/audit-events")
      .then((d) => setEvents(d.audit_events))
      .catch(setError);
  }, []);
  if (error)
    return (
      <>
        <h2 className="page">Audit log</h2>
        <Banner tone="error" title="Operator access required">
          {error instanceof Error ? error.message : String(error)}
        </Banner>
      </>
    );
  if (!events) return <Loading />;
  return (
    <>
      <h2 className="page">Audit log</h2>
      <p className="hint">State-changing operations only. Secrets are never recorded — actor, action, target, outcome.</p>
      {events.length === 0 ? (
        <div className="empty">No audit events yet.</div>
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
            {events.map((a) => (
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
