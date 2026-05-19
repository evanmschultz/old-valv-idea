package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	claudeservice "github.com/evanmschultz/valv/internal/services/claude"
	imagesservice "github.com/evanmschultz/valv/internal/services/images"
)

type claudeRunFunc func(*cobra.Command, []string) error

func newClaudeCommand(paths config.Paths, run claudeRunFunc) *cobra.Command {
	if run == nil {
		run = func(cmd *cobra.Command, args []string) error {
			return runClaudeCommand(cmd, paths, args)
		}
	}

	return &cobra.Command{
		Use:   "claude",
		Short: "Run Claude through Valv's Docker runtime",
		Long: strings.TrimSpace(`
Run the Claude CLI inside a Valv-managed Docker runtime for the current bound project.

Everything after ` + "`valv claude`" + ` is passed through to Claude as directly as possible. Use the management surface to create accounts and bind projects before launching Claude.

Claude device-code authentication runs inside the container at first launch. Credentials are persisted in the Valv-managed account home and reused on subsequent launches.

Valv checks that the Claude image is built before launch and rebuilds only when needed.
`),
		Example: strings.TrimSpace(`
valv claude
valv claude --help
valv claude --version
valv claude --resume
`),
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		SilenceUsage:       true,
		RunE:               run,
	}
}

func runClaudeCommand(cmd *cobra.Command, paths config.Paths, args []string) error {
	if logger := LoggerFromContext(cmd.Context()); logger != nil {
		logger.Debug("run claude command arguments", "args", args)
	}

	// Extract --account override before any skip-binding check so the flag is
	// consumed even when the caller passes a help/version arg combination.
	accountName, args := stripAccountFlag(args)

	if claudeArgsSkipProjectBinding(args) {
		return runClaudeImageOnlyCommand(cmd, paths, args)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("run claude command: resolve working directory: %w", err)
	}

	// When --account is supplied, resolve and auth the override profile before
	// the image check. When accountName is empty, auth runs container-side at
	// first launch so no host-side check is needed here.
	if accountName != "" {
		if err := ensureClaudeAccountReady(cmd, domain.Profile{}, accountAuthOptions{Paths: paths}, accountName); err != nil {
			return fmt.Errorf("run claude command: %w", err)
		}
	}

	// When an account override is active, skip ValidateBinding (which would fail
	// for unbound projects). The resolved profile is threaded into the launch
	// service by Unit 8.4 via OverrideProfile on claudeservice.Options.
	skipValidate := accountName != ""

	// No ensureClaudeBindingReady: unbound projects surface as ErrUnboundProject
	// from service.ValidateBinding. Claude device-code auth runs inside the
	// container at first launch; no host-side auth check required.

	stdinTTY := commandHasTTY(cmd.InOrStdin())
	stdoutTTY := commandHasTTY(cmd.OutOrStdout())
	stderrTTY := commandHasTTY(cmd.ErrOrStderr())
	logger := LoggerFromContext(cmd.Context())
	if logger != nil {
		logger.Debug(
			"resolved claude terminal state",
			"stdin_tty", stdinTTY,
			"stdout_tty", stdoutTTY,
			"stderr_tty", stderrTTY,
			"term", strings.TrimSpace(os.Getenv("TERM")),
			"colorterm", strings.TrimSpace(os.Getenv("COLORTERM")),
			"term_program", strings.TrimSpace(os.Getenv("TERM_PROGRAM")),
		)
	}

	store, err := openStore(paths)
	if err != nil {
		return fmt.Errorf("run claude command: %w", err)
	}
	defer store.Close()

	service, err := claudeservice.New(claudeservice.Options{
		Store:    store,
		Executor: dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())),
		Image:    claudeImageRef(),
		User:     currentContainerUser(),
		TTY:      stdinTTY && stdoutTTY,
		Stdin:    stdinTTY,
		TempRoot: paths.TempCacheDir,
		RealHome: realHomeDir(),
		Logger:   logger,
		Notices:  cmd.ErrOrStderr(),
	})
	if err != nil {
		return fmt.Errorf("run claude command: initialize launcher: %w", err)
	}
	// Validate binding before the image check so unbound-project errors surface
	// quickly without triggering a docker build. Skip when --account override is
	// active: the override bypasses the binding store entirely. Unit 8.4 threads
	// the resolved profile into claudeservice.Options so the launch proceeds.
	if !skipValidate {
		if err := service.ValidateBinding(cmd.Context(), workingDir); err != nil {
			return fmt.Errorf("run claude command: validate binding: %w", err)
		}
	}
	if err := ensureClaudeImageCurrent(cmd, paths); err != nil {
		return fmt.Errorf("run claude command: %w", err)
	}

	if err := service.Run(cmd.Context(), workingDir, args); err != nil {
		return fmt.Errorf("run claude command: %w", err)
	}
	return nil
}

func runClaudeImageOnlyCommand(cmd *cobra.Command, paths config.Paths, args []string) error {
	if err := runWithCLIQuietSpinner(
		cmd.ErrOrStderr(),
		"Checking Claude image",
		"Claude image ready",
		"Claude image check failed",
		func() error { return ensureClaudeImageCurrent(cmd, paths) },
	); err != nil {
		return fmt.Errorf("run claude command: %w", err)
	}
	image := claudeImageRef()

	executor := dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()))
	request := dockeradapter.ContainerRunRequest{
		Name:  fmt.Sprintf("valv-claude-info-%d", timeNowUnixNano()),
		Image: image,
		Labels: map[string]string{
			"io.valv.managed":  "true",
			"io.valv.provider": "claude",
			"io.valv.scope":    "info",
		},
		Env: map[string]string{
			"CLAUDE_CONFIG_DIR": claudeprovider.ContainerClaudeDir,
			"HOME":              claudeprovider.ContainerHomeDir,
			"LOGNAME":           "valv",
			"USER":              "valv",
		},
		User:        currentContainerUser(),
		Args:        append([]string(nil), args...),
		Interactive: commandHasTTY(cmd.InOrStdin()),
		TTY:         commandHasTTY(cmd.InOrStdin()) && commandHasTTY(cmd.OutOrStdout()),
		Init:        commandHasTTY(cmd.InOrStdin()) || commandHasTTY(cmd.OutOrStdout()),
		Remove:      true,
	}
	if err := executor.Run(cmd.Context(), request); err != nil {
		return fmt.Errorf("run claude command: execute claude image-only request: %w", err)
	}
	return nil
}

func claudeArgsSkipProjectBinding(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	if len(args) == 1 {
		switch args[0] {
		case "--version", "-V":
			return true
		}
	}
	return args[0] == "help"
}

func ensureClaudeImageCurrent(cmd *cobra.Command, paths config.Paths) error {
	if strings.TrimSpace(os.Getenv("VALV_CLAUDE_IMAGE")) != "" {
		return ensureClaudeImageAvailable(cmd.Context(), dockeradapter.NewQuietRunner("docker", LoggerFromContext(cmd.Context())), claudeImageRef())
	}
	service, closeImages, err := openImagesService(cmd, paths, domain.ProviderClaude)
	if err != nil {
		return fmt.Errorf("initialize image updater: %w", err)
	}
	defer closeImages()
	result, err := service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{AllowExistingOnCheckFail: true})
	if err != nil {
		return err
	}
	if result.Action == imagesservice.EnsureActionUsingExistingImage {
		LoggerFromContext(cmd.Context()).Debug("using existing claude image after latest-version check failed", "image", result.Image.String(), "version", result.Version)
	}
	return nil
}

func ensureClaudeImageAvailable(ctx context.Context, runner interface {
	Run(context.Context, []string) error
}, image dockeradapter.ImageRef,
) error {
	// Delegate to the findDockerBinary package var (declared in codex.go).
	if _, err := findDockerBinary("docker"); err != nil {
		return nil
	}
	if err := runner.Run(ctx, []string{"image", "inspect", image.String()}); err != nil {
		if dockerImageMissingError(err) {
			return fmt.Errorf("claude image %q is not built locally; run `valv manage update claude` first", image.String())
		}
		return fmt.Errorf("inspect claude image %q: %w", image.String(), err)
	}
	return nil
}
