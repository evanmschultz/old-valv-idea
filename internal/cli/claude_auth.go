package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/evanmschultz/laslig"
	"github.com/spf13/cobra"

	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// claudeKeychainService is the macOS keychain service name that the Claude CLI
// uses when writing OAuth credentials. Verified on 2026-05-15: `claude auth login`
// writes the full-scope session JSON blob to this service.
const claudeKeychainService = "Claude Code-credentials"

// claudeAuthRunner is the injectable interface for Claude auth operations.
// Implementations wrap the host subprocess invocation so unit tests can stub
// both methods without a real Claude CLI or macOS keychain.
type claudeAuthRunner interface {
	RunAuthLogin(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error
	ExtractKeychainToken(ctx context.Context, macOSUser string) (string, error)
}

type claudeAuthRunnerKey struct{}

var hostClaudeAccountAuth claudeAuthRunner = systemClaudeAccountAuthRunner{}

// systemClaudeAccountAuthRunner is the production implementation of
// claudeAuthRunner. It shells out to the host `claude` and `security` CLIs.
type systemClaudeAccountAuthRunner struct{}

// RunAuthLogin runs `claude auth login` as a host subprocess with
// CLAUDE_CONFIG_DIR set to homePath so the CLI writes its .claude.json state
// into the managed account home. Stdio is streamed to the caller's terminal so
// the user sees the browser-open prompt and paste field. The auth login flow
// produces full-scope session credentials that container claude can use
// natively via .credentials.json.
func (systemClaudeAccountAuthRunner) RunAuthLogin(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error {
	_, err := runClaudeHostCommand(ctx, homePath, stdin, stdout, stderr, "auth", "login")
	return err
}

// ExtractKeychainToken extracts the Claude credentials JSON blob from the
// macOS keychain using `security find-generic-password`. The blob is written
// to the keychain by `claude auth login` under the service
// "Claude Code-credentials" with the macOS username as the account field.
// The returned string is the full JSON blob as stored by the Claude CLI —
// callers write it verbatim to .credentials.json. Returns an error if the
// entry is absent or the returned blob is empty.
func (systemClaudeAccountAuthRunner) ExtractKeychainToken(ctx context.Context, macOSUser string) (string, error) {
	cmd := exec.CommandContext(
		ctx, "security", "find-generic-password",
		"-s", claudeKeychainService,
		"-a", macOSUser,
		"-w",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", fmt.Errorf("extract claude keychain token: %w: %s", err, detail)
		}
		return "", fmt.Errorf("extract claude keychain token: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("extract claude keychain token: empty token returned by security command")
	}
	return token, nil
}

func claudeAuthRunnerFromContext(ctx context.Context) claudeAuthRunner {
	if runner, ok := ctx.Value(claudeAuthRunnerKey{}).(claudeAuthRunner); ok && runner != nil {
		return runner
	}
	return hostClaudeAccountAuth
}

// ensureClaudeAccountReady is the Claude-specific auth flow called from
// ensureManagedAccountReady. It runs `claude auth login` as a host subprocess,
// extracts the resulting credentials JSON blob from the macOS keychain, and
// writes it verbatim to .credentials.json in the managed account home.
//
// SkipLogin short-circuits before any credential mutation.
//
// If .credentials.json already exists and is non-empty, the account is
// considered already authenticated and the function returns nil immediately
// without running auth login. This is the normal path for account switch when
// the account has previously completed auth.
//
// A non-TTY guard is enforced when auth is actually needed: the auth login
// flow requires a terminal for browser-open and paste-prompt interaction.
func ensureClaudeAccountReady(cmd *cobra.Command, account domain.Profile, options accountAuthOptions) error {
	if options.SkipLogin {
		return nil
	}
	credPath := filepath.Join(strings.TrimSpace(account.HomePath), ".credentials.json")
	info, statErr := os.Stat(credPath)
	if statErr == nil && info.Size() > 0 {
		return nil
	}
	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("check claude credentials for account %q: %w", account.Name, statErr)
	}
	if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout()) {
		return fmt.Errorf(
			"account %q is not logged in; rerun in a TTY to complete Claude auth login",
			account.Name,
		)
	}
	if err := writeCLINotice(
		cmd.ErrOrStderr(),
		laslig.NoticeInfoLevel,
		"Claude login needed",
		fmt.Sprintf("Complete Claude auth login for account %q.", account.Name),
	); err != nil {
		return fmt.Errorf("announce claude login: %w", err)
	}
	runner := claudeAuthRunnerFromContext(cmd.Context())
	if err := runner.RunAuthLogin(cmd.Context(), account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("run claude auth login for account %q: %w", account.Name, err)
	}
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("resolve macOS user for keychain extraction: %w", err)
	}
	token, err := runner.ExtractKeychainToken(cmd.Context(), u.Username)
	if err != nil {
		return fmt.Errorf("extract claude token for account %q: %w", account.Name, err)
	}
	if token == "" {
		return fmt.Errorf("extract claude token for account %q: keychain returned empty token", account.Name)
	}
	if err := writeClaudeCredentials(account.HomePath, token); err != nil {
		return fmt.Errorf("write claude credentials for account %q: %w", account.Name, err)
	}
	identity, err := claudeprovider.ReadAccountIdentity(account.HomePath)
	if err != nil {
		return fmt.Errorf("verify claude login for account %q: %w", account.Name, err)
	}
	if !identity.LoggedIn {
		return fmt.Errorf("verify claude login for account %q: no credentials file found after login", account.Name)
	}
	return nil
}

// loginClaudeAccount performs the Claude auth login flow without the
// non-TTY guard. Used by loginManagedAccount for explicit re-login.
func loginClaudeAccount(cmd *cobra.Command, account domain.Profile, _ config.Paths) error {
	if err := wipeClaudeCredentials(account.HomePath); err != nil {
		return fmt.Errorf("prepare claude login for account %q: wipe credentials: %w", account.Name, err)
	}
	if err := writeCLINotice(
		cmd.ErrOrStderr(),
		laslig.NoticeInfoLevel,
		"Claude login",
		fmt.Sprintf("Starting Claude auth login for account %q.", account.Name),
	); err != nil {
		return fmt.Errorf("announce claude login: %w", err)
	}
	runner := claudeAuthRunnerFromContext(cmd.Context())
	if err := runner.RunAuthLogin(cmd.Context(), account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("run claude auth login for account %q: %w", account.Name, err)
	}
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("resolve macOS user for keychain extraction: %w", err)
	}
	token, err := runner.ExtractKeychainToken(cmd.Context(), u.Username)
	if err != nil {
		return fmt.Errorf("extract claude token for account %q: %w", account.Name, err)
	}
	if token == "" {
		return fmt.Errorf("extract claude token for account %q: keychain returned empty token", account.Name)
	}
	if err := writeClaudeCredentials(account.HomePath, token); err != nil {
		return fmt.Errorf("write claude credentials for account %q: %w", account.Name, err)
	}
	identity, err := claudeprovider.ReadAccountIdentity(account.HomePath)
	if err != nil {
		return fmt.Errorf("verify claude login for account %q: %w", account.Name, err)
	}
	if !identity.LoggedIn {
		return fmt.Errorf("verify claude login for account %q: no credentials file found after login", account.Name)
	}
	return nil
}

// writeClaudeCredentials writes the credentials JSON blob to .credentials.json
// in homePath. The blob is the raw value returned by ExtractKeychainToken —
// the macOS keychain stores it in the exact format container claude reads from
// .credentials.json on Linux, so it is written verbatim without re-encoding.
// The file is created with mode 0o600.
func writeClaudeCredentials(homePath, credentialsBlob string) error {
	credPath := filepath.Join(strings.TrimSpace(homePath), ".credentials.json")
	if err := os.WriteFile(credPath, []byte(credentialsBlob), 0o600); err != nil {
		return fmt.Errorf("write %q: %w", credPath, err)
	}
	return nil
}

// wipeClaudeCredentials removes .credentials.json from homePath if present.
// A missing file is not an error.
func wipeClaudeCredentials(homePath string) error {
	credPath := filepath.Join(strings.TrimSpace(homePath), ".credentials.json")
	if err := os.Remove(credPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %q: %w", credPath, err)
	}
	return nil
}

// runClaudeHostCommand runs the host `claude` binary with the given args,
// setting CLAUDE_CONFIG_DIR to homePath so state files land in the managed
// account home. If stdout is non-nil the command streams to the provided
// writers; otherwise combined output is captured and returned as a string (for
// callers that inspect output programmatically).
//
// An inline exec.LookPath check ensures a clear error message when `claude` is
// not found on PATH, matching the pattern used by runCodexHostCommand.
func runClaudeHostCommand(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer, args ...string) (string, error) {
	binary, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf(
			"claude CLI not found on PATH; install with: npm install -g @anthropic-ai/claude-code@2.1.89: %w",
			err,
		)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = appendOrReplaceEnv(os.Environ(), "CLAUDE_CONFIG_DIR", homePath)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if stdout != nil {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return "", cmd.Run()
	}
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer
	err = cmd.Run()
	return buffer.String(), err
}
