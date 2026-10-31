/* ============================================================
   CleanPoint v2.0 — Chrono-Vault 3D WebGL & Scroll Engine
   Zero external dependencies · Offline-first · 60-120fps GPU.
   1. WebGL 3D Chrono-Vault (13 crystalline snapshot monoliths,
      5-gate orbital gimbal, onset barrier plane, 650 volumetric
      GLSL entropy particles, drag-to-orbit + scroll flight) with
      automatic 2D-canvas 3D fallback.
   2. Interactive rules-v1 40-window spectrometer + live signal
      channel toggles (demonstrating missing-data honesty).
   3. Interactive 3-policy Decision Arena across 4 real scenarios.
   4. Spring cursor, specular spotlight cards, 3D tilt & magnets.
   ============================================================ */
"use strict";

const RM = matchMedia("(prefers-reduced-motion: reduce)").matches;
const FINE = matchMedia("(pointer: fine)").matches;
const MOBILE = innerWidth < 760;
const clamp = (v, a, b) => (v < a ? a : v > b ? b : v);
const lerp = (a, b, t) => a + (b - a) * t;

const SNAP_WINDOWS = [3, 6, 9, 12, 15, 18, 21, 24, 27, 30, 33, 36, 39];
const winToUTC = (w) => {
  const mins = 9 * 60 + w * 5;
  const hh = String(Math.floor(mins / 60)).padStart(2, "0");
  const mm = String(mins % 60).padStart(2, "0");
  return `${hh}:${mm} UTC`;
};

/* ============================================================
   1 · 3D CHRONO-VAULT ENGINE (WebGL GLSL + 2D Fallback)
   ============================================================ */
const Scene = (() => {
  const canvas = document.getElementById("scene");
  const engineBadge = document.getElementById("hudEngine");
  let W = innerWidth, H = innerHeight, DPR = 1;

  // Shared camera & narrative parameters (smoothly damped)
  const state = {
    camZ: 2.5, tCamZ: 2.5,
    camX: 0, tCamX: 0,
    camY: 0.4, tCamY: 0.4,
    orbitYaw: -0.18, orbitPitch: 0.08,
    dragYaw: 0, dragPitch: 0,
    attack: 0, tAttack: 0,
    scan: 0, tScan: 0,
    resolve: 0, tResolve: 0,
    verify: 0, tVerify: 0,
    voidP: 0, tVoid: 0,
    mouseX: 0, mouseY: 0,
  };

  // Drag-to-orbit interaction on background canvas
  let dragging = false, lx = 0, ly = 0;
  if (!RM) {
    canvas.addEventListener("pointerdown", (e) => {
      dragging = true; lx = e.clientX; ly = e.clientY;
      canvas.setPointerCapture?.(e.pointerId);
    });
    addEventListener("pointermove", (e) => {
      if (FINE) {
        state.mouseX = (e.clientX / Math.max(1, W) - 0.5) * 2;
        state.mouseY = (e.clientY / Math.max(1, H) - 0.5) * 2;
      }
      if (!dragging) return;
      state.dragYaw = clamp(state.dragYaw + (e.clientX - lx) * 0.0045, -0.85, 0.85);
      state.dragPitch = clamp(state.dragPitch + (e.clientY - ly) * 0.0035, -0.48, 0.48);
      lx = e.clientX; ly = e.clientY;
    }, { passive: true });
    addEventListener("pointerup", () => { dragging = false; });
  }

  function setScrollState(p) {
    const hero = p.hero || 0;
    const inc = p.incident || 0;
    const ev = p.evidence || 0;
    const pt = p.point || 0;
    const res = p.restore || 0;
    const abs = p.abstain || 0;
    const lab = p.lab || 0;
    const prf = p.proof || 0;

    // Camera Z travels down the 13-snapshot corridor (z = 0 .. -33.6), then pulls back to W24 (index 7, z = -19.6)
    let tz = 3.8 - hero * 5.0 - inc * 23.0;
    let tx = -0.35 + inc * 1.1;
    let ty = 0.45 - inc * 0.25;

    if (ev > 0.05) {
      // Side spectrometer elevation
      tz = lerp(tz, -17.5, clamp(ev * 1.5, 0, 1));
      tx = lerp(tx, 2.2, clamp(ev * 1.4, 0, 1));
    }
    if (pt > 0.05) {
      // Lock directly onto Snapshot #8 (W24, z = -19.6)
      const k = clamp(pt * 1.5, 0, 1);
      tz = lerp(tz, -14.4, k);
      tx = lerp(tx, -1.15, k);
      ty = lerp(ty, 0.15, k);
    }
    if (res > 0.05) {
      const k = clamp(res * 1.4, 0, 1);
      tz = lerp(tz, -15.2, k);
      tx = lerp(tx, 0.2, k);
    }
    if (abs > 0.05) {
      const k = clamp(abs * 1.4, 0, 1);
      tz = lerp(tz, -12.0, k);
      tx = lerp(tx, 0.0, k);
    }
    if (lab > 0.05 || prf > 0.05) {
      const k = clamp((lab + prf) * 1.2, 0, 1);
      tz = lerp(tz, -9.5, k);
      tx = lerp(tx, 0.5, k);
    }

    state.tCamZ = tz;
    state.tCamX = tx;
    state.tCamY = ty;
    state.tAttack = clamp(inc * 1.55 - abs * 0.5, 0, 1);
    state.tScan = clamp(ev * 1.4, 0, 1);
    state.tResolve = clamp(pt * 1.6 + res * 0.7 + (prf > 0.1 ? 0.55 : 0), 0, 1);
    state.tVerify = clamp(res * 1.6, 0, 1);
    state.tVoid = clamp(abs * 1.75 - lab * 1.5, 0, 1);
  }

  function stepState() {
    const e = RM ? 1 : 0.075;
    state.camZ = lerp(state.camZ, state.tCamZ, e);
    state.camX = lerp(state.camX, state.tCamX + state.mouseX * 0.55, e);
    state.camY = lerp(state.camY, state.tCamY - state.mouseY * 0.35, e);
    if (!dragging) {
      state.dragYaw = lerp(state.dragYaw, 0, 0.035);
      state.dragPitch = lerp(state.dragPitch, 0, 0.035);
    }
    state.attack = lerp(state.attack, state.tAttack, 0.08);
    state.scan = lerp(state.scan, state.tScan, 0.08);
    state.resolve = lerp(state.resolve, state.tResolve, 0.08);
    state.verify = lerp(state.verify, state.tVerify, 0.08);
    state.voidP = lerp(state.voidP, state.tVoid, 0.09);
  }

  // Try WebGL1 hardware pipeline first
  let gl = null;
  try {
    gl = canvas.getContext("webgl", { alpha: true, antialias: true, powerPreference: "high-performance" });
  } catch { gl = null; }

  if (gl) {
    if (engineBadge) engineBadge.textContent = "WEBGL 3D";
    return initWebGL(gl);
  }
  if (engineBadge) engineBadge.textContent = "CANVAS 3D";
  return initCanvas2D();

  /* ---------------- WebGL 3D Implementation ---------------- */
  function initWebGL(gl) {
    const compile = (type, src) => {
      const sh = gl.createShader(type);
      gl.shaderSource(sh, src);
      gl.compileShader(sh);
      if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
        gl.deleteShader(sh);
        return null;
      }
      return sh;
    };
    const link = (vsSrc, fsSrc) => {
      const vs = compile(gl.VERTEX_SHADER, vsSrc);
      const fs = compile(gl.FRAGMENT_SHADER, fsSrc);
      if (!vs || !fs) return null;
      const pr = gl.createProgram();
      gl.attachShader(pr, vs);
      gl.attachShader(pr, fs);
      gl.linkProgram(pr);
      return gl.getProgramParameter(pr, gl.LINK_STATUS) ? pr : null;
    };

    // Line / Wireframe Monolith Shader
    const lineProg = link(
      `attribute vec3 aPos;
       attribute vec4 aCol;
       uniform mat4 uProj;
       uniform mat4 uView;
       varying vec4 vCol;
       void main() {
         vec4 mv = uView * vec4(aPos, 1.0);
         gl_Position = uProj * mv;
         float fog = clamp(1.0 - (-mv.z - 2.0) / 38.0, 0.06, 1.0);
         vCol = vec4(aCol.rgb, aCol.a * fog);
       }`,
      `precision mediump float;
       varying vec4 vCol;
       void main() {
         gl_FragColor = vCol;
       }`
    );

    // Volumetric Entropy Point-Sprite Shader
    const ptProg = link(
      `attribute vec3 aPos;
       attribute vec4 aCol;
       attribute float aSize;
       uniform mat4 uProj;
       uniform mat4 uView;
       uniform float uDpr;
       varying vec4 vCol;
       void main() {
         vec4 mv = uView * vec4(aPos, 1.0);
         gl_Position = uProj * mv;
         float dist = max(1.2, -mv.z);
         gl_PointSize = clamp(aSize * uDpr * (16.0 / dist), 1.5, 42.0);
         float fog = clamp(1.0 - (dist - 1.5) / 36.0, 0.0, 1.0);
         vCol = vec4(aCol.rgb, aCol.a * fog);
       }`,
      `precision mediump float;
       varying vec4 vCol;
       void main() {
         vec2 p = gl_PointCoord - vec2(0.5);
         float r = length(p) * 2.0;
         if (r > 1.0) discard;
         float glow = pow(1.0 - r, 2.1);
         gl_FragColor = vec4(vCol.rgb, vCol.a * glow);
       }`
    );

    if (!lineProg || !ptProg) {
      if (engineBadge) engineBadge.textContent = "CANVAS 3D";
      return initCanvas2D();
    }

    const lineBuf = gl.createBuffer();
    const ptBuf = gl.createBuffer();

    // Pre-allocate dynamic vertex arrays (zero GC churn per frame)
    const MAX_LINE_VERTS = 4200;
    const lineData = new Float32Array(MAX_LINE_VERTS * 7);
    let lv = 0;

    const PARTICLE_COUNT = MOBILE ? 280 : 620;
    const ptData = new Float32Array(PARTICLE_COUNT * 8);
    const particles = [];
    let seed = 20261031;
    const rnd = () => ((seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296);

    for (let i = 0; i < PARTICLE_COUNT; i++) {
      particles.push({
        x: (rnd() - 0.5) * 14,
        y: (rnd() - 0.5) * 8.5,
        z: 4 - rnd() * 42,
        vx: (rnd() - 0.5) * 0.25,
        vy: (rnd() - 0.5) * 0.25,
        vz: 0.4 + rnd() * 1.4,
        size: 1.8 + rnd() * 3.8,
        ph: rnd() * 6.283,
      });
    }

    function pushSeg(x1, y1, z1, x2, y2, z2, r, g, b, a) {
      if (lv + 2 >= MAX_LINE_VERTS) return;
      let o = lv * 7;
      lineData[o] = x1; lineData[o + 1] = y1; lineData[o + 2] = z1;
      lineData[o + 3] = r; lineData[o + 4] = g; lineData[o + 5] = b; lineData[o + 6] = a;
      o += 7;
      lineData[o] = x2; lineData[o + 1] = y2; lineData[o + 2] = z2;
      lineData[o + 3] = r; lineData[o + 4] = g; lineData[o + 5] = b; lineData[o + 6] = a;
      lv += 2;
    }

    // Draw a 3D crystalline snapshot slab (beveled rectangular prism + inner hash core)
    function pushSnapshotSlab(cx, cy, cz, w, h, d, rotZ, jitter, r, g, b, a) {
      const czCos = Math.cos(rotZ), czSin = Math.sin(rotZ);
      const tx = (x, y) => cx + x * czCos - y * czSin;
      const ty = (x, y) => cy + x * czSin + y * czCos;
      const corners = [
        [-w, -h], [w, -h], [w, h], [-w, h],
      ];
      for (let i = 0; i < 4; i++) {
        const [ax, ay] = corners[i];
        const [bx, by] = corners[(i + 1) % 4];
        const jx = jitter ? Math.sin(i * 2.3 + jitter * 9) * 0.14 * state.attack : 0;
        const jy = jitter ? Math.cos(i * 1.9 + jitter * 8) * 0.14 * state.attack : 0;
        // Front face
        pushSeg(tx(ax, ay) + jx, ty(ax, ay) + jy, cz + d, tx(bx, by), ty(bx, by), cz + d, r, g, b, a);
        // Back face
        pushSeg(tx(ax, ay), ty(ax, ay), cz - d, tx(bx, by), ty(bx, by), cz - d, r, g, b, a * 0.55);
        // Depth struts
        pushSeg(tx(ax, ay) + jx, ty(ax, ay) + jy, cz + d, tx(ax, ay), ty(ax, ay), cz - d, r, g, b, a * 0.7);
      }
      // Inner diamond Merkle core
      const iw = w * 0.44, ih = h * 0.44;
      const dPts = [[0, -ih], [iw, 0], [0, ih], [-iw, 0]];
      for (let i = 0; i < 4; i++) {
        const [ax, ay] = dPts[i];
        const [bx, by] = dPts[(i + 1) % 4];
        pushSeg(tx(ax, ay), ty(ax, ay), cz, tx(bx, by), ty(bx, by), cz, r, g, b, a * 0.85);
      }
    }

    function pushRing3D(cx, cy, cz, radius, segs, tiltX, tiltY, r, g, b, a) {
      const cxCos = Math.cos(tiltX), cxSin = Math.sin(tiltX);
      const cyCos = Math.cos(tiltY), cySin = Math.sin(tiltY);
      let px = 0, py = 0, pz = 0;
      for (let i = 0; i <= segs; i++) {
        const ang = (i / segs) * Math.PI * 2;
        const x0 = Math.cos(ang) * radius;
        const y0 = Math.sin(ang) * radius;
        // Rotate X then Y
        const y1 = y0 * cxCos;
        const z1 = y0 * cxSin;
        const x2 = x0 * cyCos + z1 * cySin;
        const z2 = -x0 * cySin + z1 * cyCos;
        const wx = cx + x2, wy = cy + y1, wz = cz + z2;
        if (i > 0) pushSeg(px, py, pz, wx, wy, wz, r, g, b, a);
        px = wx; py = wy; pz = wz;
      }
    }

    function matPerspective(fovRad, aspect, near, far) {
      const f = 1.0 / Math.tan(fovRad / 2);
      const nf = 1 / (near - far);
      return new Float32Array([
        f / aspect, 0, 0, 0,
        0, f, 0, 0,
        0, 0, (far + near) * nf, -1,
        0, 0, (2 * far * near) * nf, 0,
      ]);
    }

    function matView(camX, camY, camZ, yaw, pitch) {
      const cy = Math.cos(yaw), sy = Math.sin(yaw);
      const cp = Math.cos(pitch), sp = Math.sin(pitch);
      // Rx(pitch) * Ry(yaw) * T(-cam)
      const r00 = cy, r01 = 0, r02 = -sy;
      const r10 = sy * sp, r11 = cp, r12 = cy * sp;
      const r20 = sy * cp, r21 = -sp, r22 = cy * cp;
      const tx = -(r00 * camX + r01 * camY + r02 * camZ);
      const ty = -(r10 * camX + r11 * camY + r12 * camZ);
      const tz = -(r20 * camX + r21 * camY + r22 * camZ);
      return new Float32Array([
        r00, r10, r20, 0,
        r01, r11, r21, 0,
        r02, r12, r22, 0,
        tx, ty, tz, 1,
      ]);
    }

    function resize() {
      DPR = clamp(devicePixelRatio || 1, 1, 1.85);
      W = innerWidth; H = innerHeight;
      canvas.width = Math.round(W * DPR);
      canvas.height = Math.round(H * DPR);
      gl.viewport(0, 0, canvas.width, canvas.height);
    }
    addEventListener("resize", resize, { passive: true });
    resize();

    function frame(ms) {
      const t = ms * 0.001;
      stepState();
      lv = 0;

      const dim = 1.0 - state.voidP * 0.58;

      // 1. Perspective architectural floor rails along the corridor
      for (let r = -2; r <= 2; r++) {
        const rx = r * 1.45;
        pushSeg(rx, -1.75, 4, rx, -1.75, -36, 0.25, 0.58, 0.78, 0.13 * dim);
      }
      for (let z = 2; z >= -36; z -= 2.8) {
        pushSeg(-3.2, -1.75, z, 3.2, -1.75, z, 0.25, 0.58, 0.78, 0.09 * dim);
      }

      // 2. Central Merkle SHA-256 Spine + 13 Snapshot Monoliths
      // Ordered newest-near-camera or chronological? Let's place W03..W39 along -Z so flying deeper advances simulated time!
      // Index 0..6: W03..W21 (baseline), Index 7: W24 (Defensible Point!), Index 8..12: W27..W39 (Post-Onset Compromised)
      let prevX = 0, prevY = 0, prevZ = 0;
      for (let i = 0; i < 13; i++) {
        const z = -i * 2.8;
        const spiral = i * 0.42;
        const x = Math.sin(spiral) * 0.65;
        const y = Math.cos(spiral * 0.8) * 0.28 + Math.sin(t * 1.1 + i) * 0.06;

        const isPostOnset = i >= 8;
        const isDefensible = i === 7; // W24 · 11:00 UTC

        let cr = 0.42, cg = 0.76, cb = 0.94, ca = 0.62 * dim;
        if (state.voidP > 0.35) {
          cr = lerp(cr, 0.95, state.voidP);
          cg = lerp(cg, 0.35, state.voidP);
          cb = lerp(cb, 0.25, state.voidP);
        } else if (isDefensible && state.resolve > 0.12) {
          cr = lerp(cr, 0.22, state.resolve);
          cg = lerp(cg, 0.92, state.resolve);
          cb = lerp(cb, 0.56, state.resolve);
          ca = lerp(ca, 0.96, state.resolve);
        } else if (isPostOnset && (state.attack > 0.08 || state.resolve > 0.2)) {
          const k = Math.max(state.attack, state.resolve * 0.75);
          cr = lerp(cr, 1.0, k);
          cg = lerp(cg, 0.32, k);
          cb = lerp(cb, 0.25, k);
          ca = lerp(ca, 0.88, k);
        }

        if (i > 0) {
          pushSeg(prevX, prevY, prevZ, x, y, z, cr, cg, cb, 0.38 * dim);
        }
        prevX = x; prevY = y; prevZ = z;

        const scale = isDefensible ? 1.0 + state.resolve * 0.32 : 1.0;
        const rot = Math.sin(t * 0.45 + i * 0.5) * 0.12 + (isPostOnset ? state.attack * Math.sin(t * 5 + i) * 0.09 : 0);
        pushSnapshotSlab(x, y, z, 0.92 * scale, 0.64 * scale, 0.14 * scale, rot, isPostOnset ? t + i : 0, cr, cg, cb, ca);

        // 5-Gate Orbital Gimbal around Snapshot #8 (W24)
        if (isDefensible && state.resolve > 0.08 && state.voidP < 0.5) {
          const ga = state.resolve * 0.85 * dim;
          pushRing3D(x, y, z, 1.38, 36, t * 0.7, t * 0.4, 0.24, 0.92, 0.56, ga);
          pushRing3D(x, y, z, 1.66, 40, -t * 0.5 + 0.8, t * 0.6, 0.52, 0.95, 0.74, ga * 0.55);
          // Verified Restore Outbound Arc Chamber (Act 04)
          if (state.verify > 0.1) {
            const rx = x + 2.35, ry = y + 0.25, rz = z + 0.4;
            pushSnapshotSlab(rx, ry, rz, 0.78, 0.54, 0.12, -t * 0.3, 0, 0.24, 0.92, 0.56, state.verify * 0.88);
            pushSeg(x + 0.9, y, z, rx - 0.78, ry, rz, 0.32, 0.96, 0.64, state.verify * 0.85);
          }
        }
      }

      // 3. Crimson Onset Barrier Plane between W24 (z=-19.6) and W27 (z=-22.4) at z = -21.0
      const barrierAlpha = clamp(state.attack * 0.8 + state.resolve * 0.7 + state.scan * 0.5, 0, 0.78) * dim;
      if (barrierAlpha > 0.04) {
        const bz = -21.0;
        pushSeg(-2.6, -1.6, bz, 2.6, -1.6, bz, 1.0, 0.34, 0.26, barrierAlpha);
        pushSeg(2.6, -1.6, bz, 2.6, 1.6, bz, 1.0, 0.34, 0.26, barrierAlpha);
        pushSeg(2.6, 1.6, bz, -2.6, 1.6, bz, 1.0, 0.34, 0.26, barrierAlpha);
        pushSeg(-2.6, 1.6, bz, -2.6, -1.6, bz, 1.0, 0.34, 0.26, barrierAlpha);
        pushSeg(-2.6, 0, bz, 2.6, 0, bz, 1.0, 0.34, 0.26, barrierAlpha * 0.45);
      }

      // 4. Update Volumetric Entropy Particles
      let po = 0;
      const speedMult = RM ? 0 : (1.0 + state.attack * 2.4);
      for (let i = 0; i < PARTICLE_COUNT; i++) {
        const p = particles[i];
        if (!RM) {
          p.z += p.vz * 0.022 * speedMult;
          p.x += Math.sin(t * 0.9 + p.ph) * 0.004;
          p.y += Math.cos(t * 0.8 + p.ph) * 0.004;
          if (p.z > 5) p.z -= 43;
        }
        const inAttackZone = p.z < -20.8;
        let pr = 0.48, pg = 0.80, pb = 0.96, pa = 0.58 * dim;
        if (state.voidP > 0.35) {
          pr = 0.95; pg = 0.36; pb = 0.28; pa = 0.38;
        } else if (inAttackZone && (state.attack > 0.1 || state.resolve > 0.15)) {
          pr = 1.0; pg = 0.35; pb = 0.27; pa = 0.78 * dim;
        } else if (Math.abs(p.z + 19.6) < 2.4 && state.resolve > 0.2) {
          pr = 0.26; pg = 0.93; pb = 0.58; pa = 0.85 * dim;
        }
        ptData[po] = p.x; ptData[po + 1] = p.y; ptData[po + 2] = p.z;
        ptData[po + 3] = pr; ptData[po + 4] = pg; ptData[po + 5] = pb; ptData[po + 6] = pa;
        ptData[po + 7] = p.size;
        po += 8;
      }

      // Render WebGL Scene
      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.enable(gl.BLEND);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE); // Additive volumetric glow

      const proj = matPerspective(0.88, Math.max(0.4, W / Math.max(1, H)), 0.2, 60.0);
      const yaw = state.orbitYaw + state.dragYaw + state.scan * 0.34 - state.mouseX * 0.07;
      const pitch = state.orbitPitch + state.dragPitch + state.mouseY * 0.05;
      const view = matView(state.camX, state.camY, state.camZ, yaw, pitch);

      // Draw Lines
      gl.useProgram(lineProg);
      gl.uniformMatrix4fv(gl.getUniformLocation(lineProg, "uProj"), false, proj);
      gl.uniformMatrix4fv(gl.getUniformLocation(lineProg, "uView"), false, view);
      gl.bindBuffer(gl.ARRAY_BUFFER, lineBuf);
      gl.bufferData(gl.ARRAY_BUFFER, lineData.subarray(0, lv * 7), gl.DYNAMIC_DRAW);
      const lPos = gl.getAttribLocation(lineProg, "aPos");
      const lCol = gl.getAttribLocation(lineProg, "aCol");
      gl.enableVertexAttribArray(lPos);
      gl.enableVertexAttribArray(lCol);
      gl.vertexAttribPointer(lPos, 3, gl.FLOAT, false, 28, 0);
      gl.vertexAttribPointer(lCol, 4, gl.FLOAT, false, 28, 12);
      gl.drawArrays(gl.LINES, 0, lv);

      // Draw Particles
      gl.useProgram(ptProg);
      gl.uniformMatrix4fv(gl.getUniformLocation(ptProg, "uProj"), false, proj);
      gl.uniformMatrix4fv(gl.getUniformLocation(ptProg, "uView"), false, view);
      gl.uniform1f(gl.getUniformLocation(ptProg, "uDpr"), DPR);
      gl.bindBuffer(gl.ARRAY_BUFFER, ptBuf);
      gl.bufferData(gl.ARRAY_BUFFER, ptData, gl.DYNAMIC_DRAW);
      const pPos = gl.getAttribLocation(ptProg, "aPos");
      const pCol = gl.getAttribLocation(ptProg, "aCol");
      const pSz = gl.getAttribLocation(ptProg, "aSize");
      gl.enableVertexAttribArray(pPos);
      gl.enableVertexAttribArray(pCol);
      gl.enableVertexAttribArray(pSz);
      gl.vertexAttribPointer(pPos, 3, gl.FLOAT, false, 32, 0);
      gl.vertexAttribPointer(pCol, 4, gl.FLOAT, false, 32, 12);
      gl.vertexAttribPointer(pSz, 1, gl.FLOAT, false, 32, 28);
      gl.drawArrays(gl.POINTS, 0, PARTICLE_COUNT);
    }

    return { setScrollState, frame, stillFrame: () => frame(0) };
  }

  /* ---------------- 2D Canvas 3D Fallback ---------------- */
  function initCanvas2D() {
    const ctx = canvas.getContext("2d");
    function resize() {
      DPR = clamp(devicePixelRatio || 1, 1, 1.75);
      W = innerWidth; H = innerHeight;
      canvas.width = Math.round(W * DPR);
      canvas.height = Math.round(H * DPR);
      ctx.setTransform(DPR, 0, 0, DPR, 0, 0);
    }
    addEventListener("resize", resize, { passive: true });
    resize();

    function project(x, y, z) {
      const dz = state.camZ - z;
      if (dz < 0.5) return null;
      const f = 540 / dz;
      return { x: W * 0.5 + (x - state.camX) * f, y: H * 0.52 - (y - state.camY) * f, s: f };
    }

    function frame(ms) {
      const t = ms * 0.001;
      stepState();
      ctx.clearRect(0, 0, W, H);
      for (let i = 12; i >= 0; i--) {
        const z = -i * 2.8;
        const p = project(Math.sin(i * 0.42) * 0.65, Math.cos(i * 0.34) * 0.28, z);
        if (!p) continue;
        const isDef = i === 7;
        const isAtt = i >= 8 && state.attack > 0.1;
        const col = isDef && state.resolve > 0.15 ? "52,217,130" : isAtt ? "255,90,71" : "132,206,238";
        const rw = 0.85 * p.s, rh = 0.58 * p.s;
        ctx.strokeStyle = `rgba(${col},0.65)`;
        ctx.lineWidth = 1.5;
        ctx.strokeRect(p.x - rw * 0.5, p.y - rh * 0.5, rw, rh);
        if (isDef && state.resolve > 0.15) {
          ctx.beginPath();
          ctx.arc(p.x, p.y, rw * (0.85 + Math.sin(t * 2) * 0.06), 0, Math.PI * 2);
          ctx.stroke();
        }
      }
    }
    return { setScrollState, frame, stillFrame: () => frame(0) };
  }
})();

/* ============================================================
   2 · INTERACTIVE rules-v1 40-WINDOW SPECTROMETER
   ============================================================ */
(() => {
  const svg = document.getElementById("riskCurve");
  if (!svg) return;
  const N = 40;
  const weights = { mod: 0.25, ent: 0.25, ren: 0.20, ext: 0.15, del: 0.10, sz: 0.05 };
  const enabled = { mod: true, ent: true, ren: true, ext: true, del: true, sz: true };

  // Deterministic per-window signal vector (0..1) matching rapid_destructive shape
  const winSignals = Array.from({ length: N }, (_, i) => {
    if (i < 24) {
      const n = 0.03 + Math.abs(Math.sin(i * 1.9)) * 0.04;
      return { mod: n, ent: n * 0.4, ren: 0, ext: 0, del: n * 0.2, sz: n * 0.5 };
    }
    if (i === 27) return { mod: 1.0, ent: 1.0, ren: 0.96, ext: 1.0, del: 0.35, sz: 0.85 };
    if (i === 28) return { mod: 0.82, ent: 0.88, ren: 0.74, ext: 0.90, del: 0.25, sz: 0.65 };
    return {
      mod: 0.12 + Math.sin(i) * 0.03,
      ent: i > 28 ? 0.18 : 0.04,
      ren: 0.05,
      ext: i > 27 ? 0.25 : 0,
      del: 0.04,
      sz: 0.08,
    };
  });

  const X = (i) => 64 + (i / (N - 1)) * 842;
  const Y = (r) => 252 - clamp(r, 0, 1) * 214;

  const grid = svg.querySelector(".gridlines");
  grid.innerHTML = [0, 0.25, 0.5, 0.75, 1]
    .map((v) => `<line x1="64" y1="${Y(v)}" x2="906" y2="${Y(v)}"/>`)
    .join("");

  function scoreAt(i) {
    const s = winSignals[i];
    let sum = 0;
    for (const k of Object.keys(weights)) {
      if (enabled[k]) sum += weights[k] * s[k];
    }
    return clamp(sum, 0, 1);
  }

  function renderCurve() {
    let d = "", area = "M64 252 ";
    let onsetIdx = -1;
    const scores = [];
    for (let i = 0; i < N; i++) {
      const r = scoreAt(i);
      scores.push(r);
      if (i >= 24 && r >= 0.35 && onsetIdx === -1) onsetIdx = i;
      const x = X(i), y = Y(r);
      d += (i ? "L" : "M") + x.toFixed(1) + " " + y.toFixed(1);
      area += `L${x.toFixed(1)} ${y.toFixed(1)} `;
    }
    area += `L${X(N - 1).toFixed(1)} 252 Z`;
    svg.querySelector(".line").setAttribute("d", d);
    svg.querySelector(".area").setAttribute("d", area);

    const dots = svg.querySelector(".dots");
    dots.innerHTML = scores
      .map((r, i) => `<circle class="dot ${i >= 24 && r >= 0.35 ? "alert" : ""}" cx="${X(i).toFixed(1)}" cy="${Y(r).toFixed(1)}" r="${i >= 24 && r >= 0.35 ? 5 : 2.7}"/>`)
      .join("");

    const onsetLine = document.getElementById("onsetMarker");
    const onsetLbl = document.getElementById("onsetLabel");
    if (onsetIdx >= 0) {
      const ox = X(onsetIdx).toFixed(1);
      onsetLine.setAttribute("x1", ox);
      onsetLine.setAttribute("x2", ox);
      onsetLine.style.opacity = "1";
      onsetLbl.setAttribute("x", String(Number(ox) + 8));
      onsetLbl.textContent = `estimated onset · ${winToUTC(onsetIdx)} (W${onsetIdx})`;
      onsetLbl.style.opacity = "1";
    } else {
      onsetLine.style.opacity = "0";
      onsetLbl.style.opacity = "0";
    }

    const activeCount = Object.values(enabled).filter(Boolean).length;
    const covEl = document.getElementById("activeCoverage");
    if (covEl) covEl.textContent = `${Math.round((activeCount / 6) * 100)}% (${activeCount}/6 signals)`;
  }
  renderCurve();

  // Interactive hover/touch scrubber across all 40 windows
  const scrubLine = document.getElementById("scrubLine");
  const readout = document.getElementById("windowReadout");
  svg.addEventListener("pointermove", (e) => {
    const rect = svg.getBoundingClientRect();
    const relX = ((e.clientX - rect.left) / Math.max(1, rect.width)) * 960;
    const idx = clamp(Math.round(((relX - 64) / 842) * (N - 1)), 0, N - 1);
    const sx = X(idx).toFixed(1);
    scrubLine.setAttribute("x1", sx);
    scrubLine.setAttribute("x2", sx);
    const r = scoreAt(idx);
    const zone = idx < 24 ? "baseline calibration" : r >= 0.35 ? "SUSPICIOUS" : "evaluated normal";
    const cls = idx >= 24 && r >= 0.35 ? "tone-red" : "tone-blue";
    if (readout) {
      readout.innerHTML = `W${String(idx).padStart(2, "0")} (${winToUTC(idx)}) · risk <b class="${cls}">${r.toFixed(3)}</b> · ${zone}`;
    }
  });
  svg.addEventListener("pointerleave", () => {
    scrubLine.setAttribute("x1", "-50");
    scrubLine.setAttribute("x2", "-50");
  });

  // Signal weight toggle switches
  document.querySelectorAll("#signalToggles li").forEach((li) => {
    const toggle = () => {
      const key = li.dataset.sig;
      enabled[key] = !enabled[key];
      li.classList.toggle("active", enabled[key]);
      li.setAttribute("aria-checked", String(enabled[key]));
      li.querySelector(".sg-pill").textContent = enabled[key] ? "ON" : "MUTED";
      renderCurve();
    };
    li.addEventListener("click", toggle);
    li.addEventListener("keydown", (e) => {
      if (e.key === "Enter" || e.key === " ") { e.preventDefault(); toggle(); }
    });
  });
})();

/* ============================================================
   3 · INTERACTIVE 3-POLICY DECISION ARENA
   ============================================================ */
(() => {
  const SCENARIOS = {
    rapid_destructive: {
      desc: "90% of workspace files overwritten with high-entropy bytes (H ≈ 7.95) and renamed to .lockd at Window 27 (11:15 UTC). Five subsequent snapshots seal compromised data with 100% valid SHA-256 hashes.",
      snaps: ["clear", "clear", "clear", "clear", "clear", "clear", "clear", "clear", "affected", "affected", "affected", "affected", "affected"],
      pickIdx: 7, // W24
      truth: "Hidden ground truth: attack_start = 11:15 UTC (W27) · CleanPoint RPO gap = 75 min · 0 contaminated files.",
      policies: [
        { name: "Naive Newest", ok: false, badge: "UNSAFE", pick: "Picks SNAP-W39 (12:15 UTC)", note: "Restores 108 encrypted .lockd files directly back into production." },
        { name: "Integrity-Only (SHA-256)", ok: false, badge: "UNSAFE", pick: "Picks SNAP-W39 (12:15 UTC)", note: "Manifest & object SHA-256 hashes verify 100% — blind to pre-capture encryption." },
        { name: "CleanPoint (rules-v1)", ok: true, badge: "SAFE CLEAN PICK", pick: "Picks SNAP-W24 (11:00 UTC)", note: "Pins onset at W27 (risk 0.89 >= 0.35), rejects W27..W39, verifies 120 clean files." },
      ],
    },
    slow_distributed: {
      desc: "Low-rate stealthy attack: only 10% of files rewritten per window with .crypt suffixes across 13 windows (W26–W38) to evade naive volume thresholds.",
      snaps: ["clear", "clear", "clear", "clear", "clear", "clear", "clear", "clear", "affected", "affected", "affected", "affected", "affected"],
      pickIdx: 7, // W24
      truth: "Hidden ground truth: attack_start = 11:10 UTC (W26) · Cumulative entropy shift + novel extension novelty catches W26.",
      policies: [
        { name: "Naive Newest", ok: false, badge: "UNSAFE", pick: "Picks SNAP-W39 (12:15 UTC)", note: "Restores cumulatively encrypted workspace after 13 stealth windows." },
        { name: "Integrity-Only (SHA-256)", ok: false, badge: "UNSAFE", pick: "Picks SNAP-W39 (12:15 UTC)", note: "Every incremental .crypt object was cleanly hashed and sealed." },
        { name: "CleanPoint (rules-v1)", ok: true, badge: "SAFE CLEAN PICK", pick: "Picks SNAP-W24 (11:00 UTC)", note: "Entropy shift + extension novelty cross 0.35 at W26; selects pre-onset W24." },
      ],
    },
    benign_archive: {
      desc: "False-positive stress test: engineer creates 15 compressed .7z archives (H ≈ 7.90 bits/byte) in Window 28 without mass file replacement or deletions.",
      snaps: ["unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "clear", "clear", "clear", "clear", "clear", "clear"],
      pickIdx: 12, // W39 newest!
      truth: "Hidden ground truth: benign workload (no attack) · Zero false onset · Zero unnecessary RPO loss!",
      policies: [
        { name: "Naive Newest", ok: true, badge: "CLEAN (BY LUCK)", pick: "Picks SNAP-W39 (12:15 UTC)", note: "Happens to be safe only because no attack occurred." },
        { name: "Naive Entropy-Only Rule", ok: false, badge: "FALSE ALARM", pick: "Rejects W30..W39 needlessly", note: "Single-feature entropy detectors panic on legitimate .7z / .zip archives." },
        { name: "CleanPoint (rules-v1)", ok: true, badge: "0m RPO LOSS", pick: "Picks SNAP-W39 (12:15 UTC)", note: "Multi-signal weighting stays below 0.35 & 30% manifest threshold; picks newest!" },
      ],
    },
    pre_existing_contamination: {
      desc: "Worst-case catastrophe: 90% destructive .locked encryption happens in Window 0 (09:00 UTC) BEFORE the first snapshot is ever captured at Window 3.",
      snaps: ["unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown"],
      pickIdx: -1, // Abstain!
      truth: "Hidden ground truth: all 13 snapshots are poisoned · Only safe action is explicit abstention (no_safe_candidate).",
      policies: [
        { name: "Naive Newest", ok: false, badge: "UNSAFE", pick: "Picks SNAP-W39 (12:15 UTC)", note: "Silently restores 100% encrypted workspace." },
        { name: "Integrity-Only (SHA-256)", ok: false, badge: "UNSAFE", pick: "Picks SNAP-W39 (12:15 UTC)", note: "All 13 poisoned snapshots pass SHA-256 verification." },
        { name: "CleanPoint (rules-v1)", ok: true, badge: "ABSTAINED SAFELY", pick: "status: no_safe_candidate", note: "BaselineContaminated() voids certification and refuses to guess." },
      ],
    },
  };

  const stripEl = document.getElementById("arenaStrip");
  const polEl = document.getElementById("arenaPolicies");
  const descEl = document.getElementById("arenaDesc");
  const truthEl = document.getElementById("arenaTruthNote");
  const launchBtn = document.getElementById("arenaLaunchBtn");
  if (!stripEl || !polEl) return;

  function renderArena(key) {
    const sc = SCENARIOS[key];
    descEl.textContent = sc.desc;
    truthEl.textContent = sc.truth;
    if (launchBtn) launchBtn.setAttribute("href", `/app/#/simulation?scenario=${key}`);

    stripEl.innerHTML = SNAP_WINDOWS.map((w, idx) => {
      const st = sc.snaps[idx];
      const isPick = idx === sc.pickIdx;
      return `<div class="as-node s-${st}${isPick ? " s-pick" : ""}" role="listitem">
        <b>W${String(w).padStart(2, "0")}</b>
        <span>${isPick ? "★ PICK" : st}</span>
      </div>`;
    }).join("");

    polEl.innerHTML = sc.policies.map((p) => `
      <div class="ap-card ${p.ok ? "ap-good" : "ap-bad"}">
        <div class="ap-head">
          <b>${p.name}</b>
          <span class="ap-badge">${p.badge}</span>
        </div>
        <div class="ap-pick">${p.pick}</div>
        <div class="ap-note">${p.note}</div>
      </div>
    `).join("");
  }

  document.querySelectorAll(".arena-tab").forEach((btn) => {
    btn.addEventListener("click", () => {
      document.querySelectorAll(".arena-tab").forEach((b) => {
        b.classList.remove("active");
        b.setAttribute("aria-selected", "false");
      });
      btn.classList.add("active");
      btn.setAttribute("aria-selected", "true");
      renderArena(btn.dataset.scen);
    });
  });

  renderArena("rapid_destructive");
})();

/* ============================================================
   4 · DETERMINISTIC VERIFIED HASH STREAM & API HEALTH
   ============================================================ */
(() => {
  const ul = document.getElementById("hashRows");
  if (!ul) return;
  const dirs = ["finance", "engineering", "legal", "ops"];
  const names = ["ledger-q4.csv", "architecture.md", "contracts.pdf", "runbook.yaml", "payroll.xlsx", "audit-2026.json", "roadmap.txt", "schema.sql"];
  const hex = "0123456789abcdef";
  let seed = 42;
  const rnd = () => ((seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296);
  let html = "";
  for (let i = 0; i < 12; i++) {
    const sha = Array.from({ length: 14 }, () => hex[(rnd() * 16) | 0]).join("");
    const name = `${dirs[(rnd() * 4) | 0]}/${names[i % names.length]}`;
    html += `<li>
      <svg class="tick" width="13" height="13" viewBox="0 0 13 13" aria-hidden="true"><path d="M2.5 7l2.6 2.6L10.5 4" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>
      <span class="tone-green">sha256:ok</span>
      <span style="min-width:195px;color:#EEF5FC">${name}</span>
      <span class="dim">${sha}…</span>
      <span class="bar"></span>
    </li>`;
  }
  ul.innerHTML = html;
})();

(async () => {
  const el = document.getElementById("apiStatus");
  if (!el) return;
  try {
    const r = await fetch("/api/v1/healthz");
    if (!r.ok) throw 0;
    el.classList.add("live");
    el.innerHTML = "<i></i>API live · 127.0.0.1";
  } catch {
    el.innerHTML = "<i></i>API standby · run scripts/demo.ps1";
  }
})();

/* ============================================================
   5 · SCROLL TIMELINE, HUD TELEMETRY & CHOREOGRAPHY LOOP
   ============================================================ */
const chapters = [...document.querySelectorAll(".chapter")];
const railLinks = [...document.querySelectorAll(".rail a")];
const reveals = [...document.querySelectorAll("[data-reveal]")];
const scrollFill = document.getElementById("scrollFill");
const railProgress = document.getElementById("railProgress");

const hudClock = document.getElementById("hudClock");
const hudWin = document.getElementById("hudWin");
const hudEntropy = document.getElementById("hudEntropy");
const hudVerdict = document.getElementById("hudVerdict");

const probeSnap = document.getElementById("probeSnap");
const probeEnt = document.getElementById("probeEnt");
const probeEntBar = document.getElementById("probeEntBar");
const probeExt = document.getElementById("probeExt");

const gimbalArc = document.getElementById("gimbalArc");
const gateSteps = [...document.querySelectorAll("#gateSteps li")];
const gatesPassedTxt = document.getElementById("gatesPassedTxt");

const progress = {};
let docProgress = 0;

function measure() {
  const vh = innerHeight;
  const maxScroll = Math.max(1, document.documentElement.scrollHeight - vh);
  docProgress = clamp(scrollY / maxScroll, 0, 1);

  for (const ch of chapters) {
    const id = ch.id || "hero";
    const r = ch.getBoundingClientRect();
    const total = Math.max(1, r.height + vh);
    progress[id] = clamp((vh - r.top) / total, 0, 1);
    ch._visible = r.top < vh * 0.72 && r.bottom > vh * 0.22;
  }
}

function updateDOM() {
  if (scrollFill) scrollFill.style.transform = `scaleX(${docProgress.toFixed(4)})`;
  if (railProgress) railProgress.style.transform = `scaleY(${docProgress.toFixed(4)})`;

  const active = chapters.find((c) => c._visible) || chapters[0];
  document.body.dataset.chapter = active.id;
  document.body.dataset.tone = active.dataset.tone || "origin";
  railLinks.forEach((a) => a.classList.toggle("active", a.dataset.rail === active.id));

  // Live HUD Telemetry linked to active chapter & scroll progress
  if (hudClock && hudWin && hudEntropy && hudVerdict) {
    const id = active.id;
    let w = Math.round(docProgress * 39);
    let ent = "4.18 b/B";
    let verd = "BASELINE CALM";
    let vCls = "tone-blue";

    if (id === "incident") {
      w = clamp(24 + Math.round((progress.incident || 0) * 16), 24, 39);
      const hVal = lerp(4.22, 7.95, clamp((progress.incident || 0) * 1.5, 0, 1));
      ent = `${hVal.toFixed(2)} b/B`;
      verd = w >= 27 ? "COMPROMISED (HASH OK)" : "APPROACHING ONSET";
      vCls = w >= 27 ? "tone-red" : "tone-amber";
    } else if (id === "evidence") {
      w = 27; ent = "7.95 b/B"; verd = "ONSET DETECTED W27"; vCls = "tone-red";
    } else if (id === "point") {
      w = 24; ent = "4.21 b/B"; verd = "LOCKED: SNAP-W24"; vCls = "tone-green";
    } else if (id === "restore") {
      w = 24; ent = "4.21 b/B"; verd = "ISOLATED VERIFIED"; vCls = "tone-green";
    } else if (id === "abstain") {
      w = 0; ent = "7.94 b/B"; verd = "NO_SAFE_CANDIDATE"; vCls = "tone-red";
    } else if (id === "lab") {
      w = 24; ent = "4.21 b/B"; verd = "DECISION ARENA"; vCls = "tone-blue";
    } else if (id === "proof" || id === "under") {
      w = 39; ent = "4.19 b/B"; verd = "0 / 14 UNSAFE PICKS"; vCls = "tone-green";
    }
    hudClock.textContent = winToUTC(w);
    hudWin.textContent = `W${String(w).padStart(2, "0")} / 40`;
    hudEntropy.textContent = ent;
    hudVerdict.textContent = verd;
    hudVerdict.className = vCls;
  }

  // Incident Vault Probe live scroll scrub
  if (probeSnap && progress.incident > 0) {
    const p = clamp((progress.incident - 0.12) / 0.65, 0, 1);
    const snapIdx = clamp(Math.floor(7 + p * 5.99), 7, 12);
    const sw = SNAP_WINDOWS[snapIdx];
    const poisoned = snapIdx >= 8;
    const hVal = poisoned ? (7.86 + (snapIdx - 8) * 0.022) : 4.21;
    probeSnap.textContent = `SNAP-W${sw} · ${winToUTC(sw)}`;
    probeEnt.textContent = `${hVal.toFixed(2)} bits/B`;
    probeEnt.className = poisoned ? "tone-red" : "tone-green";
    probeEntBar.style.width = `${Math.round((hVal / 8) * 100)}%`;
    probeEntBar.className = poisoned ? "m-red" : "m-green";
    probeExt.textContent = poisoned ? "90% .lockd (AFFECTED)" : "0% novel (CLEAN)";
    probeExt.className = poisoned ? "tone-red" : "tone-green";
  }

  // 5-Gate Orbital Gimbal scroll lock
  if (gimbalArc && gateSteps.length) {
    const p = RM ? 1 : clamp((progress.point - 0.14) / 0.56, 0, 1);
    const litCount = clamp(Math.ceil(p * 5), 0, 5);
    gateSteps.forEach((li, i) => li.classList.toggle("lit", i < litCount));
    const circ = 478;
    gimbalArc.style.strokeDashoffset = String(Math.round(circ * (1 - p)));
    if (gatesPassedTxt) gatesPassedTxt.textContent = `${litCount} / 5 GATES`;
  }

  // Reveal elements
  for (const el of reveals) {
    if (el.classList.contains("in")) continue;
    const r = el.getBoundingClientRect();
    if (r.top < innerHeight * 0.88 && r.bottom > 0) el.classList.add("in");
  }

  // Risk curve stroke draw
  const curve = document.querySelector(".curve-panel");
  if (curve && progress.evidence > 0.15) curve.classList.add("drawn");

  // Verified restore file stream
  const rows = document.querySelectorAll("#hashRows li");
  if (rows.length && progress.restore > 0.1) {
    const frac = RM ? 1 : clamp((progress.restore - 0.1) / 0.48, 0, 1);
    const shown = Math.round(frac * rows.length);
    rows.forEach((li, i) => li.classList.toggle("show", i < shown));
    const ok = document.getElementById("hashOk");
    if (ok) ok.textContent = String(Math.round(frac * 117));
  }

  // Proof stat counters
  document.querySelectorAll("[data-count]").forEach((el) => {
    if (!el._started && progress.proof > 0.12) {
      el._started = true;
      animateCount(el);
    }
  });
}

function animateCount(el) {
  const target = parseFloat(el.dataset.count);
  const dec = parseInt(el.dataset.decimals || "0", 10);
  const suf = el.dataset.suffix || "";
  const slash = el.dataset.slash;
  const t0 = performance.now();
  const dur = RM ? 0 : 1350;
  function step(t) {
    const p = dur ? clamp((t - t0) / dur, 0, 1) : 1;
    const ease = 1 - Math.pow(1 - p, 3);
    const v = (target * ease).toFixed(dec);
    el.querySelector("b").textContent = slash ? `${Math.round(Number(v))}/${slash}` : v + suf;
    if (p < 1) requestAnimationFrame(step);
  }
  requestAnimationFrame(step);
}

let rafActive = false;
function loop(t) {
  if (document.hidden) { rafActive = false; return; }
  measure();
  Scene.setScrollState(progress);
  updateDOM();
  Scene.frame(t);
  if (!RM) requestAnimationFrame(loop);
  else rafActive = false;
}
function startLoop() {
  if (!rafActive && !document.hidden) {
    rafActive = true;
    requestAnimationFrame(loop);
  }
}

document.addEventListener("visibilitychange", () => {
  if (!document.hidden) startLoop();
});

if (RM) {
  measure();
  Scene.setScrollState(progress);
  updateDOM();
  Scene.stillFrame();
  addEventListener("scroll", () => { measure(); Scene.setScrollState(progress); updateDOM(); }, { passive: true });
} else {
  startLoop();
}

/* ============================================================
   6 · MICRO-INTERACTIONS: CURSOR, SPOTLIGHTS, 3D TILT & MAGNETS
   ============================================================ */
if (FINE && !RM) {
  const cur = document.getElementById("cursor");
  const glow = cur.querySelector(".c-glow");
  const dot = cur.querySelector(".c-dot");
  const ring = cur.querySelector(".c-ring");
  const label = cur.querySelector(".c-label");
  let cx = innerWidth / 2, cy = innerHeight / 2, rx = cx, ry = cy, gx = cx, gy = cy;

  addEventListener("mousemove", (e) => {
    cx = e.clientX; cy = e.clientY;
    dot.style.transform = `translate3d(${cx}px,${cy}px,0) translate(-50%,-50%)`;
  }, { passive: true });

  (function cursorTick() {
    if (!document.hidden) {
      rx = lerp(rx, cx, 0.18);
      ry = lerp(ry, cy, 0.18);
      gx = lerp(gx, cx, 0.09);
      gy = lerp(gy, cy, 0.09);
      ring.style.transform = `translate3d(${rx}px,${ry}px,0) translate(-50%,-50%)`;
      glow.style.transform = `translate3d(${gx}px,${gy}px,0) translate(-50%,-50%)`;
      label.style.transform = `translate3d(${rx}px,${ry + 38}px,0) translate(-50%,0)`;
    }
    requestAnimationFrame(cursorTick);
  })();

  addEventListener("mousedown", () => cur.classList.add("down"));
  addEventListener("mouseup", () => cur.classList.remove("down"));

  // Context-aware cursor states
  const bindHover = (el) => {
    el.addEventListener("mouseenter", () => {
      cur.classList.add("hover");
      label.textContent = el.dataset.cursor || "open";
    });
    el.addEventListener("mouseleave", () => cur.classList.remove("hover"));
  };
  document.querySelectorAll("a, button, [data-cursor]").forEach(bindHover);

  // Specular radial spotlight on every .spot-card
  document.querySelectorAll(".spot-card").forEach((card) => {
    card.addEventListener("mousemove", (e) => {
      const r = card.getBoundingClientRect();
      card.style.setProperty("--mx", `${(e.clientX - r.left).toFixed(1)}px`);
      card.style.setProperty("--my", `${(e.clientY - r.top).toFixed(1)}px`);
    });
  });

  // 3D Perspective Tilt on Proof Stat Cards
  document.querySelectorAll("[data-tilt]").forEach((card) => {
    card.addEventListener("mousemove", (e) => {
      const r = card.getBoundingClientRect();
      const nx = (e.clientX - r.left) / r.width - 0.5;
      const ny = (e.clientY - r.top) / r.height - 0.5;
      card.style.transform = `rotateX(${(-ny * 10).toFixed(2)}deg) rotateY(${(nx * 12).toFixed(2)}deg) translateY(-4px)`;
    });
    card.addEventListener("mouseleave", () => {
      card.style.transform = "";
    });
  });

  // Magnetic Buttons with inner text counter-parallax
  document.querySelectorAll("[data-magnetic]").forEach((btn) => {
    const inner = btn.querySelector("span");
    btn.addEventListener("mousemove", (e) => {
      const r = btn.getBoundingClientRect();
      const dx = e.clientX - (r.left + r.width / 2);
      const dy = e.clientY - (r.top + r.height / 2);
      btn.style.transform = `translate3d(${(dx * 0.16).toFixed(1)}px,${(dy * 0.22).toFixed(1)}px,0)`;
      if (inner) inner.style.transform = `translate3d(${(dx * 0.06).toFixed(1)}px,${(dy * 0.08).toFixed(1)}px,0)`;
    });
    btn.addEventListener("mouseleave", () => {
      btn.style.transform = "";
      if (inner) inner.style.transform = "";
    });
  });
}
