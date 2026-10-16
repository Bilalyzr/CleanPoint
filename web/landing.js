/* ============================================================
   CleanPoint landing engine — zero dependencies, offline-first.
   1. Scene: a 3D "snapshot timeline corridor" on 2D canvas
      (custom perspective projection; the camera rides scroll).
      It tells the product story: calm baseline → attack flare →
      the defensible point → verified restore → abstention void.
   2. Scroll orchestration: chapter tones, reveals, rail state,
      risk-curve draw, hash rows, counters — one rAF loop.
   3. Micro-interactions: custom cursor, magnetic buttons.
   prefers-reduced-motion turns the scene into a calm static
   composition and disables cursor/magnet effects.
   ============================================================ */
"use strict";

const RM = matchMedia("(prefers-reduced-motion: reduce)").matches;
const FINE = matchMedia("(pointer: fine)").matches;
const clamp = (v, a, b) => v < a ? a : v > b ? b : v;
const lerp = (a, b, t) => a + (b - a) * t;

/* ============================================================
   1 · SCENE — snapshot timeline corridor
   ============================================================ */
const Scene = (() => {
  const canvas = document.getElementById("scene");
  const ctx = canvas.getContext("2d");
  let W = 0, H = 0, DPR = 1;

  // corridor layout: nodes along -Z; camera flies from z0 to z1 with scroll
  const NODES = 34;
  const ATTACK_FROM = 22, ATTACK_TO = 30;   // node index range that flares
  const DEFENSIBLE = 20;                    // the green point
  const nodes = [];
  const motes = [];

  function build() {
    nodes.length = 0;
    let seed = 20260105;
    const rnd = () => (seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296;
    for (let i = 0; i < NODES; i++) {
      nodes.push({
        z: -i * 220,
        x: (rnd() - .5) * 260,
        y: (rnd() - .5) * 130,
        r: 16 + rnd() * 10,
        ph: rnd() * Math.PI * 2,
        attack: i >= ATTACK_FROM && i <= ATTACK_TO,
        defensible: i === DEFENSIBLE,
      });
    }
    motes.length = 0;
    for (let i = 0; i < 70; i++) {
      motes.push({ x: (Math.random() - .5) * 900, y: (Math.random() - .5) * 520,
        z: -Math.random() * 5200, s: .5 + Math.random() * 1.8, v: 6 + Math.random() * 14 });
    }
  }
  build();

  // scroll-driven state (lerped for cinematic smoothness)
  const cam = { z: 0, x: 0, y: 0, tz: 0, tx: 0, ty: 0 };
  let attackP = 0, tAttackP = 0;    // 0..1 corruption intensity
  let resolveP = 0, tResolveP = 0;  // 0..1 green point emphasis
  let voidP = 0, tVoidP = 0;        // 0..1 abstention dimming
  let verifyP = 0, tVerifyP = 0;    // 0..1 restore shimmer
  const mouse = { x: 0, y: 0, sx: 0, sy: 0 };
  let time = 0;

  function project(x, y, z) {
    const d = z - cam.z;                 // depth ahead of camera
    if (d < 40) return null;
    const f = 620 / d;                   // perspective divide
    return {
      x: W / 2 + (x - cam.x) * f,
      y: H * .52 + (y - cam.y) * f,
      s: f,
    };
  }

  function resize() {
    DPR = clamp(devicePixelRatio || 1, 1, 1.75);
    W = innerWidth; H = innerHeight;
    canvas.width = Math.round(W * DPR);
    canvas.height = Math.round(H * DPR);
    ctx.setTransform(DPR, 0, 0, DPR, 0, 0);
  }
  addEventListener("resize", resize, { passive: true });
  resize();

  if (FINE && !RM) {
    addEventListener("mousemove", (e) => {
      mouse.x = (e.clientX / W - .5) * 46;
      mouse.y = (e.clientY / H - .5) * 30;
    }, { passive: true });
  }

  function setScrollState(chapterProgress) {
    // chapterProgress: map of chapter name -> 0..1 within-viewport progress
    const hero = chapterProgress.hero || 0;
    const incident = chapterProgress.incident || 0;
    const evidence = chapterProgress.evidence || 0;
    const point = chapterProgress.point || 0;
    const restore = chapterProgress.restore || 0;
    const abstain = chapterProgress.abstain || 0;
    const proof = chapterProgress.proof || 0;
    const under = chapterProgress.under || 0;

    const travel = clamp(hero * .8 + incident * 2.2 + evidence * .9 +
      point * 1.4 + restore * .8 + abstain * .6, 0, NODES * 220 - 900);
    cam.tz = travel;
    tAttackP = incident;
    tResolveP = point;
    tVerifyP = restore;
    tVoidP = abstain;
    if (proof > .5 || under > 0) { tVoidP = 0; tResolveP = Math.max(tResolveP, .4); }
  }

  function nodeColor(n) {
    if (n.defensible && resolveP > .25) {
      const g = `rgba(61,220,132,`;
      return [g, .55 * resolveP + .25];
    }
    if (n.attack && attackP > .1) {
      return [`rgba(255,93,73,`, clamp(attackP * .9, 0, .9)];
    }
    return [`rgba(127,197,232,`, .5];
  }

  function drawNode(n, dim) {
    const p = project(n.x, n.y, n.z);
    if (!p) return;
    const [col, alphaBase] = nodeColor(n);
    const flick = n.attack ? (Math.sin(time * 9 + n.ph) * .5 + .5) * attackP : 0;
    const alpha = alphaBase * (dim ? .3 : 1) * (1 - flick * .35);
    const r = n.r * p.s * (n.defensible && resolveP > .25 ? 1 + resolveP * .8 : 1);

    // halo
    const glow = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, r * 4);
    glow.addColorStop(0, col + (alpha * .5) + ")");
    glow.addColorStop(1, col + "0)");
    ctx.fillStyle = glow;
    ctx.beginPath(); ctx.arc(p.x, p.y, r * 4, 0, 7); ctx.fill();

    // diamond core
    ctx.save();
    ctx.translate(p.x, p.y);
    ctx.rotate(time * .35 + n.ph);
    ctx.beginPath();
    const k = r, w = r * .62;
    ctx.moveTo(0, -k); ctx.lineTo(w, 0); ctx.lineTo(0, k); ctx.lineTo(-w, 0); ctx.closePath();
    ctx.fillStyle = col + (alpha * .16) + ")";
    ctx.fill();
    ctx.strokeStyle = col + alpha + ")";
    ctx.lineWidth = Math.max(1, 1.6 * p.s);
    ctx.stroke();
    ctx.restore();

    // defensible ring
    if (n.defensible && resolveP > .2) {
      const rr = r * (1.7 + Math.sin(time * 2.2) * .16);
      ctx.strokeStyle = `rgba(124,227,164,${(resolveP * .8) * (dim ? .3 : 1)})`;
      ctx.lineWidth = 1.4;
      ctx.beginPath(); ctx.arc(p.x, p.y, rr, 0, 7); ctx.stroke();
      ctx.strokeStyle = `rgba(124,227,164,${resolveP * .3 * (dim ? .3 : 1)})`;
      ctx.beginPath(); ctx.arc(p.x, p.y, rr * 1.45, 0, 7); ctx.stroke();
    }
    // corrupted shards
    if (n.attack && attackP > .3 && !RM) {
      const shards = 5;
      for (let s = 0; s < shards; s++) {
        const a = time * 1.6 + s * (Math.PI * 2 / shards) + n.ph;
        const dist = r * (1.8 + Math.sin(time * 3 + s) * .5 * attackP);
        const sx = p.x + Math.cos(a) * dist, sy = p.y + Math.sin(a) * dist;
        ctx.fillStyle = `rgba(255,93,73,${.5 * attackP * (dim ? .3 : 1)})`;
        ctx.fillRect(sx - 1.5, sy - 1.5, 3, 3);
      }
    }
  }

  function frame(t) {
    time = t / 1000;
    // easing toward targets
    const e = RM ? 1 : .075;
    cam.z = lerp(cam.z, cam.tz, e);
    cam.x = lerp(cam.x, cam.tx + mouse.sx, e);
    cam.y = lerp(cam.y, cam.ty + mouse.sy, e);
    mouse.sx = lerp(mouse.sx, mouse.x, .04);
    mouse.sy = lerp(mouse.sy, mouse.y, .04);
    attackP = lerp(attackP, tAttackP, .08);
    resolveP = lerp(resolveP, tResolveP, .08);
    voidP = lerp(voidP, tVoidP, .1);
    verifyP = lerp(verifyP, tVerifyP, .08);

    ctx.clearRect(0, 0, W, H);
    const dim = voidP > .5 ? (voidP - .5) * 2 : 0;

    // corridor beam (perspective floor line to vanishing point)
    const vp = project(0, 46, cam.z - 6000);
    if (vp) {
      ctx.strokeStyle = `rgba(127,197,232,${.07 * (1 - dim)})`;
      ctx.lineWidth = 1;
      for (const off of [-320, 320]) {
        const a = project(off, 46, cam.z - 200);
        if (a) { ctx.beginPath(); ctx.moveTo(a.x, a.y); ctx.lineTo(vp.x, vp.y); ctx.stroke(); }
      }
    }

    // data motes
    ctx.fillStyle = `rgba(159,180,201,${.35 * (1 - dim)})`;
    for (const m of motes) {
      if (!RM) m.z += m.v;
      if (m.z > cam.z + 120) m.z -= 5200;
      const p = project(m.x, m.y, m.z);
      if (!p) continue;
      const sz = Math.max(.6, m.s * p.s * 4);
      ctx.globalAlpha = clamp(p.s * 2.4, .06, .5) * (1 - dim);
      ctx.fillRect(p.x, p.y, sz, sz);
    }
    ctx.globalAlpha = 1;

    // timeline nodes back-to-front
    for (let i = NODES - 1; i >= 0; i--) drawNode(nodes[i], dim);

    // attack haze + verify shimmer overlays
    if (attackP > .12 && dim < .5) {
      const g = ctx.createRadialGradient(W / 2, H * .45, 0, W / 2, H * .45, Math.max(W, H) * .7);
      g.addColorStop(0, `rgba(255,93,73,${.10 * attackP * (1 - dim)})`);
      g.addColorStop(1, "rgba(255,93,73,0)");
      ctx.fillStyle = g; ctx.fillRect(0, 0, W, H);
    }
    if (verifyP > .15) {
      const g = ctx.createLinearGradient(0, H, 0, H * .4);
      g.addColorStop(0, `rgba(61,220,132,${.10 * verifyP * (1 - dim)})`);
      g.addColorStop(1, "rgba(61,220,132,0)");
      ctx.fillStyle = g; ctx.fillRect(0, 0, W, H);
    }
  }

  function stillFrame() { time = 0; frame(0); }

  return { setScrollState, frame, stillFrame };
})();

/* ============================================================
   2 · SCROLL ORCHESTRATION
   ============================================================ */
const chapters = [...document.querySelectorAll(".chapter")];
const railLinks = [...document.querySelectorAll(".rail a")];
const reveals = [...document.querySelectorAll("[data-reveal]")];

// risk curve geometry (real shape from the demo workload's findings)
(() => {
  const svg = document.getElementById("riskCurve");
  if (!svg) return;
  const pts = [];
  const N = 34;
  const riskAt = (i) => {
    if (i < 24) return 0.03 + Math.sin(i * 1.7) * 0.012;         // baseline calm
    if (i === 26 || i === 27) return 0.85 + (i === 27 ? .04 : 0); // attack spike
    return 0.15 + Math.sin(i) * 0.04;                             // post-attack
  };
  const X = i => 64 + (i / (N - 1)) * 832;
  const Y = r => 252 - r * 206;
  let d = "", area = `M64 252 `;
  for (let i = 0; i < N; i++) {
    const x = X(i), y = Y(riskAt(i));
    d += (i ? "L" : "M") + x.toFixed(1) + " " + y.toFixed(1);
    area += `L${x.toFixed(1)} ${y.toFixed(1)} `;
  }
  area += `L${X(N - 1).toFixed(1)} 252 Z`;
  svg.querySelector(".line").setAttribute("d", d);
  svg.querySelector(".area").setAttribute("d", area);
  const grid = svg.querySelector(".gridlines");
  let g = "";
  for (const v of [0, .25, .5, .75, 1]) g += `<line x1="64" y1="${Y(v)}" x2="896" y2="${Y(v)}"/>`;
  grid.innerHTML = g;
  const dots = svg.querySelector(".dots");
  dots.innerHTML = Array.from({ length: N }, (_, i) =>
    `<circle class="dot ${riskAt(i) > .35 ? "alert" : ""}" cx="${X(i)}" cy="${Y(riskAt(i))}" r="${riskAt(i) > .35 ? 5 : 2.6}"/>`).join("");
})();

// per-file hash rows (deterministic pseudo-names, like a real manifest)
(() => {
  const ul = document.getElementById("hashRows");
  if (!ul) return;
  const dirs = ["docs", "src", "media", "data"];
  const names = ["report", "invoice", "ledger", "service", "asset", "notes", "plan", "spec"];
  const hex = "0123456789abcdef";
  let seed = 7;
  const rnd = () => (seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296;
  let html = "";
  for (let i = 0; i < 14; i++) {
    const sha = Array.from({ length: 10 }, () => hex[(rnd() * 16) | 0]).join("");
    const name = `${dirs[(rnd() * 4) | 0]}/${names[(rnd() * 8) | 0]}-${String(i).padStart(2, "0")}`;
    html += `<li><svg class="tick" width="13" height="13" viewBox="0 0 13 13" aria-hidden="true"><path d="M2.5 7l2.6 2.6L10.5 4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg><span class="dim">verify</span><span style="min-width:170px">${name}</span><span class="dim">${sha}…</span><span class="bar"></span></li>`;
  }
  ul.innerHTML = html;
})();

// API status pill
(async () => {
  const el = document.getElementById("apiStatus");
  try {
    const r = await fetch("/api/v1/healthz");
    if (!r.ok) throw 0;
    el.classList.add("live");
    el.innerHTML = el.innerHTML.replace("checking API…", "API live · localhost");
  } catch {
    el.innerHTML = el.innerHTML.replace("checking API…", "API offline — run scripts/demo.sh");
  }
})();

/* single rAF pipeline */
const progress = {};
let tickScheduled = false;
function measure() {
  const vh = innerHeight;
  for (const ch of chapters) {
    const id = ch.id || "hero";
    const r = ch.getBoundingClientRect();
    // 0 when chapter top hits viewport bottom, 1 when chapter bottom leaves top
    const total = r.height + vh;
    progress[id] = clamp((vh - r.top) / total, 0, 1);
    const visible = r.top < vh * .82 && r.bottom > vh * .18;
    ch._visible = visible;
  }
}
function updateDOM() {
  // chapter tone -> body attribute (rail colors, scene tinting is internal)
  const active = chapters.find(c => c._visible && (c.id !== "hero" || progress.hero < .8)) || chapters[0];
  document.body.dataset.chapter = active.id;
  document.body.dataset.tone = active.dataset.tone || "";
  railLinks.forEach(a => a.classList.toggle("active", a.dataset.rail === active.id));

  // reveals with per-index stagger inside a chapter
  for (const el of reveals) {
    const r = el.getBoundingClientRect();
    if (r.top < innerHeight * .86 && r.bottom > 0) el.classList.add("in");
  }
  // risk curve draw
  const curve = document.querySelector(".curve-panel");
  if (curve && progress.evidence > .18) curve.classList.add("drawn");
  // hash rows + counter
  const rows = document.querySelectorAll("#hashRows li");
  if (progress.restore > .1) {
    const shown = Math.round(clamp((progress.restore - .1) / .5, 0, 1) * rows.length);
    rows.forEach((li, i) => li.classList.toggle("show", i < shown));
    const ok = document.getElementById("hashOk");
    if (ok) ok.textContent = Math.round(shown / rows.length * 117);
  }
  // stat counters
  document.querySelectorAll("[data-count]").forEach(el => {
    if (!el._started && progress.proof > .12) { el._started = true; animateCount(el); }
  });
}
function animateCount(el) {
  const target = parseFloat(el.dataset.count);
  const dec = parseInt(el.dataset.decimals || "0");
  const suf = el.dataset.suffix || "";
  const slash = el.dataset.slash;
  const t0 = performance.now(), dur = RM ? 0 : 1400;
  function step(t) {
    const p = dur ? clamp((t - t0) / dur, 0, 1) : 1;
    const ease = 1 - Math.pow(1 - p, 3);
    const v = (target * ease).toFixed(dec);
    el.querySelector("b").textContent = slash ? `${Math.round(v)}/${slash}` : v + suf;
    if (p < 1) requestAnimationFrame(step);
  }
  requestAnimationFrame(step);
}

function loop(t) {
  measure();
  Scene.setScrollState(progress);
  updateDOM();
  Scene.frame(t);
  tickScheduled = false;
  schedule();
}
function schedule() {
  if (!tickScheduled && !document.hidden) {
    tickScheduled = true;
    requestAnimationFrame(loop);
  }
}
document.addEventListener("visibilitychange", schedule);
addEventListener("scroll", schedule, { passive: true });
addEventListener("resize", schedule, { passive: true });

if (RM) {
  // static composition: one measure pass + one still frame; simple fades via CSS
  measure(); updateDOM(); Scene.stillFrame();
} else {
  schedule();
}

/* ============================================================
   3 · CURSOR + MAGNETIC BUTTONS (fine pointers, motion allowed)
   ============================================================ */
if (FINE && !RM) {
  const cur = document.getElementById("cursor");
  const dot = cur.querySelector(".c-dot");
  const ring = cur.querySelector(".c-ring");
  const label = cur.querySelector(".c-label");
  let cx = innerWidth / 2, cy = innerHeight / 2, rx = cx, ry = cy;

  addEventListener("mousemove", (e) => {
    cx = e.clientX; cy = e.clientY;
    dot.style.transform = `translate(${cx}px,${cy}px) translate(-50%,-50%)`;
  }, { passive: true });

  (function cursorLoop() {
    rx = lerp(rx, cx, .16); ry = lerp(ry, cy, .16);
    ring.style.transform = `translate(${rx}px,${ry}px) translate(-50%,-50%) scale(${cur.classList.contains("hover") ? 1.7 : cur.classList.contains("down") ? .8 : 1})`;
    label.style.transform = `translate(${rx}px,${ry + 30}px) translate(-50%,0)`;
    requestAnimationFrame(cursorLoop);
  })();

  addEventListener("mousedown", () => cur.classList.add("down"));
  addEventListener("mouseup", () => cur.classList.remove("down"));

  // context-aware hover states
  document.querySelectorAll("a, button").forEach(el => {
    el.addEventListener("mouseenter", () => {
      cur.classList.add("hover");
      const tag = el.dataset.cursor;
      label.textContent = tag || (el.tagName === "A" && el.getAttribute("href")?.startsWith("#") ? "jump" : "go");
    });
    el.addEventListener("mouseleave", () => cur.classList.remove("hover"));
  });

  // magnetic pull on designated buttons
  document.querySelectorAll("[data-magnetic]").forEach(btn => {
    btn.addEventListener("mousemove", (e) => {
      const r = btn.getBoundingClientRect();
      const dx = e.clientX - (r.left + r.width / 2);
      const dy = e.clientY - (r.top + r.height / 2);
      btn.style.transform = `translate(${dx * .12}px,${dy * .18}px)`;
    });
    btn.addEventListener("mouseleave", () => { btn.style.transform = ""; });
  });
}
