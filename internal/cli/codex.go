package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/config"
	codexservice "github.com/evanmschultz/valv/internal/services/codex"
)

type codexRunFunc func(*cobra.Command, []string) error

func newCodexCommand(paths config.Paths, run codexRunFunc) *cobra.Command {
	if run == nil {
		run = func(cmd *cobra.Command, args []string) error {
			return runCodexCommand(cmd, paths, args)
		}
	}

	return &cobra.Command{
		Use:                "codex",
		Short:              "Run Codex through Valv's Docker runtime",
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

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("run codex command: resolve working directory: %w", err)
	}

	if err := service.Run(cmd.Context(), workingDir, args); err != nil {
		return fmt.Errorf("run codex command: %w", err)
	}
	return nil
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
