package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/output"
)

func newGlobalCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "global",
		Aliases: []string{"g"},
		Short:   "Host-global convenience commands",
		Long: strings.TrimSpace(`
Host-global convenience commands.

Use host-global commands when you intentionally want to manipulate the machine-level provider state instead of the normal Valv-isolated runtime path.
`),
		Example: strings.TrimSpace(`
valv global switch codex work
valv g switch codex
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newGlobalSwitchCommand(paths, opts))
	installBranchHelpCommands(cmd)
	return cmd
}

func newGlobalSwitchCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch <provider> [account]",
		Short: "Switch the host-global provider account",
		Long: strings.TrimSpace(`
Switch the host-global provider account as a convenience flow.

For Codex, this updates the host-side ` + "`~/.codex`" + ` path target rather than launching a containerized runtime.
`),
		Example: strings.TrimSpace(`
valv global switch codex work
valv g switch codex
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := domain.ParseProvider(args[0])
			if err != nil {
				return err
			}
			profileName := ""
			if len(args) == 2 {
				profileName = args[1]
			}
			return runGlobalSwitch(cmd, paths, opts, provider, profileName)
		},
	}
	return cmd
}

func runGlobalSwitch(cmd *cobra.Command, paths config.Paths, opts *rootOptions, provider domain.Provider, profileName string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	switchService, closeStore, err := openGlobalSwitchService(cmd, paths)
	if err != nil {
		return fmt.Errorf("global switch: %w", err)
	}
	defer closeStore()
	selected := profileName
	if selected == "" {
		manageService, closeManageStore, err := openManageService(cmd, paths)
		if err != nil {
			return fmt.Errorf("global switch: %w", err)
		}
		profiles, err := manageService.ListProfiles(cmd.Context(), provider)
		closeManageStore()
		if err != nil {
			return fmt.Errorf("global switch: list accounts: %w", err)
		}
		pickedProfile, pickErr := pickProfile(cmd, provider, profiles.Profiles)
		if pickErr != nil {
			if errors.Is(pickErr, errSelectionCanceled) {
				return writeNoOpRecord(cmd, opts, "No global switch made", "no account selected")
			}
			return fmt.Errorf("global switch: %w", pickErr)
		}
		selected = pickedProfile.Name
	}
	result, err := switchService.Switch(cmd.Context(), provider, selected)
	if err != nil {
		return fmt.Errorf("global switch: %w", err)
	}
	fields := []output.Field{{Label: "provider", Value: string(result.Provider), Muted: true}, {Label: "account", Value: result.ProfileName, Identifier: true}, {Label: "target", Value: result.TargetPath, Identifier: true}, {Label: "source", Value: result.ProfileHome}, {Label: "backup", Value: result.BackupPath, Muted: true}}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Global account switched", fields)
}
