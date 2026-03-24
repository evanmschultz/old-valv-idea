package codex

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/domain"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

type Store interface {
	domain.ProjectRepository
	domain.BindingRepository
	domain.ProfileRepository
}

type Executor interface {
	Run(context.Context, docker.ContainerRunRequest) error
}

type DetectFunc func(string) (projectdetect.Result, error)

type Options struct {
	Store    Store
	Executor Executor
	Detect   DetectFunc
	Image    docker.ImageRef
	User     string
	TTY      bool
	Stdin    bool
	Logger   *log.Logger
}

type Service struct {
	store    Store
	executor Executor
	detect   DetectFunc
	image    docker.ImageRef
	user     string
	tty      bool
	stdin    bool
	logger   *log.Logger
}

func New(options Options) (Service, error) {
	if options.Store == nil {
		return Service{}, fmt.Errorf("new codex launch service: store is required")
	}
	if options.Executor == nil {
		return Service{}, fmt.Errorf("new codex launch service: executor is required")
	}
	if strings.TrimSpace(options.Image.Repository) == "" {
		return Service{}, fmt.Errorf("new codex launch service: image repository is required")
	}

	detect := options.Detect
	if detect == nil {
		detect = projectdetect.DetectFrom
	}

	return Service{
		store:    options.Store,
		executor: options.Executor,
		detect:   detect,
		image:    options.Image,
		user:     strings.TrimSpace(options.User),
		tty:      options.TTY,
		stdin:    options.Stdin,
		logger:   options.Logger,
	}, nil
}

func (s Service) Run(ctx context.Context, cwd string, codexArgs []string) error {
	workingDir := filepath.Clean(strings.TrimSpace(cwd))
	if workingDir == "" || workingDir == "." {
		return fmt.Errorf("run codex launch service: working directory is required")
	}

	projectResult, err := s.detect(workingDir)
	if err != nil {
		return fmt.Errorf("run codex launch service: detect project from %q: %w", workingDir, err)
	}
	s.debug("detected project", "cwd", workingDir, "root", projectResult.Root, "has_git_marker", projectResult.HasGitMarker)

	projectRecord, err := s.store.ProjectByRoot(ctx, projectResult.Root)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("run codex launch service: project %q: %w", projectResult.Root, domain.ErrUnboundProject)
		}
		return fmt.Errorf("run codex launch service: lookup project %q: %w", projectResult.Root, err)
	}

	binding, err := s.store.BindingByProjectID(ctx, projectRecord.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("run codex launch service: project %q: %w", projectRecord.Root, domain.ErrUnboundProject)
		}
		return fmt.Errorf("run codex launch service: lookup binding for project %q: %w", projectRecord.Root, err)
	}
	if binding.Provider != domain.ProviderCodex {
		return fmt.Errorf("run codex launch service: binding provider %q: expected %q", binding.Provider, domain.ProviderCodex)
	}

	profile, err := s.store.ProfileByID(ctx, binding.ProfileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("run codex launch service: profile for project %q: %w", projectRecord.Root, domain.ErrUnboundProject)
		}
		return fmt.Errorf("run codex launch service: lookup profile %q: %w", binding.ProfileID, err)
	}
	if profile.Provider != domain.ProviderCodex {
		return fmt.Errorf("run codex launch service: profile provider %q: expected %q", profile.Provider, domain.ProviderCodex)
	}

	request, err := s.buildRequest(workingDir, projectRecord, profile, codexArgs)
	if err != nil {
		return fmt.Errorf("run codex launch service: build docker request: %w", err)
	}
	s.debug("launching codex container", "container_name", request.Name, "image", request.Image.String(), "args", request.Args)

	if err := s.executor.Run(ctx, request); err != nil {
		return fmt.Errorf("run codex launch service: execute docker request for project %q: %w", projectRecord.Root, err)
	}
	return nil
}

func (s Service) buildRequest(workingDir string, project domain.Project, profile domain.Profile, codexArgs []string) (docker.ContainerRunRequest, error) {
	withinRoot, err := withinProjectRoot(project.Root, workingDir)
	if err != nil {
		return docker.ContainerRunRequest{}, err
	}
	if !withinRoot {
		return docker.ContainerRunRequest{}, fmt.Errorf("working directory %q is outside project root %q", workingDir, project.Root)
	}

	request := docker.ContainerRunRequest{
		Image:      s.image,
		WorkingDir: workingDir,
		Env: map[string]string{
			"CODEX_HOME": profile.HomePath,
		},
		Mounts: []docker.MountSpec{
			docker.NewMountSpec(project.Root, project.Root, false),
			docker.NewMountSpec(profile.HomePath, profile.HomePath, false),
		},
		Args:        append([]string(nil), codexArgs...),
		Interactive: s.stdin,
		TTY:         s.tty,
		Remove:      true,
		User:        s.user,
	}
	return request, nil
}

func withinProjectRoot(projectRoot, workingDir string) (bool, error) {
	rel, err := filepath.Rel(projectRoot, workingDir)
	if err != nil {
		return false, fmt.Errorf("compare working directory %q to project root %q: %w", workingDir, projectRoot, err)
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}
