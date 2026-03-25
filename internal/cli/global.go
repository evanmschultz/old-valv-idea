package cli

import (
	"fmt"

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
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newGlobalSwitchCommand(paths, opts))
	return cmd
}

func newGlobalSwitchCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch <provider> [profile]",
		Short: "Switch the host-global provider profile",
		Args:  cobra.RangeArgs(1, 2),
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
			return fmt.Errorf("global switch: list profiles: %w", err)
		}
		selected, err = pickProfile(cmd, provider, profiles.Profiles)
		if err != nil {
			return fmt.Errorf("global switch: %w", err)
		}
	}
	result, err := switchService.Switch(cmd.Context(), provider, selected)
	if err != nil {
		return fmt.Errorf("global switch: %w", err)
	}
	fields := []output.Field{{Label: "provider", Value: string(result.Provider), Muted: true}, {Label: "profile", Value: result.ProfileName, Identifier: true}, {Label: "target", Value: result.TargetPath, Identifier: true}, {Label: "source", Value: result.ProfileHome}, {Label: "backup", Value: result.BackupPath, Muted: true}}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Global profile switched", fields)
}
