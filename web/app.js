/* CleanPoint operator console (M1 static console per PRD).
   Vanilla JS, hash routing, same-origin API calls with bearer token.
   Status vocabulary is always labelled text + color, never color alone. */
"use strict";

const S = {
  token: sessionStorage.getItem("cp_token") || "",
  role: null,            // "operator" | "read-only"
  workload: sessionStorage.getItem("cp_workload") || "",
  workloads: [],
};

const $ = (sel, el = document) => el.querySelector(sel);
const $$ = (sel, el = document) => [...el.querySelectorAll(sel)];
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const fmtTime = (iso) => iso ? iso.replace("T", " ").replace(".000Z", " UTC").replace("Z", " UTC") : "—";
const num = (n) => (n == null ? "—" : Number(n).toLocaleString());
const fmtBytes = (b) => b == null ? "—" : b < 1048576 ? `${(b/1024).toFixed(1)} KiB` : `${(b/1048576).toFixed(2)} MiB`;

function toast(msg, isErr = false) {
  const t = document.createElement("div");
  t.className = "toast" + (isErr ? " err" : "");
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => t.remove(), 4200);
}

/* ---------- API ---------- */
async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (S.token) opts.headers["Authorization"] = "Bearer " + S.token;
  if (body !== undefined) { opts.headers["Content-Type"] = "application/json"; opts.body = JSON.stringify(body); }
  const resp = await fetch("/api/v1" + path, opts);
  let data = {};
  try { data = await resp.json(); } catch { /* non-JSON */ }
  if (resp.status === 401 && S.token) { logout(); throw new Error("session expired — sign in again"); }
  if (!resp.ok) {
    const d = data && data.details && data.details.length ? ` (${data.details.join("; ")})` : "";
    throw new Error(`${data.message || resp.statusText}${d}`);
  }
  return data;
}

function logout() {
  sessionStorage.removeItem("cp_token");
  S.token = ""; S.role = null;
  location.hash = "#/login";
  render();
}

/* ---------- boot / login ---------- */
async function boot() {
  if (!S.token) { location.hash = "#/login"; return render(); }
  try {
    await api("GET", "/config");                       // token validity
    try { await api("GET", "/audit-events"); S.role = "operator"; }
    catch { S.role = "read-only"; }
    if (!location.hash || location.hash === "#/login") location.hash = "#/overview";
  } catch (e) {
    sessionStorage.removeItem("cp_token"); S.token = "";
    location.hash = "#/login";
    toast(String(e.message || e), true);
  }
  render();
}

function viewLogin() {
  $("#app").innerHTML = `
  <div class="login-wrap">
    <form class="login-card" id="loginForm">
      <h1><span class="logo">CP</span> CleanPoint</h1>
      <p class="login-sub">Ransomware-recovery analysis console — <b>local simulation only</b>.
      No production data-protection guarantees.</p>
      <div class="login-hint">Token lives in <code>sandbox/.tokens</code> after
      <code>scripts/demo.sh</code> — use <code>operator=…</code> for full access or
      <code>readonly=…</code> for browse-only.</div>
      <label class="fld" for="tok">Bearer token</label>
      <input type="password" id="tok" autocomplete="off" placeholder="paste token" required>
      <p class="login-error" id="loginErr"></p>
      <button class="btn primary" type="submit" style="width:100%">Sign in</button>
    </form>
  </div>`;
  $("#loginForm").addEventListener("submit", async (e) => {
    e.preventDefault();
    S.token = $("#tok").value.trim();
    try {
      await api("GET", "/config");
      try { await api("GET", "/audit-events"); S.role = "operator"; }
      catch { S.role = "read-only"; }
      sessionStorage.setItem("cp_token", S.token);
      location.hash = "#/overview";
      render();
    } catch (err) {
      S.token = "";
      $("#loginErr").textContent = "Invalid token: " + (err.message || err);
    }
  });
}

/* ---------- shell ---------- */
const NAV = [
  { sec: "Monitor" },
  { id: "overview", label: "Overview", icon: "▦", route: "#/overview" },
  { id: "workloads", label: "Workloads", icon: "▤", route: "#/workloads" },
  { sec: "Investigate" },
  { id: "snapshots", label: "Snapshots", icon: "◷", route: "#/go/snapshots" },
  { id: "findings", label: "Detection", icon: "◔", route: "#/go/findings" },
  { id: "events", label: "Events", icon: "≡", route: "#/go/events" },
  { id: "recovery", label: "Recovery", icon: "⤺", route: "#/go/recovery" },
  { sec: "Trust" },
  { id: "audit", label: "Audit log", icon: "▦", route: "#/audit" },
  { id: "settings", label: "Settings", icon: "⚙", route: "#/settings" },
];

function shell(contentHTML, activeId) {
  const wlOpts = S.workloads.map(w =>
    `<option value="${esc(w.id)}" ${w.id === S.workload ? "selected" : ""}>${esc(w.name)}</option>`).join("");
  $("#app").innerHTML = `
  <div class="shell">
    <aside class="side">
      <div class="brand"><span class="logo">CP</span><span>CleanPoint</span></div>
      <nav>
        ${NAV.map(n => n.sec ? `<div class="nav-sec">${n.sec}</div>` :
          `<a class="nav-item ${n.id === activeId ? "active" : ""}" href="${n.route}">
             <span>${n.icon}</span><span class="txt">${n.label}</span></a>`).join("")}
      </nav>
      <div class="foot">v1.0.0 · LOCAL SIMULATION<br>not a production product</div>
    </aside>
    <div class="main">
      <div class="topbar">
        <span class="badge-env">LOCAL SIMULATION</span>
        <span class="wl-pick">workload
          <select id="wlPick">${wlOpts || '<option value="">— none —</option>'}</select>
        </span>
        <span class="spacer"></span>
        <span class="role-chip">role: <b>${S.role === "operator" ? "operator" : "read-only"}</b></span>
        <button class="btn-logout" id="btnLogout" title="disconnect">⏻ sign out</button>
      </div>
      <div class="content">${contentHTML}</div>
    </div>
  </div>`;
  $("#btnLogout").addEventListener("click", logout);
  $("#wlPick").addEventListener("change", (e) => {
    S.workload = e.target.value;
    sessionStorage.setItem("cp_workload", S.workload);
    const tab = ({ snapshots: "snapshots", findings: "findings", events: "events", recovery: "recovery" })[activeId];
    location.hash = tab && S.workload ? `#/workload/${S.workload}/${tab}` : location.hash;
    if (tab) render();
  });
}

async function refreshWorkloads() {
  const d = await api("GET", "/workloads");
  S.workloads = d.workloads || [];
  if (!S.workload && S.workloads.length) {
    S.workload = S.workloads[0].id;
    sessionStorage.setItem("cp_workload", S.workload);
  }
}

function requireWorkload() {
  if (S.workload) return true;
  shell(`<div class="empty">No workloads registered yet.<br><br>
    Run <code>scripts/demo.sh</code> or <code>cleanpoint simulate</code>, or register one under Workloads.</div>`, "overview");
  return false;
}

/* ---------- overview ---------- */
async function viewOverview() {
  shell(`<h2 class="page">Overview</h2><div class="empty">Loading…</div>`, "overview");
  await refreshWorkloads();
  const cards = { workloads: S.workloads.length, snapshots: 0, suspect: 0, affected: 0, verified: 0, lastRun: null, abstained: 0 };
  const rows = [];
  for (const w of S.workloads) {
    let snaps = [], findings = null, cand = null, runs = [];
    try { snaps = (await api("GET", `/workloads/${w.id}/snapshots`)).snapshots || []; } catch {}
    try { findings = await api("GET", `/workloads/${w.id}/findings`); } catch {}
    try { cand = await api("GET", `/workloads/${w.id}/recovery-candidates`); } catch {}
    try { runs = (await api("GET", `/workloads/${w.id}/recovery-runs`)).recovery_runs || []; } catch {}
    cards.snapshots += snaps.length;
    const suspectSnaps = snaps.filter(s => s.assessment && ["suspect", "affected"].includes(s.assessment.exposure_status));
    cards.suspect += suspectSnaps.filter(s => s.assessment.exposure_status === "suspect").length;
    cards.affected += suspectSnaps.filter(s => s.assessment.exposure_status === "affected").length;
    const okRuns = runs.filter(r => r.status === "verified");
    cards.verified += okRuns.length;
    if (okRuns.length && (!cards.lastRun || okRuns[0].created_at > cards.lastRun.created_at)) cards.lastRun = okRuns[0];
    if (cand && cand.status === "no_safe_candidate") cards.abstained++;
    const susWin = findings ? (findings.windows || []).filter(x => x.alert_level === "suspicious").length : 0;
    const exposure = snaps.length && snaps[0].assessment ? snaps[0].assessment.exposure_status : null;
    rows.push(`
      <tr class="click" data-wl="${esc(w.id)}">
        <td><b>${esc(w.name)}</b><div class="path">${esc(w.id)}</div></td>
        <td>${num(snaps.length)}</td>
        <td>${exposure ? chip(exposure) : chip("neutral", "no analysis")}</td>
        <td>${findings ? (findings.incident_onset ? `<b style="color:var(--red)">${fmtTime(findings.incident_onset)}</b>` : "no onset") : "—"}</td>
        <td>${susWin}</td>
        <td>${cand ? (cand.status === "candidate_available" ? chip("eligible", "candidate") : chip("no_safe_candidate", "abstained")) : "—"}</td>
        <td>${okRuns.length ? chip("verified", `verified ×${okRuns.length}`) : "—"}</td>
      </tr>`);
  }
  shell(`
    <h2 class="page">Overview <span class="badge-env">LOCAL SIMULATION</span></h2>
    <div class="cards">
      <div class="stat"><div class="num">${cards.workloads}</div><div class="lbl">workloads</div></div>
      <div class="stat"><div class="num">${cards.snapshots}</div><div class="lbl">sealed snapshots</div></div>
      <div class="stat ${cards.affected ? "warn" : ""}"><div class="num">${cards.affected}</div><div class="lbl">affected snapshots</div></div>
      <div class="stat"><div class="num">${cards.suspect}</div><div class="lbl">suspect snapshots</div></div>
      <div class="stat"><div class="num">${cards.verified}</div><div class="lbl">verified restores</div></div>
      <div class="stat"><div class="num">${cards.abstained}</div><div class="lbl">abstentions (no safe candidate)</div></div>
    </div>
    ${cards.lastRun ? `<div class="banner ok"><b>Last verified recovery</b>
      ${esc(cards.lastRun.id)} → ${num(cards.lastRun.verified_files)} files verified,
      source unchanged, ${fmtTime(cards.lastRun.created_at)}</div>` : ""}
    ${rows.length ? `
    <table class="tbl">
      <thead><tr><th>Workload</th><th class="num">Snapshots</th><th>Newest exposure</th>
      <th>Incident onset</th><th class="num">Suspicious windows</th><th>Selector</th><th>Restores</th></tr></thead>
      <tbody>${rows.join("")}</tbody>
    </table>` : `<div class="empty">No workloads yet — run the demo or register one.</div>`}`, "overview");
  $$("tr.click").forEach(tr => tr.addEventListener("click", () => { location.hash = `#/workload/${tr.dataset.wl}`; render(); }));
}

/* ---------- workloads ---------- */
async function viewWorkloads() {
  shell(`<h2 class="page">Workloads</h2><div class="empty">Loading…</div>`, "workloads");
  await refreshWorkloads();
  const rows = S.workloads.map(w => `
    <tr class="click" data-wl="${esc(w.id)}">
      <td><b>${esc(w.name)}</b></td>
      <td class="path">${esc(w.id)}</td>
      <td class="path">${esc(w.root_path.split("/").slice(-1)[0] || w.root_path)}</td>
      <td>${fmtTime(w.created_at)}</td>
      <td>→</td>
    </tr>`).join("");
  shell(`
    <h2 class="page">Workloads
      ${S.role === "operator" ? `<button class="btn primary" id="btnNewWl" style="margin-left:auto">+ Register workload</button>` : ""}
    </h2>
    <p style="color:var(--muted)">Workload roots are sandbox-allowlisted; the client never supplies an
    unrestricted destination. Registration via API (<code>POST /workloads</code>).</p>
    ${rows ? `<table class="tbl">
      <thead><tr><th>Name</th><th>ID</th><th>Root</th><th>Created</th><th></th></tr></thead>
      <tbody>${rows}</tbody></table>` : `<div class="empty">No workloads registered.</div>`}`, "workloads");
  $$("tr.click").forEach(tr => tr.addEventListener("click", () => { location.hash = `#/workload/${tr.dataset.wl}`; render(); }));
  if (S.role === "operator") $("#btnNewWl").addEventListener("click", () => {
    modal(`<h3>Register workload</h3>
      <p class="login-sub">The server creates the root under its allowlisted sandbox — never an arbitrary host path.</p>
      <label class="fld">Name (lowercase, digits, dashes)</label>
      <input type="text" id="mName" placeholder="e.g. demo-batch-01">
      <div class="actions">
        <button class="btn" data-close>Cancel</button>
        <button class="btn primary" id="mGo">Register</button>
      </div>`, (root) => {
        $("#mGo", root).addEventListener("click", async () => {
          try {
            const w = await api("POST", "/workloads", { name: $("#mName", root).value.trim() });
            toast(`Workload ${w.name} registered`);
            root.remove(); location.hash = `#/workload/${w.id}`; render();
          } catch (e) { toast(String(e.message || e), true); }
        });
      });
  });
}

/* ---------- workload detail (tabs) ---------- */
async function viewWorkload(id, tab = "overview") {
  shell(`<h2 class="page">Workload</h2><div class="empty">Loading…</div>`, tab);
  let w; try { w = await api("GET", "/workloads").then(d => d.workloads.find(x => x.id === id)); }
  catch (e) { return shell(`<div class="banner error">${esc(e.message)}</div>`, "workloads"); }
  if (!w) return shell(`<div class="banner error">Unknown workload ${esc(id)}</div>`, "workloads");
  S.workload = id; sessionStorage.setItem("cp_workload", id);
  const tabs = ["overview", "events", "snapshots", "findings", "recovery"];
  const nav = tabs.map(t => `<div class="tab ${t === tab ? "active" : ""}" data-tab="${t}">${t[0].toUpperCase() + t.slice(1)}</div>`).join("");
  const head = `
    <div class="crumbs"><a href="#/workloads">Workloads</a> / ${esc(w.name)}</div>
    <h2 class="page">${esc(w.name)}
      <span class="chip neutral">${esc(w.id)}</span>
      ${S.role === "operator" ? `
        <button class="btn" id="btnCapture" style="margin-left:auto">Capture snapshot</button>
        <button class="btn primary" id="btnAnalyze">Analyze</button>` : ""}
    </h2>
    <div class="tabs">${nav}</div>`;
  const bindHead = () => {
    $$(".tab").forEach(t => t.addEventListener("click", () => { location.hash = `#/workload/${id}/${t.dataset.tab}`; }));
    if (S.role === "operator") {
      $("#btnCapture")?.addEventListener("click", async () => {
        try { const s = await api("POST", `/workloads/${id}/snapshots`, {}); toast(`Snapshot ${s.id} sealed (${s.file_count} files)`); render(); }
        catch (e) { toast(String(e.message || e), true); }
      });
      $("#btnAnalyze")?.addEventListener("click", async () => {
        try { await api("POST", `/workloads/${id}/analyze`, {}); toast("Analysis complete"); render(); }
        catch (e) { toast(String(e.message || e), true); }
      });
    }
  };

  if (tab === "overview") {
    let snaps = [], cand = null, runs = [], findings = null;
    try { snaps = (await api("GET", `/workloads/${id}/snapshots`)).snapshots || []; } catch {}
    try { cand = await api("GET", `/workloads/${id}/recovery-candidates`); } catch {}
    try { runs = (await api("GET", `/workloads/${id}/recovery-runs`)).recovery_runs || []; } catch {}
    try { findings = await api("GET", `/workloads/${id}/findings`); } catch {}
    const latest = snaps[0];
    shell(head + `
      <div class="cards">
        <div class="stat"><div class="num">${num(snaps.length)}</div><div class="lbl">snapshots</div></div>
        <div class="stat"><div class="num">${latest ? num(latest.file_count) : 0}</div><div class="lbl">files in newest snapshot</div></div>
        <div class="stat"><div class="num">${latest ? fmtBytes(latest.total_bytes) : "—"}</div><div class="lbl">newest snapshot size</div></div>
        <div class="stat"><div class="num">${findings ? (findings.windows || []).filter(x => x.alert_level === "suspicious").length : "—"}</div><div class="lbl">suspicious windows</div></div>
      </div>
      <div class="card"><h3 class="sec" style="margin-top:0">Latest snapshot</h3>
        ${latest ? `<dl class="kv">
          <dt>Snapshot</dt><dd class="path">${esc(latest.id)}</dd>
          <dt>Captured</dt><dd>${fmtTime(latest.captured_at)}</dd>
          <dt>Status</dt><dd>${chip(latest.status)}</dd>
          <dt>Manifest SHA-256</dt><dd class="path">${esc(latest.manifest_hash || "—")}</dd>
          <dt>Assessment</dt><dd>${latest.assessment ? chip(latest.assessment.exposure_status) + " · integrity " + (latest.assessment.integrity_ok ? "OK" : "FAILED") : "not analyzed yet"}</dd>
        </dl>` : `<div class="empty">No snapshots. Capture one (operator) or run the simulator.</div>`}
      </div>
      <h3 class="sec">Recovery runs</h3>
      ${runs.length ? `<table class="tbl"><thead><tr><th>Run</th><th>Snapshot</th><th>Status</th>
        <th class="num">Verified</th><th class="num">Failed</th><th>Source</th><th>Created</th></tr></thead>
        <tbody>${runs.map(r => `<tr><td class="path">${esc(r.id)}</td><td class="path">${esc(r.snapshot_id)}</td>
          <td>${chip(r.status)}</td><td class="num">${num(r.verified_files)}</td><td class="num">${num(r.failed_files)}</td>
          <td>${r.source_unchanged ? chip("verified", "unchanged") : "—"}</td><td>${fmtTime(r.created_at)}</td></tr>`).join("")}</tbody></table>`
        : `<div class="empty">No restores performed for this workload.</div>`}`, "overview");
    bindHead();
    return;
  }

  if (tab === "events") {
    let events = [];
    try { events = (await api("GET", `/workloads/${id}/events?limit=1000`)).events || []; } catch {}
    const ops = ["", "create", "modify", "delete", "rename"];
    shell(head + `
      <p style="color:var(--muted)">Ingested file actions (most recent ${Math.min(events.length, 1000)}).
      Long paths truncate visually but remain retrievable via the API.</p>
      <div style="display:flex;gap:10px;margin-bottom:12px;align-items:center">
        <select id="evOp" style="width:160px">${ops.map(o => `<option value="${o}">${o || "all operations"}</option>`).join("")}</select>
        <input type="text" id="evQ" placeholder="filter by path…" style="max-width:320px">
      </div>
      <table class="tbl" id="evTbl">
        <thead><tr><th>Time</th><th>Operation</th><th>Path</th><th>Destination</th>
        <th class="num">Entropy old→new</th><th class="num">Size new</th><th>Actor</th></tr></thead>
        <tbody></tbody></table>`, "events");
    bindHead();
    const renderRows = () => {
      const op = $("#evOp").value, q = $("#evQ").value.toLowerCase();
      const rows = events.filter(e => (!op || e.operation === op) && (!q || (e.path || "").toLowerCase().includes(q) || ((e.dest_path || "")).toLowerCase().includes(q)))
        .map(e => `<tr>
          <td class="path">${esc(e.timestamp)}</td>
          <td>${chip(e.operation === "delete" ? "failed" : e.operation === "create" ? "verified" : "neutral", e.operation)}</td>
          <td class="path">${esc(e.path)}</td>
          <td class="path">${esc(e.dest_path || "—")}</td>
          <td class="num">${e.old_entropy != null ? e.old_entropy.toFixed(2) : "—"} → ${e.new_entropy != null ? e.new_entropy.toFixed(2) : "—"}</td>
          <td class="num">${num(e.new_size)}</td><td>${esc(e.actor || "—")}</td></tr>`).join("");
      $("#evTbl tbody").innerHTML = rows || `<tr><td colspan="7"><div class="empty">No events match.</div></td></tr>`;
    };
    $("#evOp").addEventListener("change", renderRows);
    $("#evQ").addEventListener("input", renderRows);
    renderRows();
    return;
  }

  if (tab === "snapshots") { await viewSnapshots(id, head, bindHead); return; }
  if (tab === "findings") { await viewFindings(id, head, bindHead); return; }
  if (tab === "recovery") { await viewRecovery(id, head, bindHead); return; }
}

/* ---------- snapshots timeline ---------- */
async function viewSnapshots(id, head, bindHead) {
  let snaps = [];
  try { snaps = (await api("GET", `/workloads/${id}/snapshots`)).snapshots || []; } catch (e) {
    return shell(head + `<div class="banner error">${esc(e.message)}</div>`, "snapshots");
  }
  const chrono = [...snaps].reverse();
  const cls = (s) => s.assessment ? `c-${s.assessment.exposure_status}` : "c-none";
  const label = (s) => s.assessment ? s.assessment.exposure_status : "no analysis";
  shell(head + `
    <div class="timeline">
      <div class="tl-track">
        ${chrono.map((s, i) => `
          <div class="tl-node ${cls(s)}" data-snap="${esc(s.id)}">
            ${i ? '<div class="tl-stem"></div>' : ""}
            <div class="tl-cap">${label(s)}</div>
            <div class="dot" title="${esc(s.id)}"></div>
            <div class="tl-time">${esc(s.captured_at.slice(5, 16).replace("T", " "))}</div>
            <div class="tl-files">${num(s.file_count)} files</div>
          </div>`).join("") || ""}
      </div>
      <div class="tl-legend">
        ${["clear", "suspect", "affected", "unknown"].map(x => chip(x, x)).join("")}
        ${chip("neutral", "no analysis")}
      </div>
    </div>
    <p class="chart-note">Snapshots are chronologically ordered. A <b>valid hash does not mean clean</b> —
    exposure comes from behavioral + content analysis, not checksums.</p>
    <table class="tbl">
      <thead><tr><th>Captured</th><th>Snapshot</th><th>Status</th><th>Integrity</th>
      <th>Exposure</th><th class="num">Files</th><th class="num">Size</th><th>Manifest</th></tr></thead>
      <tbody>${snaps.map(s => `
        <tr class="click snaprow" data-snap="${esc(s.id)}">
          <td class="path">${esc(s.captured_at)}</td>
          <td class="path">${esc(s.id)}</td>
          <td>${chip(s.status)}</td>
          <td>${s.assessment ? (s.assessment.integrity_ok ? chip("verified", "OK") : chip("failed", "FAILED")) : "—"}</td>
          <td>${s.assessment ? chip(s.assessment.exposure_status) : chip("neutral", "—")}</td>
          <td class="num">${num(s.file_count)}</td>
          <td class="num">${fmtBytes(s.total_bytes)}</td>
          <td class="path">${esc(s.manifest_hash ? s.manifest_hash.slice(0, 12) + "…" : "—")}</td>
        </tr>`).join("")}</tbody>
    </table>`, "snapshots");
  bindHead();
  $$(".snaprow, .tl-node").forEach(el => el.addEventListener("click", () => snapDetail(el.dataset.snap, snaps)));
}

function snapDetail(snapId, snaps) {
  const s = snaps.find(x => x.id === snapId);
  if (!s) return;
  const a = s.assessment;
  modal(`<h3>Snapshot ${esc(s.id)}</h3>
    <dl class="kv">
      <dt>Captured at</dt><dd>${fmtTime(s.captured_at)}</dd>
      <dt>Status</dt><dd>${chip(s.status)}</dd>
      <dt>Files / size</dt><dd>${num(s.file_count)} files · ${fmtBytes(s.total_bytes)}</dd>
      <dt>Manifest SHA-256</dt><dd class="path">${esc(s.manifest_hash || "—")}</dd>
      <dt>Integrity</dt><dd>${a ? (a.integrity_ok ? chip("verified", "verified") : chip("failed", "failed")) : "—"}</dd>
      <dt>Exposure</dt><dd>${a ? chip(a.exposure_status) : chip("neutral", "not analyzed")}</dd>
      <dt>Risk score (max ended window)</dt><dd>${a ? a.risk_score.toFixed(3) : "—"}</dd>
      <dt>Affected files</dt><dd>${a ? num(a.affected_file_count) : "—"}</dd>
    </dl>
    ${a && a.explanation && a.explanation.length ? `
      <h3 class="sec">Why this classification</h3>
      <ul>${a.explanation.map(x => `<li>${esc(x)}</li>`).join("")}</ul>` : ""}
    <p class="chart-note">Sealed snapshots expose no mutation or delete controls.</p>
    <div class="actions"><button class="btn" data-close>Close</button></div>`);
}

/* ---------- findings + risk chart ---------- */
async function viewFindings(id, head, bindHead) {
  let f; try { f = await api("GET", `/workloads/${id}/findings`); }
  catch (e) { return shell(head + `<div class="banner warn"><b>No analysis yet</b>Run <i>Analyze</i> (operator)
    or <code>POST /workloads/{id}/analyze</code> first.</div>`, "findings"); }
  const wins = f.windows || [];
  const sus = wins.filter(w => w.alert_level === "suspicious");
  shell(head + `
    <div class="banner info"><b>Detector ${esc(f.detector_version)} · threshold ${f.risk_threshold}</b>
      risk_index is a <b>relative anomaly score (0..1), not a probability</b>. Onset is the earliest
      suspicious window start — a conservative lower bound, not a proven infection time.</div>
    ${f.incident_onset ? `<div class="banner error"><b>Estimated earliest plausible onset:
      ${fmtTime(f.incident_onset)}</b> Snapshots captured at/after this time are not eligible for recovery.</div>` : ""}
    <div class="chart-wrap">
      ${riskChartSVG(wins, f.risk_threshold, f.incident_onset, f.baseline_windows)}
      <div class="chart-note">Each point is one ${f.window_seconds}s window. Shaded region = assumed-normal
      baseline (${f.baseline_windows} windows). Dashed line = alert threshold. Red marker = onset.</div>
    </div>
    <h3 class="sec">Suspicious windows (${sus.length})</h3>
    ${sus.length ? `<table class="tbl"><thead><tr><th>Window start</th><th class="num">Risk</th>
      <th>Contributing signals</th><th class="num">Events</th></tr></thead>
      <tbody>${sus.map(w => `<tr>
        <td class="path">${esc(w.starts_at)}</td>
        <td class="num"><b style="color:var(--red)">${w.risk_score.toFixed(3)}</b></td>
        <td>${(w.signal_contributions || []).filter(c => c.signal >= 0.2).map(c =>
            `<span class="chip neutral">${esc(c.feature)} ×${c.signal.toFixed(2)}</span>`).join(" ")}</td>
        <td class="num">${w.event_count}</td></tr>`).join("")}</tbody></table>`
      : `<div class="empty">No suspicious windows in the evaluated period.</div>`}
    <h3 class="sec">Behaviorally affected files (${(f.affected_files || []).length})</h3>
    ${(f.affected_files || []).length ? `
      <p class="chart-note">Flagged from event metadata in suspicious windows — behavioral evidence,
      <b>not</b> proven ransomware encryption.</p>
      <ul class="filelist">${f.affected_files.map(p => `<li>${esc(p)}</li>`).join("")}</ul>`
      : `<div class="empty">No affected files flagged.</div>`}
    <details style="margin-top:16px"><summary style="cursor:pointer;color:var(--blue)">All windows (raw)</summary>
      <table class="tbl" style="margin-top:10px"><thead><tr><th>Start</th><th>Baseline?</th>
      <th class="num">Events</th><th class="num">Risk</th><th>Alert</th></tr></thead>
      <tbody>${wins.map(w => `<tr><td class="path">${esc(w.starts_at)}</td>
        <td>${w.is_baseline ? "yes" : ""}</td><td class="num">${w.event_count}</td>
        <td class="num">${w.is_baseline ? "—" : w.risk_score.toFixed(3)}</td>
        <td>${w.is_baseline ? "—" : chip(w.alert_level === "suspicious" ? "suspicious" : "normal", w.alert_level)}</td></tr>`).join("")}</tbody></table>
    </details>`, "findings");
  bindHead();
}

function riskChartSVG(wins, threshold, onset, baselineCount) {
  if (!wins.length) return `<div class="empty">No windows.</div>`;
  const W = 940, H = 260, P = { l: 44, r: 14, t: 18, b: 30 };
  const iw = W - P.l - P.r, ih = H - P.t - P.b;
  const t0 = Date.parse(wins[0].starts_at), t1 = Date.parse(wins[wins.length - 1].starts_at) || t0 + 1;
  const x = (iso) => P.l + (Date.parse(iso) - t0) / (t1 - t0 || 1) * iw;
  const y = (r) => P.t + (1 - Math.min(r, 1)) * ih;
  const baseEnd = baselineCount < wins.length ? wins[baselineCount - 1].ends_at : null;
  const pts = wins.filter(w => !w.is_baseline);
  const line = pts.map((w, i) => `${i ? "L" : "M"}${x(w.starts_at).toFixed(1)},${y(w.risk_score).toFixed(1)}`).join(" ");
  const dots = pts.map(w => w.alert_level === "suspicious"
    ? `<circle cx="${x(w.starts_at).toFixed(1)}" cy="${y(w.risk_score).toFixed(1)}" r="4.5" fill="var(--red)"><title>${esc(w.starts_at)} risk ${w.risk_score.toFixed(3)}</title></circle>`
    : `<circle cx="${x(w.starts_at).toFixed(1)}" cy="${y(w.risk_score).toFixed(1)}" r="3" fill="var(--blue)"><title>${esc(w.starts_at)} risk ${w.risk_score.toFixed(3)}</title></circle>`).join("");
  const onsetLine = onset ? `<line x1="${x(onset)}" y1="${P.t}" x2="${x(onset)}" y2="${P.t + ih}"
    stroke="var(--red)" stroke-width="2" stroke-dasharray="6 4"/>
    <text x="${x(onset) + 5}" y="${P.t + 12}" fill="var(--red)" font-size="11" font-weight="700">onset</text>` : "";
  const grid = [0, .25, .5, .75, 1].map(v =>
    `<line x1="${P.l}" y1="${y(v)}" x2="${W - P.r}" y2="${y(v)}" stroke="#E4EAF1"/>
     <text x="${P.l - 8}" y="${y(v) + 4}" text-anchor="end" font-size="10" fill="#5B6B7C">${v.toFixed(2)}</text>`).join("");
  return `<svg viewBox="0 0 ${W} ${H}" role="img" aria-label="Risk index per 300-second window; baseline region shaded; threshold ${threshold}; suspicious windows marked red">
    ${grid}
    ${baseEnd ? `<rect x="${P.l}" y="${P.t}" width="${(x(baseEnd) - P.l).toFixed(1)}" height="${ih}" fill="#11253F" opacity="0.07"/>
      <text x="${P.l + 6}" y="${P.t + 13}" font-size="10" fill="#5B6B7C">baseline (assumed normal)</text>` : ""}
    <line x1="${P.l}" y1="${y(threshold)}" x2="${W - P.r}" y2="${y(threshold)}" stroke="var(--amber)" stroke-dasharray="5 4"/>
    <text x="${W - P.r - 4}" y="${y(threshold) - 5}" text-anchor="end" font-size="10" fill="var(--amber)">threshold ${threshold}</text>
    <path d="${line}" fill="none" stroke="var(--blue)" stroke-width="1.6"/>
    ${dots}${onsetLine}
  </svg>`;
}

/* ---------- recovery candidates + restore ---------- */
async function viewRecovery(id, head, bindHead) {
  let cand; try { cand = await api("GET", `/workloads/${id}/recovery-candidates`); }
  catch (e) { return shell(head + `<div class="banner warn"><b>No analysis yet</b>
    Run <i>Analyze</i> first — the selector refuses to rank without evidence.</div>`, "recovery"); }
  const avail = cand.status === "candidate_available";
  const rec = cand.recommended_snapshot_id;
  shell(head + `
    ${avail ? `
      <div class="banner ok"><b>Recommendation: ${esc(rec)}</b>
        newest defensible recovery point · requires operator approval ·
        recovery gap ${num(cand.candidates?.find(c => c.snapshot_id === rec)?.data_loss_minutes ?? 0)} min</div>`
      : `<div class="banner error"><b>no_safe_candidate — CleanPoint refuses to recommend a restore point</b>
          The UI never falls back to the newest backup. Reasons:
          <ul style="margin:6px 0 0 18px">${(cand.reasons || []).map(r => `<li>${esc(r)}</li>`).join("") || "<li>no defensible candidate</li>"}</ul></div>`}
    <table class="tbl">
      <thead><tr><th>Rank</th><th>Captured</th><th>Snapshot</th><th>Integrity</th><th>Exposure</th>
      <th>Eligibility</th><th class="num">Recovery gap (min)</th><th>Reasons</th><th></th></tr></thead>
      <tbody>${(cand.candidates || []).map((c, i) => `
        <tr>
          <td class="num">${i + 1}</td>
          <td class="path">${esc(c.captured_at)}</td>
          <td class="path">${esc(c.snapshot_id)}</td>
          <td>${c.integrity_ok ? chip("verified", "OK") : chip("failed", "FAILED")}</td>
          <td>${chip(c.exposure_status)}</td>
          <td>${c.eligible ? chip("eligible") : chip("ineligible")}</td>
          <td class="num">${c.data_loss_minutes.toFixed(1)}</td>
          <td>${(c.reasons || []).slice(0, 2).map(r => `<div class="chart-note" style="margin:0">${esc(r)}</div>`).join("")}</td>
          <td>${c.eligible && S.role === "operator" ? `<button class="btn primary restore-btn" data-snap="${esc(c.snapshot_id)}" data-gap="${c.data_loss_minutes.toFixed(1)}">Restore…</button>` : ""}</td>
        </tr>`).join("")}</tbody>
    </table>
    <p class="chart-note">Snapshots are ranked newest-first. Eligibility = sealed + integrity verified +
    exposure clear + sufficient baseline coverage + captured strictly before the earliest plausible onset.
    Recovery gap is an RPO proxy (minutes between capture and onset), not business loss.</p>
    <div id="runReport"></div>`, "recovery");
  bindHead();
  $$(".restore-btn").forEach(b => b.addEventListener("click", () => restoreDialog(id, b.dataset.snap, b.dataset.gap)));
}

function restoreDialog(wlId, snapId, gap) {
  modal(`<h3>Restore confirmation</h3>
    <div class="banner warn"><b>Isolated sandbox restore — explicit operator approval</b>
      This copies the snapshot into a server-generated directory under <code>sandbox/restored/</code>.
      The protected source is never modified. Eligibility and object hashes are re-verified at restore time.</div>
    <dl class="kv">
      <dt>Snapshot</dt><dd class="path">${esc(snapId)}</dd>
      <dt>Recovery gap</dt><dd>${esc(gap)} minutes (data written after capture is not restored)</dd>
      <dt>Known caveats</dt><dd>Integrity proves consistency, <b>not</b> absence of compromise.
        The restore is irreversible in the sense that every action is audit-logged.</dd>
    </dl>
    <label class="fld">Target name (label for the run directory, [a-z0-9-])</label>
    <input type="text" id="mTarget" value="recovery-${snapId.slice(5, 13)}" pattern="[a-z0-9][a-z0-9-]{0,63}">
    <div class="checkline">
      <input type="checkbox" id="mConfirm">
      <label for="mConfirm">I am the operator and I approve this restore of <b>${esc(snapId)}</b>.</label>
    </div>
    <p class="login-error" id="mErr"></p>
    <div class="actions">
      <button class="btn" data-close>Cancel</button>
      <button class="btn danger" id="mGo" disabled>Execute restore</button>
    </div>`, (root) => {
    const go = $("#mGo", root), chk = $("#mConfirm", root);
    chk.addEventListener("change", () => { go.disabled = !chk.checked; });
    go.addEventListener("click", async () => {
      go.disabled = true; go.textContent = "Restoring & verifying…";
      try {
        const rep = await api("POST", `/workloads/${wlId}/restore`,
          { snapshot_id: snapId, target_name: $("#mTarget", root).value.trim(), confirm: true });
        root.remove();
        showRunReport(rep);
        toast(`Restore ${rep.status}: ${rep.verified_files}/${rep.total_files} files verified`);
      } catch (e) {
        $("#mErr", root).textContent = String(e.message || e);
        go.disabled = false; go.textContent = "Execute restore";
      }
    });
  });
}

function showRunReport(rep) {
  const host = $("#runReport");
  const html = `
    <h3 class="sec">Recovery run report — ${esc(rep.run_id)}</h3>
    <div class="banner ${rep.status === "verified" ? "ok" : "error"}">
      <b>${rep.status.toUpperCase()} — ${num(rep.verified_files)}/${num(rep.total_files)} files verified
      ${rep.source_unchanged ? "· source snapshot unchanged" : "· SOURCE CHANGE DETECTED"}</b>
      Manifest match is separate from synthetic ground truth (evaluated offline, never by the detector).
    </div>
    <dl class="kv">
      <dt>Output directory</dt><dd class="path">${esc(rep.output_dir)}</dd>
      <dt>Manifest SHA-256</dt><dd class="path">${esc(rep.manifest_hash)}</dd>
      <dt>Started / finished</dt><dd>${fmtTime(rep.started_at)} → ${fmtTime(rep.finished_at)}</dd>
      ${(rep.reasons || []).length ? `<dt>Reasons</dt><dd>${esc(rep.reasons.join("; "))}</dd>` : ""}
    </dl>
    <details style="margin-top:10px"><summary style="cursor:pointer;color:var(--blue)">
      Per-file verification (${(rep.files || []).length})</summary>
      <table class="tbl" style="margin-top:8px"><thead><tr><th>Path</th><th>Status</th>
      <th class="num">Bytes</th><th>SHA-256</th></tr></thead>
      <tbody>${(rep.files || []).map(f => `<tr><td class="path">${esc(f.path)}</td>
        <td>${chip(f.status === "verified" ? "verified" : "failed", f.status)}</td>
        <td class="num">${num(f.size_bytes)}</td><td class="path">${esc(f.sha256 ? f.sha256.slice(0, 16) + "…" : f.reason || "—")}</td></tr>`).join("")}</tbody></table>
    </details>`;
  if (host) host.innerHTML = html;
}

/* ---------- audit ---------- */
async function viewAudit() {
  let d; try { d = await api("GET", "/audit-events"); }
  catch (e) { return shell(`<h2 class="page">Audit log</h2>
    <div class="banner error"><b>Operator access required</b>${esc(e.message)}</div>`, "audit"); }
  const rows = (d.audit_events || []).map(a => `
    <tr><td class="num">${a.id}</td><td>${fmtTime(a.created_at)}</td><td>${esc(a.actor)}</td>
    <td class="path">${esc(a.action)}</td><td class="path">${esc(a.target_id)}</td>
    <td>${chip(a.outcome.startsWith("rejected") ? "failed" : "verified", a.outcome)}</td></tr>`).join("");
  shell(`<h2 class="page">Audit log</h2>
    <p style="color:var(--muted)">State-changing operations only. Secrets are never recorded.</p>
    ${rows ? `<table class="tbl"><thead><tr><th class="num">#</th><th>Time</th><th>Actor</th>
    <th>Action</th><th>Target</th><th>Outcome</th></tr></thead><tbody>${rows}</tbody></table>`
    : `<div class="empty">No audit events yet.</div>`}`, "audit");
}

/* ---------- settings ---------- */
async function viewSettings() {
  let c; try { c = await api("GET", "/config"); } catch (e) { return shell(`<div class="banner error">${esc(e.message)}</div>`, "settings"); }
  shell(`<h2 class="page">Settings & limitations</h2>
    <div class="card"><h3 class="sec" style="margin-top:0">Environment</h3>
      <dl class="kv">
        <dt>Mode</dt><dd><b>${esc(c.mode)}</b></dd>
        <dt>Detector</dt><dd>${esc(c.detector_version)} · window ${c.window_seconds}s · baseline ${c.baseline_windows} windows · threshold ${c.risk_threshold}</dd>
        <dt>Limits</dt><dd>${num(c.max_files)} files · ${fmtBytes(c.max_bytes)} per workload (synchronous MVP)</dd>
      </dl></div>
    <div class="card"><h3 class="sec" style="margin-top:0">Exact trust boundaries</h3>
      <ul>${(c.limits || []).map(x => `<li>${esc(x)}</li>`).join("")}</ul></div>
    <div class="card"><h3 class="sec" style="margin-top:0">CLI commands</h3>
      <pre class="json">${(c.cli || []).map(x => "cleanpoint " + esc(x.replace("cleanpoint ", ""))).join("\n")}</pre></div>`, "settings");
}

/* ---------- modal helper ---------- */
function modal(html, onOpen) {
  const back = document.createElement("div");
  back.className = "modal-back";
  back.innerHTML = `<div class="modal">${html}</div>`;
  document.body.appendChild(back);
  back.addEventListener("click", (e) => { if (e.target === back) back.remove(); });
  $$("[data-close]", back).forEach(b => b.addEventListener("click", () => back.remove()));
  if (onOpen) onOpen(back);
  const firstInput = $("input", back); if (firstInput) firstInput.focus();
}

function chip(kind, labelOverride) {
  const label = labelOverride != null ? labelOverride : kind;
  return `<span class="chip ${esc(kind)}">${esc(label)}</span>`;
}

/* ---------- router ---------- */
function render() {
  const h = location.hash || "#/overview";
  if (!S.token) return viewLogin();
  if (h.startsWith("#/login")) return viewLogin();
  if (h.startsWith("#/workload/")) {
    const parts = h.split("/");
    return viewWorkload(parts[2], parts[3] || "overview");
  }
  if (h.startsWith("#/go/")) {
    const tab = h.split("/")[2];
    if (!requireWorkload()) return;
    return viewWorkload(S.workload, tab);
  }
  if (h.startsWith("#/workloads")) return viewWorkloads();
  if (h.startsWith("#/audit")) return viewAudit();
  if (h.startsWith("#/settings")) return viewSettings();
  return viewOverview();
}

window.addEventListener("hashchange", render);
boot();
