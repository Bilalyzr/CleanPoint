import React from "react";
import { useNavigate } from "react-router-dom";
import { api, TOKEN_KEY } from "../api";

// UI-01: token input (hidden), local-only status, disconnect. Role is
// detected after sign-in; tokens are never logged.
export default function Login() {
  const nav = useNavigate();
  const [err, setErr] = React.useState("");
  const [busy, setBusy] = React.useState(false);

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
      <form className="login-card" onSubmit={submit}>
        <h1>
          <span className="logo" aria-hidden="true">CP</span> CleanPoint
        </h1>
        <p className="login-sub">
          Ransomware-recovery analysis console — <b>local simulation only</b>.
          No production data-protection guarantees.
        </p>
        <div className="login-hint">
          Tokens live in <code>sandbox/.tokens</code> after{" "}
          <code>scripts/demo.sh</code> — use <code>operator=…</code> for full
          access or <code>readonly=…</code> to browse.
        </div>
        <label className="fld" htmlFor="tok">
          Bearer token
        </label>
        <input type="password" id="tok" autoComplete="off" placeholder="paste token" required autoFocus />
        <p className="login-error" role="alert">{err}</p>
        <button className="btn primary" type="submit" disabled={busy} style={{ width: "100%" }}>
          {busy ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </div>
  );
}
