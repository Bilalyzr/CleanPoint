import React from "react";
import { useNavigate } from "react-router-dom";
import { api, TOKEN_KEY } from "../api";

// UI-01: token input (hidden by default with optional reveal), local-only status,
// ambient Chrono-Vault orbital canvas backdrop, and honest simulation disclosure.
export default function Login() {
  const nav = useNavigate();
  const [err, setErr] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [reveal, setReveal] = React.useState(false);
  const canvasRef = React.useRef<HTMLCanvasElement | null>(null);

  React.useEffect(() => {
    const saved = localStorage.getItem("cp_theme");
    document.documentElement.dataset.theme = saved === "daylight" ? "daylight" : "noir";
  }, []);

  React.useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    let raf = 0;
    let w = window.innerWidth;
    let h = window.innerHeight;
    const dpr = Math.min(window.devicePixelRatio || 1, 2);

    const resize = () => {
      w = window.innerWidth;
      h = window.innerHeight;
      canvas.width = Math.floor(w * dpr);
      canvas.height = Math.floor(h * dpr);
    };
    resize();
    window.addEventListener("resize", resize);

    const nodes = Array.from({ length: 42 }, (_, i) => ({
      a: (i / 42) * Math.PI * 2,
      r: 130 + (i % 5) * 58,
      spd: (i % 2 === 0 ? 1 : -1) * (0.08 + (i % 4) * 0.03),
      sz: 1.4 + (i % 3) * 0.8,
      emerald: i % 4 === 0,
    }));

    const draw = (now: number) => {
      const t = reduced ? 0 : now * 0.001;
      ctx.save();
      ctx.scale(dpr, dpr);
      ctx.clearRect(0, 0, w, h);

      const cx = w * 0.5;
      const cy = h * 0.5;

      // Orbital gimbal rings
      [150, 230, 320, 420].forEach((rad, idx) => {
        ctx.save();
        ctx.translate(cx, cy);
        ctx.rotate(t * (idx % 2 === 0 ? 0.07 : -0.05) + idx * 0.6);
        ctx.scale(1, 0.56 + idx * 0.06);
        ctx.beginPath();
        ctx.arc(0, 0, rad, 0, Math.PI * 2);
        ctx.strokeStyle =
          idx === 1
            ? "rgba(16, 185, 129, 0.16)"
            : "rgba(56, 189, 248, 0.11)";
        ctx.lineWidth = 1.1;
        ctx.stroke();
        ctx.restore();
      });

      // Orbiting telemetry motes
      for (const n of nodes) {
        const ang = n.a + t * n.spd;
        const x = cx + Math.cos(ang) * n.r * 1.15;
        const y = cy + Math.sin(ang) * n.r * 0.68;
        ctx.beginPath();
        ctx.arc(x, y, n.sz, 0, Math.PI * 2);
        ctx.fillStyle = n.emerald
          ? "rgba(52, 211, 153, 0.55)"
          : "rgba(56, 189, 248, 0.42)";
        ctx.fill();
      }

      ctx.restore();
      if (!reduced) {
        raf = window.requestAnimationFrame(draw);
      }
    };

    draw(performance.now());
    return () => {
      window.cancelAnimationFrame(raf);
      window.removeEventListener("resize", resize);
    };
  }, []);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const tok = (document.getElementById("tok") as HTMLInputElement).value.trim();
    setBusy(true);
    setErr("");
    try {
      sessionStorage.setItem(TOKEN_KEY, tok);
      await api("GET", "/config");
      nav("/");
    } catch (ex) {
      sessionStorage.removeItem(TOKEN_KEY);
      setErr(ex instanceof Error ? ex.message : String(ex));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="login-wrap">
      <canvas ref={canvasRef} className="login-canvas" aria-hidden="true" />
      <form className="login-card" onSubmit={submit}>
        <div className="login-top">
          <span className="badge-env">LOCAL SIMULATION</span>
          <a href="/" className="login-story-link">
            ← 3D Story
          </a>
        </div>
        <h1>
          <span className="logo" aria-hidden="true">
            CP
          </span>{" "}
          CleanPoint
        </h1>
        <p className="login-sub">
          Evidence-ranked ransomware-recovery console — <b>local simulation only</b>.
          No production data-protection or hardware WORM guarantees.
        </p>
        <div className="login-hint">
          Tokens live in <code>sandbox/.tokens</code> after{" "}
          <code>scripts/demo.sh</code> (or <code>scripts/demo.ps1</code>) — paste{" "}
          <code>operator=…</code> for full control or <code>readonly=…</code> to
          inspect telemetry.
        </div>
        <label className="fld" htmlFor="tok">
          Bearer token
        </label>
        <div className="tok-field">
          <input
            type={reveal ? "text" : "password"}
            id="tok"
            autoComplete="off"
            placeholder="paste bearer token…"
            required
            autoFocus
          />
          <button
            type="button"
            className="tok-eye"
            onClick={() => setReveal((v) => !v)}
            aria-label={reveal ? "Hide token" : "Reveal token"}
          >
            {reveal ? "Hide" : "Show"}
          </button>
        </div>
        <p className="login-error" role="alert">
          {err}
        </p>
        <button
          className="btn primary btn-lg"
          type="submit"
          disabled={busy}
          style={{ width: "100%" }}
        >
          {busy ? "Verifying session…" : "Enter Mission Console →"}
        </button>
      </form>
    </div>
  );
}
