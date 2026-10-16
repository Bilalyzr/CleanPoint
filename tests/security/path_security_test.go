// Package securitytest covers release-blocking path and filesystem security
// cases (PRD §15.5 T01/T02): traversal, symlink escape, absolute, device and
// reserved paths, plus unsafe restore destinations. Denial paths must leave
// outside directories untouched.
package securitytest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cleanpoint/internal/security"
	tt "cleanpoint/tests/harness"
)

// Twelve mandated path/filesystem security cases.
func TestPathSecurity(t *testing.T) {
	t.Run("01_plain_traversal_rejected", func(t *testing.T) {
		if err := security.ValidateRelativePath("../outside.txt"); err == nil {
			t.Error("dot-dot traversal accepted")
		}
	})
	t.Run("02_nested_traversal_rejected", func(t *testing.T) {
		if err := security.ValidateRelativePath("docs/../../escape.txt"); err == nil {
			t.Error("nested traversal accepted")
		}
	})
	t.Run("03_absolute_posix_rejected", func(t *testing.T) {
		if err := security.ValidateRelativePath("/etc/passwd"); err == nil {
			t.Error("absolute path accepted")
		}
	})
	t.Run("04_windows_drive_rejected", func(t *testing.T) {
		if err := security.ValidateRelativePath(`C:\Windows\system32\evil.dll`); err == nil {
			t.Error("windows drive path accepted")
		}
	})
	t.Run("05_backslash_smuggling_rejected", func(t *testing.T) {
		if err := security.ValidateRelativePath(`docs\..\..\escape.txt`); err == nil {
			t.Error("backslash traversal accepted")
		}
	})
	t.Run("06_device_names_rejected", func(t *testing.T) {
		if err := security.ValidateRelativePath("COM1"); err == nil {
			t.Error("reserved device name accepted")
		}
		if err := security.ValidateRelativePath("aux/NUL"); err == nil {
			t.Error("nested device name accepted")
		}
	})
	t.Run("07_empty_and_dot_components_rejected", func(t *testing.T) {
		if err := security.ValidateRelativePath(""); err == nil {
			t.Error("empty path accepted")
		}
		if err := security.ValidateRelativePath("docs//file.txt"); err == nil {
			t.Error("empty component accepted")
		}
		if err := security.ValidateRelativePath("./file.txt"); err == nil {
			t.Error("dot component accepted")
		}
	})
	t.Run("08_null_byte_rejected", func(t *testing.T) {
		if err := security.ValidateRelativePath("doc\u0000.txt"); err == nil {
			t.Error("null byte accepted")
		}
	})
	t.Run("09_valid_paths_accepted", func(t *testing.T) {
		for _, ok := range []string{"docs/report.txt", "src/main.py", "a/b/c/d.bin"} {
			if err := security.ValidateRelativePath(ok); err != nil {
				t.Errorf("valid path %q rejected: %v", ok, err)
			}
		}
	})
	t.Run("10_api_rejects_unsafe_event_paths", func(t *testing.T) {
		e := tt.New(t)
		wl, _ := e.RegisterWorkload("sec-paths")
		bad := []string{"../escape.txt", "/abs.txt", `C:\x`, "COM1", "docs/../../x", "a//b"}
		for i, p := range bad {
			st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/events", map[string]any{
				"events": []map[string]any{{
					"id":        "bad-path-" + strings.ReplaceAll(strings.ReplaceAll(p, "/", "_"), `\`, "_"),
					"timestamp": tt.TS(10 + i), "operation": "create", "path": p,
				}},
			})
			if st != 422 {
				t.Errorf("case %d path %q: expected 422, got %d (%v)", i, p, st, out)
			}
		}
	})
	t.Run("11_symlink_escape_blocked", func(t *testing.T) {
		e := tt.New(t)
		outsideDir := filepath.Join(e.Sandbox, "outside")
		if err := os.MkdirAll(outsideDir, 0o755); err != nil {
			t.Fatal(err)
		}
		secret := filepath.Join(outsideDir, "secret.txt")
		if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
			t.Fatal(err)
		}
		wl, root := e.RegisterWorkload("sec-symlink")
		e.WriteFile(root, "readme.txt", "safe content")
		if err := os.Symlink(secret, filepath.Join(filepath.FromSlash(root), "leak.txt")); err != nil {
			t.Skipf("symlinks unavailable on this filesystem: %v", err)
		}
		st, out := e.Client("operator", "POST", "/api/v1/workloads/"+wl+"/snapshots", map[string]any{})
		if st < 400 {
			t.Fatalf("symlink capture: expected failure status, got %d (%v)", st, out)
		}
		_, list := e.Client("ro", "GET", "/api/v1/workloads/"+wl+"/snapshots", nil)
		for _, raw := range list["snapshots"].([]any) {
			s := raw.(map[string]any)
			if s["status"] == "sealed" {
				t.Errorf("snapshot %s sealed despite symlink in root", s["id"])
			}
		}
		if data, err := os.ReadFile(secret); err != nil || string(data) != "top secret" {
			t.Errorf("outside file was modified: %q %v", string(data), err)
		}
	})
	t.Run("12_workload_root_containment", func(t *testing.T) {
		e := tt.New(t)
		cases := []struct {
			name string
			root string
		}{
			{"absolute outside", filepath.ToSlash(t.TempDir())},
			{"sandbox itself", e.RootsDir()},
			{"traversal", e.RootsDir() + "/../.."},
		}
		for i, c := range cases {
			st, _ := e.Client("operator", "POST", "/api/v1/workloads", map[string]any{
				"name": "root-case-" + strings.ToLower(c.name[:3]) + "-x" + string(rune('a'+i)),
				"root_path": c.root,
			})
			if st != 422 && st != 400 {
				t.Errorf("case %q: expected rejection, got %d", c.name, st)
			}
		}
		ok := e.NewRoot("sec-ok-root")
		if st, _ := e.Client("operator", "POST", "/api/v1/workloads", map[string]any{
			"name": "sec-ok", "root_path": ok}); st != 201 {
			t.Errorf("valid root rejected: %d", st)
		}
	})
}

// Supplementary boundary checks beyond the twelve mandated cases.
func TestRestoreTargetNameValidation(t *testing.T) {
	for _, bad := range []string{"", "-lead", "UPPER", "sp ace", strings.Repeat("a", 65), "a/b", "a..b"} {
		if err := security.SafeTargetName(bad); err == nil {
			t.Errorf("target_name %q accepted", bad)
		}
	}
	for _, ok := range []string{"recovery-demo-001", "a", "abc-123"} {
		if err := security.SafeTargetName(ok); err != nil {
			t.Errorf("target_name %q rejected: %v", ok, err)
		}
	}
}

func TestSafeJoinStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := security.SafeJoin(root, "../x"); err == nil {
		t.Error("SafeJoin allowed traversal")
	}
	if _, err := security.SafeJoin(root, "sub/../../x"); err == nil {
		t.Error("SafeJoin allowed nested traversal")
	}
	p, err := security.SafeJoin(root, "sub/file.txt")
	if err != nil {
		t.Fatalf("valid join rejected: %v", err)
	}
	if !strings.HasPrefix(filepath.ToSlash(p), filepath.ToSlash(root)) {
		t.Errorf("join escaped root: %s", p)
	}
}
