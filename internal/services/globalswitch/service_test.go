package globalswitch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evanmschultz/valv/internal/domain"
)

type stubStore struct {
	profile domain.Profile
	called  bool
	err     error
}

func (s *stubStore) ProfileByName(_ context.Context, provider domain.Provider, name string) (domain.Profile, error) {
	s.called = true
	if s.err != nil {
		return domain.Profile{}, s.err
	}
	return s.profile, nil
}

func TestNewRequiresStoreAndPaths(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatal("New() error = nil, want dependency failure")
	}

	if _, err := New(Options{Store: &stubStore{}, StateDir: t.TempDir()}); err == nil {
		t.Fatal("New() error = nil, want home-dir failure")
	}

	if _, err := New(Options{Store: &stubStore{}, HomeDir: t.TempDir()}); err == nil {
		t.Fatal("New() error = nil, want state-dir failure")
	}
}

func TestSwitchCodexSymlinksSelectedProfileAndBacksUpExistingDir(t *testing.T) {
	homeDir := t.TempDir()
	stateDir := filepath.Join(homeDir, "state")
	profileHome := filepath.Join(homeDir, "profiles", "codex", "work")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	target := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll(target) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "auth.json"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &stubStore{profile: domain.Profile{Name: "work", Provider: domain.ProviderCodex, HomePath: profileHome}}
	service, err := New(Options{Store: store, HomeDir: homeDir, StateDir: stateDir, Now: func() time.Time { return time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC) }, IsRunning: func(context.Context, string) (bool, error) { return false, nil }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := service.Switch(context.Background(), domain.ProviderCodex, "work")
	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if !store.called {
		t.Fatal("expected profile lookup")
	}
	if result.BackupPath == "" {
		t.Fatal("expected backup path")
	}
	linkTarget, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink(%q) error = %v", target, err)
	}
	if linkTarget != profileHome {
		t.Fatalf("symlink target = %q, want %q", linkTarget, profileHome)
	}
	if _, err := os.Stat(filepath.Join(result.BackupPath, "auth.json")); err != nil {
		t.Fatalf("backup auth.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "global-switch", "codex", "current.json")); err != nil {
		t.Fatalf("current metadata missing: %v", err)
	}
}

func TestSwitchRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	service, err := New(Options{Store: &stubStore{}, HomeDir: t.TempDir(), StateDir: t.TempDir(), IsRunning: func(context.Context, string) (bool, error) { return false, nil }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := service.Switch(context.Background(), domain.Provider("claude"), "work"); err == nil {
		t.Fatal("Switch() error = nil, want unsupported-provider failure")
	}
}

func TestSwitchPropagatesProfileLookupFailure(t *testing.T) {
	t.Parallel()

	service, err := New(Options{Store: &stubStore{err: errors.New("boom")}, HomeDir: t.TempDir(), StateDir: t.TempDir(), IsRunning: func(context.Context, string) (bool, error) { return false, nil }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := service.Switch(context.Background(), domain.ProviderCodex, "work"); err == nil {
		t.Fatal("Switch() error = nil, want lookup failure")
	}
}

func TestSwitchReplacesExistingSymlinkWithoutBackup(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	profileHome := filepath.Join(homeDir, "profiles", "codex", "dev")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	target := filepath.Join(homeDir, ".codex")
	if err := os.Symlink(filepath.Join(homeDir, "old"), target); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	service, err := New(Options{Store: &stubStore{profile: domain.Profile{Name: "dev", Provider: domain.ProviderCodex, HomePath: profileHome}}, HomeDir: homeDir, StateDir: t.TempDir(), IsRunning: func(context.Context, string) (bool, error) { return false, nil }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := service.Switch(context.Background(), domain.ProviderCodex, "dev")
	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if result.BackupPath != "" {
		t.Fatalf("BackupPath = %q, want empty for symlink replacement", result.BackupPath)
	}
	linkTarget, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if linkTarget != profileHome {
		t.Fatalf("symlink target = %q, want %q", linkTarget, profileHome)
	}
}

func TestSwitchRejectsWhenHostCodexIsRunning(t *testing.T) {
	t.Parallel()

	service, err := New(Options{
		Store:     &stubStore{profile: domain.Profile{Name: "dev", Provider: domain.ProviderCodex, HomePath: t.TempDir()}},
		HomeDir:   t.TempDir(),
		StateDir:  t.TempDir(),
		IsRunning: func(context.Context, string) (bool, error) { return true, nil },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := service.Switch(context.Background(), domain.ProviderCodex, "dev"); err == nil {
		t.Fatal("Switch() error = nil, want running-process failure")
	}
}

func TestProcessRunningUsesPgrepExitStatus(t *testing.T) {
	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "pgrep")
	script := fmt.Sprintf("#!/bin/sh\nexit %d\n", 1)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	running, err := processRunning(context.Background(), "codex")
	if err != nil {
		t.Fatalf("processRunning() error = %v", err)
	}
	if running {
		t.Fatal("processRunning() = true, want false for exit status 1")
	}
}
