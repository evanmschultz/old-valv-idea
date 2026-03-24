package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/output"
	manageservice "github.com/evanmschultz/valv/internal/services/manage"
)

func newManageCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "manage",
		Aliases: []string{"m"},
		Short:   "Operator workflows for bindings, runtimes, and updates",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newManageProfileCommand(paths, opts))
	cmd.AddCommand(newManageBindCommand(paths, opts))
	cmd.AddCommand(newManageStatusCommand(paths, opts))
	cmd.AddCommand(newStubCommand("update", "Rebuild and rotate provider client images"))
	return cmd
}

func newManageProfileCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage Valv provider profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newManageProfileAddCommand(paths, opts))
	return cmd
}

func newManageProfileAddCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var homePath string
	cmd := &cobra.Command{
		Use:   "add <provider> <name>",
		Short: "Create a Valv-managed provider profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			policy, err := outputPolicyFromCommand(cmd, opts)
			if err != nil {
				return fmt.Errorf("resolve output policy: %w", err)
			}
			mode := output.ResolveMode(cmd.OutOrStdout(), policy)

			provider, err := domain.ParseProvider(args[0])
			if err != nil {
				return err
			}

			store, err := openStore(paths)
			if err != nil {
				return fmt.Errorf("manage profile add: %w", err)
			}
			defer store.Close()

			service, err := manageservice.New(manageservice.Options{
				Store:        store,
				ProviderRoot: paths.ProviderRoot,
				Logger:       LoggerFromContext(cmd.Context()),
			})
			if err != nil {
				return fmt.Errorf("manage profile add: initialize service: %w", err)
			}

			profile, err := service.CreateProfile(cmd.Context(), provider, args[1], homePath)
			if err != nil {
				return fmt.Errorf("manage profile add: %w", err)
			}

			fields := []output.Field{{Label: "provider", Value: string(profile.Provider)}, {Label: "name", Value: profile.Name}, {Label: "home", Value: profile.HomePath}}
			if err := output.WriteRecord(cmd.OutOrStdout(), mode, "Profile created", fields); err != nil {
				return fmt.Errorf("manage profile add: write output: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&homePath, "home", "", "explicit provider profile home path")
	return cmd
}

func newManageBindCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	cmd := &cobra.Command{
		Use:   "bind <provider> <profile>",
		Short: "Bind the current project to a provider profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			policy, err := outputPolicyFromCommand(cmd, opts)
			if err != nil {
				return fmt.Errorf("resolve output policy: %w", err)
			}
			mode := output.ResolveMode(cmd.OutOrStdout(), policy)

			provider, err := domain.ParseProvider(args[0])
			if err != nil {
				return err
			}

			store, err := openStore(paths)
			if err != nil {
				return fmt.Errorf("manage bind: %w", err)
			}
			defer store.Close()

			service, err := manageservice.New(manageservice.Options{
				Store:        store,
				ProviderRoot: paths.ProviderRoot,
				Logger:       LoggerFromContext(cmd.Context()),
			})
			if err != nil {
				return fmt.Errorf("manage bind: initialize service: %w", err)
			}

			startPath := strings.TrimSpace(projectPath)
			if startPath == "" {
				startPath, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("manage bind: resolve working directory: %w", err)
				}
			}

			result, err := service.BindProject(cmd.Context(), provider, args[1], startPath)
			if err != nil {
				return fmt.Errorf("manage bind: %w", err)
			}

			fields := []output.Field{{Label: "project", Value: result.Project.Root}, {Label: "provider", Value: string(result.Profile.Provider)}, {Label: "profile", Value: result.Profile.Name}, {Label: "home", Value: result.Profile.HomePath}}
			if err := output.WriteRecord(cmd.OutOrStdout(), mode, "Project binding updated", fields); err != nil {
				return fmt.Errorf("manage bind: write output: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to bind instead of the current working directory")
	return cmd
}

func newManageStatusCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current project's Valv binding status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			policy, err := outputPolicyFromCommand(cmd, opts)
			if err != nil {
				return fmt.Errorf("resolve output policy: %w", err)
			}
			mode := output.ResolveMode(cmd.OutOrStdout(), policy)

			store, err := openStore(paths)
			if err != nil {
				return fmt.Errorf("manage status: %w", err)
			}
			defer store.Close()

			service, err := manageservice.New(manageservice.Options{
				Store:        store,
				ProviderRoot: paths.ProviderRoot,
				Logger:       LoggerFromContext(cmd.Context()),
			})
			if err != nil {
				return fmt.Errorf("manage status: initialize service: %w", err)
			}

			startPath := strings.TrimSpace(projectPath)
			if startPath == "" {
				startPath, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("manage status: resolve working directory: %w", err)
				}
			}

			status, err := service.Status(cmd.Context(), startPath)
			if err != nil {
				return err
			}

			fields := []output.Field{{Label: "project", Value: status.Project.Root}, {Label: "provider", Value: string(status.Profile.Provider)}, {Label: "profile", Value: status.Profile.Name}, {Label: "home", Value: status.Profile.HomePath}, {Label: "git marker", Value: status.Detected.GitMarker}}
			if err := output.WriteRecord(cmd.OutOrStdout(), mode, "Project status", fields); err != nil {
				return fmt.Errorf("manage status: write output: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to inspect instead of the current working directory")
	return cmd
}
