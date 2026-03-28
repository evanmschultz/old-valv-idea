package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/logging"
	"github.com/evanmschultz/valv/internal/output"
)

type rootOptions struct {
	configPath string
	format     string
	style      bool
	noStyle    bool
	debug      bool
}

type loggerKey struct{}
type effectiveConfigKey struct{}
type logCloserKey struct{}

func NewRootCommand(ctx context.Context, stdout, stderr io.Writer) (*cobra.Command, error) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return nil, fmt.Errorf("resolve default paths: %w", err)
	}
	return newRootCommandWithPaths(ctx, stdout, stderr, paths)
}

func NewRootCommandWithPaths(ctx context.Context, stdout, stderr io.Writer, paths config.Paths) (*cobra.Command, error) {
	return newRootCommandWithPaths(ctx, stdout, stderr, paths)
}

func newRootCommandWithPaths(ctx context.Context, stdout, stderr io.Writer, paths config.Paths) (*cobra.Command, error) {
	opts := &rootOptions{}
	cmd := &cobra.Command{
		Use:   "valv",
		Short: "Valv control plane for containerized AI CLIs",
		Long: strings.TrimSpace(`
Valv manages provider accounts, Docker runtimes, and API execution for local AI CLIs.
Use the direct runtime commands for provider execution and the management surface for setup and operator workflows.
`),
		Example: strings.TrimSpace(`
valv paths
valv manage update
valv manage account add codex
valv manage account add codex work
valv codex --help
valv api serve --runtime-ttl 2m
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := paths.Ensure(); err != nil {
				return fmt.Errorf("ensure paths: %w", err)
			}
			effective, _, err := loadEffectiveConfig(opts.configPath)
			if err != nil {
				return err
			}
			level := effective.Logging.Level
			if opts.debug {
				level = "debug"
			}
			logFile, err := logging.OpenFile(paths.LogsDir, "valv.log")
			if err != nil {
				return fmt.Errorf("initialize log file: %w", err)
			}
			writer := io.MultiWriter(stderr, logFile)
			logger, err := logging.New(logging.Options{Writer: writer, Level: level, Prefix: "valv"})
			if err != nil {
				_ = logFile.Close()
				return fmt.Errorf("initialize logger: %w", err)
			}
			nextCtx := context.WithValue(cmd.Context(), loggerKey{}, logger)
			nextCtx = context.WithValue(nextCtx, effectiveConfigKey{}, effective)
			nextCtx = context.WithValue(nextCtx, logCloserKey{}, io.Closer(logFile))
			cmd.SetContext(nextCtx)
			logger.Debug("initialized valv logger", "log_path", paths.LogsDir)
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, _ []string) {
			closer, _ := cmd.Context().Value(logCloserKey{}).(io.Closer)
			if closer != nil {
				_ = closer.Close()
			}
		},
	}
	cmd.SetContext(ctx)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.AddGroup(
		&cobra.Group{ID: "inspect", Title: "Inspect Commands"},
		&cobra.Group{ID: "runtime", Title: "Runtime Commands"},
		&cobra.Group{ID: "manage", Title: "Management Commands"},
	)

	cmd.PersistentFlags().StringVar(&opts.configPath, "config", config.DefaultConfigPath(paths), "path to the valv config file")
	cmd.PersistentFlags().StringVar(&opts.format, "format", "auto", "output format: auto, human, plain, json")
	cmd.PersistentFlags().BoolVar(&opts.style, "style", false, "force ANSI styling for human output")
	cmd.PersistentFlags().BoolVar(&opts.noStyle, "no-style", false, "disable ANSI styling for human output")
	cmd.PersistentFlags().BoolVar(&opts.debug, "debug", false, "enable debug logging")

	pathsCmd := newPathsCommand(paths, opts)
	pathsCmd.GroupID = "inspect"
	versionCmd := newVersionCommand(opts)
	versionCmd.GroupID = "inspect"
	codexCmd := newCodexCommand(paths, nil)
	codexCmd.GroupID = "runtime"
	apiCmd := newAPICommand(paths, opts)
	apiCmd.GroupID = "runtime"
	manageCmd := newManageCommand(paths, opts)
	manageCmd.GroupID = "manage"
	globalCmd := newGlobalCommand(paths, opts)
	globalCmd.GroupID = "manage"

	cmd.AddCommand(pathsCmd, versionCmd, codexCmd, apiCmd, manageCmd, globalCmd)
	installBranchHelpCommands(cmd)

	return cmd, nil
}

func LoggerFromContext(ctx context.Context) *log.Logger {
	logger, _ := ctx.Value(loggerKey{}).(*log.Logger)
	return logger
}

func EffectiveConfigFromContext(ctx context.Context) (config.EffectiveConfig, bool) {
	effective, ok := ctx.Value(effectiveConfigKey{}).(config.EffectiveConfig)
	return effective, ok
}

func newStubCommand(use string, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("%s command not implemented yet", use)
		},
	}
}

func loadEffectiveConfig(path string) (config.EffectiveConfig, config.Config, error) {
	cfg := config.Default()
	loaded, err := config.Load(path)
	if err == nil {
		cfg = loaded
	} else if !errors.Is(err, domain.ErrConfigNotFound) {
		return config.EffectiveConfig{}, config.Config{}, fmt.Errorf("load config: %w", err)
	}
	effective, err := cfg.Effective()
	if err != nil {
		return config.EffectiveConfig{}, config.Config{}, fmt.Errorf("resolve config: %w", err)
	}
	return effective, cfg, nil
}

func outputPolicyFromCommand(cmd *cobra.Command, opts *rootOptions) (output.Policy, error) {
	forceStyle := false
	if flag := cmd.Flags().Lookup("style"); flag != nil {
		forceStyle = flag.Changed && opts.style
	}
	disableStyle := false
	if flag := cmd.Flags().Lookup("no-style"); flag != nil {
		disableStyle = flag.Changed && opts.noStyle
	}
	if forceStyle && disableStyle {
		return output.Policy{}, fmt.Errorf("output flags: --style and --no-style cannot be used together")
	}

	effective, ok := EffectiveConfigFromContext(cmd.Context())
	if !ok {
		effective = config.EffectiveConfig{
			Output: config.EffectiveOutputConfig{
				Format: domain.OutputFormatAuto,
				Style:  domain.OutputStyleAuto,
			},
		}
	}

	policy := output.Policy{
		Format: effective.Output.Format,
		Style:  effective.Output.Style,
	}
	if flag := cmd.Flags().Lookup("format"); flag != nil && flag.Changed {
		parsedFormat, err := domain.ParseOutputFormat(opts.format)
		if err != nil {
			return output.Policy{}, err
		}
		policy.Format = parsedFormat
	}
	switch {
	case forceStyle:
		policy.Style = domain.OutputStyleAlways
	case disableStyle:
		policy.Style = domain.OutputStyleNever
	}
	return policy, nil
}

func installBranchHelpCommands(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		installBranchHelpCommands(child)
	}
	if !hasNonHelpSubcommands(cmd) || hasHelpSubcommand(cmd) {
		return
	}
	cmd.SetHelpCommand(newHelpCommand(cmd))
	cmd.InitDefaultHelpCmd()
}

func hasNonHelpSubcommands(cmd *cobra.Command) bool {
	for _, child := range cmd.Commands() {
		if child.Name() == "help" {
			continue
		}
		return true
	}
	return false
}

func hasHelpSubcommand(cmd *cobra.Command) bool {
	for _, child := range cmd.Commands() {
		if child.Name() == "help" {
			return true
		}
	}
	return false
}

func newHelpCommand(target *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:     "help [command]",
		Aliases: []string{"h"},
		Short:   "Help about any command",
		Args:    cobra.ArbitraryArgs,
		Run: func(_ *cobra.Command, args []string) {
			target.HelpFunc()(target, args)
		},
	}
}
