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
	"strings"
)

func main() {
	py := os.Getenv("CLEANPOINT_PYTHON")
	if py == "" {
		py = "python"
	}
	root := "."
	if exe, err := os.Executable(); err == nil {
		if d := filepath.Dir(filepath.Dir(exe)); strings.HasSuffix(filepath.ToSlash(d), "/cmd") {
			root = filepath.Dir(d)
		}
	}
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
