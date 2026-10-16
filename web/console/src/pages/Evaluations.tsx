import React from "react";
import { api } from "../api";
import { Stat, Banner, Loading, ErrorBanner } from "../components";

interface EvalReport {
  available?: boolean;
  note?: string;
  metrics?: Record<string, any>;
  methodology?: Record<string, any>;
  results?: any[];
}

// UI-13: scenario seeds, metrics, newest/integrity-only/CleanPoint
// comparisons. Serves only measured artifacts — never invented results.
export default function Evaluations() {
  const [rep, setRep] = React.useState<EvalReport | null>(null);
  const [error, setError] = React.useState<unknown>(null);

  React.useEffect(() => {
    api<EvalReport>("GET", "/evaluations").then(setRep).catch(setError);
  }, []);

  if (error) return <ErrorBanner error={error} />;
  if (!rep) return <Loading />;

  if (rep.available === false) {
    return (
      <>
        <h2 className="page">Evaluations & benchmarks</h2>
        <Banner tone="warn" title="No evaluation report on disk">
          {rep.note}
        </Banner>
      </>
    );
  }

  const m = rep.metrics ?? {};
  const rows: [string, any][] = [
    ["Scenarios evaluated", m.scenarios],
    ["Selections made", m.scenarios_with_selection],
    ["Unsafe selections (release-blocking)", `${m.unsafe_selections} (${m.unsafe_selection_rate ?? "—"} rate)`],
    ["Selection success (newest-eligible-clean)", `${m.selection_success_num}/${m.selection_success_den}`],
    ["Known-attack recall", `${m.attack_detected}/${m.attack_scenarios}`],
    ["Benign-window alert rate", `${m.benign_flagged_windows}/${m.benign_windows}`],
    ["Pre-existing abstention correct", `${m.pre_existing_abstained}/${m.pre_existing_total}`],
    ["Restores verified / performed", `${m.restores_verified}/${m.restores_performed}`],
    ["All sources unchanged", String(m.all_restores_source_unchanged)],
    ["Detection delay (min)", Array.isArray(m.detection_delay_minutes) ? m.detection_delay_minutes.join(", ") : "—"],
  ];

  return (
    <>
      <h2 className="page">Evaluations &amp; benchmarks</h2>
      <Banner tone="info" title="Measured evidence — not production statistics">
        Locked corpus (8 families × 2 held-out seeds). Selector comparisons are reported separately
        and never merged into a single accuracy figure. Reports live in <code>results/</code>.
      </Banner>
      <div className="cards">
        <Stat n={String(m.unsafe_selections ?? "—")} label="unsafe selections (target 0)" warn={Number(m.unsafe_selections) > 0} />
        <Stat n={`${m.selection_success_num ?? 0}/${m.selection_success_den ?? 0}`} label="newest-eligible-clean chosen" />
        <Stat n={`${m.attack_detected ?? 0}/${m.attack_scenarios ?? 0}`} label="attacks detected" />
        <Stat n={`${m.benign_flagged_windows ?? 0}/${m.benign_windows ?? 0}`} label="benign windows flagged" />
        <Stat n={`${m.restores_verified ?? 0}/${m.restores_performed ?? 0}`} label="verified restores" />
      </div>

      <div className="card">
        <h3 className="sec" style={{ marginTop: 0 }}>Selector comparison (same evidence)</h3>
        <table className="tbl">
          <thead>
            <tr>
              <th>Selector</th>
              <th className="num">Contaminated choices</th>
              <th>of scenarios</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><b>newest</b> sealed snapshot (ignores evidence)</td>
              <td className="num red"><b>{m.baseline_selector_unsafe?.newest ?? "—"}</b></td>
              <td className="num">{m.baseline_selector_den}</td>
            </tr>
            <tr>
              <td><b>integrity-only</b> newest valid-hash snapshot</td>
              <td className="num red"><b>{m.baseline_selector_unsafe?.integrity_only ?? "—"}</b></td>
              <td className="num">{m.baseline_selector_den}</td>
            </tr>
            <tr className="row-good">
              <td><b>CleanPoint</b> (may abstain)</td>
              <td className="num green"><b>{m.unsafe_selections ?? "—"}</b></td>
              <td className="num">{m.scenarios_with_selection} selections</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div className="card">
        <h3 className="sec" style={{ marginTop: 0 }}>Full metrics</h3>
        <dl className="kv">
          {rows.map(([k, v]) => (
            <React.Fragment key={k}>
              <dt>{k}</dt>
              <dd>{String(v)}</dd>
            </React.Fragment>
          ))}
        </dl>
        {rep.methodology && (
          <p className="hint">
            Splits: dev {JSON.stringify(rep.methodology.splits?.development)} · validation{" "}
            {JSON.stringify(rep.methodology.splits?.validation)} · locked {JSON.stringify(rep.methodology.splits?.locked)}
          </p>
        )}
      </div>

      <p className="hint">
        Regenerate from a terminal: <code>cleanpoint evaluate --scenario locked --seeds 9,10 --json --output results/eval-locked.json</code>
      </p>
    </>
  );
}
