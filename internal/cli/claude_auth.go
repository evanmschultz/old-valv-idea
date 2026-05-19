package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanmschultz/laslig"
	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// authContainerExecutor is the local interface over docker.Executor.Run so that
// tests can stub the container without a real Docker daemon.
type authContainerExecutor interface {
	Run(context.Context, dockeradapter.ContainerRunRequest) error
}

// claudeAuthRunner is the injectable interface for Claude auth operations.
// The single method spins up the managed Claude container with homePath
// bind-mounted to /home/valv/.claude, runs plain `claude` (no subcommand),
// and waits for the container to exit. The container writes .credentials.json
// natively to the bind-mounted directory.
type claudeAuthRunner interface {
	RunInContainer(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error
}

type claudeAuthRunnerKey struct{}

// hostClaudeAccountAuth is the package-level default runner. It stores only
// the image ref; the executor is constructed per-call in RunInContainer so
// that the caller's stdin/stdout/stderr are wired correctly.
//
// claudeImageRef() is used here — not a hardcoded ref — so that VALV_CLAUDE_IMAGE
// overrides apply symmetrically to both the launch path and the auth path.
// Using different images for auth and launch risks credential-format mismatch
// across CLI versions.
var hostClaudeAccountAuth claudeAuthRunner = systemClaudeAccountAuthRunner{
	image: claudeImageRef(),
}

// systemClaudeAccountAuthRunner is the production implementation of
// claudeAuthRunner. It launches the valv-claude:dev container with the
// managed account home bind-mounted and runs plain `claude` (no subcommand)
// so the Claude CLI auto-prompts for device-code OAuth. The user completes
// OAuth in the browser and presses Ctrl-C twice to exit after auth is done.
type systemClaudeAccountAuthRunner struct {
	// executor is nil in production; non-nil when injected by tests.
	executor authContainerExecutor
	image    dockeradapter.ImageRef
}

// RunInContainer starts the Claude container with homePath bind-mounted to
// /home/valv/.claude. Plain `claude` (no subcommand) auto-prompts for OAuth.
// Returns the container exit error directly; no post-detection logic.
func (r systemClaudeAccountAuthRunner) RunInContainer(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error {
	containerExec := r.executor
	termValue := strings.TrimSpace(os.Getenv("TERM"))
	if termValue == "" {
		termValue = "xterm-256color"
	}
	containerName := fmt.Sprintf("valv-claude-auth-%d", time.Now().UTC().UnixNano())
	if containerExec == nil {
		containerExec = dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", stdin, stdout, stderr))
	}

	request := dockeradapter.ContainerRunRequest{
		Name:  containerName,
		Image: r.image,
		Env: map[string]string{
			"CLAUDE_CONFIG_DIR": claudeprovider.ContainerClaudeDir,
			"HOME":              claudeprovider.ContainerHomeDir,
			"LOGNAME":           "valv",
			"TERM":              termValue,
			"USER":              "valv",
		},
		// EnvPassthrough forwards terminal-locale vars (LANG, LC_CTYPE,
		// COLORTERM, TERM_PROGRAM, TERM_PROGRAM_VERSION) to the auth container,
		// matching the launch-path behaviour from PrepareRuntime. Without these,
		// claude's TUI OAuth prompt may render incorrectly through the Docker pty.
		EnvPassthrough: claudeprovider.TerminalEnvPassthrough(),
		Mounts: []dockeradapter.MountSpec{
			dockeradapter.NewMountSpec(homePath, claudeprovider.ContainerClaudeDir, false),
		},
		Args:        []string{},
		Interactive: stdin != nil,
		TTY:         commandHasTTY(stdin),
		Init:        true,
		Remove:      true,
		User:        currentContainerUser(),
	}

	return containerExec.Run(ctx, request)
}

func claudeAuthRunnerFromContext(ctx context.Context) claudeAuthRunner {
	if runner, ok := ctx.Value(claudeAuthRunnerKey{}).(claudeAuthRunner); ok && runner != nil {
		return runner
	}
	return hostClaudeAccountAuth
}

// ensureClaudeAccountReady is the Claude-specific auth flow called from
// ensureManagedAccountReady and (when --account is supplied) from
// runClaudeCommand. When accountOverride is non-empty, the named profile is
// resolved via ProfileByName and the resolved profile replaces account for the
// auth flow below. When accountOverride is empty, account is used as-is.
//
// Step order:
//  1. accountOverride non-empty → resolve profile by name; replace account.
//  2. SkipLogin → return nil immediately.
//  3. .credentials.json exists and non-empty → already authed, return nil.
//  4. Non-TTY guard → return error mentioning "TTY".
//  5. writeCLINotice to announce login.
//  6. RunInContainer → if error → return.
//  7. ReadAccountIdentity → if not LoggedIn → return error.
func ensureClaudeAccountReady(cmd *cobra.Command, account domain.Profile, options accountAuthOptions, accountOverride string) error {
	if accountOverride != "" {
		service, closeStore, err := openManageService(cmd, options.Paths)
		if err != nil {
			return fmt.Errorf("initialize manage service for account override: %w", err)
		}
		defer closeStore()
		resolved, err := service.ProfileByName(cmd.Context(), domain.ProviderClaude, accountOverride)
		if err != nil {
			return fmt.Errorf("resolve override account %q: %w", accountOverride, err)
		}
		account = resolved
	}
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
	if !commandHasTTY(cmd.InOrStdin()) {
		return fmt.Errorf(
			"account %q is not logged in; rerun in a TTY to complete Claude auth",
			account.Name,
		)
	}
	if err := writeCLINotice(
		cmd.ErrOrStderr(),
		laslig.NoticeInfoLevel,
		"Claude login needed",
		fmt.Sprintf("Complete Claude login for account %q.", account.Name),
	); err != nil {
		return fmt.Errorf("announce claude login: %w", err)
	}
	runner := claudeAuthRunnerFromContext(cmd.Context())
	if runErr := runner.RunInContainer(cmd.Context(), account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); runErr != nil {
		// Ctrl-C × 2 causes a non-zero exit but credentials may have landed.
		// Check whether .credentials.json is present and non-empty before
		// treating the container error as fatal.
		if info, statErr := os.Stat(credPath); statErr == nil && info.Size() > 0 {
			if logger := LoggerFromContext(cmd.Context()); logger != nil {
				logger.Debug("container exited non-zero but credentials present; treating as success", "error", runErr)
			}
			// Fall through to ReadAccountIdentity.
		} else {
			return fmt.Errorf("run claude auth container for account %q: %w", account.Name, runErr)
		}
	}
	identity, err := claudeprovider.ReadAccountIdentity(account.HomePath)
	if err != nil {
		return fmt.Errorf("verify claude login for account %q: %w", account.Name, err)
	}
	if !identity.LoggedIn {
		return fmt.Errorf("verify claude login for account %q: no credentials file found after container auth", account.Name)
	}
	return nil
}

// loginClaudeAccount performs the Claude container auth flow without the
// non-TTY guard. Used by loginManagedAccount for explicit re-login.
//
// Step order:
//  1. wipeClaudeCredentials to force fresh auth (prevents stale creds from
//     being detected as already-authed on the next ensureClaudeAccountReady call).
//  2. writeCLINotice to announce login.
//  3. RunInContainer → if error → return.
//  4. ReadAccountIdentity → if not LoggedIn → return error.
func loginClaudeAccount(cmd *cobra.Command, account domain.Profile, _ config.Paths) error {
	if err := wipeClaudeCredentials(account.HomePath); err != nil {
		return err
	}
	if err := writeCLINotice(
		cmd.ErrOrStderr(),
		laslig.NoticeInfoLevel,
		"Claude login",
		fmt.Sprintf("Starting Claude login for account %q.", account.Name),
	); err != nil {
		return fmt.Errorf("announce claude login: %w", err)
	}
	credPath := filepath.Join(strings.TrimSpace(account.HomePath), ".credentials.json")
	runner := claudeAuthRunnerFromContext(cmd.Context())
	if runErr := runner.RunInContainer(cmd.Context(), account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); runErr != nil {
		// Ctrl-C × 2 causes a non-zero exit but credentials may have landed.
		// Check whether .credentials.json is present and non-empty before
		// treating the container error as fatal.
		if info, statErr := os.Stat(credPath); statErr == nil && info.Size() > 0 {
			if logger := LoggerFromContext(cmd.Context()); logger != nil {
				logger.Debug("container exited non-zero but credentials present; treating as success", "error", runErr)
			}
			// Fall through to ReadAccountIdentity.
		} else {
			return fmt.Errorf("run claude auth container for account %q: %w", account.Name, runErr)
		}
	}
	identity, err := claudeprovider.ReadAccountIdentity(account.HomePath)
	if err != nil {
		return fmt.Errorf("verify claude login for account %q: %w", account.Name, err)
	}
	if !identity.LoggedIn {
		return fmt.Errorf("verify claude login for account %q: no credentials file found after container auth", account.Name)
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
