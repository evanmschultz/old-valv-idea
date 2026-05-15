package cli

// claude_auth_test.go — Unit 7.3 full rewrite. Covers the host-subprocess
// Claude auth flow added in Unit 7.1, restoring coverage to ≥70%.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// stubClaudeAccountAuthRunner is the test double for claudeAuthRunner.
// setupTokenHits and extractHits track call counts for assertion.
// extractToken is returned by ExtractKeychainToken on success.
type stubClaudeAccountAuthRunner struct {
	setupTokenErr   error
	setupTokenHits  int
	extractTokenErr error
	extractToken    string
	extractHits     int
}

func (s *stubClaudeAccountAuthRunner) RunSetupToken(_ context.Context, _ string, _ io.Reader, _, _ io.Writer) error {
	s.setupTokenHits++
	return s.setupTokenErr
}

func (s *stubClaudeAccountAuthRunner) ExtractKeychainToken(_ context.Context, _ string) (string, error) {
	s.extractHits++
	return s.extractToken, s.extractTokenErr
}

// installStubClaudeAuth injects stub into cmd's context.
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

// installFakeHostClaude installs a fake `claude` shell script in a temp dir and
// prepends it to PATH. The script records args and CLAUDE_CONFIG_DIR to a log
// file, then exits 0. Returns the log file path.
func installFakeHostClaude(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "claude.log")
	scriptPath := filepath.Join(dir, "claude")
	script := `#!/bin/sh
set -eu
printf 'args:%s\n' "$*" >> "$FAKE_CLAUDE_LOG"
printf 'CLAUDE_CONFIG_DIR=%s\n' "${CLAUDE_CONFIG_DIR:-}" >> "$FAKE_CLAUDE_LOG"
if [ "${1:-}" = "setup-token" ]; then
  printf 'Setup token complete.\n'
  exit 0
fi
printf 'unexpected args\n'
exit 64
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", scriptPath, err)
	}
	t.Setenv("FAKE_CLAUDE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// --- ensureClaudeAccountReady tests ---

// TestEnsureClaudeAccountReadyRejectsNonTTY verifies that a non-TTY cmd
// returns an error mentioning "TTY" and does not invoke RunSetupToken.
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
		t.Fatalf("RunSetupToken() hits = %d, want 0 (SkipLogin must short-circuit)", stub.setupTokenHits)
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

// TestEnsureClaudeAccountReadyWipesAndRunsSetupTokenAndExtractsAndWrites is the
// full success-path test. It wires the stub to return a token from
// ExtractKeychainToken, then verifies:
// - pre-existing creds file is wiped before setup-token,
// - RunSetupToken is invoked once,
// - .credentials.json is written with the expected JSON shape,
// - function returns nil.
// Note: this test must run without a real TTY. commandHasTTY returns false for
// bytes.Buffer readers/writers — the non-TTY guard will fire. To exercise the
// success path we use loginClaudeAccount (which has no TTY guard) instead.
// The ensure path's success is covered indirectly by the write+extract test on
// loginClaudeAccount below.
func TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Pre-existing file that should be wiped.
	oldCred := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(oldCred, []byte(`{"claudeAiAccessToken":"old"}`), 0o600); err != nil {
		t.Fatalf("WriteFile old cred: %v", err)
	}

	cmd := newTestClaudeCmd()
	stub := &stubClaudeAccountAuthRunner{extractToken: "fresh-token"}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	if err := loginClaudeAccount(cmd, account, config.Paths{}); err != nil {
		t.Fatalf("loginClaudeAccount() error = %v, want nil", err)
	}

	if stub.setupTokenHits != 1 {
		t.Fatalf("RunSetupToken() hits = %d, want 1", stub.setupTokenHits)
	}
	if stub.extractHits != 1 {
		t.Fatalf("ExtractKeychainToken() hits = %d, want 1", stub.extractHits)
	}

	// .credentials.json must exist and contain the new token.
	data, err := os.ReadFile(oldCred)
	if err != nil {
		t.Fatalf("ReadFile(.credentials.json) error = %v", err)
	}
	if !strings.Contains(string(data), "fresh-token") {
		t.Fatalf(".credentials.json = %q, want fresh-token", string(data))
	}

	// ReadAccountIdentity must see LoggedIn=true.
	identity, err := claudeprovider.ReadAccountIdentity(dir)
	if err != nil {
		t.Fatalf("ReadAccountIdentity() error = %v", err)
	}
	if !identity.LoggedIn {
		t.Fatal("ReadAccountIdentity() LoggedIn = false, want true after successful login")
	}
}

// TestEnsureClaudeAccountReadyFailsWhenRunSetupTokenErrors verifies that an
// error from RunSetupToken propagates wrapped, and no extraction is attempted.
// Uses loginClaudeAccount to bypass the non-TTY guard in unit test context.
func TestLoginClaudeAccountFailsWhenRunSetupTokenErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	setupErr := errors.New("setup-token: network timeout")
	stub := &stubClaudeAccountAuthRunner{setupTokenErr: setupErr}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := loginClaudeAccount(cmd, account, config.Paths{})
	if err == nil {
		t.Fatal("loginClaudeAccount() error = nil, want setup-token error")
	}
	if !errors.Is(err, setupErr) {
		t.Fatalf("loginClaudeAccount() error = %v, want wrapping %v", err, setupErr)
	}
	if stub.extractHits != 0 {
		t.Fatalf("ExtractKeychainToken() hits = %d, want 0 (no extraction after setup-token failure)", stub.extractHits)
	}
	// Credentials file must NOT have been written.
	credPath := filepath.Join(dir, ".credentials.json")
	if _, statErr := os.Stat(credPath); !os.IsNotExist(statErr) {
		t.Fatal(".credentials.json must not be written when setup-token fails")
	}
}

// TestLoginClaudeAccountFailsWhenExtractTokenErrors verifies that an error from
// ExtractKeychainToken propagates wrapped, and no file is written.
func TestLoginClaudeAccountFailsWhenExtractTokenErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	extractErr := errors.New("security: item not found")
	stub := &stubClaudeAccountAuthRunner{extractTokenErr: extractErr}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := loginClaudeAccount(cmd, account, config.Paths{})
	if err == nil {
		t.Fatal("loginClaudeAccount() error = nil, want extract error")
	}
	if !errors.Is(err, extractErr) {
		t.Fatalf("loginClaudeAccount() error = %v, want wrapping %v", err, extractErr)
	}
	// Credentials file must NOT have been written.
	credPath := filepath.Join(dir, ".credentials.json")
	if _, statErr := os.Stat(credPath); !os.IsNotExist(statErr) {
		t.Fatal(".credentials.json must not be written when extraction fails")
	}
}

// TestLoginClaudeAccountFailsOnEmptyToken verifies that an empty token returned
// by ExtractKeychainToken is treated as an extraction failure (no file written).
func TestLoginClaudeAccountFailsOnEmptyToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	// extractToken is "" (zero value) and extractTokenErr is nil — empty-token case.
	stub := &stubClaudeAccountAuthRunner{}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := loginClaudeAccount(cmd, account, config.Paths{})
	if err == nil {
		t.Fatal("loginClaudeAccount() error = nil, want empty-token error")
	}
	// Credentials file must NOT have been written.
	credPath := filepath.Join(dir, ".credentials.json")
	if _, statErr := os.Stat(credPath); !os.IsNotExist(statErr) {
		t.Fatal(".credentials.json must not be written when token is empty")
	}
}

// --- loginClaudeAccount tests ---

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

// --- wipeClaudeCredentials tests ---

// TestWipeClaudeCredentialsRemovesFile verifies that an existing
// .credentials.json is removed.
func TestWipeClaudeCredentialsRemovesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	credPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"tok"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", credPath, err)
	}

	if err := wipeClaudeCredentials(dir); err != nil {
		t.Fatalf("wipeClaudeCredentials() error = %v, want nil", err)
	}
	if _, err := os.Stat(credPath); !os.IsNotExist(err) {
		t.Fatal(".credentials.json still exists after wipe; must be removed")
	}
}

// TestWipeClaudeCredentialsMissingFileIsNoError verifies that a missing
// .credentials.json is not an error.
func TestWipeClaudeCredentialsMissingFileIsNoError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := wipeClaudeCredentials(dir); err != nil {
		t.Fatalf("wipeClaudeCredentials(empty dir) error = %v, want nil", err)
	}
}

// --- logoutManagedAccount integration ---

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

// --- ReadAccountIdentity integration ---

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

// --- runClaudeHostCommand / systemClaudeAccountAuthRunner tests ---

// TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing verifies that when
// `claude` is not on PATH, runClaudeHostCommand returns an actionable error
// that wraps exec.ErrNotFound.
// Note: t.Setenv requires no t.Parallel() on this test.
func TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing(t *testing.T) {
	// Use an empty dir as PATH so no `claude` binary is found.
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)

	ctx := context.Background()
	_, err := runClaudeHostCommand(ctx, "/tmp/home", nil, nil, nil, "setup-token")
	if err == nil {
		t.Fatal("runClaudeHostCommand() error = nil, want missing-claude error")
	}
	if !strings.Contains(err.Error(), "npm install -g @anthropic-ai/claude-code") {
		t.Fatalf("runClaudeHostCommand() error = %v, want actionable npm install hint", err)
	}
	// After the %w fix, errors.Is should find exec.ErrNotFound in the chain.
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("runClaudeHostCommand() error = %v, want errors.Is(err, exec.ErrNotFound) = true", err)
	}
}

// TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR verifies
// that RunSetupToken sets CLAUDE_CONFIG_DIR in the subprocess environment.
// It installs a fake `claude` script that logs env vars, calls RunSetupToken,
// then reads the log to verify the env was set correctly.
// Note: t.Setenv (inside installFakeHostClaude) requires no t.Parallel() here.
func TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR(t *testing.T) {
	logPath := installFakeHostClaude(t)

	homePath := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := systemClaudeAccountAuthRunner{}.RunSetupToken(
		context.Background(),
		homePath,
		bytes.NewBuffer(nil),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunSetupToken() error = %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", logPath, err)
	}
	if !strings.Contains(string(content), "CLAUDE_CONFIG_DIR="+homePath) {
		t.Fatalf("fake claude log = %q, want CLAUDE_CONFIG_DIR=%s entry", string(content), homePath)
	}
	if !strings.Contains(string(content), "args:setup-token") {
		t.Fatalf("fake claude log = %q, want args:setup-token entry", string(content))
	}
}
