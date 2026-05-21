// Package cli — tools subcommand wires the declarative .valv/tools.toml
// surface into the cobra command tree. Unit 11.4 of DROP_11 introduces the
// `valv tools validate` command as the only `tools` subcommand. The
// `list` subcommand was cut per dev decision Y3.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/project"
	"github.com/evanmschultz/valv/internal/tools"
)

// newToolsCommand returns the `tools` cobra branch. The branch has a single
// subcommand `validate` that parses + validates the project's
// `.valv/tools.toml` and reports the result to the operator. The branch
// itself prints help when invoked without a subcommand.
func newToolsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Inspect the project's declarative tool manifest",
		Long: strings.TrimSpace(`
Inspect the per-project ` + "`.valv/tools.toml`" + ` manifest that declares which tools the Valv container should provide for the current project.

DROP_11 ships the schema, parser, and validation. DROP_12 will read the resolved manifest to build per-project layered images. A project without ` + "`.valv/tools.toml`" + ` is valid — it declares no tools.
`),
		Example: strings.TrimSpace(`
valv tools validate
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newToolsValidateCommand())
	installBranchHelpCommands(cmd)
	return cmd
}

// newToolsValidateCommand returns the `tools validate` subcommand. It
// resolves the project root via project.Detect (matching the behavior of
// `valv claude` and `valv codex`) and calls tools.Resolve on it. On success
// it prints either `"tools.toml is valid"` (non-empty manifest) or
// `"no tools declared"` (absent file or empty manifest). On failure it
// returns a wrapped error so cobra renders it to stderr and exits non-zero.
func newToolsValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the project's .valv/tools.toml manifest",
		Long: strings.TrimSpace(`
Parse and validate the project's ` + "`.valv/tools.toml`" + ` manifest.

The project root is resolved by walking up from the current working directory to the nearest git marker (matching the behavior of ` + "`valv claude`" + ` and ` + "`valv codex`" + `). A project without a manifest is valid and reports ` + "`no tools declared`" + `.

On success: exit code 0 and print a one-line status. On parse or validation failure: exit code 1 with a human-readable error.
`),
		Example: strings.TrimSpace(`
valv tools validate
`),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runToolsValidate(cmd)
		},
	}
}

// runToolsValidate is the runE handler for `valv tools validate`. It is
// split out so tests can exercise the project-root + manifest path through
// the public command surface (via cmd.Execute) while keeping the cobra
// wiring in newToolsValidateCommand.
func runToolsValidate(cmd *cobra.Command) error {
	result, err := project.Detect()
	if err != nil {
		return fmt.Errorf("tools validate: detect project: %w", err)
	}

	manifest, err := tools.Resolve(result.Root)
	if err != nil {
		return fmt.Errorf("tools validate: %w", err)
	}

	if len(manifest.Tools) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "no tools declared"); err != nil {
			return fmt.Errorf("tools validate: write output: %w", err)
		}
		return nil
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "tools.toml is valid"); err != nil {
		return fmt.Errorf("tools validate: write output: %w", err)
	}
	return nil
}
