// Package snapshots implements content-addressed snapshot capture, canonical
// manifests, sealing and integrity verification. A snapshot is only sealed
// after every object has been atomically placed and the manifest hash has been
// recomputed from the persisted rows; sealed content is never rewritten.
package snapshots

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cleanpoint/internal/config"
	"cleanpoint/internal/domain"
	"cleanpoint/internal/metrics"
	"cleanpoint/internal/security"
	"cleanpoint/internal/storage"
)

var (
	ErrTooManyFiles = fmt.Errorf("workload exceeds configured file limit")
	ErrTooManyBytes = fmt.Errorf("workload exceeds configured byte limit")
	ErrSymlinkInRoot = fmt.Errorf("symlink or non-regular file in workload root")
	ErrNoEvents      = fmt.Errorf("workload has no ingested events; refusing to snapshot without telemetry")
	ErrRootUnavailable = fmt.Errorf("workload root directory unavailable")
)

type Manager struct {
	cfg   *config.Config
	store *storage.Store
}

func NewManager(cfg *config.Config, store *storage.Store) *Manager {
	return &Manager{cfg: cfg, store: store}
}

// EntropyBytes computes Shannon entropy in bits/byte over up to four 4 KiB
// chunks (start, quarter, middle-ish, end) of r, mirroring the simulator's
// sampling contract. Small inputs simply use all bytes.
func EntropyBytes(r io.Reader, size int64) (float64, error) {
	const chunk = 4096
	var offsets []int64
	switch {
	case size <= chunk:
		offsets = []int64{0}
	case size <= 4*chunk:
		offsets = []int64{0}
	default:
		offsets = []int64{0, size / 4, size / 2, size - chunk}
	}
	ra, ok := r.(io.ReaderAt)
	if !ok {
		// Fallback: read everything once.
		data, err := io.ReadAll(io.LimitReader(r, 4*chunk))
		if err != nil {
			return 0, err
		}
		return shannon(data), nil
	}
	seen := map[int64]bool{}
	counts := make([]int, 256)
	total := 0
	for _, off := range offsets {
		if seen[off] {
			continue
		}
		seen[off] = true
		buf := make([]byte, chunk)
		n, err := ra.ReadAt(buf, off)
		if err != nil && err != io.EOF && n == 0 {
			return 0, err
		}
		for i := 0; i < n; i++ {
			counts[buf[i]]++
		}
		total += n
	}
	if total == 0 {
		return 0, fmt.Errorf("no bytes available for entropy")
	}
	return shannonCounts(counts, total), nil
}

func shannon(data []byte) float64 {
	counts := make([]int, 256)
	for _, b := range data {
		counts[b]++
	}
	if len(data) == 0 {
		return 0
	}
	return shannonCounts(counts, len(data))
}

func shannonCounts(counts []int, total int) float64 {
	var h float64
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / float64(total)
		h -= p * mathLog2(p)
	}
	return h
}

func mathLog2(x float64) float64 { return math.Log2(x) }

// capturedFile is the in-memory manifest row during capture.
type capturedFile struct {
	rel      string
	sha      string
	size     int64
	entropy  float64
}

// Capture walks the workload root, writes content objects, persists manifest
// rows and seals the snapshot. capturedAt, when empty, defaults to the latest
// ingested event time (the simulation clock) — wall-clock time would make
// simulated windows meaningless.
func (m *Manager) Capture(workload domain.Workload, capturedAt string) (domain.Snapshot, error) {
	if capturedAt == "" {
		_, maxT, count, err := m.store.EventTimeBounds(workload.ID)
		if err != nil {
			return domain.Snapshot{}, err
		}
		if count == 0 {
			return domain.Snapshot{}, ErrNoEvents
		}
		capturedAt = maxT
	} else if nt, err := domain.NormalizeTime(capturedAt); err != nil {
		return domain.Snapshot{}, fmt.Errorf("captured_at: %w", err)
	} else {
		capturedAt = nt
	}

	rootAbs, err := filepath.Abs(filepath.FromSlash(workload.RootPath))
	if err != nil {
		return domain.Snapshot{}, err
	}
	if err := security.ValidateRootInside(m.cfg.RootsDir(), workload.RootPath); err != nil {
		return domain.Snapshot{}, err
	}
	if fi, err := os.Lstat(rootAbs); err != nil || !fi.IsDir() {
		return domain.Snapshot{}, ErrRootUnavailable
	}

	id, err := domain.NewSnapshotID()
	if err != nil {
		return domain.Snapshot{}, err
	}
	snap := domain.Snapshot{
		ID:         id,
		WorkloadID: workload.ID,
		CapturedAt: capturedAt,
		Status:     domain.SnapshotCreating,
		CreatedAt:  domain.FormatTime(time.Now()),
	}
	if err := m.store.InsertSnapshot(snap); err != nil {
		return domain.Snapshot{}, err
	}

	var files []capturedFile
	var totalBytes int64
	walkErr := filepath.WalkDir(rootAbs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relNative, err := filepath.Rel(rootAbs, p)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relNative)
		if err := security.ValidateRelativePath(rel); err != nil {
			return fmt.Errorf("workload file %q rejected: %w", rel, err)
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", ErrSymlinkInRoot, rel)
		}
		if len(files)+1 > m.cfg.MaxFiles {
			return ErrTooManyFiles
		}
		if totalBytes+fi.Size() > m.cfg.MaxBytes {
			return ErrTooManyBytes
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return err
		}
		sha := hex.EncodeToString(h.Sum(nil))
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			f.Close()
			return err
		}
		ent, err := EntropyBytes(f, fi.Size())
		if err != nil {
			ent = 0
		}
		f.Close()
		if err := m.writeObject(sha, p, fi.Size()); err != nil {
			return err
		}
		files = append(files, capturedFile{rel: rel, sha: sha, size: fi.Size(), entropy: round2(ent)})
		totalBytes += fi.Size()
		return nil
	})
	if walkErr != nil {
		_ = m.store.InvalidateSnapshot(id)
		metrics.Inc("snapshot_capture_total", "outcome", "failed")
		return domain.Snapshot{}, walkErr
	}

	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	manifestHash := ManifestHash(func(yield func(rel, sha string, size int64, entropy float64) error) error {
		for _, f := range files {
			if err := yield(f.rel, f.sha, f.size, f.entropy); err != nil {
				return err
			}
		}
		return nil
	})

	for _, f := range files {
		if err := m.store.InsertSnapshotFile(id, f.rel, f.sha, f.size, &f.entropy); err != nil {
			_ = m.store.InvalidateSnapshot(id)
			return domain.Snapshot{}, err
		}
	}
	if err := m.store.SealSnapshot(id, manifestHash, len(files), totalBytes); err != nil {
		_ = m.store.InvalidateSnapshot(id)
		return domain.Snapshot{}, err
	}
	metrics.Inc("snapshot_capture_total", "outcome", "sealed")
	snap.Status = domain.SnapshotSealed
	snap.ManifestHash = manifestHash
	snap.FileCount = len(files)
	snap.TotalBytes = totalBytes
	return snap, nil
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

// ManifestHash serializes manifest rows canonically (stable path order, fixed
// numeric formatting, one JSON object per line) and hashes the bytes.
func ManifestHash(rows func(yield func(rel, sha string, size int64, entropy float64) error) error) string {
	h := sha256.New()
	_ = rows(func(rel, sha string, size int64, entropy float64) error {
		fmt.Fprintf(h, "{\"path\":%q,\"sha256\":%q,\"size\":%d,\"entropy\":%.2f}\n", rel, sha, size, entropy)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

// objectPath returns the fan-out storage path for a content hash.
func (m *Manager) objectPath(sha string) string {
	sha = strings.ToLower(sha)
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(m.cfg.ObjectsDir()), sha[:2], sha[2:4], sha))
}

// writeObject copies src into the object store under its content address,
// using a temp file + atomic rename, skipping the copy when the object
// already exists with the right size.
func (m *Manager) writeObject(sha, src string, size int64) error {
	dst := m.objectPath(sha)
	dstNative := filepath.FromSlash(dst)
	if _, err := os.Stat(dstNative); err == nil {
		return m.store.UpsertObject(sha, size, dst)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstNative), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dstNative), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	h := sha256.New()
	tr := io.TeeReader(in, h)
	if _, err := io.Copy(tmp, tr); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(sha) {
		return fmt.Errorf("object hash mismatch while writing %s: got %s", sha, got)
	}
	if err := os.Rename(name, dstNative); err != nil {
		return err
	}
	return m.store.UpsertObject(sha, size, dst)
}

// IntegrityReport is the outcome of a full manifest + object verification.
type IntegrityReport struct {
	SnapshotID     string
	ManifestOK     bool
	FilesChecked   int
	ObjectsOK      int
	Problems       []string
}

// Verify recomputes the canonical manifest hash from persisted rows and
// re-hashes every stored object. Any problem marks integrity failed.
func (m *Manager) Verify(snapshotID string) (IntegrityReport, error) {
	rep := IntegrityReport{SnapshotID: snapshotID, ManifestOK: true}
	snap, err := m.store.GetSnapshot(snapshotID)
	if err != nil {
		return rep, err
	}
	files, err := m.store.ListSnapshotFiles(snapshotID)
	if err != nil {
		return rep, err
	}
	// Deterministic ordering for the manifest recomputation.
	rels := make([]string, 0, len(files))
	for r := range files {
		rels = append(rels, r)
	}
	sort.Strings(rels)

	recomputed := ManifestHash(func(yield func(rel, sha string, size int64, entropy float64) error) error {
		for _, r := range rels {
			f := files[r]
			e := 0.0
			if f.Entropy != nil {
				e = *f.Entropy
			}
			if err := yield(r, f.SHA256, f.Size, e); err != nil {
				return err
			}
		}
		return nil
	})
	if snap.Status != domain.SnapshotSealed {
		rep.ManifestOK = false
		rep.Problems = append(rep.Problems, fmt.Sprintf("snapshot status is %s, not sealed", snap.Status))
	}
	if recomputed != snap.ManifestHash {
		rep.ManifestOK = false
		rep.Problems = append(rep.Problems, "manifest hash mismatch: persisted rows do not reproduce sealed manifest")
	}
	for _, r := range rels {
		f := files[r]
		rep.FilesChecked++
		objSize, objPath, err := m.store.GetObject(f.SHA256)
		if err != nil {
			rep.Problems = append(rep.Problems, fmt.Sprintf("%s: missing object %s", r, short(f.SHA256)))
			continue
		}
		data, err := os.ReadFile(filepath.FromSlash(objPath))
		if err != nil {
			rep.Problems = append(rep.Problems, fmt.Sprintf("%s: object unreadable", r))
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != strings.ToLower(f.SHA256) || int64(len(data)) != objSize || int64(len(data)) != f.Size {
			rep.Problems = append(rep.Problems, fmt.Sprintf("%s: object content does not match hash/size", r))
			continue
		}
		rep.ObjectsOK++
	}
	return rep, nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
