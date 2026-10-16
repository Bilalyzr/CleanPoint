// Package domain defines CleanPoint core entities, state vocabularies and
// identifier helpers shared by every internal package.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Time layout used for ALL persisted timestamps. Fixed-width milliseconds in
// UTC keep lexicographic string order identical to chronological order.
const TimeLayout = "2006-01-02T15:04:05.000Z"

func FormatTime(t time.Time) string { return t.UTC().Format(TimeLayout) }

func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid ISO-8601 timestamp %q", s)
	}
	return t.UTC(), nil
}

// NormalizeTime accepts any RFC3339 timestamp and renders it in the canonical
// fixed-width layout. Ingestion rejects values that cannot be normalized.
func NormalizeTime(s string) (string, error) {
	t, err := ParseTime(s)
	if err != nil {
		return "", err
	}
	return FormatTime(t), nil
}

// Snapshot lifecycle: creating -> sealed | creating -> invalid.
// After sealed, content is logically read-only via the API.
const (
	SnapshotCreating = "creating"
	SnapshotSealed   = "sealed"
	SnapshotInvalid  = "invalid"
)

// Exposure classification stored in snapshot_assessments (mutable, kept
// separate from immutable sealed snapshot bytes).
const (
	ExposureUnknown = "unknown"
	ExposureClear   = "clear"
	ExposureSuspect = "suspect"
	ExposureAffected = "affected"
)

// Recovery run lifecycle: requested -> verifying -> verified | failed.
const (
	RunRequested = "requested"
	RunVerifying = "verifying"
	RunVerified  = "verified"
	RunFailed    = "failed"
)

// Window alert levels produced by the detector.
const (
	AlertNormal     = "normal"
	AlertSuspicious = "suspicious"
)

// Selector outcomes.
const (
	SelectorCandidateAvailable = "candidate_available"
	SelectorNoSafeCandidate    = "no_safe_candidate"
)

// Operations accepted for file events.
var ValidOperations = map[string]bool{
	"create": true, "modify": true, "delete": true, "rename": true,
}

// ErrInvalidInput marks semantically invalid caller input (bad JSON shapes,
// bad enum values) so the API can map it to 400.
var ErrInvalidInput = errors.New("invalid input")

type Workload struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RootPath  string `json:"root_path"`
	CreatedAt string `json:"created_at"`
}

type FileEvent struct {
	ID         string  `json:"id"`
	WorkloadID string  `json:"workload_id"`
	EventTime  string  `json:"timestamp"`
	Operation  string  `json:"operation"`
	Path       string  `json:"path"`
	DestPath   *string `json:"dest_path,omitempty"`
	OldSHA256  *string `json:"old_sha256,omitempty"`
	NewSHA256  *string `json:"new_sha256,omitempty"`
	OldEntropy *float64 `json:"old_entropy,omitempty"`
	NewEntropy *float64 `json:"new_entropy,omitempty"`
	OldSize    *int64  `json:"old_size,omitempty"`
	NewSize    *int64  `json:"new_size,omitempty"`
	Actor      *string `json:"actor,omitempty"`
	ScenarioID *string `json:"scenario_id,omitempty"`
	IngestedAt string  `json:"ingested_at"`
}

type Snapshot struct {
	ID           string `json:"id"`
	WorkloadID   string `json:"workload_id"`
	CapturedAt   string `json:"captured_at"`
	Status       string `json:"status"`
	ManifestHash string `json:"manifest_hash,omitempty"`
	FileCount    int    `json:"file_count"`
	TotalBytes   int64  `json:"total_bytes"`
	CreatedAt    string `json:"created_at"`
}

type SnapshotAssessment struct {
	SnapshotID        string   `json:"snapshot_id"`
	AnalysisRunID     string   `json:"analysis_run_id"`
	IntegrityOK       bool     `json:"integrity_ok"`
	CoverageOK        bool     `json:"coverage_ok"`
	ExposureStatus    string   `json:"exposure_status"`
	RiskScore         float64  `json:"risk_score"`
	AffectedFileCount int      `json:"affected_file_count"`
	Explanation       []string `json:"explanation"`
	EvaluatedAt       string   `json:"evaluated_at"`
}

type DetectionWindow struct {
	ID            string             `json:"id"`
	AnalysisRunID string             `json:"analysis_run_id"`
	WorkloadID    string             `json:"workload_id"`
	StartsAt      string             `json:"starts_at"`
	EndsAt        string             `json:"ends_at"`
	Features      map[string]float64 `json:"features"`
	RiskScore     float64            `json:"risk_score"`
	AlertLevel    string             `json:"alert_level"`
	Contributions []SignalContribution `json:"signal_contributions,omitempty"`
	IsBaseline    bool               `json:"is_baseline"`
	EventCount    int                `json:"event_count"`
}

type SignalContribution struct {
	Feature string  `json:"feature"`
	Weight  float64 `json:"weight"`
	Signal  float64 `json:"signal"`
	Value   float64 `json:"value"`
	Center  float64 `json:"center"`
	ZScore  float64 `json:"z_score"`
	Missing bool    `json:"missing,omitempty"`
}

type AnalysisRun struct {
	ID                  string  `json:"id"`
	WorkloadID          string  `json:"workload_id"`
	DetectorVersion     string  `json:"detector_version"`
	BaselinePeriod      string  `json:"baseline_period"`
	WindowSeconds       int     `json:"window_seconds"`
	BaselineWindowCount int     `json:"baseline_window_count"`
	Status              string  `json:"status"`
	OnsetTime           *string `json:"incident_onset,omitempty"`
	WindowsEvaluated    int     `json:"windows_evaluated"`
	RiskThreshold       float64 `json:"risk_threshold"`
	StartedAt           string  `json:"started_at"`
	FinishedAt          string  `json:"finished_at,omitempty"`
}

type Candidate struct {
	SnapshotID      string   `json:"snapshot_id"`
	CapturedAt      string   `json:"captured_at"`
	IntegrityOK     bool     `json:"integrity_ok"`
	ExposureStatus  string   `json:"exposure_status"`
	Eligible        bool     `json:"eligible"`
	DataLossMinutes float64  `json:"data_loss_minutes"`
	Reasons         []string `json:"reasons"`
	FileCount       int      `json:"file_count"`
}

type RecoveryRun struct {
	ID              string `json:"id"`
	WorkloadID      string `json:"workload_id"`
	SnapshotID      string `json:"snapshot_id"`
	Status          string `json:"status"`
	TargetName      string `json:"target_name"`
	OutputPath      string `json:"output_path"`
	TotalFiles      int    `json:"total_files"`
	VerifiedFiles   int    `json:"verified_files"`
	FailedFiles     int    `json:"failed_files"`
	SourceUnchanged bool   `json:"source_unchanged"`
	CreatedAt       string `json:"created_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
}

type AuditEvent struct {
	ID        int64  `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	TargetID  string `json:"target_id"`
	Outcome   string `json:"outcome"`
	RequestID string `json:"request_id"`
	CreatedAt string `json:"created_at"`
}

// randomID returns a short deterministic-format identifier such as
// "snap-1a2b3c4d5e6f". Randomness comes from crypto/rand; ties in ordering
// are broken lexicographically elsewhere.
func randomID(prefix string) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(b), nil
}

func NewWorkloadID() (string, error)    { return randomID("wl") }
func NewSnapshotID() (string, error)    { return randomID("snap") }
func NewAnalysisRunID() (string, error) { return randomID("ar") }
func NewRecoveryRunID() (string, error) { return randomID("run") }
func NewWindowID() (string, error)      { return randomID("win") }
