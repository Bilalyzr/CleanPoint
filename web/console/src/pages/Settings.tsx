import React from "react";
import { api, fmtBytes } from "../api";
import { Loading, ErrorBanner, KV } from "../components";

// UI-15: API URL, detector version, limits, CLI commands, exact trust
// boundaries. Never presented as an enterprise-protected deployment.
export default function Settings() {
  const [cfg, setCfg] = React.useState<any>(null);
  const [error, setError] = React.useState<unknown>(null);
  React.useEffect(() => {
    api("GET", "/config").then(setCfg).catch(setError);
  }, []);
  if (error) return <ErrorBanner error={error} />;
  if (!cfg) return <Loading />;
  return (
    <>
      <h2 className="page">Settings &amp; limitations</h2>
      <div className="card">
        <h3 className="sec" style={{ marginTop: 0 }}>Environment</h3>
        <KV
          rows={[
            ["Mode", <b>{String(cfg.mode)}</b>],
            ["Detector", `${cfg.detector_version} · window ${cfg.window_seconds}s · baseline ${cfg.baseline_windows} windows · threshold ${cfg.risk_threshold}`],
            ["Limits", `${cfg.max_files} files · ${fmtBytes(cfg.max_bytes)} per workload (synchronous MVP)`],
            ["API origin", window.location.origin],
          ]}
        />
      </div>
      <div className="card">
        <h3 className="sec" style={{ marginTop: 0 }}>Exact trust boundaries</h3>
        <ul className="explain">
          {(cfg.limits ?? []).map((x: string, i: number) => (
            <li key={i}>{x}</li>
          ))}
        </ul>
      </div>
      <div className="card">
        <h3 className="sec" style={{ marginTop: 0 }}>CLI commands</h3>
        <pre className="term">{(cfg.cli ?? []).join("\n")}</pre>
      </div>
    </>
  );
}
