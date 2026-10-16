import React from "react";
import type { Window, Snapshot, Candidate } from "./api";

// Risk chart: hand-rolled SVG (PRD: "Recharts or simple SVG" — we keep the
// zero-dependency option). Baseline band, threshold, onset marker, tooltips.
export function RiskChart({
  windows,
  threshold,
  onset,
  baselineCount,
  height = 260,
}: {
  windows: Window[];
  threshold: number;
  onset: string | null;
  baselineCount: number;
  height?: number;
}) {
  if (!windows.length) return <div className="empty">No windows.</div>;
  const W = 960, H = height, P = { l: 46, r: 16, t: 18, b: 30 };
  const iw = W - P.l - P.r, ih = H - P.t - P.b;
  const t0 = Date.parse(windows[0].starts_at);
  const t1 = Date.parse(windows[windows.length - 1].starts_at) || t0 + 1;
  const x = (iso: string) => P.l + ((Date.parse(iso) - t0) / (t1 - t0)) * iw;
  const y = (r: number) => P.t + (1 - Math.min(r, 1)) * ih;
  const ev = windows.filter((w) => !w.is_baseline);
  const line = ev.map((w, i) => `${i ? "L" : "M"}${x(w.starts_at).toFixed(1)},${y(w.risk_score).toFixed(1)}`).join(" ");
  const baseEnd = baselineCount < windows.length ? windows[baselineCount - 1].ends_at : null;
  const [hover, setHover] = React.useState<Window | null>(null);

  return (
    <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`Risk index per window; threshold ${threshold}; suspicious windows marked red${onset ? "; onset marker present" : ""}`}>
      {[0, 0.25, 0.5, 0.75, 1].map((v) => (
        <g key={v}>
          <line x1={P.l} y1={y(v)} x2={W - P.r} y2={y(v)} stroke="#E4EAF1" />
          <text x={P.l - 8} y={y(v) + 4} textAnchor="end" fontSize="10" fill="#5B6B7C">{v.toFixed(2)}</text>
        </g>
      ))}
      {baseEnd && (
        <>
          <rect x={P.l} y={P.t} width={x(baseEnd) - P.l} height={ih} fill="rgba(17,37,63,.07)" />
          <text x={P.l + 6} y={P.t + 13} fontSize="10" fill="#5B6B7C">baseline (assumed normal)</text>
        </>
      )}
      <line x1={P.l} y1={y(threshold)} x2={W - P.r} y2={y(threshold)} stroke="#9A6700" strokeDasharray="5 4" />
      <text x={W - P.r - 4} y={y(threshold) - 5} textAnchor="end" fontSize="10" fill="#9A6700">threshold {threshold}</text>
      <path d={line} fill="none" stroke="#14628D" strokeWidth="1.6" />
      {ev.map((w) => (
        <circle
          key={w.id}
          cx={x(w.starts_at)}
          cy={y(w.risk_score)}
          r={w.alert_level === "suspicious" ? 4.5 : 3}
          fill={w.alert_level === "suspicious" ? "#C0392B" : "#14628D"}
          style={{ cursor: "pointer" }}
          onClick={() => setHover(w)}
        >
          <title>{`${w.starts_at} — risk ${w.risk_score.toFixed(3)} (${w.alert_level})`}</title>
        </circle>
      ))}
      {onset && (
        <>
          <line x1={x(onset)} y1={P.t} x2={x(onset)} y2={P.t + ih} stroke="#C0392B" strokeWidth="2" strokeDasharray="6 4" />
          <text x={x(onset) + 5} y={P.t + 12} fill="#C0392B" fontSize="11" fontWeight="700">onset</text>
        </>
      )}
      {hover && (
        <g>
          <rect x={8} y={8} width="300" height="86" rx="8" fill="#11253F" opacity="0.96" />
          <text x={20} y={30} fontSize="12" fill="#EAF2FA" fontFamily="monospace">{hover.starts_at}</text>
          <text x={20} y={48} fontSize="12" fill="#9FB4C9">risk {hover.risk_score.toFixed(3)} · {hover.event_count} events</text>
          {(hover.signal_contributions ?? []).filter((c) => c.signal >= 0.2).slice(0, 2).map((c, i) => (
            <text key={c.feature} x={20} y={64 + i * 14} fontSize="11" fill="#7FC5E8" fontFamily="monospace">
              {c.feature} ×{c.signal.toFixed(2)}
            </text>
          ))}
        </g>
      )}
    </svg>
  );
}

// Snapshot timeline: chronological nodes colored+labelled by assessment.
export function SnapshotTimeline({
  snapshots,
  onSelect,
}: {
  snapshots: Snapshot[];
  onSelect: (s: Snapshot) => void;
}) {
  const chrono = [...snapshots].reverse();
  return (
    <div className="timeline" role="list" aria-label="Snapshot timeline">
      <div className="tl-track">
        {chrono.map((s, i) => {
          const exp = s.assessment?.exposure_status;
          const cls = exp ? `c-${exp}` : "c-none";
          return (
            <button
              key={s.id}
              role="listitem"
              className={`tl-node btnless ${cls}`}
              onClick={() => onSelect(s)}
              title={`${s.captured_at} — ${exp ?? "no analysis"}`}
            >
              {i > 0 && <span className="tl-stem" aria-hidden="true" />}
              <span className="tl-cap">{exp ?? "no analysis"}</span>
              <span className="tl-dot" aria-hidden="true" />
              <span className="tl-time">{s.captured_at.slice(5, 16).replace("T", " ")}</span>
              <span className="tl-files">{s.file_count} files</span>
            </button>
          );
        })}
      </div>
      <div className="tl-legend" aria-hidden="true">
        {["clear", "suspect", "affected", "unknown"].map((k) => (
          <span key={k} className={`tl-leg c-${k}`}><i />{k}</span>
        ))}
      </div>
    </div>
  );
}

// Mini activity sparkline (events per window) for workload overview.
export function ActivitySpark({ windows }: { windows: Window[] }) {
  const W = 320, H = 60;
  const max = Math.max(1, ...windows.map((w) => w.event_count));
  const bw = W / Math.max(1, windows.length);
  return (
    <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Events per window activity trend">
      {windows.map((w, i) => {
        const h = (w.event_count / max) * (H - 12);
        return (
          <rect
            key={w.id}
            x={i * bw + 1}
            y={H - h}
            width={Math.max(1.5, bw - 2)}
            height={h}
            rx="1.5"
            fill={w.alert_level === "suspicious" ? "#C0392B" : w.is_baseline ? "#7FA8C9" : "#14628D"}
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
  const pct = max > 0 ? Math.max(3, (minutes / max) * 100) : 3;
  return (
    <div className="gapbar" title={`${minutes.toFixed(1)} min recovery gap`}>
      <span style={{ width: `${pct}%` }} />
      <em>{minutes.toFixed(0)}m</em>
    </div>
  );
}

export function maxGap(candidates: Candidate[]): number {
  return Math.max(1, ...candidates.map((c) => c.data_loss_minutes));
}
