import React from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { api, TOKEN_KEY } from "./api";

export const Session = React.createContext<{
  role: "operator" | "read-only";
  workload: string;
  setWorkload: (id: string) => void;
}>({ role: "read-only", workload: "", setWorkload: () => {} });

const NAV = [
  { to: "/", label: "Overview", icon: "▦", end: true },
  { to: "/workloads", label: "Workloads", icon: "▤" },
  { to: "/simulation", label: "Simulation Lab", icon: "◉" },
  { to: "/evaluations", label: "Evaluations", icon: "◫" },
  { to: "/audit", label: "Audit log", icon: "≡" },
  { to: "/settings", label: "Settings", icon: "⚙" },
];

export default function App() {
  const nav = useNavigate();
  const [role, setRole] = React.useState<"operator" | "read-only">("read-only");
  const [workload, setWorkload] = React.useState(
    () => localStorage.getItem("cp_workload") || ""
  );
  const [names, setNames] = React.useState<Record<string, string>>({});

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
    api("GET", "/workloads").then((d: any) => {
      if (!alive) return;
      const m: Record<string, string> = {};
      for (const w of d.workloads ?? []) m[w.id] = w.name;
      setNames(m);
    }).catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  const setWl = (id: string) => {
    setWorkload(id);
    localStorage.setItem("cp_workload", id);
  };

  return (
    <Session.Provider value={{ role, workload, setWorkload: setWl }}>
      <div className="shell">
        <aside className="side">
          <div className="brand">
            <span className="logo" aria-hidden="true">CP</span>
            <span>CleanPoint</span>
          </div>
          <nav aria-label="Console sections">
            {NAV.map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end} className={({ isActive }) => "nav-item" + (isActive ? " active" : "")}>
                <span aria-hidden="true">{n.icon}</span>
                <span className="txt">{n.label}</span>
              </NavLink>
            ))}
          </nav>
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
                <option value="">— none —</option>
                {Object.entries(names).map(([id, name]) => (
                  <option key={id} value={id}>{name}</option>
                ))}
              </select>
            </label>
            <span className="spacer" />
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
          <main className="content">
            <Outlet />
          </main>
        </div>
      </div>
    </Session.Provider>
  );
}
