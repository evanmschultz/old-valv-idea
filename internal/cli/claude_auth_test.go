package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// stubClaudeAuthRunner is the test double for claudeAuthRunner. It can
// optionally write a fake .credentials.json to the account home during
// RunContainer to simulate a successful auth flow.
type stubClaudeAuthRunner struct {
	imageErr      error
	containerErr  error
	imageHits     int
	containerHits int

	// If writeCreds is true, RunContainer writes a minimal .credentials.json
	// to the accountHomePath before returning. Used by success-path tests.
	writeCreds      bool
	accountHomePath string
}

func (s *stubClaudeAuthRunner) EnsureImage(_ *cobra.Command, _ config.Paths) error {
	s.imageHits++
	return s.imageErr
}

func (s *stubClaudeAuthRunner) RunContainer(_ context.Context, request dockeradapter.ContainerRunRequest) error {
	s.containerHits++
	if s.writeCreds && s.accountHomePath != "" {
		credPath := filepath.Join(s.accountHomePath, ".credentials.json")
		_ = os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"test-token"}`), 0o600)
	}
	return s.containerErr
}

// installStubClaudeAuth injects stub into cmd's context and returns the stub.
func installStubClaudeAuth(t *testing.T, cmd *cobra.Command, stub *stubClaudeAuthRunner) {
	t.Helper()
	cmd.SetContext(context.WithValue(cmd.Context(), claudeAuthRunnerKey{}, claudeAuthRunner(stub)))
}

// newTestClaudeCmd creates a minimal cobra.Command wired with bytes buffers for
// stdin/stdout/stderr (non-TTY), ready for Claude auth tests.
func newTestClaudeCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}

func TestEnsureClaudeAccountReadyRejectsNonTTY(t *testing.T) {
	t.Parallel()

	cmd := newTestClaudeCmd()
	stub := &stubClaudeAuthRunner{}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: t.TempDir()}
	err := ensureClaudeAccountReady(cmd, account, accountAuthOptions{})
	if err == nil {
		t.Fatal("ensureClaudeAccountReady() error = nil, want non-tty error")
	}
	if !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("ensureClaudeAccountReady() error = %v, want TTY mention", err)
	}
	if stub.containerHits != 0 {
		t.Fatalf("RunContainer() hits = %d, want 0 (should reject before container launch)", stub.containerHits)
	}
}

// TestEnsureClaudeAccountReadyRespectsSkipLogin verifies C1: when SkipLogin is
// true the function returns nil immediately without wiping credentials or
// launching a container.
func TestEnsureClaudeAccountReadyRespectsSkipLogin(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	credPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"existing"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", credPath, err)
	}

	cmd := newTestClaudeCmd()
	stub := &stubClaudeAuthRunner{}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := ensureClaudeAccountReady(cmd, account, accountAuthOptions{SkipLogin: true})
	if err != nil {
		t.Fatalf("ensureClaudeAccountReady(SkipLogin=true) error = %v, want nil", err)
	}
	if stub.containerHits != 0 {
		t.Fatalf("RunContainer() hits = %d, want 0 (SkipLogin should short-circuit before container)", stub.containerHits)
	}
	if stub.imageHits != 0 {
		t.Fatalf("EnsureImage() hits = %d, want 0 (SkipLogin should short-circuit before image check)", stub.imageHits)
	}
	// Credentials must NOT be wiped — SkipLogin means leave the account as-is.
	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		t.Fatal("credentials file was wiped with SkipLogin=true; must be preserved")
	}
}

// TestEnsureClaudeAccountReadyNonTTYDoesNotWipe verifies C2: the TTY check
// fires BEFORE the credential wipe, so a non-TTY caller with pre-existing
// credentials receives the error and the credentials remain on disk.
func TestEnsureClaudeAccountReadyNonTTYDoesNotWipe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	credPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"existing"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", credPath, err)
	}

	cmd := newTestClaudeCmd()
	stub := &stubClaudeAuthRunner{}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := ensureClaudeAccountReady(cmd, account, accountAuthOptions{})
	if err == nil {
		t.Fatal("ensureClaudeAccountReady() error = nil, want non-tty error")
	}
	if !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("ensureClaudeAccountReady() error = %v, want TTY mention", err)
	}
	// Credentials must be preserved — the TTY guard fired before the wipe.
	if _, statErr := os.Stat(credPath); os.IsNotExist(statErr) {
		t.Fatal("credentials file was wiped on non-TTY call; TTY check must precede wipe")
	}
}

// TestBuildClaudeAuthContainerRequestShape verifies the ContainerRunRequest
// produced by buildClaudeAuthContainerRequest has the expected mount, args,
// and interactive/TTY flags.
func TestBuildClaudeAuthContainerRequestShape(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	account := domain.Profile{Name: "personal", HomePath: dir}

	req := buildClaudeAuthContainerRequest(account)
	if len(req.Mounts) != 1 {
		t.Fatalf("ContainerRunRequest.Mounts len = %d, want 1", len(req.Mounts))
	}
	if req.Mounts[0].Source != dir {
		t.Fatalf("Mount source = %q, want %q", req.Mounts[0].Source, dir)
	}
	if len(req.Args) < 2 || req.Args[0] != "auth" || req.Args[1] != "login" {
		t.Fatalf("ContainerRunRequest.Args = %v, want [auth login]", req.Args)
	}
	if !req.Interactive {
		t.Fatal("ContainerRunRequest.Interactive = false, want true")
	}
	if !req.TTY {
		t.Fatal("ContainerRunRequest.TTY = false, want true")
	}
}

// TestReadAccountIdentityReturnsLoggedOutWhenNoCreds verifies that
// claudeprovider.ReadAccountIdentity returns LoggedIn=false when no
// .credentials.json file exists in the account home directory. This is the
// condition ensureClaudeAccountReady checks post-container exit.
func TestReadAccountIdentityReturnsLoggedOutWhenNoCreds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Wipe any pre-existing creds to guarantee the directory is empty.
	if err := wipeClaudeCredentials(dir); err != nil {
		t.Fatalf("wipeClaudeCredentials(%q) error = %v", dir, err)
	}

	identity, err := claudeprovider.ReadAccountIdentity(dir)
	if err != nil {
		t.Fatalf("ReadAccountIdentity(%q) error = %v", dir, err)
	}
	if identity.LoggedIn {
		t.Fatal("ReadAccountIdentity() LoggedIn = true, want false (no creds written)")
	}
}

func TestLoginClaudeAccountSkipsNonTTYGuard(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Pre-write creds so ReadAccountIdentity returns LoggedIn=true after container "runs".
	credPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"tok"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", credPath, err)
	}

	cmd := newTestClaudeCmd()
	stub := &stubClaudeAuthRunner{writeCreds: true, accountHomePath: dir}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	// loginClaudeAccount does not have a non-TTY guard; it should proceed to
	// container launch even with a non-TTY cmd (bytes.Buffer stdin).
	err := loginClaudeAccount(cmd, account, config.Paths{})
	if err != nil {
		t.Fatalf("loginClaudeAccount() error = %v, want nil", err)
	}
	if stub.containerHits != 1 {
		t.Fatalf("RunContainer() hits = %d, want 1", stub.containerHits)
	}
}

// TestLoginClaudeAccountFailsWhenNoCredsAfterContainer verifies the
// "no credentials file found after login" error path. Since loginClaudeAccount
// has no TTY guard, a non-TTY stub can drive the full wipe→image→container→verify
// sequence. The stub does NOT write creds, so ReadAccountIdentity returns
// LoggedIn=false and the function must return an error.
func TestLoginClaudeAccountFailsWhenNoCredsAfterContainer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	// Stub does not write creds; simulates a container that exits without
	// producing a .credentials.json.
	stub := &stubClaudeAuthRunner{writeCreds: false}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := loginClaudeAccount(cmd, account, config.Paths{})
	if err == nil {
		t.Fatal("loginClaudeAccount() error = nil, want no-credentials error")
	}
	if !strings.Contains(err.Error(), "no credentials file found after login") {
		t.Fatalf("loginClaudeAccount() error = %v, want 'no credentials file found after login'", err)
	}
	if stub.containerHits != 1 {
		t.Fatalf("RunContainer() hits = %d, want 1", stub.containerHits)
	}
}

func TestLogoutManagedAccountWipesClaudeCredentials(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	credPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"tok"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", credPath, err)
	}

	cmd := newTestClaudeCmd()
	account := domain.Profile{Name: "personal", Provider: domain.ProviderClaude, HomePath: dir}

	if err := logoutManagedAccount(cmd, domain.ProviderClaude, account); err != nil {
		t.Fatalf("logoutManagedAccount(ProviderClaude) error = %v, want nil", err)
	}
	if _, err := os.Stat(credPath); !os.IsNotExist(err) {
		t.Fatalf("credentials file still exists at %q after logout; wipe should have removed it", credPath)
	}
}

func TestWipeClaudeCredentialsMissingFileIsOK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := wipeClaudeCredentials(dir); err != nil {
		t.Fatalf("wipeClaudeCredentials(empty dir) error = %v, want nil", err)
	}
}
