import React from "react";
import { api } from "../api";
import { Banner, ErrorBanner, SkeletonPage } from "../components";

function RaceRow({ name, desc, unsafe, total, hero }: { name: string; desc: string; unsafe: number; total: number; hero?: boolean }) {
  const pct = Math.max(2, Math.round((unsafe / Math.max(1, total)) * 100));
  return (
    <div className={"race-row" + (hero ? " hero" : unsafe > 0 ? " bad" : "")}>
      <div className="rr-label">
        <b>{name}</b>
        <span>{desc}</span>
      </div>
      <div className="rr-track" role="img" aria-label={`${name}: ${unsafe} of ${total} contaminated`}>
        <span style={{ width: `${pct}%` }} />
      </div>
      <div className="rr-num">
        <b>{unsafe}</b>
        <i>/{total}</i>
      </div>
    </div>
  );
}

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
  if (!rep) return <SkeletonPage />;

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
      <div className="hero-band">
        <div className="hb-item">
          <b>{String(m.unsafe_selections ?? "—")}</b>
          <span>unsafe selections · release target 0</span>
        </div>
        <div className="hb-item">
          <b>{m.selection_success_num ?? 0}<i>/{m.selection_success_den ?? 0}</i></b>
          <span>newest-eligible-clean chosen</span>
        </div>
        <div className="hb-item">
          <b>{m.attack_detected ?? 0}<i>/{m.attack_scenarios ?? 0}</i></b>
          <span>attacks detected</span>
        </div>
        <div className="hb-item">
          <b>{m.restores_verified ?? 0}<i>/{m.restores_performed ?? 0}</i></b>
          <span>verified restores · sources unchanged</span>
        </div>
      </div>

      <div className="card">
        <h3 className="sec" style={{ marginTop: 0 }}>The selector race — same evidence, three policies</h3>
        <p className="hint" style={{ marginBottom: 16 }}>
          Contaminated choices per policy across {m.baseline_selector_den ?? m.scenarios} scored scenarios.
          Shorter is safer.
        </p>
        <div className="race">
          <RaceRow
            name="newest snapshot"
            desc="ignores all evidence — the default policy everywhere else"
            unsafe={Number(m.baseline_selector_unsafe?.newest ?? 0)}
            total={Number(m.baseline_selector_den || m.scenarios || 1)}
          />
          <RaceRow
            name="integrity-only"
            desc="newest snapshot whose hashes verify — still blind to contamination"
            unsafe={Number(m.baseline_selector_unsafe?.integrity_only ?? 0)}
            total={Number(m.baseline_selector_den || m.scenarios || 1)}
          />
          <RaceRow
            name="CleanPoint"
            desc="evidence-ranked, may abstain — this system"
            unsafe={Number(m.unsafe_selections ?? 0)}
            total={Number(m.baseline_selector_den || m.scenarios || 1)}
            hero
          />
        </div>
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
