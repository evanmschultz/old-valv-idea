package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	openaihandler "github.com/evanmschultz/valv/internal/api/openai"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/output"
	cleanupservice "github.com/evanmschultz/valv/internal/services/cleanup"
	globalswitchservice "github.com/evanmschultz/valv/internal/services/globalswitch"
	imagesservice "github.com/evanmschultz/valv/internal/services/images"
	manageservice "github.com/evanmschultz/valv/internal/services/manage"
	openaiapiservice "github.com/evanmschultz/valv/internal/services/openaiapi"
	managetui "github.com/evanmschultz/valv/internal/tui/manage"
)

var errSelectionCanceled = errors.New("selection canceled")

func openManageService(cmd *cobra.Command, paths config.Paths) (manageservice.Service, func(), error) {
	store, err := openStore(paths)
	if err != nil {
		return manageservice.Service{}, nil, err
	}
	service, err := manageservice.New(manageservice.Options{
		Store:        store,
		ProviderRoot: paths.ProviderRoot,
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

func newImagesService(cmd *cobra.Command, paths config.Paths) (imagesservice.Service, error) {
	contextDir := filepath.Join(paths.BuildCacheDir, string(domain.ProviderCodex))
	if _, err := imagesservice.WriteDefaultCodexContext(contextDir); err != nil {
		return imagesservice.Service{}, err
	}
	return imagesservice.New(imagesservice.Options{
		Runner:     dockeradapter.NewQuietRunner("docker", LoggerFromContext(cmd.Context())),
		Repository: codexImageRepository(),
		ContextDir: contextDir,
		Dockerfile: "Dockerfile",
		DefaultTag: codexImageTag(),
		UserID:     os.Getuid(),
		GroupID:    os.Getgid(),
		Logger:     LoggerFromContext(cmd.Context()),
	})
}

func newCleanupService(cmd *cobra.Command, paths config.Paths) (cleanupservice.Service, error) {
	return cleanupservice.New(cleanupservice.Options{
		Runner: dockeradapter.NewQuietRunner("docker", LoggerFromContext(cmd.Context())),
		Logger: LoggerFromContext(cmd.Context()),
	})
}

func newOpenAIAPIService(cmd *cobra.Command, paths config.Paths, projectPath string, workspaceAccess bool, idleTTL time.Duration) (openaiapiservice.Service, func(), error) {
	store, err := openStore(paths)
	if err != nil {
		return openaiapiservice.Service{}, nil, err
	}
	service, err := openaiapiservice.New(openaiapiservice.Options{
		Store:           store,
		Executor:        dockeradapter.NewExecutor(dockeradapter.NewQuietRunner("docker", LoggerFromContext(cmd.Context()))),
		Image:           codexImageRef(),
		TempRoot:        paths.TempCacheDir,
		StartPath:       projectPath,
		WorkspaceAccess: workspaceAccess,
		IdleTTL:         idleTTL,
		Logger:          LoggerFromContext(cmd.Context()),
	})
	if err != nil {
		_ = store.Close()
		return openaiapiservice.Service{}, nil, err
	}
	return service, func() { _ = store.Close() }, nil
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
		return "", fmt.Errorf("no %s profiles found; run `valv manage profile add %s <name>` first", provider, provider)
	}
	if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout()) {
		return "", fmt.Errorf("profile is required when not running in a TTY")
	}
	sorted := append([]domain.Profile(nil), profiles...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	program := tea.NewProgram(managetui.NewProfilePicker(provider, sorted), tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
	finalModel, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("run profile picker: %w", err)
	}
	model, ok := finalModel.(managetui.ProfilePickerModel)
	if !ok {
		return "", fmt.Errorf("run profile picker: unexpected final model %T", finalModel)
	}
	selected, ok := model.Selected()
	if !ok {
		return "", errSelectionCanceled
	}
	return selected, nil
}

func listItemsForProfiles(profiles []domain.Profile) []output.ListItem {
	items := make([]output.ListItem, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, output.ListItem{
			Title: profile.Name,
			Fields: []output.Field{
				{Label: "provider", Value: string(profile.Provider), Muted: true},
				{Label: "home", Value: profile.HomePath, Identifier: true},
			},
		})
	}
	return items
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

var _ = openaihandler.ChatCompletionsPath
