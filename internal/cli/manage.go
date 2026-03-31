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

Use the management surface to create provider accounts, bind projects, rebuild provider images, and clean up Valv-managed state.

This surface owns setup and repair flows. The direct ` + "`valv codex ...`" + ` path stays a Codex pass-through launcher.
`),
		Example: strings.TrimSpace(`
valv manage account add codex
valv manage account add codex work
valv manage account list
valv manage project list
valv manage status
valv manage update
valv manage cleanup all
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runManageHome(cmd, paths, opts)
		},
	}

	cmd.AddCommand(newManageAccountCommand(paths, opts))
	cmd.AddCommand(newManageBindCommand(paths, opts))
	cmd.AddCommand(newManageProjectCommand(paths, opts))
	cmd.AddCommand(newManageStatusCommand(paths, opts))
	cmd.AddCommand(newManageUpdateCommand(paths, opts))
	cmd.AddCommand(newManageCleanupCommand(paths, opts))
	installBranchHelpCommands(cmd)
	return cmd
}

func newManageAccountCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Manage Valv provider accounts",
		Long: strings.TrimSpace(`
Manage Valv provider accounts.

Accounts are the user-facing way to manage provider homes, auth, session history, and MCP config.

For Codex, that home path becomes ` + "`CODEX_HOME`" + ` inside the container.
`),
		Example: strings.TrimSpace(`
valv manage account add codex
valv manage account add codex work
valv manage account switch
valv manage account switch work
valv manage account list
valv manage account list codex
valv manage account inspect
valv manage account rename personal hylla
valv manage account delete hylla
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newManageAccountAddCommand(paths, opts))
	cmd.AddCommand(newManageAccountInspectCommand(paths, opts))
	cmd.AddCommand(newManageAccountListCommand(paths, opts))
	cmd.AddCommand(newManageAccountRenameCommand(paths, opts))
	cmd.AddCommand(newManageAccountDeleteCommand(paths, opts))
	cmd.AddCommand(newManageAccountSwitchCommand(paths, opts))
	return cmd
}

func newManageAccountInspectCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	cmd := &cobra.Command{
		Use:     "inspect [provider] [account]",
		Aliases: []string{"whoami"},
		Short:   "Show one account's resolved identity and home",
		Long: strings.TrimSpace(`
Show one provider account's resolved auth identity, mounted home, and how many projects are currently bound to it.

With no args, Valv inspects the account currently bound to the detected project. With one arg, Valv treats it as an account name under the current provider unless it parses as a provider.
`),
		Example: strings.TrimSpace(`
valv manage account inspect
valv manage account inspect personal
valv manage account inspect codex personal
valv manage account whoami
`),
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountInspect(cmd, paths, opts, args, projectPath)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to inspect instead of the current working directory")
	return cmd
}

func newManageAccountAddCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var homePath string
	var noBind bool
	var skipLogin bool
	var projectPath string
	cmd := &cobra.Command{
		Use:   "add <provider> [name]",
		Short: "Create one provider account, log in if needed, and bind it by default",
		Long: strings.TrimSpace(`
Create one provider account, ensure it is logged in on the host, and, by default, bind the current project to it.

Semantics:
- ` + "`valv manage account add codex`" + ` creates or reuses the inferred host-backed default account for Codex
- ` + "`valv manage account add codex work`" + ` creates or reuses an isolated named account under Valv's provider root
- isolated named accounts seed their initial ` + "`config.toml`" + ` from the default host Codex home when available so MCP/tool config carries across without sharing auth state
- ` + "`--no-bind`" + ` keeps the account ready without changing the current project's binding
- ` + "`--skip-login`" + ` skips the host-side Codex login step for advanced automation
- ` + "`--home`" + ` is an expert override for custom host paths

Output fields:
- project: bound project root when Valv also updated the current project binding
- provider: provider family for the account
- account: Valv account name
- auth: current auth mode inferred from the account home
- email: email identity from the account when available
- home: host path mounted into provider runtimes as that account's home
`),
		Example: strings.TrimSpace(`
valv manage account add codex
valv manage account add codex work
valv manage account add codex work --no-bind
valv manage account add codex work --home /absolute/path/to/custom-home --skip-login --no-bind
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
			return runManageAccountAdd(cmd, paths, opts, provider, name, homePath, !noBind, skipLogin, projectPath)
		},
	}
	cmd.Flags().StringVar(&homePath, "home", "", "explicit provider account home path")
	cmd.Flags().BoolVar(&noBind, "no-bind", false, "create or reuse the account without binding the current project")
	cmd.Flags().BoolVar(&skipLogin, "skip-login", false, "skip host-side account login for advanced automation")
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to bind instead of the current working directory")
	return cmd
}

func newManageAccountListCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [provider]",
		Short: "List Valv-managed provider accounts",
		Long: strings.TrimSpace(`
List the accounts Valv knows for one provider or, if no provider is given, group accounts by provider.

Human output shows the account name, auth mode, email identity when available, and mounted home path. JSON output uses one stable command-owned top-level key.
`),
		Example: strings.TrimSpace(`
valv manage account list
valv manage account list codex
valv manage account list codex --format json
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := commandOutputMode(cmd, opts)
			if err != nil {
				return fmt.Errorf("resolve output policy: %w", err)
			}
			service, closeStore, err := openManageService(cmd, paths)
			if err != nil {
				return fmt.Errorf("manage account list: %w", err)
			}
			defer closeStore()
			if len(args) == 1 {
				provider, err := domain.ParseProvider(args[0])
				if err != nil {
					return err
				}
				result, err := service.ListProfiles(cmd.Context(), provider)
				if err != nil {
					return fmt.Errorf("manage account list: %w", err)
				}
				return output.WriteListWithKey(cmd.OutOrStdout(), mode, fmt.Sprintf("%s accounts", provider), "accounts", listItemsForAccounts(result.Profiles))
			}
			return writeAccountsByProvider(cmd, mode, service)
		},
	}
	return cmd
}

func newManageAccountRenameCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename [provider] <account> <new-name>",
		Short: "Rename one provider account without changing its home",
		Long: strings.TrimSpace(`
Rename one provider account while keeping the same provider home, auth state, session history, and MCP config.

With no explicit provider, Valv uses the current project's bound provider or the default supported provider.
`),
		Example: strings.TrimSpace(`
valv manage account rename personal hylla
valv manage account rename codex personal hylla
`),
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountRename(cmd, paths, opts, args)
		},
	}
	return cmd
}

func newManageAccountDeleteCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [provider] <account>",
		Short: "Delete one Valv account record",
		Long: strings.TrimSpace(`
Delete one Valv account record without deleting the underlying home directory on disk.

Valv refuses to delete accounts that are still bound to one or more projects.
`),
		Example: strings.TrimSpace(`
valv manage account delete hylla
valv manage account delete codex hylla
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountDelete(cmd, paths, opts, args)
		},
	}
	return cmd
}

func newManageAccountSwitchCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	var skipLogin bool
	cmd := &cobra.Command{
		Use:   "switch [provider] [account]",
		Short: "Switch the current project's bound account",
		Long: strings.TrimSpace(`
Switch the current project's provider account without leaving the management surface.

When no provider is given, Valv uses the currently bound provider for the project. When no account is given, Valv opens a picker in TTY mode.
`),
		Example: strings.TrimSpace(`
valv manage account switch
valv manage account switch work
valv manage account switch codex work
`),
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountSwitch(cmd, paths, opts, args, projectPath, skipLogin)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to switch instead of the current working directory")
	cmd.Flags().BoolVar(&skipLogin, "skip-login", false, "skip host-side account login for advanced automation")
	return cmd
}

func newManageBindCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	cmd := &cobra.Command{
		Use:   "bind <provider> <account>",
		Short: "Bind the current project to a provider account",
		Long: strings.TrimSpace(`
Bind one detected project root to one provider account.

After binding, direct runtime commands like ` + "`valv codex ...`" + ` can resolve the account without additional setup.

Output fields:
- project: detected project root now bound in Valv
- provider: provider family for the account
- account: Valv account name
- auth: current auth mode inferred from the account home
- email: email identity from the account when available
- home: host path mounted into provider runtimes as that account's home
`),
		Example: strings.TrimSpace(`
valv manage bind codex work
valv manage bind codex work --project /absolute/path/to/repo
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

func newManageProjectCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Inspect project-to-account bindings",
		Long: strings.TrimSpace(`
Inspect the project paths Valv knows and the provider accounts currently bound to them.
`),
		Example: strings.TrimSpace(`
valv manage project list
valv manage project list codex
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newManageProjectListCommand(paths, opts))
	return cmd
}

func newManageProjectListCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [provider]",
		Short: "List known project bindings",
		Long: strings.TrimSpace(`
List the detected project roots Valv knows and the provider accounts currently bound to each one.

Output fields:
- project: detected project root
- provider: bound provider
- account: bound account name
- auth: current auth mode inferred from the bound account home
- email: email identity from the bound account when available
- home: bound provider home path
`),
		Example: strings.TrimSpace(`
valv manage project list
valv manage project list codex
valv manage project list codex --format json
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageProjectList(cmd, paths, opts, args)
		},
	}
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
		return fmt.Errorf("manage bind: list accounts: %w", err)
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

func runManageAccountAdd(cmd *cobra.Command, paths config.Paths, opts *rootOptions, provider domain.Provider, name string, homePath string, bind bool, skipLogin bool, projectPath string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage account add: %w", err)
	}
	defer closeStore()

	name = strings.TrimSpace(name)
	homePath = strings.TrimSpace(homePath)

	var profile domain.Profile
	if name == "" {
		if homePath != "" {
			spec, specErr := service.DefaultHostProfile(provider)
			if specErr != nil {
				return fmt.Errorf("manage account add: %w", specErr)
			}
			profile, err = service.CreateProfile(cmd.Context(), provider, spec.Name, homePath)
		} else {
			profile, err = service.CreateDefaultHostProfile(cmd.Context(), provider)
		}
	} else {
		profile, err = service.CreateProfile(cmd.Context(), provider, name, homePath)
	}
	if err != nil {
		return fmt.Errorf("manage account add: %w", err)
	}
	if err := ensureManagedAccountReady(cmd, provider, profile, accountAuthOptions{SkipLogin: skipLogin}); err != nil {
		return fmt.Errorf("manage account add: %w", err)
	}
	identity := readAccountIdentity(profile)
	if !bind {
		return output.WriteRecord(cmd.OutOrStdout(), mode, "Account ready", []output.Field{
			{Label: "provider", Value: string(profile.Provider), Muted: true},
			{Label: "account", Value: profile.Name, Identifier: true},
			{Label: "auth", Value: identity.authDisplay, Muted: true},
			{Label: "email", Value: identity.emailDisplay},
			{Label: "home", Value: profile.HomePath},
		})
	}

	startPath := strings.TrimSpace(projectPath)
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("manage account add: resolve working directory: %w", err)
		}
	}
	result, err := service.BindProject(cmd.Context(), provider, profile.Name, startPath)
	if err != nil {
		return fmt.Errorf("manage account add: bind project: %w", err)
	}
	identity = readAccountIdentity(result.Profile)
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Account ready and project bound", []output.Field{
		{Label: "project", Value: result.Project.Root, Identifier: true},
		{Label: "provider", Value: string(result.Profile.Provider), Muted: true},
		{Label: "account", Value: result.Profile.Name, Identifier: true},
		{Label: "auth", Value: identity.authDisplay, Muted: true},
		{Label: "email", Value: identity.emailDisplay},
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
	identity := readAccountIdentity(result.Profile)
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Project binding updated", []output.Field{
		{Label: "project", Value: result.Project.Root, Identifier: true},
		{Label: "provider", Value: string(result.Profile.Provider), Muted: true},
		{Label: "account", Value: result.Profile.Name, Identifier: true},
		{Label: "auth", Value: identity.authDisplay, Muted: true},
		{Label: "email", Value: identity.emailDisplay},
		{Label: "home", Value: result.Profile.HomePath},
	})
}

func runManageAccountSwitch(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string, projectPath string, skipLogin bool) error {
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage account switch: %w", err)
	}
	defer closeStore()

	startPath := strings.TrimSpace(projectPath)
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("manage account switch: resolve working directory: %w", err)
		}
	}

	provider, profileName, err := resolveProfileSwitchTarget(cmd, service, startPath, args)
	if err != nil {
		return fmt.Errorf("manage account switch: %w", err)
	}
	if profileName == "" {
		profiles, err := service.ListProfiles(cmd.Context(), provider)
		if err != nil {
			return fmt.Errorf("manage account switch: list accounts: %w", err)
		}
		profileName, err = pickProfile(cmd, provider, profiles.Profiles)
		if err != nil {
			if errors.Is(err, errSelectionCanceled) {
				return writeNoOpRecord(cmd, opts, "No account switch made", "no account selected")
			}
			return fmt.Errorf("manage account switch: %w", err)
		}
	}
	account, err := service.ProfileByName(cmd.Context(), provider, profileName)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("manage account switch: account %q not found for provider %q; run `valv manage account add %s %s` or `valv manage account list %s`", profileName, provider, provider, profileName, provider)
		}
		return fmt.Errorf("manage account switch: resolve account %q: %w", profileName, err)
	}
	if err := ensureManagedAccountReady(cmd, provider, account, accountAuthOptions{SkipLogin: skipLogin}); err != nil {
		return fmt.Errorf("manage account switch: %w", err)
	}
	return runManageBind(cmd, paths, opts, provider, profileName, projectPath)
}

func runManageAccountInspect(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string, projectPath string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage account inspect: %w", err)
	}
	defer closeStore()

	startPath := strings.TrimSpace(projectPath)
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("manage account inspect: resolve working directory: %w", err)
		}
	}

	provider, profileName, err := resolveProfileSwitchTarget(cmd, service, startPath, args)
	if err != nil {
		return fmt.Errorf("manage account inspect: %w", err)
	}
	var profile domain.Profile
	if profileName == "" {
		status, err := service.Status(cmd.Context(), startPath)
		if err != nil {
			return fmt.Errorf("manage account inspect: %w", err)
		}
		profile = status.Profile
	} else {
		profile, err = service.ProfileByName(cmd.Context(), provider, profileName)
		if err != nil {
			return fmt.Errorf("manage account inspect: resolve account %q: %w", profileName, err)
		}
	}
	bindings, err := service.ListBindings(cmd.Context(), profile.Provider)
	if err != nil {
		return fmt.Errorf("manage account inspect: %w", err)
	}
	boundProjects := 0
	for _, binding := range bindings {
		if binding.Profile.ID == profile.ID {
			boundProjects++
		}
	}
	identity := readAccountIdentity(profile)
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Account details", []output.Field{
		{Label: "provider", Value: string(profile.Provider), Muted: true},
		{Label: "account", Value: profile.Name, Identifier: true},
		{Label: "auth", Value: identity.authDisplay, Muted: true},
		{Label: "email", Value: identity.emailDisplay},
		{Label: "home", Value: profile.HomePath},
		{Label: "bound projects", Value: fmt.Sprintf("%d", boundProjects), Muted: true},
	})
}

func runManageAccountRename(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage account rename: %w", err)
	}
	defer closeStore()

	provider, currentName, newName, err := resolveRenameArgs(args)
	if err != nil {
		return fmt.Errorf("manage account rename: %w", err)
	}
	if provider == "" {
		provider = domain.ProviderCodex
	}
	profile, err := service.RenameProfile(cmd.Context(), provider, currentName, newName)
	if err != nil {
		return fmt.Errorf("manage account rename: %w", err)
	}
	identity := readAccountIdentity(profile)
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Account renamed", []output.Field{
		{Label: "provider", Value: string(profile.Provider), Muted: true},
		{Label: "account", Value: profile.Name, Identifier: true},
		{Label: "auth", Value: identity.authDisplay, Muted: true},
		{Label: "email", Value: identity.emailDisplay},
		{Label: "home", Value: profile.HomePath},
	})
}

func runManageAccountDelete(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage account delete: %w", err)
	}
	defer closeStore()

	provider, profileName, err := resolveDeleteArgs(args)
	if err != nil {
		return fmt.Errorf("manage account delete: %w", err)
	}
	if provider == "" {
		provider = domain.ProviderCodex
	}
	profile, err := service.DeleteProfile(cmd.Context(), provider, profileName)
	if err != nil {
		return fmt.Errorf("manage account delete: %w", err)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Account deleted", []output.Field{
		{Label: "provider", Value: string(profile.Provider), Muted: true},
		{Label: "account", Value: profile.Name, Identifier: true},
		{Label: "home", Value: profile.HomePath},
		{Label: "result", Value: "record removed; home retained", Muted: true},
	})
}

func resolveProfileSwitchTarget(cmd *cobra.Command, service interface {
	Status(context.Context, string) (manageservice.StatusResult, error)
}, startPath string, args []string,
) (domain.Provider, string, error) {
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

func resolveRenameArgs(args []string) (domain.Provider, string, string, error) {
	switch len(args) {
	case 2:
		return domain.ProviderCodex, args[0], args[1], nil
	case 3:
		provider, err := domain.ParseProvider(args[0])
		if err != nil {
			return "", "", "", err
		}
		return provider, args[1], args[2], nil
	default:
		return "", "", "", fmt.Errorf("unsupported arg count %d", len(args))
	}
}

func resolveDeleteArgs(args []string) (domain.Provider, string, error) {
	switch len(args) {
	case 1:
		return domain.ProviderCodex, args[0], nil
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

func writeAccountsByProvider(cmd *cobra.Command, mode output.Mode, service accountLister) error {
	sections := make([]accountSection, 0, len(supportedProviders()))
	for _, provider := range supportedProviders() {
		result, err := service.ListProfiles(cmd.Context(), provider)
		if err != nil {
			return fmt.Errorf("manage account list: %w", err)
		}
		sections = append(sections, accountSection{
			Provider: provider,
			Items:    listItemsForAccounts(result.Profiles),
		})
	}

	if mode.Format == domain.OutputFormatJSON {
		payload := struct {
			AccountsByProvider []accountSection `json:"accounts_by_provider"`
		}{AccountsByProvider: sections}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			return fmt.Errorf("manage account list: write json output: %w", err)
		}
		return nil
	}

	for i, section := range sections {
		if i > 0 && mode.Format != domain.OutputFormatHuman {
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return fmt.Errorf("manage account list: separate provider sections: %w", err)
			}
		}
		if err := output.WriteListWithKey(cmd.OutOrStdout(), mode, fmt.Sprintf("%s accounts", section.Provider), "accounts", section.Items); err != nil {
			return fmt.Errorf("manage account list: write %s section: %w", section.Provider, err)
		}
	}
	return nil
}

func runManageProjectList(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage project list: %w", err)
	}
	defer closeStore()

	provider, err := parseOptionalProvider(args, "")
	if err != nil {
		return err
	}
	bindings, err := service.ListBindings(cmd.Context(), provider)
	if err != nil {
		return fmt.Errorf("manage project list: %w", err)
	}
	return output.WriteListWithKey(cmd.OutOrStdout(), mode, "project bindings", "projects", listItemsForBindings(bindings))
}

type accountLister interface {
	ListProfiles(context.Context, domain.Provider) (manageservice.ProfileListResult, error)
}

type accountSection struct {
	Provider domain.Provider   `json:"provider"`
	Items    []output.ListItem `json:"accounts"`
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
- account: bound Valv account name
- auth: current auth mode inferred from the bound account home
- email: email identity from the bound account when available
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
	identity := readAccountIdentity(status.Profile)
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Project status", []output.Field{{Label: "project", Value: status.Project.Root, Identifier: true}, {Label: "provider", Value: string(status.Profile.Provider), Muted: true}, {Label: "account", Value: status.Profile.Name, Identifier: true}, {Label: "auth", Value: identity.authDisplay, Muted: true}, {Label: "email", Value: identity.emailDisplay}, {Label: "home", Value: status.Profile.HomePath}, {Label: "git marker", Value: status.Detected.GitMarker, Muted: true}})
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
	var result imagesservice.EnsureResult
	err = runWithCLIQuietSpinner(
		cmd.ErrOrStderr(),
		"Checking provider image",
		"Provider image check complete",
		"Provider image update failed",
		func() error {
			var runErr error
			result, runErr = service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{})
			return runErr
		},
	)
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
	case "state", "images", "docker", "all":
	default:
		return fmt.Errorf("manage cleanup: unsupported scope %q", scope)
	}
	err = runWithCLIQuietSpinner(
		cmd.ErrOrStderr(),
		fmt.Sprintf("Running %s cleanup", scope),
		"Cleanup complete",
		"Cleanup failed",
		func() error {
			switch scope {
			case "state":
				result, err := service.CleanLocal(cmd.Context(), local)
				if err != nil {
					return err
				}
				summary = []output.Field{{Label: "scope", Value: "state", Badge: true}, {Label: "removed", Value: fmt.Sprintf("%d paths", len(result.Removed)), Identifier: true}}
			case "images":
				result, err := service.CleanDocker(cmd.Context(), cleanupservice.DockerCleanupRequest{ImageFilters: providerCleanupImageFilters(), Force: true})
				if err != nil {
					return err
				}
				summary = []output.Field{{Label: "scope", Value: "images", Badge: true}, {Label: "images", Value: fmt.Sprintf("%d refs", len(result.RemovedImages)), Identifier: true}}
			case "docker":
				result, err := service.CleanDocker(cmd.Context(), dockerRequest)
				if err != nil {
					return err
				}
				summary = []output.Field{{Label: "scope", Value: "docker", Badge: true}, {Label: "containers", Value: fmt.Sprintf("%d removed", len(result.RemovedContainers)), Identifier: true}, {Label: "images", Value: fmt.Sprintf("%d refs", len(result.RemovedImages)), Identifier: true}}
			case "all":
				localResult, dockerResult, err := service.Clean(cmd.Context(), local, dockerRequest)
				if err != nil {
					return err
				}
				summary = []output.Field{{Label: "scope", Value: "all", Badge: true}, {Label: "removed", Value: fmt.Sprintf("%d paths", len(localResult.Removed)), Identifier: true}, {Label: "containers", Value: fmt.Sprintf("%d removed", len(dockerResult.RemovedContainers)), Identifier: true}, {Label: "images", Value: fmt.Sprintf("%d refs", len(dockerResult.RemovedImages)), Identifier: true}}
			}
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("manage cleanup: %w", err)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Cleanup completed", summary)
}

func providerCleanupImageFilters() map[string]string {
	return map[string]string{
		"label": "io.valv.managed=true",
	}
}
