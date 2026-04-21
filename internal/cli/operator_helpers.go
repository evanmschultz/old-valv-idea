package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	codexprovider "github.com/evanmschultz/valv/internal/adapters/providers/codex"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/output"
	cleanupservice "github.com/evanmschultz/valv/internal/services/cleanup"
	globalswitchservice "github.com/evanmschultz/valv/internal/services/globalswitch"
	imagesservice "github.com/evanmschultz/valv/internal/services/images"
	manageservice "github.com/evanmschultz/valv/internal/services/manage"
	managetui "github.com/evanmschultz/valv/internal/tui/manage"
)

var (
	errSelectionCanceled        = errors.New("selection canceled")
	codexVersionResolverFactory = imagesservice.NewCodexVersionResolver
)

func openManageService(cmd *cobra.Command, paths config.Paths) (manageservice.Service, func(), error) {
	store, err := openStore(paths)
	if err != nil {
		return manageservice.Service{}, nil, err
	}
	service, err := manageservice.New(manageservice.Options{
		Store:        store,
		ProviderRoot: paths.ProviderRoot,
		HomeDir:      paths.HomeDir,
		Logger:       LoggerFromContext(cmd.Context()),
	})
	if err != nil {
		_ = store.Close()
		return manageservice.Service{}, nil, err
	}
	return service, func() { _ = store.Close() }, nil
}

func openGlobalSwitchService(cmd *cobra.Command, paths config.Paths) (globalswitchservice.Service, func(), error) {
	store, err := openStore(paths)
	if err != nil {
		return globalswitchservice.Service{}, nil, err
	}
	service, err := globalswitchservice.New(globalswitchservice.Options{
		Store:       store,
		HomeDir:     paths.HomeDir,
		RealHomeDir: realHomeDir(),
		StateDir:    paths.StateDir,
		Logger:      LoggerFromContext(cmd.Context()),
	})
	if err != nil {
		_ = store.Close()
		return globalswitchservice.Service{}, nil, err
	}
	return service, func() { _ = store.Close() }, nil
}

func openImagesService(cmd *cobra.Command, paths config.Paths, provider domain.Provider) (imagesservice.Service, func(), error) {
	store, err := openStore(paths)
	if err != nil {
		return imagesservice.Service{}, nil, err
	}
	contextDir := filepath.Join(paths.BuildCacheDir, string(provider))
	options := imagesservice.Options{
		Runner:     dockeradapter.NewQuietRunner("docker", LoggerFromContext(cmd.Context())),
		StateStore: store,
		ContextDir: contextDir,
		Dockerfile: "Dockerfile",
		UserID:     os.Getuid(),
		GroupID:    os.Getgid(),
		Logger:     LoggerFromContext(cmd.Context()),
	}
	switch provider {
	case domain.ProviderCodex:
		if _, err := imagesservice.WriteDefaultCodexContext(contextDir); err != nil {
			_ = store.Close()
			return imagesservice.Service{}, nil, err
		}
		options.Resolver = codexVersionResolverFactory(nil)
		options.Repository = codexImageRepository()
		options.DefaultTag = codexImageTag()
		options.Provider = domain.ProviderCodex
	case domain.ProviderClaude:
		if _, err := imagesservice.WriteDefaultClaudeContext(contextDir); err != nil {
			_ = store.Close()
			return imagesservice.Service{}, nil, err
		}
		options.Resolver = nil
		options.Repository = claudeImageRepository()
		options.DefaultTag = claudeImageTag()
		options.Provider = domain.ProviderClaude
	default:
		_ = store.Close()
		return imagesservice.Service{}, nil, fmt.Errorf("initialize image service: unsupported provider %q", provider)
	}
	service, err := imagesservice.New(options)
	if err != nil {
		_ = store.Close()
		return imagesservice.Service{}, nil, err
	}
	return service, func() { _ = store.Close() }, nil
}

func newCleanupService(cmd *cobra.Command, paths config.Paths) (cleanupservice.Service, error) {
	return cleanupservice.New(cleanupservice.Options{
		Runner: dockeradapter.NewQuietRunner("docker", LoggerFromContext(cmd.Context())),
		Logger: LoggerFromContext(cmd.Context()),
	})
}

func runManageHome(cmd *cobra.Command, paths config.Paths, opts *rootOptions) error {
	if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout()) {
		return cmd.Help()
	}
	program := tea.NewProgram(managetui.New(), tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
	finalModel, err := program.Run()
	if err != nil {
		return fmt.Errorf("run manage tui: %w", err)
	}
	model, ok := finalModel.(managetui.Model)
	if !ok {
		return fmt.Errorf("run manage tui: unexpected final model %T", finalModel)
	}
	action, ok := model.Selected()
	if !ok {
		return nil
	}
	switch action {
	case managetui.ActionStatus:
		return runManageStatus(cmd, paths, opts, "")
	case managetui.ActionProfiles:
		return runManageBindInteractive(cmd, paths, opts)
	case managetui.ActionUpdate:
		return runManageUpdate(cmd, paths, opts, domain.ProviderCodex)
	case managetui.ActionCleanup:
		return runManageCleanup(cmd, paths, opts, "all")
	case managetui.ActionGlobalSwitch:
		return runGlobalSwitch(cmd, paths, opts, domain.ProviderCodex, "")
	default:
		return fmt.Errorf("run manage tui: unsupported action %q", action)
	}
}

func pickProfile(cmd *cobra.Command, provider domain.Provider, profiles []domain.Profile) (string, error) {
	if len(profiles) == 0 {
		return "", fmt.Errorf("no %s accounts found; run `valv manage account add %s` for the default host-backed account or `valv manage account add %s account-name` for an isolated account first", provider, provider, provider)
	}
	if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout()) {
		return "", fmt.Errorf("account is required when not running in a TTY")
	}
	sorted := append([]domain.Profile(nil), profiles...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	program := tea.NewProgram(managetui.NewProfilePicker(provider, sorted), tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
	finalModel, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("run account picker: %w", err)
	}
	model, ok := finalModel.(managetui.ProfilePickerModel)
	if !ok {
		return "", fmt.Errorf("run account picker: unexpected final model %T", finalModel)
	}
	selected, ok := model.Selected()
	if !ok {
		return "", errSelectionCanceled
	}
	return selected, nil
}

func listItemsForAccounts(profiles []domain.Profile) []output.ListItem {
	items := make([]output.ListItem, 0, len(profiles))
	for _, profile := range profiles {
		identity := readAccountIdentity(profile)
		items = append(items, output.ListItem{
			Title: profile.Name,
			Fields: []output.Field{
				{Label: "provider", Value: string(profile.Provider), Muted: true},
				{Label: "auth", Value: identity.authDisplay, Muted: true},
				{Label: "email", Value: identity.emailDisplay},
				{Label: "home", Value: profile.HomePath, Identifier: true},
			},
		})
	}
	return items
}

func listItemsForBindings(bindings []manageservice.BindingView) []output.ListItem {
	items := make([]output.ListItem, 0, len(bindings))
	for _, binding := range bindings {
		identity := readAccountIdentity(binding.Profile)
		items = append(items, output.ListItem{
			Title: binding.Project.Root,
			Fields: []output.Field{
				{Label: "provider", Value: string(binding.Profile.Provider), Muted: true},
				{Label: "account", Value: binding.Profile.Name, Identifier: true},
				{Label: "auth", Value: identity.authDisplay, Muted: true},
				{Label: "email", Value: identity.emailDisplay},
				{Label: "home", Value: binding.Profile.HomePath},
			},
		})
	}
	return items
}

type accountDisplayIdentity struct {
	authDisplay  string
	emailDisplay string
}

func readAccountIdentity(profile domain.Profile) accountDisplayIdentity {
	switch profile.Provider {
	case domain.ProviderCodex:
		identity, err := codexprovider.ReadAccountIdentity(profile.HomePath)
		if err != nil {
			return accountDisplayIdentity{
				authDisplay:  "unavailable",
				emailDisplay: "(unavailable)",
			}
		}
		return accountDisplayIdentity{
			authDisplay:  codexAuthDisplay(identity),
			emailDisplay: codexEmailDisplay(identity),
		}
	default:
		return accountDisplayIdentity{
			authDisplay:  "unknown",
			emailDisplay: "(unavailable)",
		}
	}
}

func codexAuthDisplay(identity codexprovider.AccountIdentity) string {
	switch strings.ToLower(strings.TrimSpace(identity.AuthMode)) {
	case "chatgpt":
		return "ChatGPT"
	case "api_key":
		return "API key"
	}
	if identity.LoggedIn {
		return "logged in"
	}
	return "not logged in"
}

func codexEmailDisplay(identity codexprovider.AccountIdentity) string {
	if email := strings.TrimSpace(identity.Email); email != "" {
		return email
	}
	switch strings.ToLower(strings.TrimSpace(identity.AuthMode)) {
	case "api_key":
		return "(api key login)"
	}
	if identity.LoggedIn {
		return "(identity unavailable)"
	}
	return "(not logged in)"
}

func realHomeDir() string {
	if value := strings.TrimSpace(os.Getenv("VALV_REAL_HOME")); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(home)
}

func currentContainerUser() string {
	return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
}

func commandOutputMode(cmd *cobra.Command, opts *rootOptions) (output.Mode, error) {
	policy, err := outputPolicyFromCommand(cmd, opts)
	if err != nil {
		return output.Mode{}, err
	}
	return output.ResolveMode(cmd.OutOrStdout(), policy), nil
}

func parseOptionalProvider(args []string, fallback domain.Provider) (domain.Provider, error) {
	if len(args) == 0 {
		return fallback, nil
	}
	provider, err := domain.ParseProvider(args[0])
	if err != nil {
		return "", err
	}
	return provider, nil
}

func requireProjectPath(value string) string {
	return strings.TrimSpace(value)
}
