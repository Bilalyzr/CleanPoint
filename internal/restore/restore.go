// Package restore performs operator-confirmed, isolated, verified restores.
// The destination is ALWAYS a server-generated run directory under the
// configured sandbox; the sealed source objects are re-verified before the
// copy and re-checked afterwards to prove non-mutation.
package restore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cleanpoint/internal/config"
	"cleanpoint/internal/domain"
	"cleanpoint/internal/metrics"
	"cleanpoint/internal/security"
	"cleanpoint/internal/selector"
	"cleanpoint/internal/snapshots"
	"cleanpoint/internal/storage"
)

var (
	ErrConfirmRequired = fmt.Errorf("restore requires confirm=true and an operator token")
	ErrIneligible      = fmt.Errorf("snapshot is not currently eligible")
)

type Restorer struct {
	cfg      *config.Config
	store    *storage.Store
	manager  *snapshots.Manager
	selector *selector.Selector
}

func New(cfg *config.Config, store *storage.Store, manager *snapshots.Manager, sel *selector.Selector) *Restorer {
	return &Restorer{cfg: cfg, store: store, manager: manager, selector: sel}
}

// FileOutcome records per-file verification.
type FileOutcome struct {
	Path    string `json:"path"`
	Status  string `json:"status"` // verified | failed
	Reason  string `json:"reason,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Size    int64  `json:"size_bytes,omitempty"`
}

type Report struct {
	RunID           string         `json:"run_id"`
	SnapshotID      string         `json:"snapshot_id"`
	Status          string         `json:"status"`
	OutputDir       string         `json:"output_dir"`
	TargetName      string         `json:"target_name"`
	TotalFiles      int            `json:"total_files"`
	VerifiedFiles   int            `json:"verified_files"`
	FailedFiles     int            `json:"failed_files"`
	SourceUnchanged bool           `json:"source_unchanged"`
	ManifestHash    string         `json:"manifest_hash"`
	Files           []FileOutcome  `json:"files"`
	Reasons         []string       `json:"reasons,omitempty"`
	StartedAt       string         `json:"started_at"`
	FinishedAt      string         `json:"finished_at,omitempty"`
}

// Execute performs the restore procedure:
//  1. eligibility re-check (fresh integrity verification)
//  2. unique server-generated destination under <sandbox>/restored
//  3. per-file copy to temp + hash re-computation + atomic rename
//  4. whole-manifest non-mutation proof of the source snapshot
//  5. persisted recovery run row + audit event
func (r *Restorer) Execute(workload domain.Workload, snapshotID, targetName string, confirm bool) (*Report, error) {
	if !confirm {
		return nil, ErrConfirmRequired
	}
	if err := security.SafeTargetName(targetName); err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	// Fresh eligibility: never trust an earlier UI state.
	el, reasons, err := r.selector.EligibleNow(workload, snapshotID, func(id string) (bool, error) {
		rep, err := r.manager.Verify(id)
		if err != nil {
			return false, err
		}
		return rep.ManifestOK && len(rep.Problems) == 0, nil
	})
	if err != nil {
		return nil, err
	}
	if !el {
		return nil, fmt.Errorf("%w: %s", ErrIneligible, strings.Join(reasons, "; "))
	}

	snap, err := r.store.GetSnapshot(snapshotID)
	if err != nil {
		return nil, err
	}
	files, err := r.store.ListSnapshotFiles(snapshotID)
	if err != nil {
		return nil, err
	}

	runID, err := domain.NewRecoveryRunID()
	if err != nil {
		return nil, err
	}
	started := time.Now()
	// Server-generated destination; the client only supplies a validated label.
	outDir := filepath.ToSlash(filepath.Join(filepath.FromSlash(r.cfg.RestoredDir()),
		fmt.Sprintf("%s-%s", targetName, strings.TrimPrefix(runID, "run-"))))
	outDirNative := filepath.FromSlash(outDir)
	if err := os.MkdirAll(outDirNative, 0o755); err != nil {
		return nil, err
	}

	run := domain.RecoveryRun{
		ID: runID, WorkloadID: workload.ID, SnapshotID: snapshotID,
		Status: domain.RunVerifying, TargetName: targetName, OutputPath: outDir,
		CreatedAt: domain.FormatTime(started),
	}
	if err := r.store.InsertRecoveryRun(run); err != nil {
		return nil, err
	}

	rep := &Report{
		RunID: runID, SnapshotID: snapshotID, Status: domain.RunVerifying,
		OutputDir: outDir, TargetName: targetName, ManifestHash: snap.ManifestHash,
		StartedAt: domain.FormatTime(started),
	}

	// Deterministic order for stable reports.
	rels := make([]string, 0, len(files))
	for p := range files {
		rels = append(rels, p)
	}
	// simple lexical sort
	for i := 1; i < len(rels); i++ {
		for j := i; j > 0 && rels[j] < rels[j-1]; j-- {
			rels[j], rels[j-1] = rels[j-1], rels[j]
		}
	}

	failed := 0
	for _, rel := range rels {
		f := files[rel]
		outcome := FileOutcome{Path: rel}
		_, objPath, err := r.store.GetObject(f.SHA256)
		if err != nil {
			outcome.Status = "failed"
			outcome.Reason = "object missing from store"
			rep.Files = append(rep.Files, outcome)
			failed++
			continue
		}
		dstNative, err := security.SafeJoin(outDirNative, rel)
		if err != nil {
			outcome.Status = "failed"
			outcome.Reason = fmt.Sprintf("unsafe path rejected: %v", err)
			rep.Files = append(rep.Files, outcome)
			failed++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dstNative), 0o755); err != nil {
			outcome.Status = "failed"
			outcome.Reason = "cannot create output directory"
			rep.Files = append(rep.Files, outcome)
			failed++
			continue
		}
		// Copy to temp file, hash the written bytes, atomically rename.
		in, err := os.ReadFile(filepath.FromSlash(objPath))
		if err != nil {
			outcome.Status = "failed"
			outcome.Reason = "object unreadable"
			rep.Files = append(rep.Files, outcome)
			failed++
			continue
		}
		sum := sha256.Sum256(in)
		gotSHA := hex.EncodeToString(sum[:])
		if gotSHA != strings.ToLower(f.SHA256) {
			outcome.Status = "failed"
			outcome.Reason = "object bytes do not hash to manifest value"
			rep.Files = append(rep.Files, outcome)
			failed++
			continue
		}
		if _, err := security.AtomicWrite(dstNative, in, 0o644); err != nil {
			outcome.Status = "failed"
			outcome.Reason = "write failed"
			rep.Files = append(rep.Files, outcome)
			failed++
			continue
		}
		outcome.Status = "verified"
		outcome.SHA256 = gotSHA
		outcome.Size = int64(len(in))
		rep.Files = append(rep.Files, outcome)
	}

	rep.TotalFiles = len(files)
	rep.VerifiedFiles = len(files) - failed
	rep.FailedFiles = failed

	// Prove the sealed source was not modified by re-verifying it fully.
	postVerify, err := r.manager.Verify(snapshotID)
	sourceUnchanged := err == nil && postVerify.ManifestOK && len(postVerify.Problems) == 0
	rep.SourceUnchanged = sourceUnchanged

	if failed == 0 && sourceUnchanged {
		rep.Status = domain.RunVerified
	} else {
		rep.Status = domain.RunFailed
		if failed > 0 {
			rep.Reasons = append(rep.Reasons, fmt.Sprintf("%d file(s) failed verification", failed))
		}
		if !sourceUnchanged {
			rep.Reasons = append(rep.Reasons, "source snapshot verification changed after restore; run flagged failed")
		}
	}
	rep.FinishedAt = domain.FormatTime(time.Now())
	repJSON, _ := json.MarshalIndent(rep, "", "  ")
	if err := r.store.FinishRecoveryRun(runID, rep.Status, rep.TotalFiles, rep.VerifiedFiles,
		rep.FailedFiles, sourceUnchanged, string(repJSON)); err != nil {
		return rep, err
	}
	metrics.Observe("restore_duration_seconds", time.Since(started))
	metrics.Inc("restore_total", "outcome", rep.Status)
	return rep, nil
}
