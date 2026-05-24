package manage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/log"

	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	codexprovider "github.com/evanmschultz/valv/internal/adapters/providers/codex"
	"github.com/evanmschultz/valv/internal/domain"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

type Store interface {
	domain.ProjectRepository
	domain.BindingRepository
	domain.ProfileRepository
	domain.AccountEnvRepository
}

type DetectFunc func(string) (projectdetect.Result, error)

type Options struct {
	Store        Store
	ProviderRoot string
	HomeDir      string
	Detect       DetectFunc
	Logger       *log.Logger
}

type Service struct {
	store        Store
	providerRoot string
	homeDir      string
	detect       DetectFunc
	logger       *log.Logger
}

type BindResult struct {
	Project domain.Project
	Profile domain.Profile
	Binding domain.ProjectBinding
}

type StatusResult struct {
	Detected projectdetect.Result
	Project  domain.Project
	Profile  domain.Profile
	Binding  domain.ProjectBinding
}

type BindingView struct {
	Project domain.Project
	Profile domain.Profile
	Binding domain.ProjectBinding
}

type ProfileListResult struct {
	Provider domain.Provider
	Profiles []domain.Profile
}

type CleanupAliasesResult struct {
	Deleted []domain.Profile
	Kept    []domain.Profile
	Skipped []domain.Profile
}

type HostProfileSpec struct {
	Provider domain.Provider
	Name     string
	HomePath string
}

func New(options Options) (Service, error) {
	if options.Store == nil {
		return Service{}, fmt.Errorf("new manage service: store is required")
	}
	if strings.TrimSpace(options.ProviderRoot) == "" {
		return Service{}, fmt.Errorf("new manage service: provider root is required")
	}

	detect := options.Detect
	if detect == nil {
		detect = projectdetect.DetectFrom
	}

	return Service{
		store:        options.Store,
		providerRoot: strings.TrimSpace(options.ProviderRoot),
		homeDir:      strings.TrimSpace(options.HomeDir),
		detect:       detect,
		logger:       options.Logger,
	}, nil
}

func (s Service) CreateProfile(ctx context.Context, provider domain.Provider, name, homePath string) (domain.Profile, error) {
	resolvedHome := strings.TrimSpace(homePath)
	if resolvedHome == "" {
		resolvedHome = filepath.Join(s.providerRoot, string(provider), "profiles", strings.TrimSpace(name))
	}
	if err := os.MkdirAll(resolvedHome, 0o755); err != nil {
		return domain.Profile{}, fmt.Errorf("create profile %q: ensure home %q: %w", name, resolvedHome, err)
	}

	profile, err := domain.NewProfile(provider, name, resolvedHome)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("create profile %q: %w", name, err)
	}
	existingByHome, err := s.profileByHome(ctx, provider, profile.HomePath)
	if err == nil {
		if existingByHome.Name == profile.Name {
			s.debug("provider profile already exists", "provider", existingByHome.Provider, "name", existingByHome.Name, "home", existingByHome.HomePath)
			if err := s.seedProfileConfig(existingByHome); err != nil {
				return domain.Profile{}, fmt.Errorf("create profile %q: %w", existingByHome.Name, err)
			}
			return existingByHome, nil
		}
		return domain.Profile{}, fmt.Errorf("create profile %q: home %q already belongs to account %q", profile.Name, existingByHome.HomePath, existingByHome.Name)
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.Profile{}, fmt.Errorf("create profile %q: lookup existing home %q: %w", profile.Name, profile.HomePath, err)
	}
	created, err := s.store.CreateProfile(ctx, profile)
	if err != nil {
		existing, lookupErr := s.store.ProfileByName(ctx, provider, profile.Name)
		if lookupErr == nil {
			if existing.HomePath != profile.HomePath {
				return domain.Profile{}, fmt.Errorf("create profile %q: profile already exists with home %q", profile.Name, existing.HomePath)
			}
			s.debug("provider profile already exists", "provider", existing.Provider, "name", existing.Name, "home", existing.HomePath)
			if err := s.seedProfileConfig(existing); err != nil {
				return domain.Profile{}, fmt.Errorf("create profile %q: %w", existing.Name, err)
			}
			return existing, nil
		}
		return domain.Profile{}, fmt.Errorf("create profile %q: persist profile: %w", profile.Name, err)
	}
	s.debug("created provider profile", "provider", created.Provider, "name", created.Name, "home", created.HomePath)
	if err := s.seedProfileConfig(created); err != nil {
		return domain.Profile{}, fmt.Errorf("create profile %q: %w", created.Name, err)
	}
	return created, nil
}

func (s Service) DefaultHostProfile(provider domain.Provider) (HostProfileSpec, error) {
	switch provider {
	case domain.ProviderCodex:
		name, homePath, err := codexprovider.DefaultHostProfile(s.homeDir)
		if err != nil {
			return HostProfileSpec{}, fmt.Errorf("resolve default host profile for provider %q: %w", provider, err)
		}
		return HostProfileSpec{Provider: provider, Name: name, HomePath: homePath}, nil
	case domain.ProviderClaude:
		name, homePath, err := claudeprovider.DefaultHostProfile(s.homeDir)
		if err != nil {
			return HostProfileSpec{}, fmt.Errorf("resolve default host profile for provider %q: %w", provider, err)
		}
		return HostProfileSpec{Provider: provider, Name: name, HomePath: homePath}, nil
	default:
		return HostProfileSpec{}, fmt.Errorf("resolve default host profile for provider %q: unsupported provider", provider)
	}
}

func (s Service) CreateDefaultHostProfile(ctx context.Context, provider domain.Provider) (domain.Profile, error) {
	spec, err := s.DefaultHostProfile(provider)
	if err != nil {
		return domain.Profile{}, err
	}
	existing, err := s.defaultHostProfileCandidate(ctx, spec)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Profile{}, err
	}
	return s.CreateProfile(ctx, spec.Provider, spec.Name, spec.HomePath)
}

func (s Service) ProfileByName(ctx context.Context, provider domain.Provider, name string) (domain.Profile, error) {
	profile, err := s.store.ProfileByName(ctx, provider, strings.TrimSpace(name))
	if err != nil {
		return domain.Profile{}, fmt.Errorf("lookup profile %q/%q: %w", provider, name, err)
	}
	return profile, nil
}

func (s Service) BindProject(ctx context.Context, provider domain.Provider, profileName, startPath string) (BindResult, error) {
	projectResult, err := s.detect(startPath)
	if err != nil {
		return BindResult{}, fmt.Errorf("bind project: detect project from %q: %w", startPath, err)
	}

	projectRecord, err := s.store.ProjectByRoot(ctx, projectResult.Root)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return BindResult{}, fmt.Errorf("bind project: lookup project %q: %w", projectResult.Root, err)
		}
		projectRecord, err = domain.NewProject(projectResult.Root)
		if err != nil {
			return BindResult{}, fmt.Errorf("bind project: %w", err)
		}
		if _, err := s.store.CreateProject(ctx, projectRecord); err != nil {
			return BindResult{}, fmt.Errorf("bind project: persist project %q: %w", projectRecord.Root, err)
		}
	}

	profile, err := s.store.ProfileByName(ctx, provider, strings.TrimSpace(profileName))
	if err != nil {
		return BindResult{}, fmt.Errorf("bind project: lookup profile %q/%q: %w", provider, profileName, err)
	}

	binding, err := domain.NewProjectBinding(projectRecord.ID, profile.ID, provider)
	if err != nil {
		return BindResult{}, fmt.Errorf("bind project: %w", err)
	}
	if _, err := s.store.UpsertProjectBinding(ctx, binding); err != nil {
		return BindResult{}, fmt.Errorf("bind project: persist binding for project %q: %w", projectRecord.Root, err)
	}

	s.debug("bound project to profile", "project_root", projectRecord.Root, "provider", provider, "profile", profile.Name)
	return BindResult{Project: projectRecord, Profile: profile, Binding: binding}, nil
}

func (s Service) UnbindProject(ctx context.Context, provider domain.Provider, startPath string) error {
	projectResult, err := s.detect(startPath)
	if err != nil {
		return fmt.Errorf("unbind project: detect project from %q: %w", startPath, err)
	}

	projectRecord, err := s.store.ProjectByRoot(ctx, projectResult.Root)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("unbind project: project %q: %w", projectResult.Root, domain.ErrNotFound)
		}
		return fmt.Errorf("unbind project: lookup project %q: %w", projectResult.Root, err)
	}

	if err := s.store.DeleteBinding(ctx, projectRecord.ID, provider); err != nil {
		return fmt.Errorf("unbind project: %w", err)
	}

	s.debug("unbound project from provider", "project_root", projectRecord.Root, "provider", provider)
	return nil
}

func (s Service) Status(ctx context.Context, startPath string) (StatusResult, error) {
	projectResult, err := s.detect(startPath)
	if err != nil {
		return StatusResult{}, fmt.Errorf("manage status: detect project from %q: %w", startPath, err)
	}

	projectRecord, err := s.store.ProjectByRoot(ctx, projectResult.Root)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return StatusResult{}, fmt.Errorf("manage status: project %q: %w", projectResult.Root, domain.ErrUnboundProject)
		}
		return StatusResult{}, fmt.Errorf("manage status: lookup project %q: %w", projectResult.Root, err)
	}

	binding, err := s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderCodex)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return StatusResult{}, fmt.Errorf("manage status: project %q: %w", projectRecord.Root, domain.ErrUnboundProject)
		}
		return StatusResult{}, fmt.Errorf("manage status: lookup binding for project %q: %w", projectRecord.Root, err)
	}

	profile, err := s.store.ProfileByID(ctx, binding.ProfileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return StatusResult{}, fmt.Errorf("manage status: profile for project %q: %w", projectRecord.Root, domain.ErrUnboundProject)
		}
		return StatusResult{}, fmt.Errorf("manage status: lookup profile %q: %w", binding.ProfileID, err)
	}

	return StatusResult{Detected: projectResult, Project: projectRecord, Profile: profile, Binding: binding}, nil
}

// StatusForProvider returns the binding status for the given provider.
// It mirrors Status exactly but passes provider to BindingByProjectID instead
// of hardcoding domain.ProviderCodex. This is the correct lookup for non-Codex
// providers (e.g. Claude). Do NOT use Status for Claude binding checks — it
// always queries the Codex binding row.
func (s Service) StatusForProvider(ctx context.Context, startPath string, provider domain.Provider) (StatusResult, error) {
	projectResult, err := s.detect(startPath)
	if err != nil {
		return StatusResult{}, fmt.Errorf("manage status: detect project from %q: %w", startPath, err)
	}

	projectRecord, err := s.store.ProjectByRoot(ctx, projectResult.Root)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return StatusResult{}, fmt.Errorf("manage status: project %q: %w", projectResult.Root, domain.ErrUnboundProject)
		}
		return StatusResult{}, fmt.Errorf("manage status: lookup project %q: %w", projectResult.Root, err)
	}

	binding, err := s.store.BindingByProjectID(ctx, projectRecord.ID, provider)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return StatusResult{}, fmt.Errorf("manage status: project %q: %w", projectResult.Root, domain.ErrUnboundProject)
		}
		return StatusResult{}, fmt.Errorf("manage status: lookup binding for project %q: %w", projectResult.Root, err)
	}

	profile, err := s.store.ProfileByID(ctx, binding.ProfileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return StatusResult{}, fmt.Errorf("manage status: profile for project %q: %w", projectResult.Root, domain.ErrUnboundProject)
		}
		return StatusResult{}, fmt.Errorf("manage status: lookup profile %q: %w", binding.ProfileID, err)
	}

	return StatusResult{Detected: projectResult, Project: projectRecord, Profile: profile, Binding: binding}, nil
}

func (s Service) ListProfiles(ctx context.Context, provider domain.Provider) (ProfileListResult, error) {
	profiles, err := s.store.ListProfilesByProvider(ctx, provider)
	if err != nil {
		return ProfileListResult{}, fmt.Errorf("list profiles for provider %q: %w", provider, err)
	}
	profiles = s.presentableProfiles(provider, profiles)
	s.debug("listed provider accounts", "provider", provider, "count", len(profiles))
	return ProfileListResult{
		Provider: provider,
		Profiles: profiles,
	}, nil
}

func (s Service) RenameProfile(ctx context.Context, provider domain.Provider, currentName, newName string) (domain.Profile, error) {
	currentName = strings.TrimSpace(currentName)
	newName = strings.TrimSpace(newName)
	if currentName == "" {
		return domain.Profile{}, fmt.Errorf("rename profile: current account name is required")
	}
	if newName == "" {
		return domain.Profile{}, fmt.Errorf("rename profile: new account name is required")
	}
	if currentName == newName {
		return s.ProfileByName(ctx, provider, currentName)
	}
	if _, err := s.store.ProfileByName(ctx, provider, newName); err == nil {
		return domain.Profile{}, fmt.Errorf("rename profile: account %q already exists for provider %q", newName, provider)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Profile{}, fmt.Errorf("rename profile: lookup target account %q: %w", newName, err)
	}
	profile, err := s.store.UpdateProfileName(ctx, provider, currentName, newName)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("rename profile: %w", err)
	}
	return profile, nil
}

func (s Service) DeleteProfile(ctx context.Context, provider domain.Provider, name string) (domain.Profile, error) {
	profile, err := s.ProfileByName(ctx, provider, name)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("delete profile: %w", err)
	}
	bindings, err := s.ListBindings(ctx, provider)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("delete profile: %w", err)
	}
	var boundProjects []string
	for _, binding := range bindings {
		if binding.Profile.ID == profile.ID {
			boundProjects = append(boundProjects, binding.Project.Root)
		}
	}
	if len(boundProjects) > 0 {
		sort.Strings(boundProjects)
		return domain.Profile{}, fmt.Errorf("delete profile: account %q is still bound to project paths: %s", profile.Name, strings.Join(boundProjects, ", "))
	}
	if err := s.store.DeleteProfile(ctx, provider, profile.Name); err != nil {
		return domain.Profile{}, fmt.Errorf("delete profile: %w", err)
	}
	// Remove the managed-home directory from disk only when it is under the
	// Valv-managed providers root. Custom --home paths outside that tree are
	// left untouched to avoid accidental removal of user-owned directories.
	//
	// Both sides are resolved via filepath.EvalSymlinks before comparison so
	// that macOS /var → /private/var symlink aliasing does not cause a
	// spurious prefix mismatch.
	resolvedProviderRoot := filepath.Clean(s.providerRoot)
	if resolved, err := filepath.EvalSymlinks(resolvedProviderRoot); err == nil {
		resolvedProviderRoot = resolved
	}
	resolvedHomePath := filepath.Clean(profile.HomePath)
	if resolved, err := filepath.EvalSymlinks(resolvedHomePath); err == nil {
		resolvedHomePath = resolved
	}
	if strings.HasPrefix(resolvedHomePath, resolvedProviderRoot) {
		if err := os.RemoveAll(profile.HomePath); err != nil {
			return domain.Profile{}, fmt.Errorf("delete profile: remove managed home %q: %w", profile.HomePath, err)
		}
	} else {
		s.debug("delete profile: skipping removal of custom home (outside managed-providers root)", "home", profile.HomePath)
	}
	return profile, nil
}

func (s Service) ListBindings(ctx context.Context, provider domain.Provider) ([]BindingView, error) {
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	projectByID := make(map[string]domain.Project, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
	}
	bindings, err := s.store.ListBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	out := make([]BindingView, 0, len(bindings))
	for _, binding := range bindings {
		if provider != "" && binding.Provider != provider {
			continue
		}
		project, ok := projectByID[binding.ProjectID]
		if !ok {
			return nil, fmt.Errorf("list bindings: project %q: %w", binding.ProjectID, domain.ErrNotFound)
		}
		profile, err := s.store.ProfileByID(ctx, binding.ProfileID)
		if err != nil {
			return nil, fmt.Errorf("list bindings: profile %q: %w", binding.ProfileID, err)
		}
		out = append(out, BindingView{
			Project: project,
			Profile: profile,
			Binding: binding,
		})
	}
	return out, nil
}

func (s Service) CleanupDuplicateAliases(ctx context.Context, provider domain.Provider) (CleanupAliasesResult, error) {
	profiles, err := s.store.ListProfilesByProvider(ctx, provider)
	if err != nil {
		return CleanupAliasesResult{}, fmt.Errorf("cleanup duplicate aliases: list profiles for provider %q: %w", provider, err)
	}
	bindings, err := s.ListBindings(ctx, provider)
	if err != nil {
		return CleanupAliasesResult{}, fmt.Errorf("cleanup duplicate aliases: %w", err)
	}
	bound := map[string]bool{}
	for _, binding := range bindings {
		bound[binding.Profile.ID] = true
	}
	grouped := map[string][]domain.Profile{}
	for _, profile := range profiles {
		grouped[profile.HomePath] = append(grouped[profile.HomePath], profile)
	}
	var result CleanupAliasesResult
	hostSpec, _ := s.DefaultHostProfile(provider)
	for _, group := range grouped {
		if len(group) < 2 {
			continue
		}
		keep := group[0]
		for _, candidate := range group[1:] {
			if shouldPreferPresentableProfile(hostSpec, candidate, keep) {
				keep = candidate
			}
		}
		result.Kept = append(result.Kept, keep)
		for _, profile := range group {
			if profile.ID == keep.ID {
				continue
			}
			if bound[profile.ID] {
				result.Skipped = append(result.Skipped, profile)
				continue
			}
			if err := s.store.DeleteProfile(ctx, profile.Provider, profile.Name); err != nil {
				return CleanupAliasesResult{}, fmt.Errorf("cleanup duplicate aliases: delete account %q: %w", profile.Name, err)
			}
			result.Deleted = append(result.Deleted, profile)
		}
	}
	sort.Slice(result.Deleted, func(i, j int) bool { return result.Deleted[i].Name < result.Deleted[j].Name })
	sort.Slice(result.Kept, func(i, j int) bool { return result.Kept[i].Name < result.Kept[j].Name })
	sort.Slice(result.Skipped, func(i, j int) bool { return result.Skipped[i].Name < result.Skipped[j].Name })
	return result, nil
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}

func (s Service) profileByHome(ctx context.Context, provider domain.Provider, homePath string) (domain.Profile, error) {
	profiles, err := s.store.ListProfilesByProvider(ctx, provider)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("list profiles for provider %q: %w", provider, err)
	}
	homePath = strings.TrimSpace(homePath)
	for _, profile := range profiles {
		if profile.HomePath == homePath {
			return profile, nil
		}
	}
	return domain.Profile{}, domain.ErrNotFound
}

func (s Service) defaultHostProfileCandidate(ctx context.Context, spec HostProfileSpec) (domain.Profile, error) {
	profiles, err := s.store.ListProfilesByProvider(ctx, spec.Provider)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("list profiles for provider %q: %w", spec.Provider, err)
	}
	var sameHome []domain.Profile
	for _, profile := range profiles {
		if profile.HomePath == spec.HomePath {
			sameHome = append(sameHome, profile)
		}
	}
	if len(sameHome) == 0 {
		return domain.Profile{}, domain.ErrNotFound
	}
	for _, profile := range sameHome {
		if profile.Name == spec.Name {
			return profile, nil
		}
	}
	return sameHome[0], nil
}

func (s Service) seedProfileConfig(profile domain.Profile) error {
	spec, err := s.DefaultHostProfile(profile.Provider)
	if err != nil {
		return nil
	}
	if profile.HomePath == spec.HomePath {
		return nil
	}

	sourcePath := filepath.Join(spec.HomePath, "config.toml")
	targetPath := filepath.Join(profile.HomePath, "config.toml")
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check existing config %q: %w", targetPath, err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat default host config %q: %w", sourcePath, err)
	}
	if err := copyFile(sourcePath, targetPath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("seed config from %q to %q: %w", sourcePath, targetPath, err)
	}
	s.debug("seeded provider profile config from default host home", "provider", profile.Provider, "name", profile.Name, "source", sourcePath, "target", targetPath)
	return nil
}

func (s Service) presentableProfiles(provider domain.Provider, profiles []domain.Profile) []domain.Profile {
	if len(profiles) < 2 {
		return profiles
	}

	var hostSpec HostProfileSpec
	hostSpec, _ = s.DefaultHostProfile(provider)
	canonicalByHome := make(map[string]domain.Profile, len(profiles))
	for _, profile := range profiles {
		current, ok := canonicalByHome[profile.HomePath]
		if !ok || shouldPreferPresentableProfile(hostSpec, profile, current) {
			canonicalByHome[profile.HomePath] = profile
		}
	}

	out := make([]domain.Profile, 0, len(canonicalByHome))
	for _, profile := range canonicalByHome {
		out = append(out, profile)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func shouldPreferPresentableProfile(hostSpec HostProfileSpec, candidate, current domain.Profile) bool {
	if candidate.HomePath == hostSpec.HomePath || current.HomePath == hostSpec.HomePath {
		switch {
		case candidate.Name == hostSpec.Name && current.Name != hostSpec.Name:
			return true
		case current.Name == hostSpec.Name && candidate.Name != hostSpec.Name:
			return false
		case isLegacyHostAlias(current.Name) && !isLegacyHostAlias(candidate.Name):
			return true
		case isLegacyHostAlias(candidate.Name) && !isLegacyHostAlias(current.Name):
			return false
		}
	}
	return candidate.Name < current.Name
}

func isLegacyHostAlias(name string) bool {
	switch strings.TrimSpace(name) {
	case "default", "host", "host-codex":
		return true
	default:
		return false
	}
}

// accountEnvKeyPattern is the literal regex string surfaced in env-key
// validation errors so operators see the exact constraint the service
// applied. The compiled form is held in accountEnvKeyRegexp.
const accountEnvKeyPattern = `^[A-Za-z_][A-Za-z0-9_]*$`

// accountEnvKeyRegexp validates env-var keys per accountEnvKeyPattern.
// Compiled once at package init via regexp.MustCompile.
var accountEnvKeyRegexp = regexp.MustCompile(accountEnvKeyPattern)

// reservedAccountEnvKeys are env-var names owned by the Valv runtime / launch
// path. The account-env map must not be allowed to override them — the
// runtime adapters set CODEX_HOME / CLAUDE_CONFIG_DIR, and the container
// image owns HOME / LOGNAME / TERM / USER for an isolated Linux user.
var reservedAccountEnvKeys = map[string]struct{}{
	"CODEX_HOME":        {},
	"CLAUDE_CONFIG_DIR": {},
	"HOME":              {},
	"LOGNAME":           {},
	"TERM":              {},
	"USER":              {},
}

// AccountEnvEntryView is the service-facing projection of a single account
// env-var row. Today it is a thin wrapper over domain.AccountEnvEntry; it
// exists so future redaction / metadata fields land in one place.
type AccountEnvEntryView = domain.AccountEnvEntry

// SetAccountEnv validates the env key, resolves the named account via
// ProfileByName, and upserts the (profileID, envKey) row through the store.
// Returns the persisted entry on success.
func (s Service) SetAccountEnv(ctx context.Context, provider domain.Provider, accountName, envKey, envValue string) (domain.AccountEnvEntry, error) {
	if err := validateAccountEnvKey(envKey); err != nil {
		return domain.AccountEnvEntry{}, fmt.Errorf("set account env: %w", err)
	}
	profile, err := s.ProfileByName(ctx, provider, accountName)
	if err != nil {
		return domain.AccountEnvEntry{}, fmt.Errorf("set account env: %w", err)
	}
	entry, err := s.store.SetAccountEnv(ctx, profile.ID, envKey, envValue)
	if err != nil {
		return domain.AccountEnvEntry{}, fmt.Errorf("set account env %q/%q: %w", provider, accountName, err)
	}
	s.debug("set account env", "provider", provider, "account", profile.Name, "key", envKey)
	return entry, nil
}

// UnsetAccountEnv validates the env key, resolves the named account, and
// removes the (profileID, envKey) row through the store. Wraps
// domain.ErrNotFound when no row matches.
func (s Service) UnsetAccountEnv(ctx context.Context, provider domain.Provider, accountName, envKey string) error {
	if err := validateAccountEnvKey(envKey); err != nil {
		return fmt.Errorf("unset account env: %w", err)
	}
	profile, err := s.ProfileByName(ctx, provider, accountName)
	if err != nil {
		return fmt.Errorf("unset account env: %w", err)
	}
	if err := s.store.UnsetAccountEnv(ctx, profile.ID, envKey); err != nil {
		return fmt.Errorf("unset account env %q/%q: %w", provider, accountName, err)
	}
	s.debug("unset account env", "provider", provider, "account", profile.Name, "key", envKey)
	return nil
}

// ListAccountEnv resolves the named account and returns its env-var entries
// ordered alphabetically by env_key. Returns a nil slice when no entries
// exist.
func (s Service) ListAccountEnv(ctx context.Context, provider domain.Provider, accountName string) ([]domain.AccountEnvEntry, error) {
	profile, err := s.ProfileByName(ctx, provider, accountName)
	if err != nil {
		return nil, fmt.Errorf("list account env: %w", err)
	}
	entries, err := s.store.ListAccountEnv(ctx, profile.ID)
	if err != nil {
		return nil, fmt.Errorf("list account env %q/%q: %w", provider, accountName, err)
	}
	s.debug("listed account env", "provider", provider, "account", profile.Name, "count", len(entries))
	return entries, nil
}

// validateAccountEnvKey enforces the env-key regex and the reserved-key
// blocklist. Rejection errors include the literal regex string so operators
// see the constraint that fired.
func validateAccountEnvKey(envKey string) error {
	if envKey == "" {
		return fmt.Errorf("env key is empty: must match %s", accountEnvKeyPattern)
	}
	if !accountEnvKeyRegexp.MatchString(envKey) {
		return fmt.Errorf("env key %q is invalid: must match %s", envKey, accountEnvKeyPattern)
	}
	if _, reserved := reservedAccountEnvKeys[envKey]; reserved {
		return fmt.Errorf("env key %q is reserved by the Valv runtime and cannot be set per account", envKey)
	}
	return nil
}

func copyFile(sourcePath, targetPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		_ = target.Close()
	}()
	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return target.Close()
}
