package manage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	created, err := s.store.CreateProfile(ctx, profile)
	if err != nil {
		existing, lookupErr := s.store.ProfileByName(ctx, provider, profile.Name)
		if lookupErr == nil {
			if existing.HomePath != profile.HomePath {
				return domain.Profile{}, fmt.Errorf("create profile %q: profile already exists with home %q", profile.Name, existing.HomePath)
			}
			s.debug("provider profile already exists", "provider", existing.Provider, "name", existing.Name, "home", existing.HomePath)
			return existing, nil
		}
		return domain.Profile{}, fmt.Errorf("create profile %q: persist profile: %w", profile.Name, err)
	}
	s.debug("created provider profile", "provider", created.Provider, "name", created.Name, "home", created.HomePath)
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
	return s.CreateProfile(ctx, spec.Provider, spec.Name, spec.HomePath)
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
	s.debug("listed provider profiles", "provider", provider, "count", len(profiles))
	return ProfileListResult{
		Provider: provider,
		Profiles: profiles,
	}, nil
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}
