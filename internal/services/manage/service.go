package manage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"

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
	Detect       DetectFunc
	Logger       *log.Logger
}

type Service struct {
	store        Store
	providerRoot string
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
	if _, err := s.store.CreateProfile(ctx, profile); err != nil {
		return domain.Profile{}, fmt.Errorf("create profile %q: persist profile: %w", profile.Name, err)
	}
	s.debug("created provider profile", "provider", profile.Provider, "name", profile.Name, "home", profile.HomePath)
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

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}
