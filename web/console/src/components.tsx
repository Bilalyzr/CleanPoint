import React from "react";

// Status chips: always labelled text + color, never color alone (PRD §6.2).
const CHIP_CLASS: Record<string, string> = {
  clear: "chip good", eligible: "chip good", verified: "chip good",
  sealed: "chip good", normal: "chip good", completed: "chip good",
  suspect: "chip warn", unknown: "chip warn", creating: "chip warn",
  requested: "chip warn", verifying: "chip warn", running: "chip warn",
  affected: "chip bad", failed: "chip bad", invalid: "chip bad",
  ineligible: "chip bad", no_safe_candidate: "chip bad", suspicious: "chip bad",
};
export function Chip({ kind, label }: { kind: string; label?: string }) {
  const cls = CHIP_CLASS[kind] ?? "chip";
  return (
    <span className={cls}>
      <i aria-hidden="true" />
      {label ?? kind}
    </span>
  );
}

export function Stat({
  n,
  label,
  warn,
}: {
  n: React.ReactNode;
  label: string;
  warn?: boolean;
}) {
  return (
    <div className={"stat" + (warn ? " warn" : "")}>
      <div className="num">{n}</div>
      <div className="lbl">{label}</div>
    </div>
  );
}

export function Banner({
  tone,
  title,
  children,
}: {
  tone: "ok" | "warn" | "error" | "info";
  title?: React.ReactNode;
  children?: React.ReactNode;
}) {
  return (
    <div className={`banner ${tone}`} role={tone === "error" ? "alert" : "status"}>
      {title && <b>{title}</b>}
      {children}
    </div>
  );
}

export function Empty({ children }: { children: React.ReactNode }) {
  return <div className="empty">{children}</div>;
}

export function Loading() {
  return <div className="empty">Loading…</div>;
}

export function ErrorBanner({ error }: { error: unknown }) {
  const msg = error instanceof Error ? error.message : String(error);
  return (
    <Banner tone="error" title="Something went wrong">
      {msg}
    </Banner>
  );
}

export function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: React.ReactNode;
  onClose: () => void;
  children: React.ReactNode;
  wide?: boolean;
}) {
  React.useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [onClose]);
  return (
    <div className="modal-back" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={"modal" + (wide ? " wide" : "")} role="dialog" aria-modal="true" aria-label={typeof title === "string" ? title : "dialog"}>
        <h3>{title}</h3>
        {children}
      </div>
    </div>
  );
}

export function Tabs({
  tabs,
  active,
  onSelect,
}: {
  tabs: { id: string; label: string }[];
  active: string;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="tabs" role="tablist">
      {tabs.map((t) => (
        <button
          key={t.id}
          role="tab"
          aria-selected={t.id === active}
          className={"tab" + (t.id === active ? " active" : "")}
          onClick={() => onSelect(t.id)}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}

export function KV({ rows }: { rows: [string, React.ReactNode][] }) {
  return (
    <dl className="kv">
      {rows.map(([k, v]) => (
        <React.Fragment key={k}>
          <dt>{k}</dt>
          <dd>{v}</dd>
        </React.Fragment>
      ))}
    </dl>
  );
}
