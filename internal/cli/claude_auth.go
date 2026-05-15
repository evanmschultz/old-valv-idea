package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanmschultz/laslig"
	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// claudeAuthRunner is the injectable interface for Claude auth operations.
// Implementations wrap the Docker image check and container execution so
// unit tests can stub both without a real Docker daemon.
type claudeAuthRunner interface {
	EnsureImage(cmd *cobra.Command, paths config.Paths) error
	RunContainer(ctx context.Context, request dockeradapter.ContainerRunRequest) error
}

type claudeAuthRunnerKey struct{}

// systemClaudeAuthRunner is the production implementation of claudeAuthRunner.
type systemClaudeAuthRunner struct {
	cmd *cobra.Command
}

func (r systemClaudeAuthRunner) EnsureImage(cmd *cobra.Command, paths config.Paths) error {
	return ensureClaudeImageCurrent(cmd, paths)
}

func (r systemClaudeAuthRunner) RunContainer(ctx context.Context, request dockeradapter.ContainerRunRequest) error {
	executor := dockeradapter.NewExecutor(
		dockeradapter.NewSystemRunner("docker", r.cmd.InOrStdin(), r.cmd.OutOrStdout(), r.cmd.ErrOrStderr()),
	)
	return executor.Run(ctx, request)
}

func claudeAuthRunnerFromContext(ctx context.Context, cmd *cobra.Command) claudeAuthRunner {
	if runner, ok := ctx.Value(claudeAuthRunnerKey{}).(claudeAuthRunner); ok && runner != nil {
		return runner
	}
	return systemClaudeAuthRunner{cmd: cmd}
}

// ensureClaudeAccountReady is the Claude-specific auth flow called from
// ensureManagedAccountReady. It wipes any pre-existing credentials,
// ensures the Claude image is built, launches a Claude container with the
// account home bind-mounted, and verifies credentials were written.
//
// A non-TTY guard is enforced: the device-code login flow requires a
// terminal. If no TTY is present the function returns an actionable error.
func ensureClaudeAccountReady(cmd *cobra.Command, account domain.Profile, paths config.Paths) error {
	if err := wipeClaudeCredentials(account.HomePath); err != nil {
		return fmt.Errorf("prepare claude account %q: wipe credentials: %w", account.Name, err)
	}
	if !commandHasTTY(cmd.InOrStdin()) {
		return fmt.Errorf(
			"account %q is not logged in; rerun in a TTY to complete Claude device-code login",
			account.Name,
		)
	}
	runner := claudeAuthRunnerFromContext(cmd.Context(), cmd)
	if err := runner.EnsureImage(cmd, paths); err != nil {
		return fmt.Errorf("ensure claude account %q: %w", account.Name, err)
	}
	if err := writeCLINotice(
		cmd.ErrOrStderr(),
		laslig.NoticeInfoLevel,
		"Claude login needed",
		fmt.Sprintf("Complete Claude device-code login for account %q.", account.Name),
	); err != nil {
		return fmt.Errorf("announce claude login: %w", err)
	}
	if err := runner.RunContainer(cmd.Context(), buildClaudeAuthContainerRequest(account)); err != nil {
		return fmt.Errorf("run claude login container for account %q: %w", account.Name, err)
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

// loginClaudeAccount performs the Claude device-code auth flow without the
// non-TTY guard. Used by loginManagedAccount for explicit re-login.
func loginClaudeAccount(cmd *cobra.Command, account domain.Profile, paths config.Paths) error {
	if err := wipeClaudeCredentials(account.HomePath); err != nil {
		return fmt.Errorf("prepare claude login for account %q: wipe credentials: %w", account.Name, err)
	}
	runner := claudeAuthRunnerFromContext(cmd.Context(), cmd)
	if err := runner.EnsureImage(cmd, paths); err != nil {
		return fmt.Errorf("ensure claude image for account %q: %w", account.Name, err)
	}
	if err := writeCLINotice(
		cmd.ErrOrStderr(),
		laslig.NoticeInfoLevel,
		"Claude login",
		fmt.Sprintf("Starting Claude device-code login for account %q.", account.Name),
	); err != nil {
		return fmt.Errorf("announce claude login: %w", err)
	}
	if err := runner.RunContainer(cmd.Context(), buildClaudeAuthContainerRequest(account)); err != nil {
		return fmt.Errorf("run claude login container for account %q: %w", account.Name, err)
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

// wipeClaudeCredentials removes .credentials.json from homePath if present.
// A missing file is not an error.
func wipeClaudeCredentials(homePath string) error {
	credPath := filepath.Join(strings.TrimSpace(homePath), ".credentials.json")
	if err := os.Remove(credPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %q: %w", credPath, err)
	}
	return nil
}

// buildClaudeAuthContainerRequest constructs the ContainerRunRequest for the
// Claude device-code login flow. The account home is bind-mounted to
// ContainerClaudeDir so credentials written by claude auth login persist on
// the host.
func buildClaudeAuthContainerRequest(account domain.Profile) dockeradapter.ContainerRunRequest {
	return dockeradapter.ContainerRunRequest{
		Name:  fmt.Sprintf("valv-claude-auth-%d", timeNowUnixNano()),
		Image: claudeImageRef(),
		Labels: map[string]string{
			"io.valv.managed":  "true",
			"io.valv.provider": "claude",
			"io.valv.scope":    "auth",
		},
		Env: map[string]string{
			"CLAUDE_CONFIG_DIR": claudeprovider.ContainerClaudeDir,
			"HOME":              claudeprovider.ContainerHomeDir,
			"LOGNAME":           "valv",
			"USER":              "valv",
		},
		Mounts: []dockeradapter.MountSpec{
			dockeradapter.NewMountSpec(account.HomePath, claudeprovider.ContainerClaudeDir, false),
		},
		User:        currentContainerUser(),
		Args:        []string{"auth", "login"},
		Interactive: true,
		TTY:         true,
		Init:        true,
		Remove:      true,
	}
}
