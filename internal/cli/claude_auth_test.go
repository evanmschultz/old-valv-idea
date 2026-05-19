package cli

// claude_auth_test.go — Unit 7.5 full rewrite (Path B in-container auth).
// R3: auto-open machinery stripped per dev correction 2026-05-16.
// The 16 surviving tests cover: ensureClaudeAccountReady, loginClaudeAccount,
// wipeClaudeCredentials, logoutManagedAccount integration, ReadAccountIdentity
// integration, and systemClaudeAccountAuthRunner behaviour.

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

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// stubClaudeAccountAuthRunner is the test double for claudeAuthRunner.
// runHits tracks call count, lastHomePath records the most recent homePath
// argument, runErr is returned by RunInContainer, and stubRunFunc is called
// after recording hits — tests use it to simulate the container writing
// .credentials.json.
type stubClaudeAccountAuthRunner struct {
	runErr       error
	runHits      int
	lastHomePath string
	stubRunFunc  func(homePath string)
}

func (s *stubClaudeAccountAuthRunner) RunInContainer(_ context.Context, homePath string, _ io.Reader, _, _ io.Writer) error {
	s.runHits++
	s.lastHomePath = homePath
	if s.stubRunFunc != nil {
		s.stubRunFunc(homePath)
	}
	return s.runErr
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

// writeCredsToDir writes a minimal .credentials.json to dir without needing a
// testing.T (used from stubRunFunc callbacks).
func writeCredsToDir(dir string) {
	credPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiOauth":{"accessToken":"tok"}}`), 0o600); err != nil {
		panic("writeCredsToDir: " + err.Error())
	}
}

// stubAuthContainerExecutor captures ContainerRunRequest for assertions.
type stubAuthContainerExecutor struct {
	lastRequest dockeradapter.ContainerRunRequest
	err         error
}

func (s *stubAuthContainerExecutor) Run(_ context.Context, req dockeradapter.ContainerRunRequest) error {
	s.lastRequest = req
	return s.err
}

// --- ensureClaudeAccountReady tests ---

// TestEnsureClaudeAccountReadyRejectsNonTTY verifies that a non-TTY cmd with
// no existing credentials returns an error mentioning "TTY" without invoking
// RunInContainer.
func TestEnsureClaudeAccountReadyRejectsNonTTY(t *testing.T) {
	t.Parallel()

	cmd := newTestClaudeCmd()
	stub := &stubClaudeAccountAuthRunner{}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: t.TempDir()}
	err := ensureClaudeAccountReady(cmd, account, accountAuthOptions{}, "")
	if err == nil {
		t.Fatal("ensureClaudeAccountReady() error = nil, want non-tty error")
	}
	if !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("ensureClaudeAccountReady() error = %v, want TTY mention", err)
	}
	if stub.runHits != 0 {
		t.Fatalf("RunInContainer() hits = %d, want 0 (should reject before container launch)", stub.runHits)
	}
}

// TestEnsureClaudeAccountReadyRespectsSkipLogin verifies that when SkipLogin
// is true the function returns nil immediately without launching a container.
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
	err := ensureClaudeAccountReady(cmd, account, accountAuthOptions{SkipLogin: true}, "")
	if err != nil {
		t.Fatalf("ensureClaudeAccountReady(SkipLogin=true) error = %v, want nil", err)
	}
	if stub.runHits != 0 {
		t.Fatalf("RunInContainer() hits = %d, want 0 (SkipLogin must short-circuit)", stub.runHits)
	}
	// Credentials file must NOT be wiped.
	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		t.Fatal("credentials file was removed with SkipLogin=true; must be preserved")
	}
}

// TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY verifies that
// when .credentials.json already exists and is non-empty, ensureClaudeAccountReady
// returns nil immediately — even in a non-TTY context — without invoking
// RunInContainer.
func TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY(t *testing.T) {
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
	err := ensureClaudeAccountReady(cmd, account, accountAuthOptions{}, "")
	if err != nil {
		t.Fatalf("ensureClaudeAccountReady() error = %v, want nil (already authed)", err)
	}
	if stub.runHits != 0 {
		t.Fatalf("RunInContainer() hits = %d, want 0 (already-authed must short-circuit before runner)", stub.runHits)
	}
	if _, statErr := os.Stat(credPath); os.IsNotExist(statErr) {
		t.Fatal("credentials file was removed; must be preserved when already authed")
	}
}

// TestEnsureClaudeAccountReadyFailsWhenContainerRunFails verifies that an
// error from RunInContainer propagates and runHits is 1. This test requires a
// TTY — since we can't get a real TTY in unit tests, we exercise this via
// loginClaudeAccount (no TTY guard) to confirm container-run error propagation.
// The ensure path TTY guard is already covered by TestEnsureClaudeAccountReadyRejectsNonTTY.
func TestEnsureClaudeAccountReadyFailsWhenContainerRunFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	containerErr := errors.New("container: docker daemon not running")
	stub := &stubClaudeAccountAuthRunner{runErr: containerErr}
	installStubClaudeAuth(t, cmd, stub)

	// loginClaudeAccount has no TTY guard so we can exercise the container-run
	// failure path from a non-TTY test.
	account := domain.Profile{Name: "personal", HomePath: dir}
	err := loginClaudeAccount(cmd, account, config.Paths{})
	if err == nil {
		t.Fatal("loginClaudeAccount() error = nil, want container-run error")
	}
	if !errors.Is(err, containerErr) {
		t.Fatalf("loginClaudeAccount() error = %v, want wrapping %v", err, containerErr)
	}
	if stub.runHits != 1 {
		t.Fatalf("RunInContainer() hits = %d, want 1", stub.runHits)
	}
}

// TestEnsureClaudeAccountReadyFailsWhenNotLoggedInAfterContainer verifies that
// when RunInContainer returns nil but the container did not write
// .credentials.json (stub does nothing), ReadAccountIdentity returns
// LoggedIn=false and the function returns an error.
func TestEnsureClaudeAccountReadyFailsWhenNotLoggedInAfterContainer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	// Stub returns nil but does NOT write .credentials.json (stubRunFunc is nil).
	stub := &stubClaudeAccountAuthRunner{}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	// loginClaudeAccount bypasses TTY guard; same container+identity-check path.
	err := loginClaudeAccount(cmd, account, config.Paths{})
	if err == nil {
		t.Fatal("loginClaudeAccount() error = nil, want not-logged-in error")
	}
	if stub.runHits != 1 {
		t.Fatalf("RunInContainer() hits = %d, want 1", stub.runHits)
	}
}

// TestEnsureClaudeAccountReadySucceeds verifies the full success path:
// stub writes .credentials.json via stubRunFunc, ReadAccountIdentity returns
// LoggedIn=true, function returns nil. Exercises loginClaudeAccount (no TTY
// guard) to run the complete container+identity-check sequence.
func TestEnsureClaudeAccountReadySucceeds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	stub := &stubClaudeAccountAuthRunner{
		stubRunFunc: func(homePath string) {
			// Simulate container writing .credentials.json natively.
			credPath := filepath.Join(homePath, ".credentials.json")
			if err := os.WriteFile(credPath, []byte(`{"claudeAiOauth":{"accessToken":"container-tok"}}`), 0o600); err != nil {
				panic("stubRunFunc: WriteFile: " + err.Error())
			}
		},
	}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	if err := loginClaudeAccount(cmd, account, config.Paths{}); err != nil {
		t.Fatalf("loginClaudeAccount() error = %v, want nil", err)
	}
	if stub.runHits != 1 {
		t.Fatalf("RunInContainer() hits = %d, want 1", stub.runHits)
	}
	if stub.lastHomePath != dir {
		t.Fatalf("RunInContainer() lastHomePath = %q, want %q", stub.lastHomePath, dir)
	}
	identity, err := claudeprovider.ReadAccountIdentity(dir)
	if err != nil {
		t.Fatalf("ReadAccountIdentity() error = %v", err)
	}
	if !identity.LoggedIn {
		t.Fatal("ReadAccountIdentity() LoggedIn = false, want true after successful container auth")
	}
}

// --- loginClaudeAccount tests ---

// TestLoginClaudeAccountSkipsNonTTYGuard verifies loginClaudeAccount has no
// non-TTY guard: RunInContainer is reached even with a non-TTY cmd. The stub
// returns an error to terminate early without real container access — but the
// hit count proves no TTY guard blocked it.
func TestLoginClaudeAccountSkipsNonTTYGuard(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	containerErr := errors.New("stub: container error used to terminate early")
	stub := &stubClaudeAccountAuthRunner{runErr: containerErr}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := loginClaudeAccount(cmd, account, config.Paths{})
	if err == nil {
		t.Fatal("loginClaudeAccount() error = nil, want container error (no TTY guard)")
	}
	if strings.Contains(err.Error(), "TTY") {
		t.Fatalf("loginClaudeAccount() returned TTY guard error = %v; loginClaudeAccount must not have a TTY guard", err)
	}
	if stub.runHits != 1 {
		t.Fatalf("RunInContainer() hits = %d, want 1 (no TTY guard should block it)", stub.runHits)
	}
}

// TestLoginClaudeAccountFailsWhenContainerRunFails verifies that a container
// run error propagates wrapped with errors.Is.
func TestLoginClaudeAccountFailsWhenContainerRunFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	containerErr := errors.New("container: daemon error")
	stub := &stubClaudeAccountAuthRunner{runErr: containerErr}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "personal", HomePath: dir}
	err := loginClaudeAccount(cmd, account, config.Paths{})
	if err == nil {
		t.Fatal("loginClaudeAccount() error = nil, want container error")
	}
	if !errors.Is(err, containerErr) {
		t.Fatalf("loginClaudeAccount() error = %v, want wrapping %v", err, containerErr)
	}
}

// TestLoginClaudeAccountSucceeds verifies full success: stub writes
// .credentials.json, ReadAccountIdentity returns LoggedIn=true, nil returned.
func TestLoginClaudeAccountSucceeds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := newTestClaudeCmd()
	stub := &stubClaudeAccountAuthRunner{
		stubRunFunc: func(homePath string) {
			writeCredsToDir(homePath)
		},
	}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "work", HomePath: dir}
	if err := loginClaudeAccount(cmd, account, config.Paths{}); err != nil {
		t.Fatalf("loginClaudeAccount() error = %v, want nil", err)
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

// --- systemClaudeAccountAuthRunner unit tests ---

// TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef verifies that the
// production runner respects the VALV_CLAUDE_IMAGE override — the same env var
// that the launch path honours — rather than a hardcoded ref.
func TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "test/myimg:v2")

	runner := systemClaudeAccountAuthRunner{
		image: claudeImageRef(), // production construction pattern
	}

	want := "test/myimg:v2"
	if got := runner.image.String(); got != want {
		t.Fatalf("runner.image = %q, want %q (claudeImageRef must honour VALV_CLAUDE_IMAGE)", got, want)
	}
}

// TestSystemClaudeAccountAuthRunnerPassesThroughTerminalEnv verifies that
// RunInContainer populates ContainerRunRequest.EnvPassthrough with the
// terminal locale variables that launch-path PrepareRuntime also passes through.
// Without these, the claude TUI prompt may render incorrectly through the Docker
// pty — the same class of failure that affected DROP_6.2.
func TestSystemClaudeAccountAuthRunnerPassesThroughTerminalEnv(t *testing.T) {
	// Set at least one env var that terminalEnvPassthrough tracks.
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_CTYPE", "en_US.UTF-8")

	stub := &stubAuthContainerExecutor{}
	runner := systemClaudeAccountAuthRunner{
		executor: stub,
		image:    claudeImageRef(),
	}

	homePath := t.TempDir()
	if err := runner.RunInContainer(
		context.Background(),
		homePath,
		nil, // stdin nil → Interactive=false, TTY=false
		&bytes.Buffer{},
		&bytes.Buffer{},
	); err != nil {
		t.Fatalf("RunInContainer() error = %v, want nil", err)
	}

	passthrough := stub.lastRequest.EnvPassthrough
	if len(passthrough) == 0 {
		t.Fatal("ContainerRunRequest.EnvPassthrough is empty; expected at least LANG/LC_CTYPE")
	}

	want := map[string]bool{"LANG": true, "LC_CTYPE": true}
	for _, name := range passthrough {
		delete(want, name)
	}
	if len(want) > 0 {
		remaining := make([]string, 0, len(want))
		for k := range want {
			remaining = append(remaining, k)
		}
		t.Fatalf("ContainerRunRequest.EnvPassthrough missing expected vars %v; got %v", remaining, passthrough)
	}
}

// --- Unit 7.11 Round 2 tests (preserved through R3) ---

// TestLoginClaudeAccountWipesExistingCredsBeforeRunning verifies AC1-R2:
// loginClaudeAccount wipes a pre-existing .credentials.json before invoking the
// auth runner, preventing stale credentials from short-circuiting re-auth.
// The key assertion: at the moment RunInContainer is called, the credentials
// file must NOT exist (wipe must have already run).
func TestLoginClaudeAccountWipesExistingCredsBeforeRunning(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	credPath := filepath.Join(dir, ".credentials.json")

	// Pre-write stale credentials.
	if err := os.WriteFile(credPath, []byte(`"stale"`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) stale creds error = %v", credPath, err)
	}

	var fileExistedAtRunTime bool
	cmd := newTestClaudeCmd()
	stub := &stubClaudeAccountAuthRunner{
		stubRunFunc: func(homePath string) {
			// Record whether the creds file exists when the runner is invoked.
			// If wipe ran first, the file must NOT exist at this point.
			_, statErr := os.Stat(filepath.Join(homePath, ".credentials.json"))
			fileExistedAtRunTime = !os.IsNotExist(statErr)
			// Simulate the container writing fresh credentials after wipe.
			writeCredsToDir(homePath)
		},
	}
	installStubClaudeAuth(t, cmd, stub)

	account := domain.Profile{Name: "hylla", HomePath: dir}
	if err := loginClaudeAccount(cmd, account, config.Paths{}); err != nil {
		t.Fatalf("loginClaudeAccount() error = %v, want nil", err)
	}

	// Stub must have been called.
	if stub.runHits != 1 {
		t.Fatalf("RunInContainer() hits = %d, want 1", stub.runHits)
	}

	// At the moment the runner was invoked, the file must NOT have existed —
	// proving that wipeClaudeCredentials ran BEFORE RunInContainer was called.
	if fileExistedAtRunTime {
		t.Fatal("credentials file existed when RunInContainer was called; wipe must run before runner invocation")
	}
}
