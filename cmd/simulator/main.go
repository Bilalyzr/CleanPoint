// Command simulator is a thin launcher for the canonical Python scenario
// generator. Keeping one deterministic generator (Python) avoids divergent
// Go/Python fixture logic; this binary exists to satisfy the documented
// `cmd/simulator` entry point.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func findRootFrom(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "simulator", "generate.py")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func resolveRoot() string {
	if exe, err := os.Executable(); err == nil {
		if r := findRootFrom(filepath.Dir(exe)); r != "" {
			return r
		}
	}
	if wd, err := os.Getwd(); err == nil {
		if r := findRootFrom(wd); r != "" {
			return r
		}
		return wd
	}
	return "."
}

func main() {
	py := os.Getenv("CLEANPOINT_PYTHON")
	if py == "" {
		py = "python"
	}
	root := resolveRoot()
	script := filepath.Join(root, "simulator", "generate.py")
	args := append([]string{script}, os.Args[1:]...)
	cmd := exec.Command(py, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "simulator failed:", err)
		os.Exit(1)
	}
}
