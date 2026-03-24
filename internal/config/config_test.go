package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	t.Parallel()
	cfg := Default()
	if cfg.Output.Format != "auto" {
		t.Fatalf("Default().Output.Format = %q", cfg.Output.Format)
	}
	if cfg.Logging.Level != "info" {
		t.Fatalf("Default().Logging.Level = %q", cfg.Logging.Level)
	}
}

func TestLoadAndEffective(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte("[output]\nformat='json'\nstyle='never'\n[logging]\nlevel='debug'\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	effective, err := cfg.Effective()
	if err != nil {
		t.Fatalf("Effective() error = %v", err)
	}
	if effective.Output.Format != "json" {
		t.Fatalf("Effective().Output.Format = %q", effective.Output.Format)
	}
	if effective.Logging.Level != "debug" {
		t.Fatalf("Effective().Logging.Level = %q", effective.Logging.Level)
	}
}

func TestLoadRejectsUndecodedKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte("unknown='value'\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for undecoded keys")
	}
}

func TestLoadMissingPath(t *testing.T) {
	t.Parallel()

	if _, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("expected missing config error")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	t.Parallel()

	paths, err := ResolvePaths(t.TempDir())
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if got := DefaultConfigPath(paths); got != filepath.Join(paths.ConfigDir, "config.toml") {
		t.Fatalf("DefaultConfigPath() = %q", got)
	}
}
