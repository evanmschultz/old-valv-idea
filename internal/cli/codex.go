package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	codexprovider "github.com/evanmschultz/valv/internal/adapters/providers/codex"
	"github.com/evanmschultz/valv/internal/config"
	codexservice "github.com/evanmschultz/valv/internal/services/codex"
	imagesservice "github.com/evanmschultz/valv/internal/services/images"
)

type codexRunFunc func(*cobra.Command, []string) error

var findDockerBinary = exec.LookPath

func newCodexCommand(paths config.Paths, run codexRunFunc) *cobra.Command {
	if run == nil {
		run = func(cmd *cobra.Command, args []string) error {
			return runCodexCommand(cmd, paths, args)
		}
	}

	return &cobra.Command{
		Use:   "codex",
		Short: "Run Codex through Valv's Docker runtime",
		Long: strings.TrimSpace(`
Run the Codex CLI inside a Valv-managed Docker runtime for the current bound project.

Everything after ` + "`valv codex`" + ` is passed through to Codex as directly as possible. Use the management surface to create accounts and bind projects before launching Codex.

Before launch, Valv ensures the bound account is authenticated on the host so browser-based Codex login happens outside Docker when needed.

Valv checks for a newer Codex release before launch and rebuilds the runtime image only when needed.
`),
		Example: strings.TrimSpace(`
valv codex
valv codex --help
valv codex resume --last
valv codex exec "summarize the latest diff"
`),
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		SilenceUsage:       true,
		RunE:               run,
	}
}

func runCodexCommand(cmd *cobra.Command, paths config.Paths, args []string) error {
	if codexArgsSkipProjectBinding(args) {
		return runCodexImageOnlyCommand(cmd, paths, args)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("run codex command: resolve working directory: %w", err)
	}
	if err := ensureCodexBindingReady(cmd, paths, workingDir); err != nil {
		if errors.Is(err, errCodexSetupCanceled) {
			return nil
		}
		return fmt.Errorf("run codex command: %w", err)
	}
	if err := ensureBoundCodexAccountReady(cmd, paths, workingDir, args); err != nil {
		return fmt.Errorf("run codex command: %w", err)
	}

	stdinTTY := commandHasTTY(cmd.InOrStdin())
	stdoutTTY := commandHasTTY(cmd.OutOrStdout())
	stderrTTY := commandHasTTY(cmd.ErrOrStderr())
	logger := LoggerFromContext(cmd.Context())
	if logger != nil {
		logger.Debug(
			"resolved codex terminal state",
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
		return fmt.Errorf("run codex command: %w", err)
	}
	defer store.Close()

	service, err := codexservice.New(codexservice.Options{
		Store:    store,
		Executor: dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())),
		Image:    codexImageRef(),
		User:     currentContainerUser(),
		TTY:      stdinTTY && stdoutTTY,
		Stdin:    stdinTTY,
		TempRoot: paths.TempCacheDir,
		RealHome: realHomeDir(),
		Logger:   logger,
		Notices:  cmd.ErrOrStderr(),
	})
	if err != nil {
		return fmt.Errorf("run codex command: initialize launcher: %w", err)
	}
	if err := ensureCodexImageCurrent(cmd, paths); err != nil {
		return fmt.Errorf("run codex command: %w", err)
	}
	if err := service.ValidateBinding(cmd.Context(), workingDir); err != nil {
		return fmt.Errorf("run codex command: validate binding: %w", err)
	}

	if err := service.Run(cmd.Context(), workingDir, args); err != nil {
		return fmt.Errorf("run codex command: %w", err)
	}
	return nil
}

func runCodexImageOnlyCommand(cmd *cobra.Command, paths config.Paths, args []string) error {
	if err := runWithCLIQuietSpinner(
		cmd.ErrOrStderr(),
		"Checking Codex image",
		"Codex image ready",
		"Codex image check failed",
		func() error { return ensureCodexImageCurrent(cmd, paths) },
	); err != nil {
		return fmt.Errorf("run codex command: %w", err)
	}
	image := codexImageRef()

	executor := dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()))
	request := dockeradapter.ContainerRunRequest{
		Name:  fmt.Sprintf("valv-codex-info-%d", timeNowUnixNano()),
		Image: image,
		Labels: map[string]string{
			"io.valv.managed":  "true",
			"io.valv.provider": "codex",
			"io.valv.scope":    "info",
		},
		Env: map[string]string{
			"CODEX_HOME": codexprovider.ContainerCodexDir,
			"HOME":       codexprovider.ContainerHomeDir,
			"LOGNAME":    "valv",
			"USER":       "valv",
		},
		User:        currentContainerUser(),
		Args:        append([]string(nil), args...),
		Interactive: commandHasTTY(cmd.InOrStdin()),
		TTY:         commandHasTTY(cmd.InOrStdin()) && commandHasTTY(cmd.OutOrStdout()),
		Init:        commandHasTTY(cmd.InOrStdin()) || commandHasTTY(cmd.OutOrStdout()),
		Remove:      true,
	}
	if err := executor.Run(cmd.Context(), request); err != nil {
		return fmt.Errorf("run codex command: execute codex image-only request: %w", err)
	}
	return nil
}

var timeNowUnixNano = func() int64 { return time.Now().UTC().UnixNano() }

func codexArgsSkipProjectBinding(args []string) bool {
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

func codexArgsSkipAccountReady(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "help", "login", "logout":
		return true
	default:
		return false
	}
}

func ensureBoundCodexAccountReady(cmd *cobra.Command, paths config.Paths, workingDir string, args []string) error {
	if codexArgsSkipAccountReady(args) {
		return nil
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("initialize account service: %w", err)
	}
	defer closeStore()
	status, err := service.Status(cmd.Context(), workingDir)
	if err != nil {
		return fmt.Errorf("resolve bound account: %w", err)
	}
	if err := ensureManagedAccountReady(cmd, status.Profile.Provider, status.Profile, accountAuthOptions{}); err != nil {
		return fmt.Errorf("ensure bound account %q is ready: %w", status.Profile.Name, err)
	}
	return nil
}

func ensureCodexImageAvailable(ctx context.Context, runner interface {
	Run(context.Context, []string) error
}, image dockeradapter.ImageRef,
) error {
	if _, err := findDockerBinary("docker"); err != nil {
		return nil
	}
	if err := runner.Run(ctx, []string{"image", "inspect", image.String()}); err != nil {
		if dockerImageMissingError(err) {
			return fmt.Errorf("codex image %q is not built locally; run `valv manage update` first", image.String())
		}
		return fmt.Errorf("inspect codex image %q: %w", image.String(), err)
	}
	return nil
}

func ensureCodexImageCurrent(cmd *cobra.Command, paths config.Paths) error {
	if strings.TrimSpace(os.Getenv("VALV_CODEX_IMAGE")) != "" {
		return ensureCodexImageAvailable(cmd.Context(), dockeradapter.NewQuietRunner("docker", LoggerFromContext(cmd.Context())), codexImageRef())
	}
	service, closeImages, err := openImagesService(cmd, paths)
	if err != nil {
		return fmt.Errorf("initialize image updater: %w", err)
	}
	defer closeImages()
	result, err := service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{AllowExistingOnCheckFail: true})
	if err != nil {
		return err
	}
	if result.Action == imagesservice.EnsureActionUsingExistingImage {
		LoggerFromContext(cmd.Context()).Debug("using existing codex image after latest-version check failed", "image", result.Image.String(), "version", result.Version)
	}
	return nil
}

func dockerImageMissingError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "No such image") ||
		strings.Contains(message, "No such object") ||
		strings.Contains(message, "pull access denied")
}

func commandHasTTY(stream any) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(file.Fd())
}

func codexImageRef() dockeradapter.ImageRef {
	value := strings.TrimSpace(os.Getenv("VALV_CODEX_IMAGE"))
	if value == "" {
		return dockeradapter.NewImageRef("valv-codex", "dev")
	}

	lastSlash := strings.LastIndex(value, "/")
	lastColon := strings.LastIndex(value, ":")
	if lastColon > lastSlash {
		return dockeradapter.NewImageRef(value[:lastColon], value[lastColon+1:])
	}
	return dockeradapter.NewImageRef(value, "")
}

func codexImageRepository() string {
	ref := codexImageRef()
	return ref.Repository
}

func codexImageTag() string {
	ref := codexImageRef()
	if strings.TrimSpace(ref.Tag) == "" {
		return "dev"
	}
	return ref.Tag
}

func codexImageVersionRef(version string) dockeradapter.ImageRef {
	return dockeradapter.NewImageRef(codexImageRepository(), strings.ReplaceAll(strings.TrimSpace(version), ".", "-"))
}
