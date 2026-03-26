package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/output"
)

const version = "dev"

func newVersionCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Show the current Valv version",
		Long:    "Show the current Valv build version.",
		Example: `valv version`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			policy, err := outputPolicyFromCommand(cmd, opts)
			if err != nil {
				return fmt.Errorf("resolve output policy: %w", err)
			}
			mode := output.ResolveMode(cmd.OutOrStdout(), policy)
			return output.WriteRecord(cmd.OutOrStdout(), mode, "Valv version", []output.Field{{Label: "version", Value: version}})
		},
	}
}
