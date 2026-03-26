package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/output"
)

func newPathsCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "Show the resolved Valv filesystem paths",
		Long: strings.TrimSpace(`
Show the filesystem locations Valv will use for durable state, logs, caches, and runtime scratch data.

In disposable dev mode, these paths intentionally point into a temp-home root created by Just. That temp-home is safe to remove with ` + "`just dev-clean`" + `.
`),
		Example: strings.TrimSpace(`
valv paths
valv paths --format json
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			policy, err := outputPolicyFromCommand(cmd, opts)
			if err != nil {
				return fmt.Errorf("resolve output policy: %w", err)
			}
			mode := output.ResolveMode(cmd.OutOrStdout(), policy)
			fields := []output.Field{
				{Label: "app support", Value: paths.AppSupportRoot},
				{Label: "database", Value: paths.DatabasePath},
				{Label: "providers", Value: paths.ProviderRoot},
				{Label: "logs", Value: paths.LogsDir},
				{Label: "cache", Value: paths.CachesDir},
				{Label: "runtime tmp", Value: paths.RuntimeTmpDir},
			}
			if err := output.WriteRecord(cmd.OutOrStdout(), mode, "Valv paths", fields); err != nil {
				return fmt.Errorf("write paths output: %w", err)
			}
			return nil
		},
	}
}
