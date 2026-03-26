package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/output"
	cleanupservice "github.com/evanmschultz/valv/internal/services/cleanup"
	imagesservice "github.com/evanmschultz/valv/internal/services/images"
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
valv manage profile add codex dev
valv manage bind codex dev
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
valv manage profile add codex dev
valv manage profile list codex
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newManageProfileAddCommand(paths, opts))
	cmd.AddCommand(newManageProfileListCommand(paths, opts))
	return cmd
}

func newManageProfileAddCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var homePath string
	cmd := &cobra.Command{
		Use:   "add <provider> <name>",
		Short: "Create a Valv-managed provider profile",
		Long: strings.TrimSpace(`
Create one provider profile and ensure its home directory exists.

Output fields:
- provider: provider family for the profile
- name: Valv profile name
- home: host path mounted into provider runtimes as that profile's home
`),
		Example: strings.TrimSpace(`
valv manage profile add codex dev
valv manage profile add codex work --home "$HOME/.codex"
`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := commandOutputMode(cmd, opts)
			if err != nil {
				return fmt.Errorf("resolve output policy: %w", err)
			}
			provider, err := domain.ParseProvider(args[0])
			if err != nil {
				return err
			}
			service, closeStore, err := openManageService(cmd, paths)
			if err != nil {
				return fmt.Errorf("manage profile add: %w", err)
			}
			defer closeStore()
			profile, err := service.CreateProfile(cmd.Context(), provider, args[1], homePath)
			if err != nil {
				return fmt.Errorf("manage profile add: %w", err)
			}
			return output.WriteRecord(cmd.OutOrStdout(), mode, "Profile created", []output.Field{{Label: "provider", Value: string(profile.Provider), Muted: true}, {Label: "name", Value: profile.Name, Identifier: true}, {Label: "home", Value: profile.HomePath}})
		},
	}
	cmd.Flags().StringVar(&homePath, "home", "", "explicit provider profile home path")
	return cmd
}

func newManageProfileListCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <provider>",
		Short: "List Valv-managed provider profiles",
		Long: strings.TrimSpace(`
List the profiles Valv knows for one provider.

Human output shows the profile name and mounted home path. JSON output uses one stable command-owned top-level key.
`),
		Example: strings.TrimSpace(`
valv manage profile list codex
valv manage profile list codex --format json
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := commandOutputMode(cmd, opts)
			if err != nil {
				return fmt.Errorf("resolve output policy: %w", err)
			}
			provider, err := domain.ParseProvider(args[0])
			if err != nil {
				return err
			}
			service, closeStore, err := openManageService(cmd, paths)
			if err != nil {
				return fmt.Errorf("manage profile list: %w", err)
			}
			defer closeStore()
			result, err := service.ListProfiles(cmd.Context(), provider)
			if err != nil {
				return fmt.Errorf("manage profile list: %w", err)
			}
			return output.WriteListWithKey(cmd.OutOrStdout(), mode, fmt.Sprintf("%s profiles", provider), "profiles", listItemsForProfiles(result.Profiles))
		},
	}
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
valv manage bind codex dev
valv manage bind codex dev --project /absolute/path/to/repo
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
		return fmt.Errorf("manage bind: %w", err)
	}
	return runManageBind(cmd, paths, opts, domain.ProviderCodex, selected, "")
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
- provider: provider whose runtime image was rebuilt
- image: default image tag Valv will run next
- tags: all image tags refreshed by the rebuild
- version: pinned client version installed into the image
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
	service, err := newImagesService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage update: initialize image service: %w", err)
	}
	result, err := service.Update(cmd.Context(), imagesservice.UpdateRequest{BuildRequest: imagesservice.BuildRequest{Version: imagesservice.DefaultCodexVersion, ExtraTags: []dockeradapter.ImageRef{codexImageVersionRef(imagesservice.DefaultCodexVersion)}}})
	if err != nil {
		return fmt.Errorf("manage update: %w", err)
	}
	tagValues := make([]string, 0, len(result.Tags))
	for _, tag := range result.Tags {
		tagValues = append(tagValues, tag.String())
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Provider image rebuilt", []output.Field{{Label: "provider", Value: string(provider), Muted: true}, {Label: "image", Value: result.Image.String(), Identifier: true}, {Label: "tags", Value: strings.Join(tagValues, ", "), Muted: true}, {Label: "version", Value: result.Version, Identifier: true}, {Label: "context", Value: result.ContextDir, Muted: true}})
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
	imageRefs := providerCleanupImageRefs()
	dockerRequest := cleanupservice.DockerCleanupRequest{
		ContainerLabels: map[string]string{
			"label": "io.valv.managed=true",
		},
		ImageRefs: imageRefs,
		Force:     true,
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
		if _, err := service.CleanDocker(cmd.Context(), cleanupservice.DockerCleanupRequest{ImageRefs: imageRefs, Force: true}); err != nil {
			return fmt.Errorf("manage cleanup: %w", err)
		}
		summary = []output.Field{{Label: "scope", Value: "images", Badge: true}, {Label: "images", Value: fmt.Sprintf("%d refs", len(imageRefs)), Identifier: true}}
	case "docker":
		result, err := service.CleanDocker(cmd.Context(), dockerRequest)
		if err != nil {
			return fmt.Errorf("manage cleanup: %w", err)
		}
		summary = []output.Field{{Label: "scope", Value: "docker", Badge: true}, {Label: "containers", Value: fmt.Sprintf("%d removed", len(result.RemovedContainers)), Identifier: true}, {Label: "images", Value: fmt.Sprintf("%d refs", len(imageRefs)), Identifier: true}}
	case "all":
		localResult, dockerResult, err := service.Clean(cmd.Context(), local, dockerRequest)
		if err != nil {
			return fmt.Errorf("manage cleanup: %w", err)
		}
		summary = []output.Field{{Label: "scope", Value: "all", Badge: true}, {Label: "removed", Value: fmt.Sprintf("%d paths", len(localResult.Removed)), Identifier: true}, {Label: "containers", Value: fmt.Sprintf("%d removed", len(dockerResult.RemovedContainers)), Identifier: true}, {Label: "images", Value: fmt.Sprintf("%d refs", len(imageRefs)), Identifier: true}}
	default:
		return fmt.Errorf("manage cleanup: unsupported scope %q", scope)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Cleanup completed", summary)
}

func providerCleanupImageRefs() []dockeradapter.ImageRef {
	return []dockeradapter.ImageRef{
		codexImageRef(),
		codexImageVersionRef(imagesservice.DefaultCodexVersion),
	}
}
