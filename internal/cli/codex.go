package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/config"
	codexservice "github.com/evanmschultz/valv/internal/services/codex"
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

Everything after ` + "`valv codex`" + ` is passed through to Codex as directly as possible. Use the management surface to create profiles and bind projects before launching Codex.

If the Valv-managed Codex image has not been built yet, run ` + "`valv manage update`" + ` first.
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
	store, err := openStore(paths)
	if err != nil {
		return fmt.Errorf("run codex command: %w", err)
	}
	defer store.Close()

	service, err := codexservice.New(codexservice.Options{
		Store:    store,
		Executor: dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())),
		Image:    codexImageRef(),
		User:     fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		TTY:      commandHasTTY(cmd.InOrStdin()) && commandHasTTY(cmd.OutOrStdout()),
		Stdin:    commandHasTTY(cmd.InOrStdin()),
		Logger:   LoggerFromContext(cmd.Context()),
	})
	if err != nil {
		return fmt.Errorf("run codex command: initialize launcher: %w", err)
	}

	if err := ensureCodexImageAvailable(cmd.Context(), dockeradapter.NewQuietRunner("docker", LoggerFromContext(cmd.Context())), codexImageRef()); err != nil {
		return fmt.Errorf("run codex command: %w", err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("run codex command: resolve working directory: %w", err)
	}

	if err := service.Run(cmd.Context(), workingDir, args); err != nil {
		return fmt.Errorf("run codex command: %w", err)
	}
	return nil
}

func ensureCodexImageAvailable(ctx context.Context, runner interface {
	Run(context.Context, []string) error
}, image dockeradapter.ImageRef) error {
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
