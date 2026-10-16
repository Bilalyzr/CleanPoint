// API client + auth/session context for the CleanPoint console.
export const TOKEN_KEY = "cp_token";

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export async function api<T = any>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  const headers: Record<string, string> = {};
  const token = sessionStorage.getItem(TOKEN_KEY) || "";
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const resp = await fetch(`/api/v1${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  let data: any = {};
  try {
    data = await resp.json();
  } catch {
    /* non-JSON */
  }
  if (!resp.ok) {
    throw new ApiError(resp.status, data.code ?? "error", data.message ?? resp.statusText);
  }
  return data as T;
}

// Shared types (subset used by the UI)
export interface Workload {
  id: string;
  name: string;
  root_path: string;
  created_at: string;
}
export interface Assessment {
  integrity_ok: boolean;
  coverage_ok: boolean;
  exposure_status: "clear" | "suspect" | "affected" | "unknown";
  risk_score: number;
  affected_file_count: number;
  explanation: string[];
  evaluated_at: string;
}
export interface Snapshot {
  id: string;
  workload_id: string;
  captured_at: string;
  status: "creating" | "sealed" | "invalid";
  manifest_hash?: string;
  file_count: number;
  total_bytes: number;
  created_at: string;
  assessment?: Assessment;
}
export interface Contribution {
  feature: string;
  weight: number;
  signal: number;
  value: number;
  center: number;
  z_score?: number;
  missing?: boolean;
}
export interface Window {
  id: string;
  starts_at: string;
  ends_at: string;
  features: Record<string, number>;
  risk_score: number;
  alert_level: "normal" | "suspicious";
  signal_contributions?: Contribution[];
  is_baseline: boolean;
  event_count: number;
}
export interface Findings {
  analysis_run_id: string;
  workload_id: string;
  detector_version: string;
  risk_threshold: number;
  baseline_period: string;
  baseline_windows: number;
  window_seconds: number;
  windows_evaluated: number;
  incident_onset: string | null;
  windows: Window[];
  affected_files: string[];
  note?: string;
  warning?: string;
}
export interface Candidate {
  snapshot_id: string;
  captured_at: string;
  integrity_ok: boolean;
  exposure_status: string;
  eligible: boolean;
  data_loss_minutes: number;
  file_count: number;
  reasons: string[];
}
export interface SelectorReport {
  status: "candidate_available" | "no_safe_candidate";
  incident_onset?: string | null;
  recommended_snapshot_id?: string | null;
  requires_operator_approval: boolean;
  detector_version: string;
  evaluated_at: string;
  reasons?: string[];
  candidates: Candidate[];
}
export interface FileEvent {
  id: string;
  timestamp: string;
  operation: "create" | "modify" | "delete" | "rename";
  path: string;
  dest_path?: string;
  old_entropy?: number;
  new_entropy?: number;
  new_size?: number;
  old_size?: number;
  actor?: string;
}
export interface RestoreReport {
  run_id: string;
  snapshot_id: string;
  status: string;
  output_dir: string;
  target_name: string;
  total_files: number;
  verified_files: number;
  failed_files: number;
  source_unchanged: boolean;
  manifest_hash: string;
  files: { path: string; status: string; reason?: string; sha256?: string; size_bytes?: number }[];
  reasons?: string[];
  started_at: string;
  finished_at?: string;
}
export interface RecoveryRun {
  id: string;
  workload_id: string;
  snapshot_id: string;
  status: string;
  verified_files: number;
  failed_files: number;
  source_unchanged: boolean;
  created_at: string;
}

export interface SimJob {
  id: string;
  scenario: string;
  seed: number;
  status: "running" | "completed" | "failed";
  log: string[];
  result?: Record<string, any>;
  error?: string;
  started_at: string;
  finished_at?: string;
}

export const SCENARIO_FAMILIES = [
  "normal_office",
  "benign_archive",
  "benign_bulk_update",
  "rapid_destructive",
  "slow_distributed",
  "extension_churn",
  "mixed_activity",
  "pre_existing_contamination",
];

export const fmtTime = (iso?: string | null) =>
  iso ? iso.replace("T", " ").replace(".000Z", " UTC").replace("Z", " UTC") : "—";
export const fmtBytes = (b?: number) =>
  b == null ? "—" : b < 1048576 ? `${(b / 1024).toFixed(1)} KiB` : `${(b / 1048576).toFixed(2)} MiB`;
export const num = (n?: number | null) => (n == null ? "—" : Number(n).toLocaleString());
