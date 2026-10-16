package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"cleanpoint/internal/domain"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

func now() string { return domain.FormatTime(time.Now()) }

// ---- workloads ----

func (s *Store) CreateWorkload(w domain.Workload) error {
	_, err := s.db.Exec(`INSERT INTO workloads(id,name,root_path,created_at) VALUES(?,?,?,?)`,
		w.ID, w.Name, w.RootPath, w.CreatedAt)
	return err
}

func (s *Store) ListWorkloads() ([]domain.Workload, error) {
	rows, err := s.db.Query(`SELECT id,name,root_path,created_at FROM workloads ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Workload
	for rows.Next() {
		var w domain.Workload
		if err := rows.Scan(&w.ID, &w.Name, &w.RootPath, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) GetWorkload(id string) (domain.Workload, error) {
	var w domain.Workload
	err := s.db.QueryRow(`SELECT id,name,root_path,created_at FROM workloads WHERE id=?`, id).
		Scan(&w.ID, &w.Name, &w.RootPath, &w.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return w, ErrNotFound
	}
	return w, err
}

// ---- file events ----

// IngestEvent is idempotent by event id: identical replay is a no-op (returns
// ingested=false), a conflicting payload under the same id is an error.
func (s *Store) IngestEvent(e domain.FileEvent) (bool, error) {
	existing, err := s.getEvent(e.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return false, err
	}
	if err == nil {
		if eventsEquivalent(existing, e) {
			return false, nil
		}
		return false, fmt.Errorf("event id %s already exists with different content: %w", e.ID, ErrConflict)
	}
	_, err = s.db.Exec(`INSERT INTO file_events
		(id,workload_id,event_time,operation,path,dest_path,old_sha256,new_sha256,old_entropy,new_entropy,old_size,new_size,actor,scenario_id,ingested_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.WorkloadID, e.EventTime, e.Operation, e.Path, e.DestPath, e.OldSHA256, e.NewSHA256,
		e.OldEntropy, e.NewEntropy, e.OldSize, e.NewSize, e.Actor, e.ScenarioID, e.IngestedAt)
	if err != nil {
		return false, err
	}
	return true, nil
}

func nullStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullF(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullI(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func eventsEquivalent(a, b domain.FileEvent) bool {
	samePtrStr := func(x, y *string) bool {
		if (x == nil) != (y == nil) {
			return false
		}
		return x == nil || *x == *y
	}
	samePtrF := func(x, y *float64) bool {
		if (x == nil) != (y == nil) {
			return false
		}
		return x == nil || *x == *y
	}
	samePtrI := func(x, y *int64) bool {
		if (x == nil) != (y == nil) {
			return false
		}
		return x == nil || *x == *y
	}
	return a.WorkloadID == b.WorkloadID && a.EventTime == b.EventTime && a.Operation == b.Operation &&
		a.Path == b.Path && samePtrStr(a.DestPath, b.DestPath) && samePtrStr(a.OldSHA256, b.OldSHA256) &&
		samePtrStr(a.NewSHA256, b.NewSHA256) && samePtrF(a.OldEntropy, b.OldEntropy) &&
		samePtrF(a.NewEntropy, b.NewEntropy) && samePtrI(a.OldSize, b.OldSize) &&
		samePtrI(a.NewSize, b.NewSize) && samePtrStr(a.ScenarioID, b.ScenarioID)
}

func (s *Store) getEvent(id string) (domain.FileEvent, error) {
	var e domain.FileEvent
	var dest, oldH, newH, actor, scen sql.NullString
	var oldE, newE sql.NullFloat64
	var oldS, newS sql.NullInt64
	err := s.db.QueryRow(`SELECT id,workload_id,event_time,operation,path,dest_path,old_sha256,new_sha256,
		old_entropy,new_entropy,old_size,new_size,actor,scenario_id,ingested_at FROM file_events WHERE id=?`, id).
		Scan(&e.ID, &e.WorkloadID, &e.EventTime, &e.Operation, &e.Path, &dest, &oldH, &newH,
			&oldE, &newE, &oldS, &newS, &actor, &scen, &e.IngestedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	set := func(dst **string, v sql.NullString) {
		if v.Valid {
			x := v.String
			*dst = &x
		}
	}
	set(&e.DestPath, dest)
	set(&e.OldSHA256, oldH)
	set(&e.NewSHA256, newH)
	set(&e.Actor, actor)
	set(&e.ScenarioID, scen)
	if oldE.Valid {
		x := oldE.Float64
		e.OldEntropy = &x
	}
	if newE.Valid {
		x := newE.Float64
		e.NewEntropy = &x
	}
	if oldS.Valid {
		x := oldS.Int64
		e.OldSize = &x
	}
	if newS.Valid {
		x := newS.Int64
		e.NewSize = &x
	}
	return e, nil
}

func (s *Store) ListEvents(workloadID string, limit int) ([]domain.FileEvent, error) {
	q := `SELECT id,workload_id,event_time,operation,path,dest_path,old_sha256,new_sha256,
		old_entropy,new_entropy,old_size,new_size,actor,scenario_id,ingested_at
		FROM file_events WHERE workload_id=? ORDER BY event_time, id`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.FileEvent
	for rows.Next() {
		var e domain.FileEvent
		var dest, oldH, newH, actor, scen sql.NullString
		var oldE, newE sql.NullFloat64
		var oldS, newS sql.NullInt64
		if err := rows.Scan(&e.ID, &e.WorkloadID, &e.EventTime, &e.Operation, &e.Path, &dest, &oldH, &newH,
			&oldE, &newE, &oldS, &newS, &actor, &scen, &e.IngestedAt); err != nil {
			return nil, err
		}
		if dest.Valid {
			x := dest.String
			e.DestPath = &x
		}
		if oldH.Valid {
			x := oldH.String
			e.OldSHA256 = &x
		}
		if newH.Valid {
			x := newH.String
			e.NewSHA256 = &x
		}
		if actor.Valid {
			x := actor.String
			e.Actor = &x
		}
		if scen.Valid {
			x := scen.String
			e.ScenarioID = &x
		}
		if oldE.Valid {
			x := oldE.Float64
			e.OldEntropy = &x
		}
		if newE.Valid {
			x := newE.Float64
			e.NewEntropy = &x
		}
		if oldS.Valid {
			x := oldS.Int64
			e.OldSize = &x
		}
		if newS.Valid {
			x := newS.Int64
			e.NewSize = &x
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EventTimeBounds returns min and max event times for a workload.
func (s *Store) EventTimeBounds(workloadID string) (minT, maxT string, count int, err error) {
	err = s.db.QueryRow(`SELECT MIN(event_time),MAX(event_time),COUNT(1) FROM file_events WHERE workload_id=?`,
		workloadID).Scan(&minT, &maxT, &count)
	if minT == "" {
		minT, maxT = "", ""
	}
	return
}

// ---- snapshots ----

func (s *Store) InsertSnapshot(snap domain.Snapshot) error {
	_, err := s.db.Exec(`INSERT INTO snapshots(id,workload_id,captured_at,status,manifest_hash,file_count,total_bytes,created_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		snap.ID, snap.WorkloadID, snap.CapturedAt, snap.Status, snap.ManifestHash, snap.FileCount, snap.TotalBytes, snap.CreatedAt)
	return err
}

func (s *Store) SealSnapshot(id, manifestHash string, fileCount int, totalBytes int64) error {
	res, err := s.db.Exec(`UPDATE snapshots SET status='sealed', manifest_hash=?, file_count=?, total_bytes=? WHERE id=? AND status='creating'`,
		manifestHash, fileCount, totalBytes, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("snapshot %s not in creating state: %w", id, ErrConflict)
	}
	return nil
}

func (s *Store) InvalidateSnapshot(id string) error {
	_, err := s.db.Exec(`UPDATE snapshots SET status='invalid' WHERE id=? AND status='creating'`, id)
	return err
}

func (s *Store) ListSnapshots(workloadID string) ([]domain.Snapshot, error) {
	rows, err := s.db.Query(`SELECT id,workload_id,captured_at,status,COALESCE(manifest_hash,''),file_count,total_bytes,created_at
		FROM snapshots WHERE workload_id=? ORDER BY captured_at DESC, id DESC`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Snapshot
	for rows.Next() {
		var sn domain.Snapshot
		if err := rows.Scan(&sn.ID, &sn.WorkloadID, &sn.CapturedAt, &sn.Status, &sn.ManifestHash, &sn.FileCount, &sn.TotalBytes, &sn.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

func (s *Store) GetSnapshot(id string) (domain.Snapshot, error) {
	var sn domain.Snapshot
	err := s.db.QueryRow(`SELECT id,workload_id,captured_at,status,COALESCE(manifest_hash,''),file_count,total_bytes,created_at
		FROM snapshots WHERE id=?`, id).
		Scan(&sn.ID, &sn.WorkloadID, &sn.CapturedAt, &sn.Status, &sn.ManifestHash, &sn.FileCount, &sn.TotalBytes, &sn.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sn, ErrNotFound
	}
	return sn, err
}

func (s *Store) InsertSnapshotFile(snapshotID, relPath, sha string, size int64, entropy *float64) error {
	var e any
	if entropy != nil {
		e = *entropy
	}
	_, err := s.db.Exec(`INSERT INTO snapshot_files(snapshot_id,relative_path,sha256,size_bytes,entropy) VALUES(?,?,?,?,?)`,
		snapshotID, relPath, sha, size, e)
	return err
}

func (s *Store) ListSnapshotFiles(snapshotID string) (map[string]domainFileRow, error) {
	rows, err := s.db.Query(`SELECT relative_path,sha256,size_bytes,entropy FROM snapshot_files WHERE snapshot_id=?`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]domainFileRow{}
	for rows.Next() {
		var r domainFileRow
		var ent sql.NullFloat64
		if err := rows.Scan(&r.Path, &r.SHA256, &r.Size, &ent); err != nil {
			return nil, err
		}
		if ent.Valid {
			x := ent.Float64
			r.Entropy = &x
		}
		out[r.Path] = r
	}
	return out, rows.Err()
}

type domainFileRow struct {
	Path    string
	SHA256  string
	Size    int64
	Entropy *float64
}

// ---- file objects ----

// UpsertObject records a content object and increments its reference count.
func (s *Store) UpsertObject(sha string, size int64, storagePath string) error {
	_, err := s.db.Exec(`INSERT INTO file_objects(sha256,size_bytes,storage_path,ref_count,created_at)
		VALUES(?,?,?,1,?)
		ON CONFLICT(sha256) DO UPDATE SET ref_count=ref_count+1`, sha, size, storagePath, now())
	return err
}

func (s *Store) GetObject(sha string) (size int64, storagePath string, err error) {
	err = s.db.QueryRow(`SELECT size_bytes,storage_path FROM file_objects WHERE sha256=?`, sha).Scan(&size, &storagePath)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return
}

// ---- analysis runs / windows / assessments ----

func (s *Store) InsertAnalysisRun(r domain.AnalysisRun) error {
	var onset any
	if r.OnsetTime != nil {
		onset = *r.OnsetTime
	}
	_, err := s.db.Exec(`INSERT INTO analysis_runs(id,workload_id,detector_version,baseline_period,window_seconds,
		baseline_window_count,risk_threshold,status,onset_time,windows_evaluated,started_at,finished_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.WorkloadID, r.DetectorVersion, r.BaselinePeriod, r.WindowSeconds, r.BaselineWindowCount,
		r.RiskThreshold, r.Status, onset, r.WindowsEvaluated, r.StartedAt, nullableString(r.FinishedAt))
	return err
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) FinishAnalysisRun(id string, status string, onset *string, windows int) error {
	var o any
	if onset != nil {
		o = *onset
	}
	_, err := s.db.Exec(`UPDATE analysis_runs SET status=?, onset_time=?, windows_evaluated=?, finished_at=? WHERE id=?`,
		status, o, windows, now(), id)
	return err
}

func (s *Store) LatestAnalysisRun(workloadID string) (domain.AnalysisRun, error) {
	var r domain.AnalysisRun
	var onset sql.NullString
	var fin sql.NullString
	err := s.db.QueryRow(`SELECT id,workload_id,detector_version,baseline_period,window_seconds,baseline_window_count,
		risk_threshold,status,onset_time,windows_evaluated,started_at,finished_at
		FROM analysis_runs WHERE workload_id=? AND status='completed' ORDER BY started_at DESC, id DESC LIMIT 1`, workloadID).
		Scan(&r.ID, &r.WorkloadID, &r.DetectorVersion, &r.BaselinePeriod, &r.WindowSeconds, &r.BaselineWindowCount,
			&r.RiskThreshold, &r.Status, &onset, &r.WindowsEvaluated, &r.StartedAt, &fin)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if onset.Valid {
		x := onset.String
		r.OnsetTime = &x
	}
	if fin.Valid {
		x := fin.String
		r.FinishedAt = x
	}
	return r, nil
}

func (s *Store) InsertDetectionWindow(w domain.DetectionWindow, featureJSON, explanationJSON string) error {
	_, err := s.db.Exec(`INSERT INTO detection_windows(id,analysis_run_id,workload_id,starts_at,ends_at,feature_json,
		risk_score,alert_level,explanation_json,is_baseline,event_count) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		w.ID, w.AnalysisRunID, w.WorkloadID, w.StartsAt, w.EndsAt, featureJSON, w.RiskScore, w.AlertLevel,
		explanationJSON, boolInt(w.IsBaseline), w.EventCount)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) ListDetectionWindows(analysisRunID string) ([]domain.DetectionWindow, error) {
	rows, err := s.db.Query(`SELECT id,analysis_run_id,workload_id,starts_at,ends_at,feature_json,risk_score,alert_level,
		explanation_json,is_baseline,event_count FROM detection_windows WHERE analysis_run_id=? ORDER BY starts_at`, analysisRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DetectionWindow
	for rows.Next() {
		var w domain.DetectionWindow
		var fj, ej string
		var ib int
		if err := rows.Scan(&w.ID, &w.AnalysisRunID, &w.WorkloadID, &w.StartsAt, &w.EndsAt, &fj, &w.RiskScore,
			&w.AlertLevel, &ej, &ib, &w.EventCount); err != nil {
			return nil, err
		}
		w.IsBaseline = ib == 1
		w.Features = decodeFeatureJSON(fj)
		w.Contributions = decodeContribJSON(ej)
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) UpsertSnapshotAssessment(a domain.SnapshotAssessment, explanationJSON string) error {
	_, err := s.db.Exec(`INSERT INTO snapshot_assessments(snapshot_id,analysis_run_id,integrity_ok,coverage_ok,exposure_status,
		risk_score,affected_file_count,explanation_json,evaluated_at) VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(snapshot_id) DO UPDATE SET analysis_run_id=excluded.analysis_run_id, integrity_ok=excluded.integrity_ok,
		coverage_ok=excluded.coverage_ok, exposure_status=excluded.exposure_status, risk_score=excluded.risk_score,
		affected_file_count=excluded.affected_file_count, explanation_json=excluded.explanation_json, evaluated_at=excluded.evaluated_at`,
		a.SnapshotID, a.AnalysisRunID, boolInt(a.IntegrityOK), boolInt(a.CoverageOK), a.ExposureStatus,
		a.RiskScore, a.AffectedFileCount, explanationJSON, a.EvaluatedAt)
	return err
}

func (s *Store) GetSnapshotAssessment(snapshotID string) (domain.SnapshotAssessment, error) {
	var a domain.SnapshotAssessment
	var ej string
	var iok, cok int
	err := s.db.QueryRow(`SELECT snapshot_id,analysis_run_id,integrity_ok,coverage_ok,exposure_status,risk_score,
		affected_file_count,explanation_json,evaluated_at FROM snapshot_assessments WHERE snapshot_id=?`, snapshotID).
		Scan(&a.SnapshotID, &a.AnalysisRunID, &iok, &cok, &a.ExposureStatus, &a.RiskScore, &a.AffectedFileCount, &ej, &a.EvaluatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	a.IntegrityOK = iok == 1
	a.CoverageOK = cok == 1
	a.Explanation = decodeStringListJSON(ej)
	return a, nil
}

// ---- recovery runs ----

func (s *Store) InsertRecoveryRun(r domain.RecoveryRun) error {
	_, err := s.db.Exec(`INSERT INTO recovery_runs(id,workload_id,snapshot_id,status,target_name,output_path,total_files,
		verified_files,failed_files,source_unchanged,created_at) VALUES(?,?,?,?,?,?,0,0,0,0,?)`,
		r.ID, r.WorkloadID, r.SnapshotID, r.Status, r.TargetName, r.OutputPath, r.CreatedAt)
	return err
}

func (s *Store) FinishRecoveryRun(id, status string, total, verified, failed int, sourceUnchanged bool, reportJSON string) error {
	_, err := s.db.Exec(`UPDATE recovery_runs SET status=?, total_files=?, verified_files=?, failed_files=?,
		source_unchanged=?, finished_at=?, report_json=? WHERE id=?`,
		status, total, verified, failed, boolInt(sourceUnchanged), now(), reportJSON, id)
	return err
}

func (s *Store) GetRecoveryRun(id string) (domain.RecoveryRun, error) {
	var r domain.RecoveryRun
	var fin sql.NullString
	var su int
	err := s.db.QueryRow(`SELECT id,workload_id,snapshot_id,status,target_name,output_path,total_files,verified_files,
		failed_files,source_unchanged,created_at,finished_at FROM recovery_runs WHERE id=?`, id).
		Scan(&r.ID, &r.WorkloadID, &r.SnapshotID, &r.Status, &r.TargetName, &r.OutputPath, &r.TotalFiles,
			&r.VerifiedFiles, &r.FailedFiles, &su, &r.CreatedAt, &fin)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	r.SourceUnchanged = su == 1
	if fin.Valid {
		r.FinishedAt = fin.String
	}
	return r, nil
}

func (s *Store) ListRecoveryRuns(workloadID string) ([]domain.RecoveryRun, error) {
	rows, err := s.db.Query(`SELECT id,workload_id,snapshot_id,status,target_name,output_path,total_files,verified_files,
		failed_files,source_unchanged,created_at,finished_at FROM recovery_runs WHERE workload_id=? ORDER BY created_at DESC`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RecoveryRun
	for rows.Next() {
		var r domain.RecoveryRun
		var fin sql.NullString
		var su int
		if err := rows.Scan(&r.ID, &r.WorkloadID, &r.SnapshotID, &r.Status, &r.TargetName, &r.OutputPath, &r.TotalFiles,
			&r.VerifiedFiles, &r.FailedFiles, &su, &r.CreatedAt, &fin); err != nil {
			return nil, err
		}
		r.SourceUnchanged = su == 1
		if fin.Valid {
			r.FinishedAt = fin.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- audit ----

func (s *Store) InsertAuditEvent(actor, action, target, outcome, requestID string) error {
	_, err := s.db.Exec(`INSERT INTO audit_events(actor,action,target_id,outcome,request_id,created_at) VALUES(?,?,?,?,?,?)`,
		actor, action, target, outcome, requestID, now())
	return err
}

func (s *Store) ListAuditEvents(limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT id,actor,action,target_id,outcome,request_id,created_at FROM audit_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AuditEvent
	for rows.Next() {
		var e domain.AuditEvent
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.TargetID, &e.Outcome, &e.RequestID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
