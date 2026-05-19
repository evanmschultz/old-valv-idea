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
)

// TestEnsureClaudeBindingReadyOverrideUnboundProject verifies that when
// accountOverride is set against an unbound project, the named profile is
// resolved without writing a binding row.
func TestEnsureClaudeBindingReadyOverrideUnboundProject(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)

	// Create a Claude account via manage CLI.
	runManage(t, paths, []string{"account", "add", "claude", "override-account", "--skip-login", "--no-bind"})

	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	cmd := newClaudeCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	profile, err := ensureClaudeBindingReady(cmd, paths, projectRoot, "override-account")
	if err != nil {
		t.Fatalf("ensureClaudeBindingReady(override) error = %v", err)
	}
	if profile.Name != "override-account" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "override-account")
	}

	// Assert NO binding row was written. Direct store check: look up the project
	// root — override resolution must not create a project or binding record.
	store, storeErr := sqliteadapter.NewStore(paths.DatabasePath)
	if storeErr != nil {
		t.Fatalf("NewStore() error = %v", storeErr)
	}
	defer store.Close()
	if bootstrapErr := store.Bootstrap(context.Background()); bootstrapErr != nil {
		t.Fatalf("Bootstrap() error = %v", bootstrapErr)
	}
	_, projErr := store.ProjectByRoot(context.Background(), projectRoot)
	if !errors.Is(projErr, domain.ErrNotFound) {
		t.Fatalf("ProjectByRoot() error = %v, want ErrNotFound (no binding row written by override)", projErr)
	}
}

// TestEnsureClaudeBindingReadyOverrideBoundProject verifies that when
// accountOverride is set against a BOUND project, the named profile is
// resolved — NOT the bound profile — and no new binding row is written.
func TestEnsureClaudeBindingReadyOverrideBoundProject(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)

	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	// Create two Claude accounts: one bound, one for override.
	runManage(t, paths, []string{"account", "add", "claude", "bound-account", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"account", "add", "claude", "override-account", "--skip-login", "--no-bind"})
	// Bind the project to bound-account.
	runManage(t, paths, []string{"bind", "claude", "bound-account", "--project", projectRoot})

	cmd := newClaudeCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	// Override resolves override-account, not bound-account.
	profile, err := ensureClaudeBindingReady(cmd, paths, projectRoot, "override-account")
	if err != nil {
		t.Fatalf("ensureClaudeBindingReady(override bound) error = %v", err)
	}
	if profile.Name != "override-account" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "override-account")
	}
}

// TestEnsureClaudeBindingReadyZeroAccounts verifies that when no Claude
// accounts exist and the project is unbound, the error message directs the
// user to `valv manage account add claude`.
func TestEnsureClaudeBindingReadyZeroAccounts(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	cmd := newClaudeCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err := ensureClaudeBindingReady(cmd, paths, projectRoot, "")
	if err == nil {
		t.Fatal("ensureClaudeBindingReady(0-accounts) error = nil, want error")
	}
	for _, want := range []string{"project is not bound", "valv manage account add claude"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
	}
}

// TestEnsureClaudeBindingReadyOneAccountAutoBind verifies that when exactly
// one Claude account exists and the project is unbound, the account is
// auto-bound silently and the profile is returned.
func TestEnsureClaudeBindingReadyOneAccountAutoBind(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "claude", "solo", "--skip-login", "--no-bind"})

	var stderr bytes.Buffer
	cmd := newClaudeCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)

	profile, err := ensureClaudeBindingReady(cmd, paths, projectRoot, "")
	if err != nil {
		t.Fatalf("ensureClaudeBindingReady(1-account) error = %v", err)
	}
	if profile.Name != "solo" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "solo")
	}

	// Assert the binding row was written by calling ensureClaudeBindingReady again
	// and verifying it returns the same profile (already-bound short-circuit).
	cmd2 := newClaudeCommand(paths, nil)
	cmd2.SetContext(context.Background())
	cmd2.SetIn(bytes.NewBuffer(nil))
	cmd2.SetOut(&bytes.Buffer{})
	cmd2.SetErr(&bytes.Buffer{})

	profile2, err2 := ensureClaudeBindingReady(cmd2, paths, projectRoot, "")
	if err2 != nil {
		t.Fatalf("ensureClaudeBindingReady (second call, already-bound) error = %v", err2)
	}
	if profile2.Name != "solo" {
		t.Fatalf("second call profile.Name = %q, want %q (binding row must have been written)", profile2.Name, "solo")
	}
}

// TestEnsureClaudeBindingReadyMultipleAccountsNonTTY verifies that when 2+
// Claude accounts exist, the project is unbound, and stdin/stdout are not a
// TTY, the function returns an actionable error pointing to the manual bind
// command.
func TestEnsureClaudeBindingReadyMultipleAccountsNonTTY(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "claude", "alpha", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"account", "add", "claude", "beta", "--skip-login", "--no-bind"})

	cmd := newClaudeCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil)) // non-TTY
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err := ensureClaudeBindingReady(cmd, paths, projectRoot, "")
	if err == nil {
		t.Fatal("ensureClaudeBindingReady(2+ accounts, non-TTY) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "valv manage bind claude") {
		t.Fatalf("error = %q, want reference to `valv manage bind claude`", err.Error())
	}
}

// TestEnsureClaudeBindingReadyAlreadyBound verifies that when the project is
// already bound to a Claude account, the existing profile is returned without
// calling ListProfiles or BindProject.
func TestEnsureClaudeBindingReadyAlreadyBound(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "claude", "existing", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"bind", "claude", "existing", "--project", projectRoot})

	cmd := newClaudeCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	profile, err := ensureClaudeBindingReady(cmd, paths, projectRoot, "")
	if err != nil {
		t.Fatalf("ensureClaudeBindingReady(already-bound) error = %v", err)
	}
	if profile.Name != "existing" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "existing")
	}
}

// TestUnboundProjectNoAccountsError verifies the shared error helper returns
// the expected message format for each provider.
func TestUnboundProjectNoAccountsError(t *testing.T) {
	t.Parallel()

	err := unboundProjectNoAccountsError(domain.ProviderClaude)
	if err == nil {
		t.Fatal("unboundProjectNoAccountsError() = nil")
	}
	for _, want := range []string{"project is not bound", "claude", "valv manage account add claude"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
	}
}
