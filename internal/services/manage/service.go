package manage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/log"

	codexprovider "github.com/evanmschultz/valv/internal/adapters/providers/codex"
	"github.com/evanmschultz/valv/internal/domain"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

type Store interface {
	domain.ProjectRepository
	domain.BindingRepository
	domain.ProfileRepository
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

	binding, err := s.store.BindingByProjectID(ctx, projectRecord.ID)
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
		}
	}
	return candidate.Name < current.Name
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
