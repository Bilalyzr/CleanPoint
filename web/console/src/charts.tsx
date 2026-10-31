import React from "react";
import type { Window, Snapshot, Candidate } from "./api";

// Risk chart: zero-dependency precision SVG spectrometer with gradient area fill,
// baseline calibration band, threshold line, onset beacon, and interactive crosshair HUD.
export function RiskChart({
  windows,
  threshold,
  onset,
  baselineCount,
  height = 278,
}: {
  windows: Window[];
  threshold: number;
  onset: string | null;
  baselineCount: number;
  height?: number;
}) {
  const [locked, setLocked] = React.useState<Window | null>(null);
  const [scrub, setScrub] = React.useState<Window | null>(null);

  if (!windows.length) return <div className="empty">No windows.</div>;

  const W = 960;
  const H = height;
  const P = { l: 50, r: 20, t: 22, b: 32 };
  const iw = W - P.l - P.r;
  const ih = H - P.t - P.b;
  const t0 = Date.parse(windows[0].starts_at);
  const t1 = Date.parse(windows[windows.length - 1].starts_at) || t0 + 1;
  const x = (iso: string) => P.l + ((Date.parse(iso) - t0) / Math.max(1, t1 - t0)) * iw;
  const y = (r: number) => P.t + (1 - Math.max(0, Math.min(r, 1))) * ih;

  const ev = windows.filter((w) => !w.is_baseline);
  const line = ev
    .map((w, i) => `${i ? "L" : "M"}${x(w.starts_at).toFixed(1)},${y(w.risk_score).toFixed(1)}`)
    .join(" ");
  const area =
    ev.length > 1
      ? `${line} L${x(ev[ev.length - 1].starts_at).toFixed(1)},${(P.t + ih).toFixed(1)} L${x(ev[0].starts_at).toFixed(1)},${(P.t + ih).toFixed(1)} Z`
      : "";
  const baseEnd =
    baselineCount > 0 && baselineCount < windows.length
      ? windows[baselineCount - 1].ends_at
      : null;

  const active = locked ?? scrub;

  const onMove = (e: React.MouseEvent<SVGSVGElement>) => {
    if (!ev.length) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const vx = ((e.clientX - rect.left) / Math.max(1, rect.width)) * W;
    let nearest = ev[0];
    let best = Infinity;
    for (const w of ev) {
      const d = Math.abs(x(w.starts_at) - vx);
      if (d < best) {
        best = d;
        nearest = w;
      }
    }
    setScrub(nearest);
  };

  const activeX = active ? x(active.starts_at) : 0;
  const activeY = active ? y(active.risk_score) : 0;
  const hudOnRight = activeX < W * 0.54;
  const hudX = hudOnRight ? W - 324 : P.l + 10;

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      role="img"
      aria-label={`Risk index per window; threshold ${threshold}; suspicious windows marked red${onset ? "; onset marker present" : ""}`}
      onMouseMove={onMove}
      onMouseLeave={() => setScrub(null)}
    >
      <defs>
        <linearGradient id="cpRiskArea" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="var(--chart-line)" stopOpacity="0.34" />
          <stop offset="100%" stopColor="var(--chart-line)" stopOpacity="0.0" />
        </linearGradient>
        <linearGradient id="cpBaseBand" x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor="var(--chart-base)" stopOpacity="0.18" />
          <stop offset="100%" stopColor="var(--chart-base)" stopOpacity="0.06" />
        </linearGradient>
      </defs>

      {[0, 0.25, 0.5, 0.75, 1].map((v) => (
        <g key={v}>
          <line x1={P.l} y1={y(v)} x2={W - P.r} y2={y(v)} stroke="var(--chart-grid)" strokeWidth="1" />
          <text
            x={P.l - 9}
            y={y(v) + 4}
            textAnchor="end"
            fontSize="10.5"
            fontFamily="var(--mono)"
            fill="var(--chart-muted)"
          >
            {v.toFixed(2)}
          </text>
        </g>
      ))}

      {baseEnd && (
        <g>
          <rect
            x={P.l}
            y={P.t}
            width={Math.max(0, x(baseEnd) - P.l)}
            height={ih}
            fill="url(#cpBaseBand)"
            rx="4"
          />
          <line
            x1={x(baseEnd)}
            y1={P.t}
            x2={x(baseEnd)}
            y2={P.t + ih}
            stroke="var(--chart-grid)"
            strokeDasharray="3 3"
          />
          <text x={P.l + 8} y={P.t + 15} fontSize="10.5" fill="var(--chart-muted)">
            baseline ({baselineCount} windows · assumed normal)
          </text>
        </g>
      )}

      <line
        x1={P.l}
        y1={y(threshold)}
        x2={W - P.r}
        y2={y(threshold)}
        stroke="var(--amber)"
        strokeWidth="1.4"
        strokeDasharray="6 4"
      />
      <text
        x={W - P.r - 4}
        y={y(threshold) - 6}
        textAnchor="end"
        fontSize="10.5"
        fontFamily="var(--mono)"
        fill="var(--amber)"
      >
        threshold {threshold}
      </text>

      {area && <path d={area} fill="url(#cpRiskArea)" />}
      {line && (
        <path
          d={line}
          fill="none"
          stroke="var(--chart-line)"
          strokeWidth="2.2"
          strokeLinejoin="round"
          strokeLinecap="round"
        />
      )}

      {onset && (
        <g>
          <line
            x1={x(onset)}
            y1={P.t}
            x2={x(onset)}
            y2={P.t + ih}
            stroke="var(--red)"
            strokeWidth="2"
            strokeDasharray="6 4"
          />
          <rect
            x={Math.min(W - P.r - 92, x(onset) + 5)}
            y={P.t + 2}
            width="88"
            height="18"
            rx="4"
            fill="var(--red-bg)"
            stroke="var(--red)"
            strokeOpacity="0.45"
          />
          <text
            x={Math.min(W - P.r - 48, x(onset) + 49)}
            y={P.t + 14}
            textAnchor="middle"
            fill="var(--red)"
            fontSize="10"
            fontFamily="var(--mono)"
            fontWeight="700"
          >
            ONSET {onset.slice(11, 16)}
          </text>
        </g>
      )}

      {active && (
        <line
          x1={activeX}
          y1={P.t}
          x2={activeX}
          y2={P.t + ih}
          stroke="var(--chart-line)"
          strokeOpacity="0.45"
          strokeDasharray="2 3"
        />
      )}

      {ev.map((w) => {
        const sus = w.alert_level === "suspicious";
        const isSel = active?.id === w.id;
        return (
          <g key={w.id}>
            {(sus || isSel) && (
              <circle
                cx={x(w.starts_at)}
                cy={y(w.risk_score)}
                r={isSel ? 9 : 7}
                fill={sus ? "var(--red)" : "var(--chart-line)"}
                opacity={isSel ? 0.28 : 0.16}
              />
            )}
            <circle
              cx={x(w.starts_at)}
              cy={y(w.risk_score)}
              r={isSel ? 5.5 : sus ? 4.4 : 3.1}
              fill={sus ? "var(--red)" : "var(--chart-line)"}
              stroke="var(--surface)"
              strokeWidth="1.4"
              style={{ cursor: "pointer" }}
              onClick={() => setLocked((cur) => (cur?.id === w.id ? null : w))}
            >
              <title>{`${w.starts_at} — risk ${w.risk_score.toFixed(3)} (${w.alert_level})`}</title>
            </circle>
          </g>
        );
      })}

      {active && (
        <g transform={`translate(${hudX}, 10)`}>
          <rect
            x={0}
            y={0}
            width="304"
            height="104"
            rx="10"
            fill="var(--hud-bg)"
            stroke="var(--hud-border)"
            strokeWidth="1"
          />
          <circle
            cx={16}
            cy={19}
            r={4}
            fill={active.alert_level === "suspicious" ? "var(--red)" : "var(--green)"}
          />
          <text x={26} y={23} fontSize="11.5" fill="var(--hud-text)" fontFamily="var(--mono)">
            {active.starts_at.slice(0, 19).replace("T", " ")}
          </text>
          {locked && (
            <text x={292} y={22} textAnchor="end" fontSize="9.5" fill="var(--chart-line)" fontFamily="var(--mono)">
              PINNED
            </text>
          )}
          <text x={16} y={42} fontSize="12" fill="var(--hud-sub)">
            risk <tspan fill="var(--hud-text)" fontWeight="700">{active.risk_score.toFixed(3)}</tspan> ·{" "}
            {active.event_count} events · {active.alert_level}
          </text>
          {(active.signal_contributions ?? [])
            .filter((c) => !c.missing)
            .sort((a, b) => b.signal * b.weight - a.signal * a.weight)
            .slice(0, 2)
            .map((c, i) => {
              const barW = Math.max(4, Math.min(92, c.signal * 92));
              return (
                <g key={c.feature} transform={`translate(16, ${54 + i * 20})`}>
                  <text x={0} y={10} fontSize="10.5" fill="var(--hud-accent)" fontFamily="var(--mono)">
                    {c.feature.slice(0, 18)}
                  </text>
                  <rect x={144} y={2} width={92} height={8} rx={4} fill="rgba(148,163,184,0.18)" />
                  <rect x={144} y={2} width={barW} height={8} rx={4} fill="var(--chart-line)" />
                  <text x={274} y={10} textAnchor="end" fontSize="10" fill="var(--hud-text)" fontFamily="var(--mono)">
                    ×{c.signal.toFixed(2)}
                  </text>
                </g>
              );
            })}
          <circle
            cx={activeX - hudX}
            cy={activeY - 10}
            r={0}
            fill="none"
          />
        </g>
      )}
    </svg>
  );
}

// Snapshot timeline: chronological nodes colored+labelled by assessment + recommended crown.
export function SnapshotTimeline({
  snapshots,
  recommendedId,
  onSelect,
}: {
  snapshots: Snapshot[];
  recommendedId?: string | null;
  onSelect: (s: Snapshot) => void;
}) {
  const chrono = [...snapshots].reverse();
  return (
    <div className="timeline" role="list" aria-label="Snapshot timeline">
      <div className="tl-track">
        {chrono.map((s, i) => {
          const exp = s.assessment?.exposure_status;
          const cls = exp ? `c-${exp}` : "c-none";
          const isRec = Boolean(recommendedId && s.id === recommendedId);
          return (
            <button
              key={s.id}
              type="button"
              role="listitem"
              className={`tl-node btnless ${cls}` + (isRec ? " is-rec" : "")}
              onClick={() => onSelect(s)}
              title={`${s.captured_at} — ${exp ?? "no analysis"}${isRec ? " (recommended clean point)" : ""}`}
            >
              {i > 0 && <span className="tl-stem" aria-hidden="true" />}
              <span className="tl-cap">
                {isRec ? "★ clean point" : exp ?? "no analysis"}
              </span>
              <span className="tl-dot" aria-hidden="true" />
              <span className="tl-time">{s.captured_at.slice(5, 16).replace("T", " ")}</span>
              <span className="tl-files">{s.file_count} files</span>
            </button>
          );
        })}
      </div>
      <div className="tl-legend" aria-hidden="true">
        {["clear", "suspect", "affected", "unknown"].map((k) => (
          <span key={k} className={`tl-leg c-${k}`}>
            <i />
            {k}
          </span>
        ))}
        {recommendedId && (
          <span className="tl-leg c-clear">
            <i />★ 5-gate recommended clean point
          </span>
        )}
      </div>
    </div>
  );
}

// Mini activity sparkline (events per window) for workload overview.
export function ActivitySpark({ windows }: { windows: Window[] }) {
  const W = 420;
  const H = 68;
  const max = Math.max(1, ...windows.map((w) => w.event_count));
  const bw = W / Math.max(1, windows.length);
  return (
    <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Events per window activity trend">
      <line x1={0} y1={H - 1} x2={W} y2={H - 1} stroke="var(--chart-grid)" strokeWidth="1" />
      {windows.map((w, i) => {
        const h = Math.max(2, (w.event_count / max) * (H - 12));
        const fill =
          w.alert_level === "suspicious"
            ? "var(--red)"
            : w.is_baseline
            ? "var(--chart-base)"
            : "var(--chart-line)";
        return (
          <rect
            key={w.id}
            x={i * bw + 1.2}
            y={H - h - 1}
            width={Math.max(1.8, bw - 2.4)}
            height={h}
            rx="2"
            fill={fill}
            opacity={w.is_baseline ? 0.55 : 0.9}
          >
            <title>{`${w.starts_at}: ${w.event_count} events${w.alert_level === "suspicious" ? " (suspicious)" : ""}`}</title>
          </rect>
        );
      })}
    </svg>
  );
}

// Candidate eligibility bar for the comparison table.
export function GapBar({ minutes, max }: { minutes: number; max: number }) {
  const pct = max > 0 ? Math.max(4, (minutes / max) * 100) : 4;
  return (
    <div className="gapbar" title={`${minutes.toFixed(1)} min recovery gap`}>
      <div className="gapbar-track">
        <span style={{ width: `${pct}%` }} />
      </div>
      <em>{minutes.toFixed(0)}m</em>
    </div>
  );
}

export function maxGap(candidates: Candidate[]): number {
  return Math.max(1, ...candidates.map((c) => c.data_loss_minutes));
}
