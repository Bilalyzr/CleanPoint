import React from "react";
import { NavLink, Outlet, useNavigate, useLocation } from "react-router-dom";
import { api, TOKEN_KEY } from "./api";

export const Session = React.createContext<{
  role: "operator" | "read-only";
  workload: string;
  setWorkload: (id: string) => void;
}>({ role: "read-only", workload: "", setWorkload: () => {} });

const NAV_ICONS: Record<string, React.ReactNode> = {
  "/": (
    <svg viewBox="0 0 20 20" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.6">
      <rect x="2.5" y="2.5" width="6.2" height="6.2" rx="1.4" />
      <rect x="11.3" y="2.5" width="6.2" height="6.2" rx="1.4" />
      <rect x="2.5" y="11.3" width="6.2" height="6.2" rx="1.4" />
      <rect x="11.3" y="11.3" width="6.2" height="6.2" rx="1.4" />
    </svg>
  ),
  "/workloads": (
    <svg viewBox="0 0 20 20" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.6">
      <path d="M10 2.5L17.5 6.5L10 10.5L2.5 6.5L10 2.5Z" strokeLinejoin="round" />
      <path d="M2.5 10.2L10 14.2L17.5 10.2" strokeLinejoin="round" />
      <path d="M2.5 13.8L10 17.8L17.5 13.8" strokeLinejoin="round" />
    </svg>
  ),
  "/simulation": (
    <svg viewBox="0 0 20 20" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.6">
      <circle cx="10" cy="10" r="7.2" />
      <circle cx="10" cy="10" r="2.6" />
      <path d="M10 2.8V5.2M10 14.8V17.2M2.8 10H5.2M14.8 10H17.2" strokeLinecap="round" />
    </svg>
  ),
  "/evaluations": (
    <svg viewBox="0 0 20 20" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.6">
      <path d="M3 16.5H17" strokeLinecap="round" />
      <rect x="4.2" y="9.5" width="2.8" height="4.8" rx="0.8" />
      <rect x="8.6" y="6.2" width="2.8" height="8.1" rx="0.8" />
      <rect x="13" y="3.5" width="2.8" height="10.8" rx="0.8" />
    </svg>
  ),
  "/audit": (
    <svg viewBox="0 0 20 20" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.6">
      <path d="M4 5.5H16M4 10H16M4 14.5H11.5" strokeLinecap="round" />
    </svg>
  ),
  "/settings": (
    <svg viewBox="0 0 20 20" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.6">
      <circle cx="10" cy="10" r="2.7" />
      <path d="M10 2.5V4.4M10 15.6V17.5M2.5 10H4.4M15.6 10H17.5M4.7 4.7L6.1 6.1M13.9 13.9L15.3 15.3M15.3 4.7L13.9 6.1M6.1 13.9L4.7 15.3" strokeLinecap="round" />
    </svg>
  ),
};

const NAV = [
  { to: "/", label: "Overview", end: true },
  { to: "/workloads", label: "Workloads" },
  { to: "/simulation", label: "Simulation Lab" },
  { to: "/evaluations", label: "Evaluations" },
  { to: "/audit", label: "Audit log" },
  { to: "/settings", label: "Settings" },
];

export default function App() {
  const nav = useNavigate();
  const loc = useLocation();
  const [role, setRole] = React.useState<"operator" | "read-only">("read-only");
  const [workload, setWorkload] = React.useState(
    () => localStorage.getItem("cp_workload") || ""
  );
  const [names, setNames] = React.useState<Record<string, string>>({});
  const [theme, setTheme] = React.useState<"noir" | "daylight">(() => {
    const saved = localStorage.getItem("cp_theme");
    return saved === "daylight" ? "daylight" : "noir";
  });

  React.useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("cp_theme", theme);
  }, [theme]);

  React.useEffect(() => {
    (async () => {
      try {
        await api("GET", "/config");
        try {
          await api("GET", "/audit-events");
          setRole("operator");
        } catch {
          setRole("read-only");
        }
      } catch {
        sessionStorage.removeItem(TOKEN_KEY);
        nav("/login");
      }
    })();
  }, [nav]);

  React.useEffect(() => {
    let alive = true;
    api("GET", "/workloads")
      .then((d: any) => {
        if (!alive) return;
        const m: Record<string, string> = {};
        for (const w of d.workloads ?? []) m[w.id] = w.name;
        setNames(m);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [loc.pathname]);

  const setWl = (id: string) => {
    setWorkload(id);
    localStorage.setItem("cp_workload", id);
  };

  // Cursor-following specular highlight for interactive surfaces
  const handleShellPointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    const target = (e.target as HTMLElement | null)?.closest?.(
      ".card, .dash-card, .wl-card, .fam-tile, .stat, .mini-stat, .run-panel, .vault3d-card"
    ) as HTMLElement | null;
    if (!target) return;
    const rect = target.getBoundingClientRect();
    target.style.setProperty("--mx", `${(e.clientX - rect.left).toFixed(0)}px`);
    target.style.setProperty("--my", `${(e.clientY - rect.top).toFixed(0)}px`);
  };

  return (
    <Session.Provider value={{ role, workload, setWorkload: setWl }}>
      <div className="shell" onPointerMove={handleShellPointerMove}>
        <aside className="side">
          <div className="brand">
            <span className="logo" aria-hidden="true">
              CP
            </span>
            <div className="brand-text">
              <span>CleanPoint</span>
              <small>CHRONO-VAULT M2</small>
            </div>
          </div>
          <nav aria-label="Console sections">
            {NAV.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.end}
                className={({ isActive }) => "nav-item" + (isActive ? " active" : "")}
              >
                <span className="nav-ic" aria-hidden="true">
                  {NAV_ICONS[n.to]}
                </span>
                <span className="txt">{n.label}</span>
              </NavLink>
            ))}
          </nav>
          <div className="side-links">
            <a href="/" className="story-link" title="Open 3D Interactive Product Showcase">
              <span aria-hidden="true">◈</span>
              <span className="txt">3D Product Story ↗</span>
            </a>
          </div>
          <div className="foot">
            v2.0.0 · M2 console
            <br />
            LOCAL SIMULATION
          </div>
        </aside>
        <div className="main">
          <header className="topbar">
            <span className="badge-env">LOCAL SIMULATION</span>
            <label className="wl-pick" htmlFor="wlpick">
              workload
              <select
                id="wlpick"
                value={workload}
                onChange={(e) => {
                  const id = e.target.value;
                  setWl(id);
                  if (id) nav(`/workload/${id}/overview`);
                }}
              >
                <option value="">— select workload —</option>
                {Object.entries(names).map(([id, name]) => (
                  <option key={id} value={id}>
                    {name}
                  </option>
                ))}
              </select>
            </label>
            <span className="spacer" />
            <button
              type="button"
              className="theme-btn"
              onClick={() => setTheme((t) => (t === "noir" ? "daylight" : "noir"))}
              title={`Switch to ${theme === "noir" ? "Daylight" : "Chrono-Vault Noir"} theme`}
              aria-label={`Switch to ${theme === "noir" ? "Daylight" : "Chrono-Vault Noir"} theme`}
            >
              {theme === "noir" ? "☀ Daylight" : "☾ Noir"}
            </button>
            <span className="role-chip">
              role: <b>{role}</b>
            </span>
            <button
              className="btn-logout"
              onClick={() => {
                sessionStorage.removeItem(TOKEN_KEY);
                nav("/login");
              }}
            >
              ⏻ sign out
            </button>
          </header>
          <main className="content" key={loc.pathname}>
            <Outlet />
          </main>
        </div>
      </div>
    </Session.Provider>
  );
}
