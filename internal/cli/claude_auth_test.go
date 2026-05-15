package cli

// claude_auth_test.go — Unit 7.1 stub. Container-based test cases removed;
// Unit 7.3 will provide the full rewrite against the new host-subprocess
// interface. Tests below cover the file-level helpers that survive the rewrite.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// stubClaudeAccountAuthRunner is the test double for claudeAuthRunner used in
// the Unit 7.1 stub tests. Unit 7.3 replaces this with a richer version.
type stubClaudeAccountAuthRunner struct {
	setupTokenErr   error
	setupTokenHits  int
	extractTokenErr error
	extractToken    string
	extractHits     int

	// If writeCreds is true, RunSetupToken writes a .credentials.json to
	// accountHomePath before returning. Used by success-path tests.
	writeCreds      bool
	accountHomePath string
}

func (s *stubClaudeAccountAuthRunner) RunSetupToken(_ context.Context, _ string, _ io.Reader, _, _ io.Writer) error {
	s.setupTokenHits++
	if s.writeCreds && s.accountHomePath != "" {
		credPath := filepath.Join(s.accountHomePath, ".credentials.json")
		_ = os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"test-token"}`), 0o600)
	}
	return s.setupTokenErr
}

func (s *stubClaudeAccountAuthRunner) ExtractKeychainToken(_ context.Context, _ string) (string, error) {
	s.extractHits++
	return s.extractToken, s.extractTokenErr
}

// installStubClaudeAuth injects stub into cmd's context and returns the stub.
func installStubClaudeAuth(t *testing.T, cmd *cobra.Command, stub *stubClaudeAccountAuthRunner) {
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
	stub := &stubClaudeAccountAuthRunner{}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: t.TempDir()}
	err := ensureClaudeAccountReady(cmd, account, accountAuthOptions{})
	if err == nil {
		t.Fatal("ensureClaudeAccountReady() error = nil, want non-tty error")
	}
	if !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("ensureClaudeAccountReady() error = %v, want TTY mention", err)
	}
	if stub.setupTokenHits != 0 {
		t.Fatalf("RunSetupToken() hits = %d, want 0 (should reject before subprocess launch)", stub.setupTokenHits)
	}
}

// TestEnsureClaudeAccountReadyRespectsSkipLogin verifies that when SkipLogin is
// true the function returns nil immediately without wiping credentials or
// launching a subprocess.
func TestEnsureClaudeAccountReadyRespectsSkipLogin(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	credPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"existing"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", credPath, err)
	}

	cmd := newTestClaudeCmd()
	stub := &stubClaudeAccountAuthRunner{}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := ensureClaudeAccountReady(cmd, account, accountAuthOptions{SkipLogin: true})
	if err != nil {
		t.Fatalf("ensureClaudeAccountReady(SkipLogin=true) error = %v, want nil", err)
	}
	if stub.setupTokenHits != 0 {
		t.Fatalf("RunSetupToken() hits = %d, want 0 (SkipLogin should short-circuit)", stub.setupTokenHits)
	}
	// Credentials must NOT be wiped — SkipLogin means leave the account as-is.
	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		t.Fatal("credentials file was wiped with SkipLogin=true; must be preserved")
	}
}

// TestEnsureClaudeAccountReadyNonTTYDoesNotWipe verifies the TTY check fires
// BEFORE the credential wipe, so a non-TTY caller with pre-existing credentials
// receives the error and the credentials remain on disk.
func TestEnsureClaudeAccountReadyNonTTYDoesNotWipe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	credPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"existing"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", credPath, err)
	}

	cmd := newTestClaudeCmd()
	stub := &stubClaudeAccountAuthRunner{}
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

func TestReadAccountIdentityReturnsLoggedOutWhenNoCreds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
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

// TestLoginClaudeAccountSkipsNonTTYGuard verifies loginClaudeAccount has no
// non-TTY guard. The stub returns an extract error to terminate early without
// real keychain access — but RunSetupToken must be reached, proving no TTY
// guard exists.
func TestLoginClaudeAccountSkipsNonTTYGuard(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	stub := &stubClaudeAccountAuthRunner{
		extractTokenErr: errors.New("stub: no real keychain in tests"),
	}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := loginClaudeAccount(cmd, account, config.Paths{})
	// We expect an error from ExtractKeychainToken, NOT a TTY guard error.
	if err == nil {
		t.Fatal("loginClaudeAccount() error = nil, want extract error (no TTY guard)")
	}
	if strings.Contains(err.Error(), "TTY") {
		t.Fatalf("loginClaudeAccount() returned TTY guard error = %v; loginClaudeAccount must not have a TTY guard", err)
	}
	if stub.setupTokenHits != 1 {
		t.Fatalf("RunSetupToken() hits = %d, want 1 (no TTY guard should block it)", stub.setupTokenHits)
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
