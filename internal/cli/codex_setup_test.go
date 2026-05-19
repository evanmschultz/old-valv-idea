package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqliteadapter "github.com/evanmschultz/valv/internal/adapters/sqlite"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/pathutil"
)

func TestRunCodexFirstRunSetupUsesDefaultHostProfile(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBufferString("1\n"))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	installStubCodexAccountAuth(t, cmd, true)

	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		t.Fatalf("openManageService() error = %v", err)
	}
	defer closeStore()

	if err := runCodexFirstRunSetup(cmd, service, projectRoot); err != nil {
		t.Fatalf("runCodexFirstRunSetup() error = %v", err)
	}

	store, err := sqliteadapter.NewStore(paths.DatabasePath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	profile, err := store.ProfileByName(context.Background(), domain.ProviderCodex, "default")
	if err != nil {
		t.Fatalf("ProfileByName(default) error = %v", err)
	}
	wantHome, err := pathutil.Normalize(filepath.Join(paths.HomeDir, ".codex"))
	if err != nil {
		t.Fatalf("Normalize(default home) error = %v", err)
	}
	if got, want := profile.HomePath, wantHome; got != want {
		t.Fatalf("profile home = %q, want %q", got, want)
	}
	normalizedProjectRoot, err := pathutil.Normalize(projectRoot)
	if err != nil {
		t.Fatalf("Normalize(projectRoot) error = %v", err)
	}
	project, err := store.ProjectByRoot(context.Background(), normalizedProjectRoot)
	if err != nil {
		t.Fatalf("ProjectByRoot() error = %v", err)
	}
	binding, err := store.BindingByProjectID(context.Background(), project.ID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("BindingByProjectID() error = %v", err)
	}
	if binding.ProfileID != profile.ID {
		t.Fatalf("binding profile id = %q, want %q", binding.ProfileID, profile.ID)
	}
}

func TestRunCodexFirstRunSetupCreatesIsolatedProfile(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBufferString("3\nprofile-name\n"))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	installStubCodexAccountAuth(t, cmd, true)

	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		t.Fatalf("openManageService() error = %v", err)
	}
	defer closeStore()

	if err := runCodexFirstRunSetup(cmd, service, projectRoot); err != nil {
		t.Fatalf("runCodexFirstRunSetup() error = %v", err)
	}

	store, err := sqliteadapter.NewStore(paths.DatabasePath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	profile, err := store.ProfileByName(context.Background(), domain.ProviderCodex, "profile-name")
	if err != nil {
		t.Fatalf("ProfileByName(profile-name) error = %v", err)
	}
	wantHome, err := pathutil.Normalize(filepath.Join(paths.ProviderRoot, "codex", "profiles", "profile-name"))
	if err != nil {
		t.Fatalf("Normalize(profile home) error = %v", err)
	}
	if got, want := profile.HomePath, wantHome; got != want {
		t.Fatalf("profile home = %q, want %q", got, want)
	}
}

func TestEnsureCodexBindingReadyReturnsActionableGuidanceWithoutTTY(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := ensureCodexBindingReady(cmd, paths, projectRoot, "")
	if err == nil {
		t.Fatal("ensureCodexBindingReady() error = nil, want unbound guidance")
	}
	if !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("ensureCodexBindingReady() error = %v, want domain.ErrUnboundProject", err)
	}
	for _, want := range []string{"valv manage account add codex", "isolated account"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ensureCodexBindingReady() error = %q, want substring %q", err.Error(), want)
		}
	}
}
