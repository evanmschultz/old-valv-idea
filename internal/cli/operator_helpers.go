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
	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	codexprovider "github.com/evanmschultz/valv/internal/adapters/providers/codex"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/output"
	"github.com/evanmschultz/valv/internal/project"
	cleanupservice "github.com/evanmschultz/valv/internal/services/cleanup"
	globalswitchservice "github.com/evanmschultz/valv/internal/services/globalswitch"
	imagesservice "github.com/evanmschultz/valv/internal/services/images"
	manageservice "github.com/evanmschultz/valv/internal/services/manage"
	"github.com/evanmschultz/valv/internal/tools"
	managetui "github.com/evanmschultz/valv/internal/tui/manage"
)

var (
	errSelectionCanceled         = errors.New("selection canceled")
	codexVersionResolverFactory  = imagesservice.NewCodexVersionResolver
	claudeVersionResolverFactory = imagesservice.NewClaudeVersionResolver
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
		CachePath:  filepath.Join(paths.CachesDir, "version-cache.json"),
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
		options.Resolver = claudeVersionResolverFactory(nil)
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
		workingDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("run manage tui: resolve working directory: %w", err)
		}
		provider, err := detectGlobalSwitchProviderFn(cmd, paths, workingDir)
		if err != nil {
			return fmt.Errorf("run manage tui: %w", err)
		}
		return runGlobalSwitch(cmd, paths, opts, provider, "")
	default:
		return fmt.Errorf("run manage tui: unsupported action %q", action)
	}
}

// detectGlobalSwitchProviderFn is the injectable implementation of the
// provider-detection step used by the ActionGlobalSwitch TUI dispatch path.
// Tests stub this to control dispatch without a real SQLite store.
//
// Non-parallel: tests mutating this var must NOT call t.Parallel().
var detectGlobalSwitchProviderFn = realDetectGlobalSwitchProvider

// realDetectGlobalSwitchProvider detects which provider is bound for the given
// working directory, applying the two-probe policy:
//
//  1. Try Claude first. nil error → return ProviderClaude (Claude wins when
//     both bindings exist — step-1-first order encodes this policy).
//     ErrUnboundProject → fall through to step 2.
//     Any other error → wrap with "detect globalswitch provider: %w" and return.
//
//  2. Try Codex. nil error → return ProviderCodex.
//     ErrUnboundProject → return ProviderCodex (backward-compatible fallback for
//     projects with no binding at all).
//     Any other error → wrap and return.
func realDetectGlobalSwitchProvider(cmd *cobra.Command, paths config.Paths, workingDir string) (domain.Provider, error) {
	service, closeStore, err := openManageService(cmd, paths)
	if err != nil {
		return "", fmt.Errorf("detect globalswitch provider: %w", err)
	}
	defer closeStore()

	// Step 1: probe Claude.
	_, err = service.StatusForProvider(cmd.Context(), workingDir, domain.ProviderClaude)
	if err == nil {
		return domain.ProviderClaude, nil
	}
	if !errors.Is(err, domain.ErrUnboundProject) {
		return "", fmt.Errorf("detect globalswitch provider: %w", err)
	}

	// Step 2: probe Codex.
	_, err = service.StatusForProvider(cmd.Context(), workingDir, domain.ProviderCodex)
	if err == nil {
		return domain.ProviderCodex, nil
	}
	if errors.Is(err, domain.ErrUnboundProject) {
		// Neither provider is bound — fall back to Codex for backward compatibility.
		return domain.ProviderCodex, nil
	}
	return "", fmt.Errorf("detect globalswitch provider: %w", err)
}

// pickProfileFn is the injectable implementation of the profile picker used
// in tests. Production code uses the real Bubble Tea picker.
var pickProfileFn = realPickProfile

func pickProfile(cmd *cobra.Command, provider domain.Provider, profiles []domain.Profile) (domain.Profile, error) {
	return pickProfileFn(cmd, provider, profiles)
}

func realPickProfile(cmd *cobra.Command, provider domain.Provider, profiles []domain.Profile) (domain.Profile, error) {
	if len(profiles) == 0 {
		return domain.Profile{}, fmt.Errorf("no %s accounts found; run `valv account add %s` for the default host-backed account or `valv account add %s account-name` for an isolated account first", provider, provider, provider)
	}
	if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout()) {
		return domain.Profile{}, fmt.Errorf("account is required when not running in a TTY")
	}
	sorted := append([]domain.Profile(nil), profiles...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	program := tea.NewProgram(managetui.NewProfilePicker(provider, sorted), tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
	finalModel, err := program.Run()
	if err != nil {
		return domain.Profile{}, fmt.Errorf("run account picker: %w", err)
	}
	model, ok := finalModel.(managetui.ProfilePickerModel)
	if !ok {
		return domain.Profile{}, fmt.Errorf("run account picker: unexpected final model %T", finalModel)
	}
	selected, ok := model.Selected()
	if !ok {
		return domain.Profile{}, errSelectionCanceled
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
	case domain.ProviderClaude:
		identity, err := claudeprovider.ReadAccountIdentity(profile.HomePath)
		if err != nil {
			return accountDisplayIdentity{
				authDisplay:  "unavailable",
				emailDisplay: "(unavailable)",
			}
		}
		return accountDisplayIdentity{
			authDisplay:  claudeAuthDisplay(identity),
			emailDisplay: claudeEmailDisplay(identity),
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

// claudeAuthDisplay returns a human-readable auth state string for a Claude
// account identity. It mirrors the shape of codexAuthDisplay.
func claudeAuthDisplay(identity claudeprovider.AccountIdentity) string {
	if identity.LoggedIn {
		return "logged in"
	}
	return "not logged in"
}

// claudeEmailDisplay returns a human-readable email string for a Claude account
// identity. It mirrors the shape of codexEmailDisplay.
func claudeEmailDisplay(identity claudeprovider.AccountIdentity) string {
	if email := strings.TrimSpace(identity.Email); email != "" {
		return email
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

// projectImageOverrideEnvName returns the VALV_<PROVIDER>_IMAGE environment
// variable name used by the existing image-ref resolver for the given
// provider. Returns "" for an unsupported provider; callers treat that as
// "no override applies" and proceed with the EnsureProjectImage path.
func projectImageOverrideEnvName(provider domain.Provider) string {
	switch provider {
	case domain.ProviderClaude:
		return "VALV_CLAUDE_IMAGE"
	case domain.ProviderCodex:
		return "VALV_CODEX_IMAGE"
	default:
		return ""
	}
}

// resolveProjectImage returns the image reference the binding-aware launch
// path should use for the given provider in the given working directory.
//
// Behavior follows DROP_12 Unit 12.4 (PLAN.md decisions 12 + 13) plus
// DROP_15 Unit 15.0 (schema decision 8 — project-root-based resolution):
//
//   - First resolves the project root from workingDir via project.DetectFrom
//     so a repo-subdirectory launch sees the same `.valv/tools.toml` as a
//     repo-root launch. Falls back to workingDir itself when no git marker
//     is found, matching project.DetectFrom's documented behavior.
//   - When tools.Resolve reports an empty manifest (no `.valv/tools.toml`,
//     or a file with no [tools] entries) at the detected project root,
//     returns baseRef unchanged. No docker calls, no overlay build.
//   - When the manifest is non-empty AND VALV_<PROVIDER>_IMAGE is set, the
//     env-var override wins: the function emits a single stderr warning and
//     returns baseRef (which the upstream claudeImageRef/codexImageRef has
//     already resolved to the override value). The overlay is skipped to
//     keep the override path semantically identical to its pre-DROP_12
//     behavior. The override applies AFTER project-root manifest
//     resolution so subdir invocations behave identically to root
//     invocations.
//   - Otherwise, opens the images service for the provider, calls
//     EnsureProjectImage with the resolved manifest and baseRef, and returns
//     the per-project tag from the result. Any tools.Resolve error other
//     than the absent-file case (already handled by tools.Resolve itself
//     returning {Manifest:{}, nil}) is wrapped and returned.
func resolveProjectImage(cmd *cobra.Command, paths config.Paths, provider domain.Provider, workingDir string, baseRef dockeradapter.ImageRef) (dockeradapter.ImageRef, error) {
	detected, err := project.DetectFrom(workingDir)
	if err != nil {
		return dockeradapter.ImageRef{}, fmt.Errorf("resolve project image: %w", err)
	}

	manifest, err := tools.Resolve(detected.Root)
	if err != nil {
		return dockeradapter.ImageRef{}, fmt.Errorf("resolve project image: %w", err)
	}
	if len(manifest.Tools) == 0 {
		return baseRef, nil
	}

	if envName := projectImageOverrideEnvName(provider); envName != "" {
		if strings.TrimSpace(os.Getenv(envName)) != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+envName+" override active; .valv/tools.toml overlay skipped")
			return baseRef, nil
		}
	}

	service, closeImages, err := openImagesService(cmd, paths, provider)
	if err != nil {
		return dockeradapter.ImageRef{}, fmt.Errorf("resolve project image: %w", err)
	}
	defer closeImages()

	result, err := service.EnsureProjectImage(cmd.Context(), imagesservice.EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err != nil {
		return dockeradapter.ImageRef{}, fmt.Errorf("resolve project image: %w", err)
	}
	return result.Image, nil
}
