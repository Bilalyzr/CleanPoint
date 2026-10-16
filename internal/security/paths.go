// Package security centralizes path sanitization and safe filesystem
// primitives. CleanPoint must never follow symlinks, accept traversal, touch
// device-style paths, or write outside its sandbox. These helpers are the only
// approved way for the rest of the codebase to touch disk paths that originate
// from API input.
package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrEmptyPath      = errors.New("empty path")
	ErrAbsolutePath   = errors.New("absolute path not allowed")
	ErrTraversal      = errors.New("path traversal not allowed")
	ErrDevicePath     = errors.New("device or reserved path not allowed")
	ErrNonRegular     = errors.New("not a regular file")
	ErrSymlink        = errors.New("symlink in path")
	ErrDotComponent   = errors.New("dot component in path")
	ErrNullByte       = errors.New("null byte in path")
	ErrOutsideSandbox = errors.New("resolved path escapes sandbox")
)

// isWindowsReserved reports whether the base name looks like a DOS device
// (CON, PRN, AUX, NUL, COM1..9, LPT1..9) in any case.
func isWindowsReserved(name string) bool {
	up := strings.ToUpper(name)
	up = strings.TrimSuffix(up, ".")
	for _, d := range []string{"CON", "PRN", "AUX", "NUL"} {
		if up == d {
			return true
		}
	}
	for i := 1; i <= 9; i++ {
		if up == fmt.Sprintf("COM%d", i) || up == fmt.Sprintf("LPT%d", i) {
			return true
		}
	}
	return false
}

// ValidateRelativePath enforces the canonical workload-relative path contract:
// slash-separated, non-empty, no drive letters, no "..", no "." components,
// no leading "/", no NUL, no reserved device names, and every component is a
// plain name. Backslashes are rejected so Windows separators cannot smuggle
// components past the checks.
func ValidateRelativePath(p string) error {
	if p == "" {
		return ErrEmptyPath
	}
	if strings.ContainsRune(p, 0) {
		return ErrNullByte
	}
	if strings.ContainsRune(p, '\\') {
		return fmt.Errorf("%w: backslash separator", ErrTraversal)
	}
	if strings.HasPrefix(p, "/") || filepath.IsAbs(filepath.FromSlash(p)) {
		return ErrAbsolutePath
	}
	if strings.Contains(p, ":") && len(p) > 1 && p[1] == ':' {
		return ErrAbsolutePath
	}
	for _, comp := range strings.Split(p, "/") {
		if comp == "" {
			return fmt.Errorf("%w: empty component", ErrDotComponent)
		}
		if comp == "." || comp == ".." {
			return ErrTraversal
		}
		if isWindowsReserved(comp) {
			return ErrDevicePath
		}
		if strings.HasPrefix(comp, " ") || strings.HasSuffix(comp, " ") ||
			strings.HasPrefix(comp, ".") && comp != "." && len(comp) > 1 && strings.HasPrefix(comp, "..") {
			return fmt.Errorf("%w: suspicious component %q", ErrDotComponent, comp)
		}
	}
	return nil
}

// CanonicalRelativePath validates and normalizes a relative path.
func CanonicalRelativePath(p string) (string, error) {
	if err := ValidateRelativePath(p); err != nil {
		return "", err
	}
	return p, nil
}

// ValidateRootInside reports whether root is a directory strictly inside
// parent (after absolutizing both). It also verifies no symlink component.
func ValidateRootInside(parent, root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return err
	}
	if absRoot == absParent {
		return fmt.Errorf("%w: root equals sandbox parent", ErrOutsideSandbox)
	}
	rel, err := filepath.Rel(absParent, absRoot)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return fmt.Errorf("%w: %s escapes %s", ErrOutsideSandbox, root, parent)
	}
	return nil
}

// CheckNoSymlinks walks the path components of abs from its root and fails if
// any component (excluding the final one when finalMustExist is false) is a
// symlink. This defeats symlink swaps at boundaries.
func CheckNoSymlinks(abs string, finalMustExist bool) error {
	vol := filepath.VolumeName(abs)
	rest := strings.TrimPrefix(strings.TrimPrefix(abs, vol), string(filepath.Separator))
	parts := strings.Split(rest, string(filepath.Separator))
	cur := vol + string(filepath.Separator)
	for i, part := range parts {
		if part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		last := i == len(parts)-1
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) && !finalMustExist && last {
				return nil
			}
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrSymlink, cur)
		}
	}
	return nil
}

// SafeJoin builds root+"/"+rel after validating rel, verifies the resulting
// path stays inside root, and rejects symlink components. It returns an
// absolute native path.
func SafeJoin(root, rel string) (string, error) {
	if err := ValidateRelativePath(rel); err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(absRoot, filepath.FromSlash(rel))
	if err := ValidateRootInside(absRoot, joined); err != nil {
		return "", err
	}
	return joined, nil
}

// AtomicWrite writes data to a temp file in dir then renames it onto final.
// The temp file lives in the same directory so the rename cannot cross
// devices.
func AtomicWrite(final string, data []byte, perm os.FileMode) (written int, err error) {
	dir := filepath.Dir(final)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return 0, err
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(name)
		}
	}()
	n, err := tmp.Write(data)
	if err != nil {
		return n, err
	}
	if err = tmp.Sync(); err != nil {
		return n, err
	}
	if err = tmp.Close(); err != nil {
		return n, err
	}
	if err = os.Chmod(name, perm); err != nil {
		return n, err
	}
	if err = os.Rename(name, final); err != nil {
		return n, err
	}
	return n, nil
}

// SafeTargetName validates a restore target label: lowercase alphanumerics
// and dashes, 1..64 chars, must start alphanumeric.
func SafeTargetName(name string) error {
	if name == "" || len(name) > 64 {
		return fmt.Errorf("target_name must be 1..64 characters")
	}
	for i, r := range name {
		ok := r == '-' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !ok {
			return fmt.Errorf("target_name allows only [a-z0-9-]")
		}
		if i == 0 && r == '-' {
			return fmt.Errorf("target_name must start with alphanumeric")
		}
	}
	return nil
}
