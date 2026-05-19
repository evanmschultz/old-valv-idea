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
valv account add codex
valv account add codex work
valv account switch
valv account switch work
valv account list
valv account list codex
valv account inspect
valv account login
valv account logout
valv account rename personal hylla
valv account cleanup
valv account delete hylla
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newManageAccountAddCommand(paths, opts))
	cmd.AddCommand(newManageAccountBindCommand(paths, opts))
	cmd.AddCommand(newManageAccountUnbindCommand(paths, opts))
	cmd.AddCommand(newManageAccountInspectCommand(paths, opts))
	cmd.AddCommand(newManageAccountLoginCommand(paths, opts))
	cmd.AddCommand(newManageAccountLogoutCommand(paths, opts))
	cmd.AddCommand(newManageAccountListCommand(paths, opts))
	cmd.AddCommand(newManageAccountRenameCommand(paths, opts))
	cmd.AddCommand(newManageAccountCleanupCommand(paths, opts))
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
valv account inspect
valv account inspect personal
valv account inspect codex personal
valv account whoami
`),
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountInspect(cmd, paths, opts, args, projectPath)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to inspect instead of the current working directory")
	return cmd
}

func newManageAccountLoginCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	cmd := &cobra.Command{
		Use:   "login [provider] [account]",
		Short: "Log in one existing provider account on the host",
		Long: strings.TrimSpace(`
Log in one existing provider account on the host without changing the current project binding.

With no args, Valv uses the account currently bound to the detected project.
`),
		Example: strings.TrimSpace(`
valv account login
valv account login hylla
valv account login codex hylla
`),
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountLogin(cmd, paths, opts, args, projectPath)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to inspect instead of the current working directory")
	return cmd
}

func newManageAccountLogoutCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	cmd := &cobra.Command{
		Use:   "logout [provider] [account]",
		Short: "Log out one existing provider account on the host",
		Long: strings.TrimSpace(`
Log out one existing provider account on the host without deleting the Valv account record or changing project bindings.

With no args, Valv uses the account currently bound to the detected project.
`),
		Example: strings.TrimSpace(`
valv account logout
valv account logout hylla
valv account logout codex hylla
`),
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountLogout(cmd, paths, opts, args, projectPath)
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
- ` + "`valv account add codex`" + ` creates or reuses the inferred host-backed default account for Codex
- ` + "`valv account add codex work`" + ` creates or reuses an isolated named account under Valv's provider root
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
valv account add codex
valv account add codex work
valv account add codex work --no-bind
valv account add codex work --home /absolute/path/to/custom-home --skip-login --no-bind
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
valv account list
valv account list codex
valv account list codex --format json
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
valv account rename personal hylla
valv account rename codex personal hylla
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
valv account delete hylla
valv account delete codex hylla
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountDelete(cmd, paths, opts, args)
		},
	}
	return cmd
}

func newManageAccountCleanupCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup [provider]",
		Short: "Remove stale unbound same-home account aliases",
		Long: strings.TrimSpace(`
Remove stale unbound same-home account aliases while preserving the canonical account Valv should present for that home path.

This is intended to clean up historical duplicate host-home aliases such as ` + "`host`" + ` or ` + "`host-codex`" + ` after renames.
`),
		Example: strings.TrimSpace(`
valv account cleanup
valv account cleanup codex
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountCleanup(cmd, paths, opts, args)
		},
	}
	return cmd
}

func newManageAccountSwitchCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	var skipLogin bool
	var providerFlag string
	cmd := &cobra.Command{
		Use:   "switch [provider] [account]",
		Short: "Switch the current project's bound account",
		Long: strings.TrimSpace(`
Switch the current project's provider account without leaving the management surface.

When no provider is given, Valv uses the currently bound provider for the project. When no account is given, Valv opens a picker in TTY mode.

Use --provider to switch to an account from a specific provider, which is required when the same account name exists in multiple providers.
`),
		Example: strings.TrimSpace(`
valv account switch
valv account switch work
valv account switch codex work
valv account switch work --provider codex
valv account switch --provider claude
`),
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageAccountSwitch(cmd, paths, opts, args, projectPath, skipLogin, providerFlag)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to switch instead of the current working directory")
	cmd.Flags().BoolVar(&skipLogin, "skip-login", false, "skip host-side account login for advanced automation")
	cmd.Flags().StringVar(&providerFlag, "provider", "", "provider to use for account resolution (required when account name exists in multiple providers)")
	return cmd
}

func newManageAccountBindCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	var providerFlag string
	cmd := &cobra.Command{
		Use:   "bind [provider] <account>",
		Short: "Bind the current project to a provider account",
		Long: strings.TrimSpace(`
Bind one detected project root to one provider account.

After binding, direct runtime commands like ` + "`valv codex ...`" + ` can resolve the account without additional setup.

With one arg, Valv treats it as an account name and uses the default provider (codex). With two args, the first is the provider and the second is the account name.

Output fields:
- project: detected project root now bound in Valv
- provider: provider family for the account
- account: Valv account name
- auth: current auth mode inferred from the account home
- email: email identity from the account when available
- home: host path mounted into provider runtimes as that account's home
`),
		Example: strings.TrimSpace(`
valv account bind profile-name
valv account bind codex profile-name
valv account bind profile-name --provider codex
valv account bind profile-name --project /absolute/path/to/repo
`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var provider domain.Provider
			var profileName string
			if len(args) == 2 {
				p, err := domain.ParseProvider(args[0])
				if err != nil {
					return err
				}
				provider = p
				profileName = args[1]
			} else {
				profileName = args[0]
				if providerFlag != "" {
					p, err := domain.ParseProvider(providerFlag)
					if err != nil {
						return err
					}
					provider = p
				} else {
					provider = domain.ProviderCodex
				}
			}
			return runManageBind(cmd, paths, opts, provider, profileName, projectPath)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to bind instead of the current working directory")
	cmd.Flags().StringVar(&providerFlag, "provider", "", "provider to use for binding (codex or claude; defaults to codex)")
	return cmd
}

func newManageAccountUnbindCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var projectPath string
	var providerFlag string
	cmd := &cobra.Command{
		Use:   "unbind",
		Short: "Unbind the current project from its provider account",
		Long: strings.TrimSpace(`
Remove the binding between the detected project root and the currently bound provider account.

After unbinding, runtime commands like ` + "`valv codex ...`" + ` will no longer resolve an account for the project.
`),
		Example: strings.TrimSpace(`
valv account unbind
valv account unbind --provider codex
valv account unbind --provider claude
valv account unbind --project /absolute/path/to/repo
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runManageAccountUnbind(cmd, paths, opts, projectPath, providerFlag)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to unbind instead of the current working directory")
	cmd.Flags().StringVar(&providerFlag, "provider", "", "provider to unbind (codex or claude; defaults to codex)")
	return cmd
}

func runManageAccountUnbind(cmd *cobra.Command, paths config.Paths, opts *rootOptions, projectPath, providerFlag string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("account unbind: %w", err)
	}
	defer closeStore()

	provider := domain.ProviderCodex
	if providerFlag != "" {
		p, err := domain.ParseProvider(providerFlag)
		if err != nil {
			return err
		}
		provider = p
	}

	startPath := strings.TrimSpace(projectPath)
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("account unbind: resolve working directory: %w", err)
		}
	}

	if err := service.UnbindProject(cmd.Context(), provider, startPath); err != nil {
		return fmt.Errorf("account unbind: %w", err)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Project unbound", []output.Field{
		{Label: "provider", Value: string(provider), Muted: true},
		{Label: "project", Value: startPath, Identifier: true},
	})
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
valv account bind codex work
valv account bind codex work --project /absolute/path/to/repo
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
		return fmt.Errorf("manage bind: list accounts: %w", err)
	}
	selectedProfile, err := pickProfile(cmd, domain.ProviderCodex, profiles.Profiles)
	if err != nil {
		if errors.Is(err, errSelectionCanceled) {
			return nil
		}
		return fmt.Errorf("manage bind: %w", err)
	}
	return runManageBind(cmd, paths, opts, domain.ProviderCodex, selectedProfile.Name, "")
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
	// For Claude, ensure the provider image is built locally before auth.
	// Path B auth runs claude inside a container; without the image, docker run
	// fails before the OAuth prompt can render. Codex uses a host subprocess and
	// does not need this guard.
	if provider == domain.ProviderClaude && !skipLogin {
		if err := ensureClaudeImageCurrent(cmd, paths); err != nil {
			return fmt.Errorf("manage account add: %w", err)
		}
	}
	if err := ensureManagedAccountReady(cmd, provider, profile, accountAuthOptions{SkipLogin: skipLogin, Paths: paths}); err != nil {
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

func runManageAccountSwitch(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string, projectPath string, skipLogin bool, providerFlag string) error {
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

	provider, profileName, err := resolveAccountSwitchTarget(cmd, service, startPath, args, providerFlag)
	if err != nil {
		return fmt.Errorf("manage account switch: %w", err)
	}
	if profileName == "" {
		// Cross-provider picker: provider=="" means no provider was resolved yet;
		// collect all accounts across supported providers and open one picker.
		if provider == "" {
			allProfiles, pickedProvider, pickerErr := pickProfileCrossProvider(cmd, service)
			if pickerErr != nil {
				if errors.Is(pickerErr, errSelectionCanceled) {
					return writeNoOpRecord(cmd, opts, "No account switch made", "no account selected")
				}
				return fmt.Errorf("manage account switch: %w", pickerErr)
			}
			provider = pickedProvider
			profileName = allProfiles
		} else {
			profiles, listErr := service.ListProfiles(cmd.Context(), provider)
			if listErr != nil {
				return fmt.Errorf("manage account switch: list accounts: %w", listErr)
			}
			pickedProfile, pickErr := pickProfile(cmd, provider, profiles.Profiles)
			if pickErr != nil {
				if errors.Is(pickErr, errSelectionCanceled) {
					return writeNoOpRecord(cmd, opts, "No account switch made", "no account selected")
				}
				return fmt.Errorf("manage account switch: %w", pickErr)
			}
			profileName = pickedProfile.Name
		}
	}
	account, err := service.ProfileByName(cmd.Context(), provider, profileName)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("manage account switch: account %q not found for provider %q; run `valv account add %s %s` or `valv account list %s`", profileName, provider, provider, profileName, provider)
		}
		return fmt.Errorf("manage account switch: resolve account %q: %w", profileName, err)
	}
	// For Claude, ensure the provider image is built before auth (mirrors
	// runManageAccountAdd). Path B auth runs inside a container; image must
	// exist locally before docker run can start the OAuth prompt.
	if provider == domain.ProviderClaude && !skipLogin {
		if err := ensureClaudeImageCurrent(cmd, paths); err != nil {
			return fmt.Errorf("manage account switch: %w", err)
		}
	}
	if err := ensureManagedAccountReady(cmd, provider, account, accountAuthOptions{SkipLogin: skipLogin, Paths: paths}); err != nil {
		return fmt.Errorf("manage account switch: %w", err)
	}
	return runManageBind(cmd, paths, opts, provider, profileName, projectPath)
}

func runManageAccountLogin(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string, projectPath string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage account login: %w", err)
	}
	defer closeStore()
	profile, err := resolveManagedAccount(cmd, service, args, projectPath)
	if err != nil {
		return fmt.Errorf("manage account login: %w", err)
	}
	if err := loginManagedAccount(cmd, profile.Provider, profile, paths); err != nil {
		return fmt.Errorf("manage account login: %w", err)
	}
	identity := readAccountIdentity(profile)
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Account ready", []output.Field{
		{Label: "provider", Value: string(profile.Provider), Muted: true},
		{Label: "account", Value: profile.Name, Identifier: true},
		{Label: "auth", Value: identity.authDisplay, Muted: true},
		{Label: "email", Value: identity.emailDisplay},
		{Label: "home", Value: profile.HomePath},
	})
}

func runManageAccountLogout(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string, projectPath string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage account logout: %w", err)
	}
	defer closeStore()
	profile, err := resolveManagedAccount(cmd, service, args, projectPath)
	if err != nil {
		return fmt.Errorf("manage account logout: %w", err)
	}
	if err := logoutManagedAccount(cmd, profile.Provider, profile); err != nil {
		return fmt.Errorf("manage account logout: %w", err)
	}
	identity := readAccountIdentity(profile)
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Account logged out", []output.Field{
		{Label: "provider", Value: string(profile.Provider), Muted: true},
		{Label: "account", Value: profile.Name, Identifier: true},
		{Label: "auth", Value: identity.authDisplay, Muted: true},
		{Label: "email", Value: identity.emailDisplay},
		{Label: "home", Value: profile.HomePath},
	})
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

func runManageAccountCleanup(cmd *cobra.Command, paths config.Paths, opts *rootOptions, args []string) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("manage account cleanup: %w", err)
	}
	defer closeStore()
	provider, err := parseOptionalProvider(args, domain.ProviderCodex)
	if err != nil {
		return err
	}
	result, err := service.CleanupDuplicateAliases(cmd.Context(), provider)
	if err != nil {
		return fmt.Errorf("manage account cleanup: %w", err)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Account cleanup complete", []output.Field{
		{Label: "provider", Value: string(provider), Muted: true},
		{Label: "deleted", Value: strings.Join(profileNames(result.Deleted), ", "), Muted: len(result.Deleted) == 0},
		{Label: "kept", Value: strings.Join(profileNames(result.Kept), ", "), Muted: len(result.Kept) == 0},
		{Label: "skipped", Value: strings.Join(profileNames(result.Skipped), ", "), Muted: len(result.Skipped) == 0},
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
		if errors.Is(err, domain.ErrUnboundProject) {
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

// resolveAccountSwitchTarget is the cross-provider resolution function for
// "account switch". It implements the four-step resolution order from the
// Unit 8.2 AC:
//
//  1. --provider flag present → parse it; positional arg (if any) is the
//     account name. Returns (provider, name, nil) where name may be "".
//  2. No flag, one positional arg that parses as a provider → that is the
//     provider; name is "" (picker will open for that provider).
//  3. No flag, one positional arg that is NOT a provider → cross-provider
//     name search: exactly-one-match resolves; multi-match returns an error
//     listing all (provider, account) pairs; no-match returns not-found.
//  4. No flag, no positional arg → returns ("", "", nil), signalling the
//     caller to open a cross-provider picker.
//
// Two positional args with no flag follow the legacy "provider account" form
// (step 2 extended): first arg must parse as a provider, second is the name.
type accountSwitchResolver interface {
	ProfileByName(context.Context, domain.Provider, string) (domain.Profile, error)
}

func resolveAccountSwitchTarget(
	cmd *cobra.Command,
	service accountSwitchResolver,
	_ string,
	args []string,
	providerFlag string,
) (domain.Provider, string, error) {
	// Step 1: explicit --provider flag overrides everything.
	if providerFlag != "" {
		p, err := domain.ParseProvider(providerFlag)
		if err != nil {
			return "", "", err
		}
		name := ""
		if len(args) == 1 {
			name = args[0]
		} else if len(args) > 1 {
			return "", "", fmt.Errorf("expected at most one account name with --provider, got %d args", len(args))
		}
		return p, name, nil
	}

	// Step 2 / legacy two-arg: "codex work".
	if len(args) == 2 {
		p, err := domain.ParseProvider(args[0])
		if err != nil {
			return "", "", err
		}
		return p, args[1], nil
	}

	// Step 4: no flag, no positional → signal caller to open cross-provider picker.
	if len(args) == 0 {
		return "", "", nil
	}

	// Step 2 (one arg): if the positional arg parses as a provider, that is
	// the provider; the caller will open a per-provider picker.
	if p, err := domain.ParseProvider(args[0]); err == nil {
		return p, "", nil
	}

	// Step 3: positional arg is an account name — search across all providers.
	accountName := args[0]
	type match struct {
		provider domain.Provider
	}
	var matches []match
	for _, p := range supportedProviders() {
		_, lookupErr := service.ProfileByName(cmd.Context(), p, accountName)
		if lookupErr == nil {
			matches = append(matches, match{provider: p})
			continue
		}
		if errors.Is(lookupErr, domain.ErrNotFound) {
			continue
		}
		// Unexpected error — surface it.
		return "", "", fmt.Errorf("look up account %q for provider %q: %w", accountName, p, lookupErr)
	}
	switch len(matches) {
	case 0:
		return "", "", fmt.Errorf("account %q not found in any provider; run `valv account add codex %s` or `valv account list` to see all available accounts", accountName, accountName)
	case 1:
		return matches[0].provider, accountName, nil
	default:
		// Multi-match: build an error listing all (provider, account) pairs.
		pairs := make([]string, 0, len(matches))
		for _, m := range matches {
			pairs = append(pairs, fmt.Sprintf("(%s, %s)", m.provider, accountName))
		}
		return "", "", fmt.Errorf("account %q found in multiple providers: %s; use --provider to specify which one", accountName, strings.Join(pairs, ", "))
	}
}

// pickProfileCrossProvider collects all profiles from every supported
// provider and presents them in a single picker. It returns the selected
// profile name and the provider it belongs to. Because Selected() now
// returns the full domain.Profile (including Provider), no post-pick
// re-resolution is needed — the picker result is authoritative.
type crossProviderLister interface {
	ListProfiles(context.Context, domain.Provider) (manageservice.ProfileListResult, error)
}

func pickProfileCrossProvider(cmd *cobra.Command, service crossProviderLister) (string, domain.Provider, error) {
	var allProfiles []domain.Profile
	for _, p := range supportedProviders() {
		result, err := service.ListProfiles(cmd.Context(), p)
		if err != nil {
			return "", "", fmt.Errorf("list %s accounts: %w", p, err)
		}
		allProfiles = append(allProfiles, result.Profiles...)
	}
	// FIX 2: guard before calling the picker so the "all" sentinel never
	// leaks into user-facing error text.
	if len(allProfiles) == 0 {
		return "", "", fmt.Errorf("no accounts found across any provider; run `valv account add codex <name>` or `valv account add claude <name>` to create one")
	}
	// Use the generic provider label for the cross-provider picker.
	// Selected() returns a full domain.Profile with Provider set, so the
	// result is unambiguous even when the same name exists in multiple providers.
	selected, err := pickProfile(cmd, domain.Provider("all"), allProfiles)
	if err != nil {
		return "", "", err
	}
	return selected.Name, selected.Provider, nil
}

func resolveManagedAccount(cmd *cobra.Command, service interface {
	Status(context.Context, string) (manageservice.StatusResult, error)
	ProfileByName(context.Context, domain.Provider, string) (domain.Profile, error)
}, args []string, projectPath string,
) (domain.Profile, error) {
	startPath := strings.TrimSpace(projectPath)
	var err error
	if startPath == "" {
		startPath, err = os.Getwd()
		if err != nil {
			return domain.Profile{}, fmt.Errorf("resolve working directory: %w", err)
		}
	}
	provider, profileName, err := resolveProfileSwitchTarget(cmd, service, startPath, args)
	if err != nil {
		return domain.Profile{}, err
	}
	if profileName == "" {
		status, err := service.Status(cmd.Context(), startPath)
		if err != nil {
			return domain.Profile{}, err
		}
		return status.Profile, nil
	}
	profile, err := service.ProfileByName(cmd.Context(), provider, profileName)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("resolve account %q: %w", profileName, err)
	}
	return profile, nil
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

func profileNames(profiles []domain.Profile) []string {
	if len(profiles) == 0 {
		return []string{"(none)"}
	}
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}
	return names
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

type accountLister interface {
	ListProfiles(context.Context, domain.Provider) (manageservice.ProfileListResult, error)
}

type accountSection struct {
	Provider domain.Provider   `json:"provider"`
	Items    []output.ListItem `json:"accounts"`
}

func supportedProviders() []domain.Provider {
	return []domain.Provider{domain.ProviderCodex, domain.ProviderClaude}
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
	var all bool
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

Use --all to list all project bindings across all providers instead of the
current project's binding.
`),
		Example: strings.TrimSpace(`
valv status
valv status --project /absolute/path/to/repo
valv status --all
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all {
				return runStatusAll(cmd, paths, opts)
			}
			return runManageStatus(cmd, paths, opts, projectPath)
		},
	}
	cmd.Flags().StringVar(&projectPath, "project", "", "explicit project path to inspect instead of the current working directory")
	cmd.Flags().BoolVar(&all, "all", false, "list all project bindings across all providers")
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

// runStatusAll lists all known project bindings across all providers.
// It is the implementation of `valv status --all`, replacing the deleted
// `manage project list` command. The empty provider filter returns bindings
// for all providers, matching the previous `manage project list` default behavior.
func runStatusAll(cmd *cobra.Command, paths config.Paths, opts *rootOptions) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return fmt.Errorf("status --all: %w", err)
	}
	defer closeStore()
	bindings, err := service.ListBindings(cmd.Context(), "")
	if err != nil {
		return fmt.Errorf("status --all: %w", err)
	}
	return output.WriteListWithKey(cmd.OutOrStdout(), mode, "project bindings", "projects", listItemsForBindings(bindings))
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
valv image update
valv image update codex
valv image update claude
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
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	switch provider {
	case domain.ProviderCodex:
		return runManageUpdateCodex(cmd, paths, mode)
	case domain.ProviderClaude:
		return runManageUpdateClaude(cmd, paths, mode)
	default:
		return fmt.Errorf("manage update: provider %q is not supported yet", provider)
	}
}

func runManageUpdateCodex(cmd *cobra.Command, paths config.Paths, mode output.Mode) error {
	service, closeImages, err := openImagesService(cmd, paths, domain.ProviderCodex)
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
	return output.WriteRecord(cmd.OutOrStdout(), mode, heading, []output.Field{{Label: "provider", Value: string(domain.ProviderCodex), Muted: true}, {Label: "image", Value: result.Image.String(), Identifier: true}, {Label: "tags", Value: strings.Join(tagValues, ", "), Muted: true}, {Label: "version", Value: result.Version, Identifier: true}, {Label: "checked at", Value: result.LatestCheckedAt.Format(time.RFC3339), Muted: true}, {Label: "context", Value: result.ContextDir, Muted: true}})
}

func runManageUpdateClaude(cmd *cobra.Command, paths config.Paths, mode output.Mode) error {
	service, closeImages, err := openImagesService(cmd, paths, domain.ProviderClaude)
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
	return output.WriteRecord(cmd.OutOrStdout(), mode, heading, []output.Field{{Label: "provider", Value: string(domain.ProviderClaude), Muted: true}, {Label: "image", Value: result.Image.String(), Identifier: true}, {Label: "tags", Value: strings.Join(tagValues, ", "), Muted: true}, {Label: "version", Value: result.Version, Identifier: true}, {Label: "checked at", Value: result.LatestCheckedAt.Format(time.RFC3339), Muted: true}, {Label: "context", Value: result.ContextDir, Muted: true}})
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
valv cleanup state
valv cleanup images
valv cleanup docker
valv cleanup all
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

// newImageCommand constructs the "image" namespace cobra command with three
// subcommands: update, cleanup, and inspect. It is registered in root.go under
// the "account" group alongside the account and global commands.
func newImageCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Manage Valv provider images",
		Long: strings.TrimSpace(`
Manage the Docker images Valv uses for containerized provider runtimes.

Subcommands:
- update: rebuild and rotate provider client images
- cleanup: prune Valv-managed Docker artifacts (dry-run by default)
- inspect: show the current provider image state
`),
		Example: strings.TrimSpace(`
valv image update
valv image update codex
valv image update claude
valv image cleanup
valv image cleanup --images
valv image cleanup --all --apply
valv image inspect
valv image inspect claude
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newImageUpdateCommand(paths, opts))
	cmd.AddCommand(newImageCleanupCommand(paths, opts))
	cmd.AddCommand(newImageInspectCommand(paths, opts))
	return cmd
}

// newImageUpdateCommand constructs "valv image update [provider]". It calls the
// existing runManageUpdate run function, which already handles both Codex and
// Claude providers. Provider defaults to Codex when omitted.
func newImageUpdateCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
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
valv image update
valv image update codex
valv image update claude
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

// imageCleanupFlags holds the flag values for "valv image cleanup".
type imageCleanupFlags struct {
	images     bool
	containers bool
	state      bool
	buildCache bool
	all        bool
	apply      bool
}

// newImageCleanupCommand constructs "valv image cleanup". It uses a flag-driven
// interface rather than positional args: --images, --containers, --state,
// --build-cache, --all (scope selection) and --apply (disable dry-run).
//
// Flag combination rules:
//   - --all is mutually exclusive with individual scope flags.
//   - Individual scope flags are additive.
//   - No scope flag = equivalent to --all.
//   - Default is dry-run; --apply (or --yes) actually deletes.
func newImageCleanupCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	var flags imageCleanupFlags
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Prune Valv-managed Docker artifacts (dry-run by default)",
		Long: strings.TrimSpace(`
Prune Valv-managed Docker artifacts.

Only Valv-managed artifacts are touched (those with the io.valv.managed=true label).

Flags control which artifact categories are included:
  --images        provider images
  --containers    leaked Valv runtime containers
  --state         local logs, caches, and runtime scratch
  --build-cache   Docker builder cache
  --all           all of the above (mutually exclusive with individual scope flags)

By default, this command performs a dry-run and prints what would be cleaned
without deleting anything. Pass --apply to actually execute the cleanup.
`),
		Example: strings.TrimSpace(`
valv image cleanup
valv image cleanup --images
valv image cleanup --containers --state
valv image cleanup --all
valv image cleanup --all --apply
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runImageCleanup(cmd, paths, opts, flags)
		},
	}
	cmd.Flags().BoolVar(&flags.images, "images", false, "include provider images")
	cmd.Flags().BoolVar(&flags.containers, "containers", false, "include leaked Valv runtime containers")
	cmd.Flags().BoolVar(&flags.state, "state", false, "include local logs, caches, and runtime scratch")
	cmd.Flags().BoolVar(&flags.buildCache, "build-cache", false, "include Docker builder cache")
	cmd.Flags().BoolVar(&flags.all, "all", false, "include all Valv-managed artifacts (mutually exclusive with individual scope flags)")
	cmd.Flags().BoolVar(&flags.apply, "apply", false, "actually execute the cleanup (default is dry-run)")
	cmd.Flags().BoolVar(&flags.apply, "yes", false, "alias for --apply")
	return cmd
}

func runImageCleanup(cmd *cobra.Command, paths config.Paths, opts *rootOptions, flags imageCleanupFlags) error {
	// Validate --all mutual exclusivity with individual scope flags.
	if flags.all && (flags.images || flags.containers || flags.state || flags.buildCache) {
		return fmt.Errorf("--all is mutually exclusive with --images, --containers, --state, --build-cache")
	}

	// Default (no scope flag): equivalent to --all.
	noScopeSet := !flags.images && !flags.containers && !flags.state && !flags.buildCache && !flags.all
	effectiveAll := flags.all || noScopeSet

	// Determine which scopes are active.
	doImages := effectiveAll || flags.images
	doContainers := effectiveAll || flags.containers
	doState := effectiveAll || flags.state
	doBuildCache := effectiveAll || flags.buildCache

	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}

	// Dry-run: report what would be cleaned without executing.
	if !flags.apply {
		var scopes []string
		if doImages {
			scopes = append(scopes, "images")
		}
		if doContainers {
			scopes = append(scopes, "containers")
		}
		if doState {
			scopes = append(scopes, "state")
		}
		if doBuildCache {
			scopes = append(scopes, "build-cache")
		}
		return output.WriteRecord(cmd.OutOrStdout(), mode, "Cleanup dry-run (pass --apply to execute)", []output.Field{
			{Label: "scopes", Value: strings.Join(scopes, ", "), Identifier: true},
			{Label: "dry-run", Value: "true", Muted: true},
		})
	}

	service, err := newCleanupService(cmd, paths)
	if err != nil {
		return fmt.Errorf("image cleanup: initialize cleanup service: %w", err)
	}
	local := cleanupservice.LocalCleanupRequest{Paths: cleanupservice.DefaultLocalTargets(paths)}
	dockerRequest := cleanupservice.DockerCleanupRequest{
		Force: true,
	}
	if doImages {
		dockerRequest.ImageFilters = providerCleanupImageFilters()
	}
	if doContainers {
		dockerRequest.ContainerLabels = map[string]string{
			"label": "io.valv.managed=true",
		}
	}
	if doBuildCache {
		dockerRequest.PruneBuilder = true
		dockerRequest.PruneBuilderAll = true
	}

	var summary []output.Field
	err = runWithCLIQuietSpinner(
		cmd.ErrOrStderr(),
		"Running image cleanup",
		"Cleanup complete",
		"Cleanup failed",
		func() error {
			if doState {
				localResult, localErr := service.CleanLocal(cmd.Context(), local)
				if localErr != nil {
					return localErr
				}
				summary = append(summary, output.Field{Label: "removed paths", Value: fmt.Sprintf("%d", len(localResult.Removed)), Identifier: true})
			}
			if doImages || doContainers || doBuildCache {
				dockerResult, dockerErr := service.CleanDocker(cmd.Context(), dockerRequest)
				if dockerErr != nil {
					return dockerErr
				}
				if doContainers {
					summary = append(summary, output.Field{Label: "containers", Value: fmt.Sprintf("%d removed", len(dockerResult.RemovedContainers)), Identifier: true})
				}
				if doImages {
					summary = append(summary, output.Field{Label: "images", Value: fmt.Sprintf("%d refs", len(dockerResult.RemovedImages)), Identifier: true})
				}
				if doBuildCache {
					summary = append(summary, output.Field{Label: "build-cache", Value: "pruned", Muted: true})
				}
			}
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("image cleanup: %w", err)
	}
	if len(summary) == 0 {
		summary = []output.Field{{Label: "result", Value: "nothing to clean", Muted: true}}
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Cleanup completed", summary)
}

// newImageInspectCommand constructs "valv image inspect [provider]". It reads
// the current provider image state from the store (via service.CurrentState)
// without making any network calls. Provider defaults to Codex when omitted.
//
// Implementation choice: service.CurrentState is the minimal path — it reads
// the SQLite state record for the provider without calling EnsureLatest (no
// network, no Docker call). If no state record exists, it reports "not installed".
func newImageInspectCommand(paths config.Paths, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect [provider]",
		Short: "Show current provider image version and installed state",
		Long: strings.TrimSpace(`
Show the current provider image version, last-checked-at timestamp, and installed state.

Reads from the Valv state store — no network calls or Docker calls are made.

Output fields:
- provider: the queried provider
- installed version: the currently installed image version
- latest version: the last resolved upstream version
- image ref: the installed Docker image reference
- checked at: the last upstream version check timestamp
`),
		Example: strings.TrimSpace(`
valv image inspect
valv image inspect codex
valv image inspect claude
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := parseOptionalProvider(args, domain.ProviderCodex)
			if err != nil {
				return err
			}
			return runImageInspect(cmd, paths, opts, provider)
		},
	}
	return cmd
}

func runImageInspect(cmd *cobra.Command, paths config.Paths, opts *rootOptions, provider domain.Provider) error {
	mode, err := commandOutputMode(cmd, opts)
	if err != nil {
		return fmt.Errorf("resolve output policy: %w", err)
	}
	service, closeImages, err := openImagesService(cmd, paths, provider)
	if err != nil {
		return fmt.Errorf("image inspect: initialize image service: %w", err)
	}
	defer closeImages()

	state, err := service.CurrentState(cmd.Context())
	if err != nil {
		return fmt.Errorf("image inspect: %w", err)
	}

	if state.InstalledVersion == "" {
		return output.WriteRecord(cmd.OutOrStdout(), mode, "Provider image not installed", []output.Field{
			{Label: "provider", Value: string(provider), Muted: true},
			{Label: "installed", Value: "false", Muted: true},
		})
	}

	checkedAt := ""
	if !state.LatestCheckedAt.IsZero() {
		checkedAt = state.LatestCheckedAt.Format(time.RFC3339)
	}
	return output.WriteRecord(cmd.OutOrStdout(), mode, "Provider image state", []output.Field{
		{Label: "provider", Value: string(provider), Muted: true},
		{Label: "installed version", Value: state.InstalledVersion, Identifier: true},
		{Label: "latest version", Value: state.LatestVersion, Muted: state.LatestVersion == ""},
		{Label: "image ref", Value: state.InstalledImageRef, Identifier: true},
		{Label: "checked at", Value: checkedAt, Muted: checkedAt == ""},
	})
}
