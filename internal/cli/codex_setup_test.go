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

// TestEnsureCodexAccountReadyForLaunchOverrideUnboundProject verifies that when
// accountOverride is set against an unbound project, the named profile is
// resolved without writing a binding row, and host-auth is checked.
func TestEnsureCodexAccountReadyForLaunchOverrideUnboundProject(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "codex", "override-account", "--skip-login", "--no-bind"})

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	stub := installStubCodexAccountAuth(t, cmd, true)

	profile, err := ensureCodexAccountReadyForLaunch(cmd, paths, projectRoot, "override-account", []string{"resume", "--last"})
	if err != nil {
		t.Fatalf("ensureCodexAccountReadyForLaunch(override) error = %v", err)
	}
	if profile.Name != "override-account" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "override-account")
	}
	// Auth check must have been called (non-skip path).
	if stub.statusHits == 0 {
		t.Fatal("LoginStatus() hits = 0, want host-auth check on override path")
	}

	// Assert NO binding row was written.
	store, storeErr := sqliteadapter.NewStore(paths.DatabasePath)
	if storeErr != nil {
		t.Fatalf("NewStore() error = %v", storeErr)
	}
	defer store.Close()
	if bootstrapErr := store.Bootstrap(context.Background()); bootstrapErr != nil {
		t.Fatalf("Bootstrap() error = %v", bootstrapErr)
	}
	normalizedRoot, _ := pathutil.Normalize(projectRoot)
	_, projErr := store.ProjectByRoot(context.Background(), normalizedRoot)
	if !errors.Is(projErr, domain.ErrNotFound) {
		t.Fatalf("ProjectByRoot() error = %v, want ErrNotFound (no binding row written by override)", projErr)
	}
}

// TestEnsureCodexAccountReadyForLaunchOverrideBoundProject verifies that when
// accountOverride is set against a BOUND project, the named override profile is
// resolved — NOT the bound profile — and no new binding row is written.
func TestEnsureCodexAccountReadyForLaunchOverrideBoundProject(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "codex", "bound-account", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"account", "add", "codex", "override-account", "--skip-login", "--no-bind"})
	// Bind the project to bound-account.
	runManage(t, paths, []string{"bind", "codex", "bound-account", "--project", projectRoot})

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	installStubCodexAccountAuth(t, cmd, true)

	profile, err := ensureCodexAccountReadyForLaunch(cmd, paths, projectRoot, "override-account", []string{"resume", "--last"})
	if err != nil {
		t.Fatalf("ensureCodexAccountReadyForLaunch(override bound) error = %v", err)
	}
	if profile.Name != "override-account" {
		t.Fatalf("profile.Name = %q, want %q (override must win over bound account)", profile.Name, "override-account")
	}
}

// TestEnsureCodexAccountReadyForLaunchZeroAccounts verifies that when no Codex
// accounts exist and the project is unbound, the error message directs the user
// to `valv manage account add codex`.
func TestEnsureCodexAccountReadyForLaunchZeroAccounts(t *testing.T) {
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

	_, err := ensureCodexAccountReadyForLaunch(cmd, paths, projectRoot, "", []string{"resume", "--last"})
	if err == nil {
		t.Fatal("ensureCodexAccountReadyForLaunch(0-accounts) error = nil, want error")
	}
	for _, want := range []string{"project is not bound", "valv manage account add codex"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
	}
}

// TestEnsureCodexAccountReadyForLaunchOneAccountAutoBind verifies that when
// exactly one Codex account exists and the project is unbound, the account is
// auto-bound silently and the profile is returned with host-auth checked.
func TestEnsureCodexAccountReadyForLaunchOneAccountAutoBind(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "codex", "solo", "--skip-login", "--no-bind"})

	var stderr bytes.Buffer
	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	stub := installStubCodexAccountAuth(t, cmd, true)

	profile, err := ensureCodexAccountReadyForLaunch(cmd, paths, projectRoot, "", []string{"resume", "--last"})
	if err != nil {
		t.Fatalf("ensureCodexAccountReadyForLaunch(1-account) error = %v", err)
	}
	if profile.Name != "solo" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "solo")
	}
	if stub.statusHits == 0 {
		t.Fatal("LoginStatus() hits = 0, want host-auth check after auto-bind")
	}

	// Assert the binding row was written by calling again and verifying already-bound short-circuit.
	cmd2 := newCodexCommand(paths, nil)
	cmd2.SetContext(context.Background())
	cmd2.SetIn(bytes.NewBuffer(nil))
	cmd2.SetOut(&bytes.Buffer{})
	cmd2.SetErr(&bytes.Buffer{})
	installStubCodexAccountAuth(t, cmd2, true)

	profile2, err2 := ensureCodexAccountReadyForLaunch(cmd2, paths, projectRoot, "", []string{"resume", "--last"})
	if err2 != nil {
		t.Fatalf("second call (already-bound) error = %v", err2)
	}
	if profile2.Name != "solo" {
		t.Fatalf("second call profile.Name = %q, want %q (binding row must have been written)", profile2.Name, "solo")
	}
}

// TestEnsureCodexAccountReadyForLaunchMultipleAccountsNonTTY verifies that when
// 2+ Codex accounts exist, the project is unbound, and stdin/stdout are not a
// TTY, the function returns an actionable error pointing to the manual bind command.
func TestEnsureCodexAccountReadyForLaunchMultipleAccountsNonTTY(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "codex", "alpha", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"account", "add", "codex", "beta", "--skip-login", "--no-bind"})

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil)) // non-TTY
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err := ensureCodexAccountReadyForLaunch(cmd, paths, projectRoot, "", []string{"resume", "--last"})
	if err == nil {
		t.Fatal("ensureCodexAccountReadyForLaunch(2+ accounts, non-TTY) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "valv manage bind codex") {
		t.Fatalf("error = %q, want reference to `valv manage bind codex`", err.Error())
	}
}

// TestEnsureCodexAccountReadyForLaunchAlreadyBound verifies that when the
// project is already bound to a Codex account, the existing profile is returned
// and host-auth is checked.
func TestEnsureCodexAccountReadyForLaunchAlreadyBound(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "codex", "existing", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"bind", "codex", "existing", "--project", projectRoot})

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	stub := installStubCodexAccountAuth(t, cmd, true)

	profile, err := ensureCodexAccountReadyForLaunch(cmd, paths, projectRoot, "", []string{"resume", "--last"})
	if err != nil {
		t.Fatalf("ensureCodexAccountReadyForLaunch(already-bound) error = %v", err)
	}
	if profile.Name != "existing" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "existing")
	}
	if stub.statusHits == 0 {
		t.Fatal("LoginStatus() hits = 0, want host-auth check on bound path")
	}
}

// TestEnsureCodexAccountReadyForLaunchSkipAccountReadyGuard verifies that when
// codexArgsSkipAccountReady returns true (e.g., args = ["login"]), step 4
// (ensureManagedAccountReady) is skipped but the profile is still returned.
func TestEnsureCodexAccountReadyForLaunchSkipAccountReadyGuard(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "codex", "bound-account", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"bind", "codex", "bound-account", "--project", projectRoot})

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	// install stub but expect 0 hits — skip guard must fire before LoginStatus.
	stub := installStubCodexAccountAuth(t, cmd, true)

	// "login" triggers codexArgsSkipAccountReady → true.
	profile, err := ensureCodexAccountReadyForLaunch(cmd, paths, projectRoot, "", []string{"login"})
	if err != nil {
		t.Fatalf("ensureCodexAccountReadyForLaunch(skip guard) error = %v", err)
	}
	if profile.Name != "bound-account" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "bound-account")
	}
	if stub.statusHits != 0 {
		t.Fatalf("LoginStatus() hits = %d, want 0 (skip guard must bypass host-auth)", stub.statusHits)
	}
}
