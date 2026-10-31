import React from "react";
import { Snapshot, fmtTime, fmtBytes } from "./api";
import { Chip } from "./components";

interface ProjectedSlab {
  snap: Snapshot;
  index: number;
  cx: number;
  cy: number;
  hitRadius: number;
  depth: number;
}

export function SnapshotVault3D({
  snapshots,
  onset,
  recommendedId,
  abstained,
  onSelectSnapshot,
  height = 300,
}: {
  snapshots: Snapshot[];
  onset: string | null;
  recommendedId?: string | null;
  abstained?: boolean;
  onSelectSnapshot: (snapshotId: string) => void;
  height?: number;
}) {
  const canvasRef = React.useRef<HTMLCanvasElement | null>(null);
  const wrapRef = React.useRef<HTMLDivElement | null>(null);
  const slabsRef = React.useRef<ProjectedSlab[]>([]);
  const orbitRef = React.useRef({
    yaw: -0.34,
    pitch: 0.24,
    targetYaw: -0.34,
    targetPitch: 0.24,
    dragging: false,
    lastX: 0,
    lastY: 0,
    moved: 0,
  });

  const [hovered, setHovered] = React.useState<{
    snap: Snapshot;
    x: number;
    y: number;
    index: number;
  } | null>(null);

  // Chronological order (oldest -> newest)
  const chrono = React.useMemo(() => [...snapshots].reverse(), [snapshots]);

  const onsetBoundaryIndex = React.useMemo(() => {
    if (!onset || chrono.length === 0) return -1;
    const onsetMs = Date.parse(onset);
    if (Number.isNaN(onsetMs)) return -1;
    for (let i = 0; i < chrono.length; i++) {
      if (Date.parse(chrono[i].captured_at) >= onsetMs) {
        return i - 0.5;
      }
    }
    return chrono.length - 0.5;
  }, [chrono, onset]);

  const resetOrbit = () => {
    orbitRef.current.targetYaw = -0.34;
    orbitRef.current.targetPitch = 0.24;
  };

  React.useEffect(() => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    let visible = true;
    let rafId = 0;
    let width = 800;
    let h = height;
    let dpr = Math.min(window.devicePixelRatio || 1, 2);

    const particles = Array.from({ length: 56 }, (_, i) => ({
      x: ((i * 73) % 100) / 100 - 0.5,
      y: ((i * 41) % 100) / 100 - 0.5,
      z: ((i * 29) % 100) / 100 - 0.5,
      speed: 0.2 + (i % 7) * 0.06,
      size: 1.1 + (i % 3) * 0.65,
    }));

    const resize = () => {
      const rect = wrap.getBoundingClientRect();
      width = Math.max(300, Math.floor(rect.width));
      h = height;
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      canvas.width = Math.floor(width * dpr);
      canvas.height = Math.floor(h * dpr);
    };
    resize();

    const ro = new ResizeObserver(resize);
    ro.observe(wrap);

    const io = new IntersectionObserver(
      (entries) => {
        visible = entries[0]?.isIntersecting ?? true;
      },
      { threshold: 0.05 }
    );
    io.observe(wrap);

    const project = (
      wx: number,
      wy: number,
      wz: number,
      yaw: number,
      pitch: number
    ): { sx: number; sy: number; scale: number; cz: number } | null => {
      const cy = Math.cos(yaw);
      const sy = Math.sin(yaw);
      const cp = Math.cos(pitch);
      const sp = Math.sin(pitch);

      const x1 = wx * cy - wz * sy;
      const z1 = wx * sy + wz * cy;
      const y2 = wy * cp - z1 * sp;
      const z2 = wy * sp + z1 * cp + 8.6;

      if (z2 < 1.2) return null;
      const fov = Math.min(width, h * 2.15) * 0.92;
      const scale = fov / z2;
      return {
        sx: width * 0.5 + x1 * scale,
        sy: h * 0.54 - y2 * scale,
        scale,
        cz: z2,
      };
    };

    const drawBox = (
      cx: number,
      cy: number,
      cz: number,
      hx: number,
      hy: number,
      hz: number,
      yaw: number,
      pitch: number,
      stroke: string,
      fillTop: string,
      fillFront: string,
      fillSide: string,
      lineWidth = 1.25
    ) => {
      const corners = [
        [cx - hx, cy - hy, cz - hz],
        [cx + hx, cy - hy, cz - hz],
        [cx + hx, cy + hy, cz - hz],
        [cx - hx, cy + hy, cz - hz],
        [cx - hx, cy - hy, cz + hz],
        [cx + hx, cy - hy, cz + hz],
        [cx + hx, cy + hy, cz + hz],
        [cx - hx, cy + hy, cz + hz],
      ].map(([x, y, z]) => project(x, y, z, yaw, pitch));

      if (corners.some((c) => c === null)) return null;
      const pts = corners as { sx: number; sy: number; scale: number; cz: number }[];

      const faces: { idx: number[]; fill: string; z: number }[] = [
        { idx: [0, 1, 2, 3], fill: fillFront, z: (pts[0].cz + pts[2].cz) * 0.5 },
        { idx: [4, 5, 6, 7], fill: fillFront, z: (pts[4].cz + pts[6].cz) * 0.5 },
        { idx: [0, 4, 7, 3], fill: fillSide, z: (pts[0].cz + pts[7].cz) * 0.5 },
        { idx: [1, 5, 6, 2], fill: fillSide, z: (pts[1].cz + pts[6].cz) * 0.5 },
        { idx: [3, 2, 6, 7], fill: fillTop, z: (pts[3].cz + pts[6].cz) * 0.5 },
        { idx: [0, 1, 5, 4], fill: fillSide, z: (pts[0].cz + pts[5].cz) * 0.5 },
      ];
      faces.sort((a, b) => b.z - a.z);

      ctx.lineJoin = "round";
      for (const f of faces.slice(2)) {
        ctx.beginPath();
        ctx.moveTo(pts[f.idx[0]].sx, pts[f.idx[0]].sy);
        for (let k = 1; k < f.idx.length; k++) {
          ctx.lineTo(pts[f.idx[k]].sx, pts[f.idx[k]].sy);
        }
        ctx.closePath();
        ctx.fillStyle = f.fill;
        ctx.fill();
        ctx.strokeStyle = stroke;
        ctx.lineWidth = lineWidth;
        ctx.stroke();
      }

      const center = project(cx, cy, cz, yaw, pitch);
      return center;
    };

    const drawRing3D = (
      cx: number,
      cy: number,
      cz: number,
      radius: number,
      tiltX: number,
      tiltZ: number,
      rot: number,
      yaw: number,
      pitch: number,
      color: string,
      lw: number
    ) => {
      const segs = 44;
      ctx.beginPath();
      let started = false;
      const cosTX = Math.cos(tiltX);
      const stx = Math.sin(tiltX);
      const ctz = Math.cos(tiltZ);
      const stz = Math.sin(tiltZ);
      for (let i = 0; i <= segs; i++) {
        const a = (i / segs) * Math.PI * 2 + rot;
        const rx = Math.cos(a) * radius;
        const ry = 0;
        const rz = Math.sin(a) * radius;

        const y1 = ry * cosTX - rz * stx;
        const z1 = ry * stx + rz * cosTX;
        const x2 = rx * ctz - y1 * stz;
        const y2 = rx * stz + y1 * ctz;

        const p = project(cx + x2, cy + y2, cz + z1, yaw, pitch);
        if (!p) continue;
        if (!started) {
          ctx.moveTo(p.sx, p.sy);
          started = true;
        } else {
          ctx.lineTo(p.sx, p.sy);
        }
      }
      ctx.strokeStyle = color;
      ctx.lineWidth = lw;
      ctx.stroke();
    };

    const renderFrame = (now: number) => {
      const t = reducedMotion ? 0 : now * 0.001;
      const orb = orbitRef.current;

      if (!orb.dragging && !reducedMotion) {
        orb.yaw += (orb.targetYaw + Math.sin(t * 0.35) * 0.045 - orb.yaw) * 0.08;
        orb.pitch += (orb.targetPitch + Math.cos(t * 0.28) * 0.018 - orb.pitch) * 0.08;
      } else {
        orb.yaw += (orb.targetYaw - orb.yaw) * 0.16;
        orb.pitch += (orb.targetPitch - orb.pitch) * 0.16;
      }

      const isDaylight = document.documentElement.dataset.theme === "daylight";
      ctx.save();
      ctx.scale(dpr, dpr);
      ctx.clearRect(0, 0, width, h);

      // Ambient vault backdrop
      const bg = ctx.createRadialGradient(width * 0.5, h * 0.48, 20, width * 0.5, h * 0.5, width * 0.65);
      if (isDaylight) {
        bg.addColorStop(0, "#F5F9FF");
        bg.addColorStop(1, "#E6EEF8");
      } else {
        bg.addColorStop(0, "rgba(14, 32, 58, 0.72)");
        bg.addColorStop(0.6, "rgba(6, 14, 28, 0.88)");
        bg.addColorStop(1, "rgba(4, 8, 17, 0.96)");
      }
      ctx.fillStyle = bg;
      ctx.fillRect(0, 0, width, h);

      // Perspective floor grid
      const gridStroke = isDaylight ? "rgba(20, 98, 141, 0.11)" : "rgba(56, 189, 248, 0.10)";
      ctx.strokeStyle = gridStroke;
      ctx.lineWidth = 1;
      for (let gx = -5; gx <= 5; gx += 1) {
        const p0 = project(gx * 0.85, -1.25, -2.6, orb.yaw, orb.pitch);
        const p1 = project(gx * 0.85, -1.25, 2.6, orb.yaw, orb.pitch);
        if (p0 && p1) {
          ctx.beginPath();
          ctx.moveTo(p0.sx, p0.sy);
          ctx.lineTo(p1.sx, p1.sy);
          ctx.stroke();
        }
      }
      for (let gz = -3; gz <= 3; gz += 1) {
        const p0 = project(-4.25, -1.25, gz * 0.85, orb.yaw, orb.pitch);
        const p1 = project(4.25, -1.25, gz * 0.85, orb.yaw, orb.pitch);
        if (p0 && p1) {
          ctx.beginPath();
          ctx.moveTo(p0.sx, p0.sy);
          ctx.lineTo(p1.sx, p1.sy);
          ctx.stroke();
        }
      }

      // Ambient spatial particles
      for (let i = 0; i < particles.length; i++) {
        const pt = particles[i];
        const px = pt.x * 8.4;
        const py = pt.y * 2.5 + Math.sin(t * pt.speed + i) * 0.14;
        const pz = pt.z * 4.2;
        const p = project(px, py, pz, orb.yaw, orb.pitch);
        if (!p) continue;
        const isPostOnset = onsetBoundaryIndex >= -0.5 && px > ((onsetBoundaryIndex / Math.max(1, chrono.length - 1)) - 0.5) * 6.4;
        ctx.fillStyle = isPostOnset
          ? isDaylight ? "rgba(217, 45, 32, 0.28)" : "rgba(244, 63, 94, 0.45)"
          : isDaylight ? "rgba(20, 98, 141, 0.22)" : "rgba(56, 189, 248, 0.35)";
        ctx.beginPath();
        ctx.arc(p.sx, p.sy, pt.size, 0, Math.PI * 2);
        ctx.fill();
      }

      const n = chrono.length;
      const span = Math.min(6.8, Math.max(2.6, (n - 1) * 0.56));
      const xForIndex = (idx: number) => (n <= 1 ? 0 : -span * 0.5 + (idx / (n - 1)) * span);

      // Central Merkle spine connecting snapshots
      if (n > 0) {
        const s0 = project(xForIndex(0) - 0.35, -0.1, 0, orb.yaw, orb.pitch);
        const s1 = project(xForIndex(n - 1) + 0.35, -0.1, 0, orb.yaw, orb.pitch);
        if (s0 && s1) {
          const spineGrad = ctx.createLinearGradient(s0.sx, s0.sy, s1.sx, s1.sy);
          spineGrad.addColorStop(0, isDaylight ? "rgba(16, 185, 129, 0.55)" : "rgba(56, 189, 248, 0.65)");
          spineGrad.addColorStop(0.65, isDaylight ? "rgba(20, 98, 141, 0.55)" : "rgba(16, 185, 129, 0.72)");
          spineGrad.addColorStop(1, onset ? "rgba(244, 63, 94, 0.78)" : "rgba(56, 189, 248, 0.65)");
          ctx.strokeStyle = spineGrad;
          ctx.lineWidth = 2.2;
          ctx.beginPath();
          ctx.moveTo(s0.sx, s0.sy);
          ctx.lineTo(s1.sx, s1.sy);
          ctx.stroke();
        }
      }

      // Prepare depth-sorted items (slabs + onset barrier plane)
      type DrawItem =
        | { kind: "slab"; index: number; snap: Snapshot; wx: number; z: number }
        | { kind: "onset"; wx: number; z: number };

      const items: DrawItem[] = chrono.map((snap, index) => {
        const wx = xForIndex(index);
        const p = project(wx, 0, 0, orb.yaw, orb.pitch);
        return { kind: "slab", index, snap, wx, z: p ? p.cz : 10 };
      });

      if (onset && onsetBoundaryIndex >= -0.5 && n > 0) {
        const clamped = Math.max(-0.5, Math.min(n - 0.5, onsetBoundaryIndex));
        const step = n <= 1 ? 0.6 : span / (n - 1);
        const wx = xForIndex(0) + clamped * step;
        const p = project(wx, 0, 0, orb.yaw, orb.pitch);
        items.push({ kind: "onset", wx, z: p ? p.cz : 10 });
      }

      items.sort((a, b) => b.z - a.z);

      const nextSlabs: ProjectedSlab[] = [];

      for (const item of items) {
        if (item.kind === "onset") {
          // 3D Crimson Onset Barrier Plane
          const corners = [
            project(item.wx, -1.18, -1.25, orb.yaw, orb.pitch),
            project(item.wx, 1.18, -1.25, orb.yaw, orb.pitch),
            project(item.wx, 1.18, 1.25, orb.yaw, orb.pitch),
            project(item.wx, -1.18, 1.25, orb.yaw, orb.pitch),
          ];
          if (corners.every(Boolean)) {
            const c = corners as { sx: number; sy: number }[];
            ctx.beginPath();
            ctx.moveTo(c[0].sx, c[0].sy);
            ctx.lineTo(c[1].sx, c[1].sy);
            ctx.lineTo(c[2].sx, c[2].sy);
            ctx.lineTo(c[3].sx, c[3].sy);
            ctx.closePath();
            ctx.fillStyle = isDaylight ? "rgba(225, 29, 72, 0.13)" : "rgba(244, 63, 94, 0.17)";
            ctx.fill();
            ctx.setLineDash([5, 4]);
            ctx.strokeStyle = isDaylight ? "rgba(225, 29, 72, 0.8)" : "rgba(251, 113, 133, 0.9)";
            ctx.lineWidth = 1.6;
            ctx.stroke();
            ctx.setLineDash([]);

            const topLbl = project(item.wx, 1.32, 0, orb.yaw, orb.pitch);
            if (topLbl) {
              ctx.font = '700 10px "Cascadia Code", Consolas, monospace';
              ctx.fillStyle = isDaylight ? "#BE123C" : "#FDA4AF";
              ctx.textAlign = "center";
              ctx.fillText(`ONSET ${onset?.slice(11, 16) ?? ""}`, topLbl.sx, topLbl.sy);
            }
          }
          continue;
        }

        const { snap, index, wx } = item;
        const exp = snap.assessment?.exposure_status ?? "none";
        const isRec = Boolean(recommendedId && snap.id === recommendedId);
        const isHov = hovered?.snap.id === snap.id;
        const risk = snap.assessment?.risk_score ?? 0;

        let stroke = isDaylight ? "rgba(100, 116, 139, 0.65)" : "rgba(148, 163, 184, 0.6)";
        let fillTop = isDaylight ? "rgba(203, 213, 225, 0.72)" : "rgba(51, 65, 85, 0.55)";
        let fillFront = isDaylight ? "rgba(226, 232, 240, 0.66)" : "rgba(30, 41, 59, 0.55)";
        let fillSide = isDaylight ? "rgba(148, 163, 184, 0.45)" : "rgba(15, 23, 42, 0.65)";

        if (exp === "clear") {
          stroke = isRec
            ? "#34D399"
            : isDaylight ? "rgba(5, 150, 105, 0.85)" : "rgba(16, 185, 129, 0.82)";
          fillTop = isDaylight ? "rgba(16, 185, 129, 0.34)" : "rgba(16, 185, 129, 0.30)";
          fillFront = isDaylight ? "rgba(16, 185, 129, 0.22)" : "rgba(6, 95, 70, 0.42)";
          fillSide = isDaylight ? "rgba(5, 150, 105, 0.16)" : "rgba(4, 47, 46, 0.55)";
        } else if (exp === "suspect") {
          stroke = isDaylight ? "rgba(217, 119, 6, 0.88)" : "rgba(251, 191, 36, 0.88)";
          fillTop = isDaylight ? "rgba(245, 158, 11, 0.32)" : "rgba(245, 158, 11, 0.28)";
          fillFront = isDaylight ? "rgba(245, 158, 11, 0.20)" : "rgba(120, 53, 15, 0.45)";
          fillSide = isDaylight ? "rgba(217, 119, 6, 0.15)" : "rgba(69, 26, 3, 0.56)";
        } else if (exp === "affected") {
          stroke = isDaylight ? "rgba(225, 29, 72, 0.9)" : "rgba(251, 113, 133, 0.92)";
          fillTop = isDaylight ? "rgba(244, 63, 94, 0.36)" : "rgba(244, 63, 94, 0.32)";
          fillFront = isDaylight ? "rgba(244, 63, 94, 0.22)" : "rgba(136, 19, 55, 0.48)";
          fillSide = isDaylight ? "rgba(190, 18, 60, 0.18)" : "rgba(76, 5, 25, 0.62)";
        }

        const bob = reducedMotion ? 0 : Math.sin(t * 1.5 + index * 0.55) * 0.045;
        const hy = 0.46 + Math.min(0.45, risk * 0.42) + (isRec ? 0.12 : 0) + (isHov ? 0.08 : 0);
        const hx = isRec ? 0.16 : 0.125;
        const hz = isRec ? 0.46 : 0.38;
        const wy = -0.1 + bob + (hy - 0.46) * 0.45;

        const center = drawBox(
          wx,
          wy,
          0,
          hx,
          hy,
          hz,
          orb.yaw,
          orb.pitch,
          stroke,
          fillTop,
          fillFront,
          fillSide,
          isRec || isHov ? 2.1 : 1.2
        );

        if (center) {
          nextSlabs.push({
            snap,
            index,
            cx: center.sx,
            cy: center.sy,
            hitRadius: Math.max(18, center.scale * 0.42),
            depth: center.cz,
          });

          // Index label underneath monolith
          const foot = project(wx, -0.95, 0.35, orb.yaw, orb.pitch);
          if (foot) {
            ctx.font = '600 10px "Cascadia Code", Consolas, monospace';
            ctx.textAlign = "center";
            ctx.fillStyle = isRec
              ? isDaylight ? "#047857" : "#6EE7B7"
              : isDaylight ? "#475569" : "rgba(148, 163, 184, 0.85)";
            ctx.fillText(`#${index + 1}`, foot.sx, foot.sy);
          }

          // 5-Gate Orbital Gimbal Rings around recommended clean snapshot
          if (isRec) {
            drawRing3D(wx, wy, 0, 0.82, 0.45, 0.2, t * 0.9, orb.yaw, orb.pitch, "rgba(52, 211, 153, 0.85)", 1.7);
            drawRing3D(wx, wy, 0, 0.98, -0.55, 0.38, -t * 0.65, orb.yaw, orb.pitch, "rgba(56, 189, 248, 0.62)", 1.25);
            const crown = project(wx, wy + hy + 0.38, 0, orb.yaw, orb.pitch);
            if (crown) {
              ctx.font = '700 10.5px "Cascadia Code", Consolas, monospace';
              ctx.textAlign = "center";
              ctx.fillStyle = isDaylight ? "#047857" : "#6EE7B7";
              ctx.fillText("★ 5-GATE LOCK", crown.sx, crown.sy);
            }
          }
        }
      }

      slabsRef.current = nextSlabs;
      ctx.restore();
    };

    const tick = (now: number) => {
      if (visible && !document.hidden) {
        renderFrame(now);
      }
      if (!reducedMotion) {
        rafId = window.requestAnimationFrame(tick);
      }
    };

    renderFrame(performance.now());
    if (!reducedMotion) {
      rafId = window.requestAnimationFrame(tick);
    }

    return () => {
      window.cancelAnimationFrame(rafId);
      ro.disconnect();
      io.disconnect();
    };
  }, [chrono, onset, onsetBoundaryIndex, recommendedId, height, hovered?.snap.id]);

  const pickSlab = (clientX: number, clientY: number): ProjectedSlab | null => {
    const canvas = canvasRef.current;
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    const x = clientX - rect.left;
    const y = clientY - rect.top;
    let best: ProjectedSlab | null = null;
    let bestDist = Infinity;
    for (const s of slabsRef.current) {
      const d = Math.hypot(s.cx - x, s.cy - y);
      if (d <= s.hitRadius && d < bestDist) {
        best = s;
        bestDist = d;
      }
    }
    return best;
  };

  const onPointerDown = (e: React.PointerEvent<HTMLCanvasElement>) => {
    orbitRef.current.dragging = true;
    orbitRef.current.lastX = e.clientX;
    orbitRef.current.lastY = e.clientY;
    orbitRef.current.moved = 0;
    e.currentTarget.setPointerCapture(e.pointerId);
  };

  const onPointerMove = (e: React.PointerEvent<HTMLCanvasElement>) => {
    const orb = orbitRef.current;
    const rect = e.currentTarget.getBoundingClientRect();
    if (orb.dragging) {
      const dx = e.clientX - orb.lastX;
      const dy = e.clientY - orb.lastY;
      orb.lastX = e.clientX;
      orb.lastY = e.clientY;
      orb.moved += Math.abs(dx) + Math.abs(dy);
      orb.targetYaw = Math.max(-1.25, Math.min(1.25, orb.targetYaw + dx * 0.0075));
      orb.targetPitch = Math.max(-0.12, Math.min(0.75, orb.targetPitch + dy * 0.0055));
      setHovered(null);
      return;
    }

    const hit = pickSlab(e.clientX, e.clientY);
    if (hit) {
      setHovered({
        snap: hit.snap,
        index: hit.index,
        x: Math.min(rect.width - 250, Math.max(12, hit.cx + 14)),
        y: Math.max(12, Math.min(rect.height - 115, hit.cy - 44)),
      });
    } else if (hovered) {
      setHovered(null);
    }
  };

  const onPointerUp = (e: React.PointerEvent<HTMLCanvasElement>) => {
    const moved = orbitRef.current.moved;
    orbitRef.current.dragging = false;
    try {
      e.currentTarget.releasePointerCapture(e.pointerId);
    } catch {
      /* ignore */
    }
    if (moved < 6) {
      const hit = pickSlab(e.clientX, e.clientY);
      if (hit) onSelectSnapshot(hit.snap.id);
    }
  };

  if (snapshots.length === 0) return null;

  return (
    <div className="vault3d-card" ref={wrapRef}>
      <div className="vault3d-bar">
        <div className="vault3d-title">
          <span className="vault3d-dot" aria-hidden="true" />
          <b>3D Chrono-Vault Corridor</b>
          <span className="mono-id">
            {chrono.length} sealed monoliths · drag to orbit · click slab to inspect manifest
          </span>
        </div>
        <div className="vault3d-actions">
          {abstained && <Chip kind="no_safe_candidate" label="ABSTAINED · NO SAFE CANDIDATE" />}
          {recommendedId && !abstained && (
            <Chip kind="eligible" label={`5-Gate Lock: ${recommendedId.slice(0, 14)}…`} />
          )}
          <button type="button" className="btn btn-xs" onClick={resetOrbit}>
            Reset orbit
          </button>
        </div>
      </div>

      <div className="vault3d-stage" style={{ height }}>
        <canvas
          ref={canvasRef}
          style={{
            width: "100%",
            height: `${height}px`,
            cursor: orbitRef.current.dragging ? "grabbing" : hovered ? "pointer" : "grab",
          }}
          aria-label="Interactive 3D snapshot corridor visualization. Use the snapshot buttons below for keyboard access."
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerLeave={() => {
            orbitRef.current.dragging = false;
            setHovered(null);
          }}
        />
        {hovered && (
          <div
            className="vault3d-tip"
            style={{ left: hovered.x, top: hovered.y }}
            role="status"
          >
            <div className="vt-head">
              <b>#{hovered.index + 1}</b>
              <span className="mono-id">{hovered.snap.id.slice(0, 18)}…</span>
            </div>
            <div className="vt-row">
              <Chip
                kind={hovered.snap.assessment?.exposure_status ?? "unknown"}
                label={hovered.snap.assessment?.exposure_status ?? "unscored"}
              />
              {recommendedId === hovered.snap.id && <Chip kind="eligible" label="recommended" />}
            </div>
            <div className="vt-meta">
              {fmtTime(hovered.snap.captured_at)} · {hovered.snap.file_count} files ·{" "}
              {fmtBytes(hovered.snap.total_bytes)}
              {hovered.snap.assessment
                ? ` · risk ${hovered.snap.assessment.risk_score.toFixed(3)}`
                : ""}
            </div>
          </div>
        )}
      </div>

      <div className="vault3d-rail" role="list" aria-label="Keyboard accessible snapshot selector">
        {chrono.map((s, idx) => {
          const exp = s.assessment?.exposure_status ?? "none";
          const isRec = recommendedId === s.id;
          return (
            <button
              key={s.id}
              type="button"
              role="listitem"
              className={`v3d-pill exp-${exp}` + (isRec ? " is-rec" : "")}
              onClick={() => onSelectSnapshot(s.id)}
              title={`${fmtTime(s.captured_at)} — ${exp}${isRec ? " (recommended clean point)" : ""}`}
            >
              <span>#{idx + 1}</span>
              {isRec && <i aria-label="Recommended">★</i>}
            </button>
          );
        })}
      </div>
    </div>
  );
}
