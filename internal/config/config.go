// Package config loads CleanPoint runtime configuration from the environment
// with safe local-only defaults. Every trust boundary of the tool derives from
// these values; nothing here is production hardening.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const EnvPrefix = "CLEANPOINT_"

type Config struct {
	// Addr is the listen address. Default binds to loopback only; binding
	// beyond loopback is explicitly unsupported (see docs/limitations.md).
	Addr string
	// SandboxRoot is the single allowlisted parent directory for workload
	// roots, the object store and isolated restore outputs.
	SandboxRoot string
	DBPath      string
	// OperatorToken authorizes mutations; ReadOnlyToken authorizes reads.
	OperatorToken string
	ReadOnlyToken string
	MaxFiles      int
	MaxBytes      int64
	WindowSeconds int
	BaselineWindows int
	// RiskThreshold is the calibrated alert threshold for rules-v1. The
	// shipped default was calibrated on the validation seed split only
	// (docs/detection.md); it is a relative score, not a probability.
	RiskThreshold float64
	DetectorVersion string
}

func envStr(key, def string) string {
	if v := os.Getenv(EnvPrefix + key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(EnvPrefix + key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(EnvPrefix + key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// Load builds the configuration. Tokens: if the environment does not provide
// them, random tokens are generated once per server start and written to
// <sandbox>/.tokens so operators/scripts can authenticate. Tokens are never
// logged.
func Load() (*Config, error) {
	sandbox, err := filepath.Abs(envStr("SANDBOX_ROOT", "sandbox"))
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Addr:           envStr("ADDR", "127.0.0.1:8080"),
		SandboxRoot:    filepath.ToSlash(sandbox),
		DBPath:         envStr("DB_PATH", filepath.Join(filepath.FromSlash(sandbox), "cleanpoint.db")),
		MaxFiles:       envInt("MAX_FILES", 5000),
		MaxBytes:       int64(envInt("MAX_BYTES", 100*1024*1024)),
		WindowSeconds:  envInt("WINDOW_SECONDS", 300),
		BaselineWindows: envInt("BASELINE_WINDOWS", 24),
		RiskThreshold:  envFloat("RISK_THRESHOLD", 0.35),
		DetectorVersion: envStr("DETECTOR_VERSION", "rules-v1"),
	}
	if cfg.WindowSeconds <= 0 || cfg.BaselineWindows < 1 || cfg.RiskThreshold <= 0 || cfg.RiskThreshold > 1 {
		return nil, fmt.Errorf("invalid detector configuration")
	}
	for _, dir := range []string{
		filepath.Join(filepath.FromSlash(cfg.SandboxRoot), "roots"),
		filepath.Join(filepath.FromSlash(cfg.SandboxRoot), "objects"),
		filepath.Join(filepath.FromSlash(cfg.SandboxRoot), "restored"),
		filepath.Dir(filepath.FromSlash(cfg.DBPath)),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create sandbox dir: %w", err)
		}
	}
	cfg.OperatorToken = os.Getenv(EnvPrefix + "OPERATOR_TOKEN")
	cfg.ReadOnlyToken = os.Getenv(EnvPrefix + "READONLY_TOKEN")
	tokenFile := filepath.Join(filepath.FromSlash(cfg.SandboxRoot), ".tokens")
	if cfg.OperatorToken == "" || cfg.ReadOnlyToken == "" {
		// Reuse persisted tokens when present so a second start (or a failed
		// bind) never clobbers credentials a running server issued.
		if data, err := os.ReadFile(tokenFile); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if v, ok := strings.CutPrefix(line, "operator="); ok && cfg.OperatorToken == "" {
					cfg.OperatorToken = strings.TrimSpace(v)
				}
				if v, ok := strings.CutPrefix(line, "readonly="); ok && cfg.ReadOnlyToken == "" {
					cfg.ReadOnlyToken = strings.TrimSpace(v)
				}
			}
		}
	}
	if cfg.OperatorToken == "" || cfg.ReadOnlyToken == "" {
		gen := func() (string, error) {
			b := make([]byte, 24)
			if _, err := rand.Read(b); err != nil {
				return "", err
			}
			return hex.EncodeToString(b), nil
		}
		if cfg.OperatorToken == "" {
			if v, err := gen(); err == nil {
				cfg.OperatorToken = v
			}
		}
		if cfg.ReadOnlyToken == "" {
			if v, err := gen(); err == nil {
				cfg.ReadOnlyToken = v
			}
		}
		_ = os.WriteFile(tokenFile, []byte(fmt.Sprintf(
			"operator=%s\nreadonly=%s\n", cfg.OperatorToken, cfg.ReadOnlyToken)), 0o600)
	}
	if cfg.OperatorToken == "" || cfg.ReadOnlyToken == "" {
		return nil, fmt.Errorf("unable to establish auth tokens")
	}
	return cfg, nil
}

// RootsDir is the allowlisted parent of every workload root.
func (c *Config) RootsDir() string {
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(c.SandboxRoot), "roots"))
}

// ObjectsDir is the content-addressed object store root.
func (c *Config) ObjectsDir() string {
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(c.SandboxRoot), "objects"))
}

// RestoredDir is the parent of every server-generated restore destination.
func (c *Config) RestoredDir() string {
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(c.SandboxRoot), "restored"))
}

// ResolveWithin reports whether candidate (after lexical cleanup) is contained
// in parent. Both must be absolute slash paths.
func ResolveWithin(parent, candidate string) bool {
	if parent == "" || candidate == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.FromSlash(parent), filepath.FromSlash(candidate))
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel)
}
