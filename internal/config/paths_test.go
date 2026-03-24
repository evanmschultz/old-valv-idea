package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	paths, err := ResolvePaths(home)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if paths.DatabasePath != filepath.Join(home, "Library", "Application Support", "valv", "db", "valv.sqlite3") {
		t.Fatalf("unexpected database path: %s", paths.DatabasePath)
	}
	if paths.LogsDir != filepath.Join(home, "Library", "Logs", "valv") {
		t.Fatalf("unexpected logs dir: %s", paths.LogsDir)
	}
}

func TestEnsureCreatesDirectories(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	paths, err := ResolvePaths(home)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	for _, dir := range []string{paths.DatabaseDir, paths.ProviderRoot, paths.ConfigDir, paths.LogsDir, paths.RuntimeTmpDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("directory %q not created: %v", dir, err)
		}
	}
}

func TestResolvePathsUsesProvidedHome(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	paths, err := ResolvePaths(home)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if paths.HomeDir != home {
		t.Fatalf("ResolvePaths().HomeDir = %q, want %q", paths.HomeDir, home)
	}
}
