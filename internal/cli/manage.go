package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/output"
	cleanupservice "github.com/evanmschultz/valv/internal/services/cleanup"
	imagesservice "github.com/evanmschultz/valv/internal/services/images"
	manageservice "github.com/evanmschultz/valv/internal/services/manage"
)

func newManageCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "manage",
		Aliases: []string{"m"},
		Short:   "Operator workflows for bindings, runtimes, and updates",
		Long: strings.TrimSpace(`
Operator workflows for bindings, runtimes, and updates.

Use the management surface to create provider profiles, bind projects, rebuild provider images, and clean up Valv-managed state.

This surface owns setup and repair flows. The direct ` + "`valv codex ...`" + ` path stays a Codex pass-through launcher.
`),
		Example: strings.TrimSpace(`
valv manage profile add codex
valv manage profile add codex profile-name
valv manage status
valv manage update
valv manage cleanup all
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runManageHome(cmd, paths, opts)
		},
	}

	cmd.AddCommand(newManageProfileCommand(paths, opts))
	cmd.AddCommand(newManageBindCommand(paths, opts))
	cmd.AddCommand(newManageStatusCommand(paths, opts))
	cmd.AddCommand(newManageUpdateCommand(paths, opts))
	cmd.AddCommand(newManageCleanupCommand(paths, opts))
	installBranchHelpCommands(cmd)
	return cmd
}

func newManageProfileCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage Valv provider profiles",
		Long: strings.TrimSpace(`
Manage Valv provider profiles.

Provider profiles define the home directory Valv mounts into containerized provider runtimes.

For Codex, that home path becomes ` + "`CODEX_HOME`" + ` inside the container.
`),
		Example: strings.TrimSpace(`
valv manage profile add codex
valv manage profile add codex profile-name
valv manage profile switch
valv manage profile switch alternate-profile
valv manage profile list
valv manage profile list codex
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newManageProfileAddCommand(paths, opts))
	cmd.AddCommand(newManageProfileListCommand(paths, opts))
	cmd.AddCommand(newManageProfileSwitchCommand(paths, opts))
	return cmd
}

func newManageProfileAddCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var homePath string
	var noBind bool
	var projectPath string
	cmd := &cobra.Command{
		Use:   "add <provider> [name]",
		Short: "Create a provider profile and bind it by default",
		Long: strings.TrimSpace(`
Create one provider profile and, by default, bind the current project to it.

Semantics:
- ` + "`valv manage profile add codex`" + ` creates or reuses the inferred host-backed default profile for Codex
- ` + "`valv manage profile add codex profile-name`" + ` creates or reuses an isolated named profile under Valv's provider root
- ` + "`--no-bind`" + ` keeps the profile ready without changing the current project's binding
- ` + "`--home`" + ` is an expert override for custom host paths

Output fields:
- project: bound project root when Valv also updated the current project binding
- provider: provider family for the profile
- name: Valv profile name
- home: host path mounted into provider runtimes as that profile's home
`),
		Example: strings.TrimSpace(`
valv manage profile add codex
valv manage profile add codex profile-name
valv manage profile add codex --no-bind
valv manage profile add codex profile-name --home /absolute/path/to/custom-home --no-bind
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := domain.ParseProvider(args[0])
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 2 {
				name = args[1]
			}
			return runManageProfileAdd(cmd, paths, opts, provider, name, homePath, !noBind, projectPath)
		},
	}
	cmd.Flags().StringVar(&homePath, "home", "", "explicit provider profile home path")
	cmd.Flags().BoolVar(&noBind, "no-bind", false, "create or reuse the profile without binding the current project")
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to bind instead of the current working directory")
	return cmd
}

func newManageProfileListCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [provider]",
		Short: "List Valv-managed provider profiles",
		Long: strings.TrimSpace(`
List the profiles Valv knows for one provider or, if no provider is given, group profiles by provider.

Human output shows the profile name and mounted home path. JSON output uses one stable command-owned top-level key.
`),
		Example: strings.TrimSpace(`
valv manage profile list
valv manage profile list codex
valv manage profile list codex --format json
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := commandOutputMode(cmd, opts)
			if err != nil {
				return fmt.Errorf("resolve output policy: %w", err)
			}
			service, closeStore, err := openManageService(cmd, paths)
			if err != nil {
				return fmt.Errorf("manage profile list: %w", err)
			}
			defer closeStore()
			if len(args) == 1 {
				provider, err := domain.ParseProvider(args[0])
				if err != nil {
					return err
				}
				result, err := service.ListProfiles(cmd.Context(), provider)
				if err != nil {
					return fmt.Errorf("manage profile list: %w", err)
				}
				return output.WriteListWithKey(cmd.OutOrStdout(), mode, fmt.Sprintf("%s profiles", provider), "profiles", listItemsForProfiles(result.Profiles))
			}
			return writeProfilesByProvider(cmd, mode, service)
		},
	}
	return cmd
}

func newManageProfileSwitchCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	cmd := &cobra.Command{
		Use:   "switch [provider] [profile]",
		Short: "Switch the current project's bound profile",
		Long: strings.TrimSpace(`
Switch the current project's provider profile without leaving the management surface.

When no provider is given, Valv uses the currently bound provider for the project. When no profile is given, Valv opens a picker in TTY mode.
`),
		Example: strings.TrimSpace(`
valv manage profile switch
valv manage profile switch alternate-profile
valv manage profile switch codex alternate-profile
`),
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageProfileSwitch(cmd, paths, opts, args, projectPath)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to switch instead of the current working directory")
	return cmd
}

func newManageBindCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	cmd := &cobra.Command{
		Use:   "bind <provider> <profile>",
		Short: "Bind the current project to a provider profile",
		Long: strings.TrimSpace(`
Bind one detected project root to one provider profile.

After binding, direct runtime commands like ` + "`valv codex ...`" + ` can resolve the profile without additional setup.
`),
		Example: strings.TrimSpace(`
valv manage bind codex profile-name
valv manage bind codex profile-name --project /absolute/path/to/repo
`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := domain.ParseProvider(args[0])
			if err != nil {
				return err
			}
			return runManageBind(cmd, paths, opts, provider, args[1], projectPath)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to bind instead of the current working directory")
	return cmd
}

func runManageBindInteractive(cmd *cobra.Command, paths config.Paths, opts *rootOptions) error {
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage bind: %w", err)
	}
	defer closeStore()
	profiles, err := service.ListProfiles(cmd.Context(), domain.ProviderCodex)
	if err != nil {
		return fmt.Errorf("manage bind: list profiles: %w", err)
	}
	selected, err := pickProfile(cmd, domain.ProviderCodex, profiles.Profiles)
	if err != nil {
		if errors.Is(err, errSelectionCanceled) {
			return nil
		}
		return fmt.Errorf("manage bind: %w", err)
	}
	return runManageBind(cmd, paths, opts, domain.ProviderCodex, selected, "")
}

func runManageProfileAdd(cmd *cobra.Command, paths config.Paths, opts *rootOptions, provider domain.Provider, name string, homePath string, bind bool, projectPath string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage profile add: %w", err)
	}
	defer closeStore()

	name = strings.TrimSpace(name)
	homePath = strings.TrimSpace(homePath)

	var profile domain.Profile
	var hostSpec manageservice.HostProfileSpec
	if name == "" {
		hostSpec, err = service.DefaultHostProfile(provider)
		if err != nil {
			return fmt.Errorf("manage profile add: %w", err)
		}
		if homePath == "" {
			homePath = hostSpec.HomePath
		}
		profile, err = service.CreateProfile(cmd.Context(), provider, hostSpec.Name, homePath)
	} else {
		profile, err = service.CreateProfile(cmd.Context(), provider, name, homePath)
	}
	if err != nil {
		return fmt.Errorf("manage profile add: %w", err)
	}
	if !bind {
		return output.WriteRecord(cmd.OutOrStdout(), mode, "Profile ready", []output.Field{
			{Label: "provider", Value: string(profile.Provider), Muted: true},
			{Label: "name", Value: profile.Name, Identifier: true},
			{Label: "home", Value: profile.HomePath},
		})
	}

	startPath := strings.TrimSpace(projectPath)
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("manage profile add: resolve working directory: %w", err)
		}
	}
	result, err := service.BindProject(cmd.Context(), provider, profile.Name, startPath)
	if err != nil {
		return fmt.Errorf("manage profile add: bind project: %w", err)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Profile ready and project bound", []output.Field{
		{Label: "project", Value: result.Project.Root, Identifier: true},
		{Label: "provider", Value: string(result.Profile.Provider), Muted: true},
		{Label: "profile", Value: result.Profile.Name, Identifier: true},
		{Label: "home", Value: result.Profile.HomePath},
	})
}

func runManageBind(cmd *cobra.Command, paths config.Paths, opts *rootOptions, provider domain.Provider, profileName string, projectPath string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage bind: %w", err)
	}
	defer closeStore()
	startPath := strings.TrimSpace(projectPath)
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("manage bind: resolve working directory: %w", err)
		}
	}
	result, err := service.BindProject(cmd.Context(), provider, profileName, startPath)
	if err != nil {
		return fmt.Errorf("manage bind: %w", err)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Project binding updated", []output.Field{{Label: "project", Value: result.Project.Root, Identifier: true}, {Label: "provider", Value: string(result.Profile.Provider), Muted: true}, {Label: "profile", Value: result.Profile.Name, Identifier: true}, {Label: "home", Value: result.Profile.HomePath}})
}

func runManageProfileSwitch(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string, projectPath string) error {
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage profile switch: %w", err)
	}
	defer closeStore()

	startPath := strings.TrimSpace(projectPath)
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("manage profile switch: resolve working directory: %w", err)
		}
	}

	provider, profileName, err := resolveProfileSwitchTarget(cmd, service, startPath, args)
	if err != nil {
		return fmt.Errorf("manage profile switch: %w", err)
	}
	if profileName == "" {
		profiles, err := service.ListProfiles(cmd.Context(), provider)
		if err != nil {
			return fmt.Errorf("manage profile switch: list profiles: %w", err)
		}
		profileName, err = pickProfile(cmd, provider, profiles.Profiles)
		if err != nil {
			if errors.Is(err, errSelectionCanceled) {
				return writeNoOpRecord(cmd, opts, "No profile switch made", "no profile selected")
			}
			return fmt.Errorf("manage profile switch: %w", err)
		}
	}
	return runManageBind(cmd, paths, opts, provider, profileName, projectPath)
}

func resolveProfileSwitchTarget(cmd *cobra.Command, service interface {
	Status(context.Context, string) (manageservice.StatusResult, error)
}, startPath string, args []string) (domain.Provider, string, error) {
	defaultProvider := domain.ProviderCodex
	currentProvider := func() (domain.Provider, error) {
		status, err := service.Status(cmd.Context(), startPath)
		if err == nil {
			return status.Binding.Provider, nil
		}
		if strings.Contains(err.Error(), domain.ErrUnboundProject.Error()) {
			return defaultProvider, nil
		}
		return "", err
	}

	switch len(args) {
	case 0:
		provider, err := currentProvider()
		return provider, "", err
	case 1:
		if provider, err := domain.ParseProvider(args[0]); err == nil {
			return provider, "", nil
		}
		provider, err := currentProvider()
		return provider, args[0], err
	case 2:
		provider, err := domain.ParseProvider(args[0])
		if err != nil {
			return "", "", err
		}
		return provider, args[1], nil
	default:
		return "", "", fmt.Errorf("unsupported arg count %d", len(args))
	}
}

func writeProfilesByProvider(cmd *cobra.Command, mode output.Mode, service profileLister) error {
	sections := make([]profileSection, 0, len(supportedProviders()))
	for _, provider := range supportedProviders() {
		result, err := service.ListProfiles(cmd.Context(), provider)
		if err != nil {
			return fmt.Errorf("manage profile list: %w", err)
		}
		sections = append(sections, profileSection{
			Provider: provider,
			Items:    listItemsForProfiles(result.Profiles),
		})
	}

	if mode.Format == domain.OutputFormatJSON {
		payload := struct {
			ProfilesByProvider []profileSection `json:"profiles_by_provider"`
		}{ProfilesByProvider: sections}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			return fmt.Errorf("manage profile list: write json output: %w", err)
		}
		return nil
	}

	for i, section := range sections {
		if i > 0 {
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return fmt.Errorf("manage profile list: separate provider sections: %w", err)
			}
		}
		if err := output.WriteListWithKey(cmd.OutOrStdout(), mode, fmt.Sprintf("%s profiles", section.Provider), "profiles", section.Items); err != nil {
			return fmt.Errorf("manage profile list: write %s section: %w", section.Provider, err)
		}
	}
	return nil
}

type profileLister interface {
	ListProfiles(context.Context, domain.Provider) (manageservice.ProfileListResult, error)
}

type profileSection struct {
	Provider domain.Provider   `json:"provider"`
	Items    []output.ListItem `json:"profiles"`
}

func supportedProviders() []domain.Provider {
	return []domain.Provider{domain.ProviderCodex}
}

func writeNoOpRecord(cmd *cobra.Command, opts *rootOptions, heading, reason string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, heading, []output.Field{{Label: "reason", Value: reason, Muted: true}})
}

func newManageStatusCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current project's Valv binding status",
		Long: strings.TrimSpace(`
Show the resolved project binding Valv will use for the current working tree.

Output fields:
- project: detected project root
- provider: bound provider
- profile: bound Valv profile name
- home: bound provider home path
- git marker: git directory used to detect the project root
`),
		Example: strings.TrimSpace(`
valv manage status
valv manage status --project /absolute/path/to/repo
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runManageStatus(cmd, paths, opts, projectPath)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to inspect instead of the current working directory")
	return cmd
}

func runManageStatus(cmd *cobra.Command, paths config.Paths, opts *rootOptions, projectPath string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage status: %w", err)
	}
	defer closeStore()
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
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Project status", []output.Field{{Label: "project", Value: status.Project.Root, Identifier: true}, {Label: "provider", Value: string(status.Profile.Provider), Muted: true}, {Label: "profile", Value: status.Profile.Name, Identifier: true}, {Label: "home", Value: status.Profile.HomePath}, {Label: "git marker", Value: status.Detected.GitMarker, Muted: true}})
}

func newManageUpdateCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [provider]",
		Short: "Rebuild and rotate provider client images",
		Long: strings.TrimSpace(`
Rebuild the provider client image Valv uses for containerized runtime launches.

Output fields:
- provider: provider whose runtime image was checked
- image: default image tag Valv will run next
- tags: all image tags Valv expects for the current installed client
- version: latest client version Valv resolved and installed
- checked at: latest upstream version check timestamp
- context: generated Docker build context under the Valv cache root
`),
		Example: strings.TrimSpace(`
valv manage update
valv manage update codex
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := parseOptionalProvider(args, domain.ProviderCodex)
			if err != nil {
				return err
			}
			return runManageUpdate(cmd, paths, opts, provider)
		},
	}
	return cmd
}

func runManageUpdate(cmd *cobra.Command, paths config.Paths, opts *rootOptions, provider domain.Provider) error {
	if provider != domain.ProviderCodex {
		return fmt.Errorf("manage update: provider %q is not supported yet", provider)
	}
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeImages, err := openImagesService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage update: initialize image service: %w", err)
	}
	defer closeImages()
	result, err := service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{})
	if err != nil {
		return fmt.Errorf("manage update: %w", err)
	}
	tagValues := make([]string, 0, len(result.Tags))
	for _, tag := range result.Tags {
		tagValues = append(tagValues, tag.String())
	}
	heading := "Provider image updated"
	if result.Action == imagesservice.EnsureActionUpToDate {
		heading = "Provider image up to date"
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, heading, []output.Field{{Label: "provider", Value: string(provider), Muted: true}, {Label: "image", Value: result.Image.String(), Identifier: true}, {Label: "tags", Value: strings.Join(tagValues, ", "), Muted: true}, {Label: "version", Value: result.Version, Identifier: true}, {Label: "checked at", Value: result.LatestCheckedAt.Format(time.RFC3339), Muted: true}, {Label: "context", Value: result.ContextDir, Muted: true}})
}

func newManageCleanupCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup [state|images|docker|all]",
		Short: "Prune Valv-managed local state, runtime containers, and images",
		Long: strings.TrimSpace(`
Clean Valv-managed local state and Docker artifacts.

Scopes:
- state: local Valv logs, caches, and runtime scratch only
- images: provider images only
- docker: Valv-managed runtime containers plus provider images
- all: local state plus Valv-managed runtime containers plus provider images
`),
		Example: strings.TrimSpace(`
valv manage cleanup state
valv manage cleanup images
valv manage cleanup docker
valv manage cleanup all
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := "all"
			if len(args) == 1 {
				scope = strings.ToLower(strings.TrimSpace(args[0]))
			}
			return runManageCleanup(cmd, paths, opts, scope)
		},
	}
	return cmd
}

func runManageCleanup(cmd *cobra.Command, paths config.Paths, opts *rootOptions, scope string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, err := newCleanupService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage cleanup: initialize cleanup service: %w", err)
	}
	local := cleanupservice.LocalCleanupRequest{Paths: cleanupservice.DefaultLocalTargets(paths)}
	dockerRequest := cleanupservice.DockerCleanupRequest{
		ContainerLabels: map[string]string{
			"label": "io.valv.managed=true",
		},
		ImageFilters: providerCleanupImageFilters(),
		Force:        true,
	}

	var summary []output.Field
	switch scope {
	case "state":
		result, err := service.CleanLocal(cmd.Context(), local)
		if err != nil {
			return fmt.Errorf("manage cleanup: %w", err)
		}
		summary = []output.Field{{Label: "scope", Value: "state", Badge: true}, {Label: "removed", Value: fmt.Sprintf("%d paths", len(result.Removed)), Identifier: true}}
	case "images":
		result, err := service.CleanDocker(cmd.Context(), cleanupservice.DockerCleanupRequest{ImageFilters: providerCleanupImageFilters(), Force: true})
		if err != nil {
			return fmt.Errorf("manage cleanup: %w", err)
		}
		summary = []output.Field{{Label: "scope", Value: "images", Badge: true}, {Label: "images", Value: fmt.Sprintf("%d refs", len(result.RemovedImages)), Identifier: true}}
	case "docker":
		result, err := service.CleanDocker(cmd.Context(), dockerRequest)
		if err != nil {
			return fmt.Errorf("manage cleanup: %w", err)
		}
		summary = []output.Field{{Label: "scope", Value: "docker", Badge: true}, {Label: "containers", Value: fmt.Sprintf("%d removed", len(result.RemovedContainers)), Identifier: true}, {Label: "images", Value: fmt.Sprintf("%d refs", len(result.RemovedImages)), Identifier: true}}
	case "all":
		localResult, dockerResult, err := service.Clean(cmd.Context(), local, dockerRequest)
		if err != nil {
			return fmt.Errorf("manage cleanup: %w", err)
		}
		summary = []output.Field{{Label: "scope", Value: "all", Badge: true}, {Label: "removed", Value: fmt.Sprintf("%d paths", len(localResult.Removed)), Identifier: true}, {Label: "containers", Value: fmt.Sprintf("%d removed", len(dockerResult.RemovedContainers)), Identifier: true}, {Label: "images", Value: fmt.Sprintf("%d refs", len(dockerResult.RemovedImages)), Identifier: true}}
	default:
		return fmt.Errorf("manage cleanup: unsupported scope %q", scope)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Cleanup completed", summary)
}

func providerCleanupImageFilters() map[string]string {
	return map[string]string{
		"label": "io.valv.managed=true",
	}
}
