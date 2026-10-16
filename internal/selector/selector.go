// Package selector ranks recovery candidates newest-first with conservative
// rejection reasons, or abstains with no_safe_candidate. Safety beats recency:
// a snapshot is eligible only when integrity, exposure and onset evidence all
// defend it.
package selector

import (
	"fmt"
	"time"

	"cleanpoint/internal/config"
	"cleanpoint/internal/domain"
	"cleanpoint/internal/metrics"
	"cleanpoint/internal/storage"
)

type Selector struct {
	cfg   *config.Config
	store *storage.Store
}

func New(cfg *config.Config, store *storage.Store) *Selector {
	return &Selector{cfg: cfg, store: store}
}

// Report is the API-facing selector result. Status is candidate_available or
// no_safe_candidate; when abstaining, RecommendedSnapshotID is nil and Reasons
// explains why.
type Report struct {
	Status                 string      `json:"status"`
	IncidentOnset          *string     `json:"incident_onset,omitempty"`
	RecommendedSnapshotID  *string     `json:"recommended_snapshot_id"`
	RequiresOperatorApproval bool      `json:"requires_operator_approval"`
	Candidates             []domain.Candidate `json:"candidates"`
	Reasons                []string    `json:"reasons,omitempty"`
	DetectorVersion        string      `json:"detector_version"`
	EvaluatedAt            string      `json:"evaluated_at"`
}

// Candidates computes the ranked candidate list for the latest completed
// analysis of a workload. It never falls back to the newest snapshot when no
// candidate can be defended.
func (s *Selector) Candidates(workload domain.Workload) (*Report, error) {
	run, err := s.store.LatestAnalysisRun(workload.ID)
	if err != nil {
		return nil, fmt.Errorf("no completed analysis for workload; run analyze first: %w", err)
	}
	snapshotsList, err := s.store.ListSnapshots(workload.ID)
	if err != nil {
		return nil, err
	}

	// Reference time for the recovery-gap proxy: incident onset when known,
	// else the newest event time.
	reference := ""
	if run.OnsetTime != nil {
		reference = *run.OnsetTime
	} else {
		_, maxT, count, err := s.store.EventTimeBounds(workload.ID)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			reference = run.FinishedAt
		} else {
			reference = maxT
		}
	}

	report := &Report{
		Status:                   domain.SelectorNoSafeCandidate,
		IncidentOnset:            run.OnsetTime,
		RecommendedSnapshotID:    nil,
		RequiresOperatorApproval: true,
		DetectorVersion:          run.DetectorVersion,
		EvaluatedAt:              domain.FormatTime(time.Now()),
	}

	// Snapshots are already listed newest-first (captured_at DESC).
	var eligible []domain.Candidate
	for _, snap := range snapshotsList {
		cand := domain.Candidate{
			SnapshotID:     snap.ID,
			CapturedAt:     snap.CapturedAt,
			FileCount:      snap.FileCount,
			IntegrityOK:    false,
			ExposureStatus: domain.ExposureUnknown,
			Eligible:       false,
			Reasons:        []string{},
		}
		if dl, err := gapMinutes(snap.CapturedAt, reference); err == nil {
			cand.DataLossMinutes = dl
		}

		if snap.Status == domain.SnapshotCreating {
			cand.Reasons = append(cand.Reasons, "snapshot still being created")
		} else if snap.Status == domain.SnapshotInvalid {
			cand.Reasons = append(cand.Reasons, "snapshot capture failed earlier (invalid)")
		}
		assess, err := s.store.GetSnapshotAssessment(snap.ID)
		if err != nil {
			cand.Reasons = append(cand.Reasons, "no assessment yet; run analyze")
			report.Candidates = append(report.Candidates, cand)
			continue
		}
		cand.IntegrityOK = assess.IntegrityOK
		cand.ExposureStatus = assess.ExposureStatus

		if snap.Status != domain.SnapshotSealed {
			report.Candidates = append(report.Candidates, cand)
			continue
		}
		if !assess.IntegrityOK {
			cand.Reasons = append(cand.Reasons, "integrity verification failed (manifest or object mismatch)")
		}
		if !assess.CoverageOK {
			// prefer the assessment's specific explanation when present
			if len(assess.Explanation) > 0 {
				cand.Reasons = append(cand.Reasons, assess.Explanation[0])
			} else {
				cand.Reasons = append(cand.Reasons, "insufficient normal-baseline coverage at capture time")
			}
		}
		switch assess.ExposureStatus {
		case domain.ExposureUnknown:
			cand.Reasons = append(cand.Reasons, "exposure unknown; cannot certify cleanliness")
		case domain.ExposureSuspect:
			cand.Reasons = append(cand.Reasons, "suspicious activity completed before this snapshot was captured")
		case domain.ExposureAffected:
			cand.Reasons = append(cand.Reasons, "destructive signals affected files in this snapshot")
		}
		if run.OnsetTime != nil && snap.CapturedAt >= *run.OnsetTime {
			cand.Reasons = append(cand.Reasons, "captured at/after earliest plausible suspicious onset")
		}
		if len(cand.Reasons) == 0 {
			cand.Eligible = true
			cand.Reasons = append(cand.Reasons, "sealed, integrity verified, exposure clear, captured strictly before onset")
			eligible = append(eligible, cand)
		}
		report.Candidates = append(report.Candidates, cand)
	}

	if len(eligible) == 0 {
		if len(report.Candidates) == 0 {
			report.Reasons = []string{"no snapshots captured for this workload"}
		} else {
			report.Reasons = abstentionReasons(report.Candidates)
		}
		metrics.Inc("candidate_abstentions_total", "", "")
		return report, nil
	}

	// Eligible list is already newest-first; enforce deterministic tie-break
	// (captured_at DESC, then greater verified file coverage, then ID).
	sortEligible(eligible)
	report.Status = domain.SelectorCandidateAvailable
	report.RecommendedSnapshotID = &eligible[0].SnapshotID
	return report, nil
}

func abstentionReasons(cands []domain.Candidate) []string {
	seen := map[string]bool{}
	var reasons []string
	for _, c := range cands {
		for _, r := range c.Reasons {
			if !seen[r] {
				seen[r] = true
				reasons = append(reasons, r)
			}
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "no candidate could be defended as safe")
	}
	return reasons
}

func sortEligible(cands []domain.Candidate) {
	// stable insertion by (captured_at desc, file_count desc, id desc)
	for i := 1; i < len(cands); i++ {
		for j := i; j > 0 && candLess(cands[j], cands[j-1]); j-- {
			cands[j], cands[j-1] = cands[j-1], cands[j]
		}
	}
}

func candLess(a, b domain.Candidate) bool {
	if a.CapturedAt != b.CapturedAt {
		return a.CapturedAt > b.CapturedAt
	}
	if a.FileCount != b.FileCount {
		return a.FileCount > b.FileCount
	}
	return a.SnapshotID > b.SnapshotID
}

func gapMinutes(capturedAt, reference string) (float64, error) {
	c, err := domain.ParseTime(capturedAt)
	if err != nil {
		return 0, err
	}
	r, err := domain.ParseTime(reference)
	if err != nil {
		return 0, err
	}
	d := r.Sub(c).Minutes()
	if d < 0 {
		d = 0
	}
	return d, nil
}

// EligibleNow re-validates one snapshot at restore time: sealed, integrity
// freshly verified via the provided verifier callback, assessment clear,
// before onset. The restore path MUST call this rather than trusting the UI.
func (s *Selector) EligibleNow(workload domain.Workload, snapshotID string, freshVerify func(string) (bool, error)) (bool, []string, error) {
	var reasons []string
	snap, err := s.store.GetSnapshot(snapshotID)
	if err != nil {
		return false, nil, err
	}
	if snap.WorkloadID != workload.ID {
		return false, []string{"snapshot belongs to a different workload"}, nil
	}
	if snap.Status != domain.SnapshotSealed {
		reasons = append(reasons, "snapshot is not sealed")
	}
	ok, err := freshVerify(snapshotID)
	if err != nil {
		return false, nil, err
	}
	if !ok {
		reasons = append(reasons, "fresh integrity re-verification failed")
	}
	run, err := s.store.LatestAnalysisRun(workload.ID)
	if err != nil {
		return false, append(reasons, "no completed analysis"), nil
	}
	assess, err := s.store.GetSnapshotAssessment(snapshotID)
	if err != nil {
		return false, append(reasons, "no assessment for snapshot"), nil
	}
	if !assess.IntegrityOK || !assess.CoverageOK {
		reasons = append(reasons, "assessment integrity or baseline coverage failed")
	}
	if assess.ExposureStatus != domain.ExposureClear {
		reasons = append(reasons, "exposure is "+assess.ExposureStatus+", not clear")
	}
	if run.OnsetTime != nil && snap.CapturedAt >= *run.OnsetTime {
		reasons = append(reasons, "captured at/after earliest plausible onset")
	}
	if len(reasons) > 0 {
		return false, reasons, nil
	}
	return true, nil, nil
}
