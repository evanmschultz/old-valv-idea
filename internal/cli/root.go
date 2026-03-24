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

func NewRootCommand(ctx context.Context, stdout, stderr io.Writer) (*cobra.Command, error) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return nil, fmt.Errorf("resolve default paths: %w", err)
	}

	opts := &rootOptions{}
	cmd := &cobra.Command{
		Use:   "valv",
		Short: "Valv control plane for containerized AI CLIs",
		Long: strings.TrimSpace(`
Valv manages provider profiles, Docker runtimes, and API execution for local AI CLIs.
Use the direct runtime commands for provider execution and the management surface for setup and operator workflows.
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			effective, _, err := loadEffectiveConfig(opts.configPath)
			if err != nil {
				return err
			}
			level := effective.Logging.Level
			if opts.debug {
				level = "debug"
			}
			logger, err := logging.New(logging.Options{Writer: stderr, Level: level, Prefix: "valv"})
			if err != nil {
				return fmt.Errorf("initialize logger: %w", err)
			}
			nextCtx := context.WithValue(cmd.Context(), loggerKey{}, logger)
			nextCtx = context.WithValue(nextCtx, effectiveConfigKey{}, effective)
			cmd.SetContext(nextCtx)
			return nil
		},
	}
	cmd.SetContext(ctx)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	cmd.PersistentFlags().StringVar(&opts.configPath, "config", config.DefaultConfigPath(paths), "path to the valv config file")
	cmd.PersistentFlags().StringVar(&opts.format, "format", "auto", "output format: auto, human, plain, json")
	cmd.PersistentFlags().BoolVar(&opts.style, "style", false, "force ANSI styling for human output")
	cmd.PersistentFlags().BoolVar(&opts.noStyle, "no-style", false, "disable ANSI styling for human output")
	cmd.PersistentFlags().BoolVar(&opts.debug, "debug", false, "enable debug logging")

	cmd.AddCommand(newPathsCommand(paths, opts))
	cmd.AddCommand(newVersionCommand(opts))
	cmd.AddCommand(newCodexCommand(paths, nil))
	cmd.AddCommand(newManageCommand(paths, opts))
	cmd.AddCommand(newStubCommand("global", "Host-global convenience commands"))
	cmd.AddCommand(newStubCommand("api", "Run the Valv API surface"))

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
