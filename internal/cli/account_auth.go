package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/evanmschultz/laslig"
	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

type codexAccountAuthRunner interface {
	LoginStatus(context.Context, string) (bool, error)
	Login(context.Context, string, io.Reader, io.Writer, io.Writer) error
	Logout(context.Context, string, io.Reader, io.Writer, io.Writer) error
}

type accountAuthOptions struct {
	SkipLogin bool
	Paths     config.Paths
}

type codexAccountAuthRunnerKey struct{}

var hostCodexAccountAuth codexAccountAuthRunner = systemCodexAccountAuthRunner{}

const valvTestSkipHostCodexLoginEnv = "VALV_TEST_SKIP_HOST_CODEX_LOGIN"

func ensureManagedAccountReady(cmd *cobra.Command, provider domain.Provider, account domain.Profile, options accountAuthOptions) error {
	switch provider {
	case domain.ProviderCodex:
		return ensureCodexAccountReady(cmd, account, options)
	case domain.ProviderClaude:
		return ensureClaudeAccountReady(cmd, account, options.Paths)
	default:
		return nil
	}
}

func logoutManagedAccount(cmd *cobra.Command, provider domain.Provider, account domain.Profile) error {
	switch provider {
	case domain.ProviderCodex:
		return logoutCodexAccount(cmd, account)
	case domain.ProviderClaude:
		return wipeClaudeCredentials(account.HomePath)
	default:
		return nil
	}
}

func loginManagedAccount(cmd *cobra.Command, provider domain.Provider, account domain.Profile, paths config.Paths) error {
	switch provider {
	case domain.ProviderCodex:
		return loginCodexAccount(cmd, account)
	case domain.ProviderClaude:
		return loginClaudeAccount(cmd, account, paths)
	default:
		return nil
	}
}

func ensureCodexAccountReady(cmd *cobra.Command, account domain.Profile, options accountAuthOptions) error {
	if options.SkipLogin {
		return nil
	}
	if shouldSkipHostCodexLoginCheck() {
		return nil
	}
	runner := codexAccountAuthFromContext(cmd.Context())
	loggedIn, err := runner.LoginStatus(cmd.Context(), account.HomePath)
	if err != nil {
		return fmt.Errorf("ensure account %q is ready: %w", account.Name, err)
	}
	if loggedIn {
		return nil
	}
	if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout()) {
		return fmt.Errorf("account %q is not logged in; rerun in a TTY to complete host Codex login or pass `--skip-login`", account.Name)
	}
	if err := writeCLINotice(
		cmd.ErrOrStderr(),
		laslig.NoticeInfoLevel,
		"Host Codex login needed",
		fmt.Sprintf("Complete host Codex login for account %q.", account.Name),
	); err != nil {
		return fmt.Errorf("announce host codex login: %w", err)
	}
	if err := runner.Login(cmd.Context(), account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("run host Codex login for account %q: %w", account.Name, err)
	}
	loggedIn, err = runner.LoginStatus(cmd.Context(), account.HomePath)
	if err != nil {
		return fmt.Errorf("verify host Codex login for account %q: %w", account.Name, err)
	}
	if !loggedIn {
		return fmt.Errorf("verify host Codex login for account %q: no authenticated session found", account.Name)
	}
	return nil
}

func logoutCodexAccount(cmd *cobra.Command, account domain.Profile) error {
	runner := codexAccountAuthFromContext(cmd.Context())
	if err := runner.Logout(cmd.Context(), account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("run host Codex logout for account %q: %w", account.Name, err)
	}
	return nil
}

func loginCodexAccount(cmd *cobra.Command, account domain.Profile) error {
	runner := codexAccountAuthFromContext(cmd.Context())
	if err := runner.Login(cmd.Context(), account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("run host Codex login for account %q: %w", account.Name, err)
	}
	return nil
}

func codexAccountAuthFromContext(ctx context.Context) codexAccountAuthRunner {
	if runner, ok := ctx.Value(codexAccountAuthRunnerKey{}).(codexAccountAuthRunner); ok && runner != nil {
		return runner
	}
	return hostCodexAccountAuth
}

type systemCodexAccountAuthRunner struct{}

func (systemCodexAccountAuthRunner) LoginStatus(ctx context.Context, homePath string) (bool, error) {
	output, err := runCodexHostCommand(ctx, homePath, nil, nil, nil, "login", "status")
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		lower := strings.ToLower(output)
		if strings.Contains(lower, "not logged in") || strings.Contains(lower, "login required") {
			return false, nil
		}
	}
	return false, fmt.Errorf("check host Codex login status: %w%s", err, formatCodexHostOutput(output))
}

func (systemCodexAccountAuthRunner) Login(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error {
	_, err := runCodexHostCommand(ctx, homePath, stdin, stdout, stderr, "login")
	if err != nil {
		return err
	}
	return nil
}

func (systemCodexAccountAuthRunner) Logout(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error {
	_, err := runCodexHostCommand(ctx, homePath, stdin, stdout, stderr, "logout")
	if err != nil {
		return err
	}
	return nil
}

func runCodexHostCommand(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer, args ...string) (string, error) {
	binary, err := exec.LookPath("codex")
	if err != nil {
		return "", fmt.Errorf("find host codex binary: %w", err)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = appendOrReplaceEnv(os.Environ(), "CODEX_HOME", homePath)
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

func appendOrReplaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				out = append(out, prefix+value)
				replaced = true
			}
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func formatCodexHostOutput(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	return ": " + trimmed
}

func shouldSkipHostCodexLoginCheck() bool {
	return strings.TrimSpace(os.Getenv(valvTestSkipHostCodexLoginEnv)) == "1"
}
