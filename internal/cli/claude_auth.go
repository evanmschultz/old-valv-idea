package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evanmschultz/laslig"
	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// oauthURLRegex matches the OAuth authorization URLs emitted by the Claude CLI
// on both the subscription path and the Console path. \S* captures the rest of
// the non-whitespace URL token (query params, fragments) without consuming
// newlines or trailing whitespace.
var oauthURLRegex = regexp.MustCompile(
	`https://(?:claude\.com/cai|platform\.claude\.com)/oauth/authorize\S*`,
)

// authContainerExecutor is the local interface over docker.Executor.Run so that
// tests can stub the container without a real Docker daemon.
type authContainerExecutor interface {
	Run(context.Context, dockeradapter.ContainerRunRequest) error
}

// urlOpener is the interface for opening the OAuth URL in the host browser.
// The production implementation calls exec.Command("open", url).Start() (macOS).
// Tests inject a stub.
type urlOpener interface {
	Open(ctx context.Context, url string) error
}

// credsWatcher is the interface for polling the credentials file and reporting
// when it appears. The production implementation uses a 500ms ticker loop.
// Tests inject a stub.
type credsWatcher interface {
	WaitForCreds(ctx context.Context, path string) error
}

// defaultURLOpener is the production urlOpener. It launches the URL in the
// host's default browser using the macOS "open" command.
//
// TODO: add cross-platform support — xdg-open (Linux), cmd /c start (Windows).
type defaultURLOpener struct{}

func (defaultURLOpener) Open(_ context.Context, url string) error {
	// Non-blocking: Start delegates to the OS browser handler and returns
	// immediately. Errors are swallowed — the Claude CLI prints the URL as a
	// fallback, so the user always has manual recourse.
	_ = exec.Command("open", url).Start()
	return nil
}

// defaultCredsWatcher is the production credsWatcher. It polls the credentials
// file every 500ms until it appears (non-empty) or the context is cancelled.
type defaultCredsWatcher struct{}

func (defaultCredsWatcher) WaitForCreds(ctx context.Context, path string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			info, err := os.Stat(path)
			if err == nil && info.Size() > 0 {
				return nil
			}
		}
	}
}

// lineScanner is a scanning io.Writer that buffers incoming bytes and invokes
// onMatch for each complete line that matches oauthURLRegex. All bytes are also
// forwarded to the inner writer unchanged.
//
// Buffer semantics: bytes accumulate until a '\n' is encountered; each complete
// line is forwarded and scanned atomically. A trailing partial line (no '\n'
// yet) is forwarded on the next Write that completes it, so the user's terminal
// always receives all bytes in order.
type lineScanner struct {
	inner   io.Writer
	buf     []byte
	onMatch func(string)
}

func newLineScanner(inner io.Writer, onMatch func(string)) *lineScanner {
	return &lineScanner{inner: inner, onMatch: onMatch}
}

// Write appends p to the buffer, processes complete lines (splitting on '\n'),
// forwards them to the inner writer, and calls onMatch for any line that
// matches oauthURLRegex.
func (s *lineScanner) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	for {
		idx := strings.IndexByte(string(s.buf), '\n')
		if idx < 0 {
			break
		}
		// line includes the terminating '\n'.
		line := s.buf[:idx+1]
		s.buf = s.buf[idx+1:]
		if _, err := s.inner.Write(line); err != nil {
			return 0, err
		}
		if m := oauthURLRegex.FindString(string(line)); m != "" {
			s.onMatch(m)
		}
	}
	return len(p), nil
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
// so the Claude CLI auto-prompts for device-code OAuth.
type systemClaudeAccountAuthRunner struct {
	// executor is nil in production; non-nil when injected by tests.
	executor authContainerExecutor
	image    dockeradapter.ImageRef
	// urlOpener opens the OAuth URL in the host browser. Nil uses defaultURLOpener.
	urlOpener urlOpener
	// credsWatcher polls for the credentials file. Nil uses defaultCredsWatcher.
	credsWatcher credsWatcher
}

// RunInContainer starts the Claude container, scans container output for the
// OAuth URL to auto-open it in the host browser, polls for credentials, and
// SIGTERMs the container once credentials are written. Returns nil when
// credentials are successfully detected; propagates the container exit error
// otherwise.
func (r systemClaudeAccountAuthRunner) RunInContainer(parentCtx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	opener := r.urlOpener
	if opener == nil {
		opener = defaultURLOpener{}
	}
	watcher := r.credsWatcher
	if watcher == nil {
		watcher = defaultCredsWatcher{}
	}

	containerExec := r.executor
	termValue := strings.TrimSpace(os.Getenv("TERM"))
	if termValue == "" {
		termValue = "xterm-256color"
	}
	containerName := fmt.Sprintf("valv-claude-auth-%d", time.Now().UTC().UnixNano())
	if containerExec == nil {
		// Construct lineScanner-wrapped writers before building the SystemRunner.
		// Both stdout and stderr are wrapped with the same sync.Once-guarded URL
		// scanner — the OAuth URL may appear on either stream depending on TTY mode.
		var once sync.Once
		onURL := func(url string) {
			once.Do(func() {
				_ = opener.Open(ctx, url)
			})
		}
		stdoutScanner := newLineScanner(stdout, onURL)
		stderrScanner := newLineScanner(stderr, onURL)
		containerExec = dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", stdin, stdoutScanner, stderrScanner))
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

	credsPath := filepath.Join(strings.TrimSpace(homePath), ".credentials.json")
	var credDetected atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := watcher.WaitForCreds(ctx, credsPath)
		if err != nil {
			// Context cancellation is normal (container exited, parent cancelled).
			// Non-context errors: log debug, do NOT SIGTERM.
			if err != context.Canceled && err != context.DeadlineExceeded {
				if logger := LoggerFromContext(ctx); logger != nil {
					logger.Debug("creds watcher error, continuing without auto-exit", "err", err)
				}
			}
			return
		}
		// Creds file appeared: notify main flow and stop the container.
		credDetected.Store(true)
		// D6: SIGTERM via docker stop. Errors swallowed (container may already be
		// gone if the user Ctrl-C'd before this goroutine fired).
		_ = externalCommand("docker", "stop", "--time", "5", containerName).Run()
	}()

	runErr := containerExec.Run(ctx, request)

	// Cancel the derived context so WaitForCreds exits on its next tick.
	cancel()
	// Wait for the goroutine to finish before reading credDetected.
	wg.Wait()

	if credDetected.Load() {
		// Credentials were written before container exit. Emit success notice and
		// return nil regardless of runErr (docker may report non-zero on SIGTERM).
		_ = writeCLINotice(stderr, laslig.NoticeInfoLevel, "Claude auth complete", "Browser authentication complete. Credentials saved.")
		return nil
	}
	return runErr
}

// externalCommand is a package-level variable so tests can intercept the
// "docker stop" subprocess call without needing a real Docker daemon.
// The default implementation delegates to exec.Command.
var externalCommand = func(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func claudeAuthRunnerFromContext(ctx context.Context) claudeAuthRunner {
	if runner, ok := ctx.Value(claudeAuthRunnerKey{}).(claudeAuthRunner); ok && runner != nil {
		return runner
	}
	return hostClaudeAccountAuth
}

// ensureClaudeAccountReady is the Claude-specific auth flow called from
// ensureManagedAccountReady. It runs the managed Claude container with the
// account's home directory bind-mounted so the Claude CLI auto-prompts for
// device-code OAuth and writes .credentials.json natively.
//
// Step order:
//  1. SkipLogin → return nil immediately.
//  2. .credentials.json exists and non-empty → already authed, return nil.
//  3. Non-TTY guard → return error mentioning "TTY".
//  4. writeCLINotice to announce login.
//  5. RunInContainer → if error → return.
//  6. ReadAccountIdentity → if not LoggedIn → return error.
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
	if err := runner.RunInContainer(cmd.Context(), account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("run claude auth container for account %q: %w", account.Name, err)
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
//  1. writeCLINotice to announce login.
//  2. RunInContainer → if error → return.
//  3. ReadAccountIdentity → if not LoggedIn → return error.
func loginClaudeAccount(cmd *cobra.Command, account domain.Profile, _ config.Paths) error {
	if err := writeCLINotice(
		cmd.ErrOrStderr(),
		laslig.NoticeInfoLevel,
		"Claude login",
		fmt.Sprintf("Starting Claude login for account %q.", account.Name),
	); err != nil {
		return fmt.Errorf("announce claude login: %w", err)
	}
	runner := claudeAuthRunnerFromContext(cmd.Context())
	if err := runner.RunInContainer(cmd.Context(), account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("run claude auth container for account %q: %w", account.Name, err)
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
