package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	codexruntime "github.com/evanmschultz/valv/internal/adapters/providers/codex"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/pathutil"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

type Store interface {
	domain.ProjectRepository
	domain.BindingRepository
	domain.ProfileRepository
}

type Executor interface {
	Run(context.Context, docker.ContainerRunRequest) error
	Create(context.Context, docker.ContainerRunRequest) (string, error)
	Start(context.Context, docker.ContainerStartRequest) error
	RemoveContainer(context.Context, docker.ContainerRemoveRequest) error
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
	TempRoot string
	Now      func() time.Time
	RealHome string
	Logger   *log.Logger
	Notices  io.Writer
}

type Service struct {
	store    Store
	executor Executor
	detect   DetectFunc
	image    docker.ImageRef
	user     string
	tty      bool
	stdin    bool
	tempRoot string
	now      func() time.Time
	realHome string
	logger   *log.Logger
	notices  io.Writer
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
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	tempRoot := strings.TrimSpace(options.TempRoot)
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}

	return Service{
		store:    options.Store,
		executor: options.Executor,
		detect:   detect,
		image:    options.Image,
		user:     strings.TrimSpace(options.User),
		tty:      options.TTY,
		stdin:    options.Stdin,
		tempRoot: tempRoot,
		now:      now,
		realHome: strings.TrimSpace(options.RealHome),
		logger:   options.Logger,
		notices:  options.Notices,
	}, nil
}

func (s Service) Run(ctx context.Context, cwd string, codexArgs []string) error {
	resolved, err := s.resolveBinding(ctx, cwd)
	if err != nil {
		return err
	}

	sharedHome := s.sharedCodexStateHome(resolved.profile)
	prepared, err := codexruntime.PrepareRuntime(ctx, codexruntime.PrepareRequest{
		ProfileHome: resolved.profile.HomePath,
		SharedHome:  sharedHome,
		ProjectRoot: resolved.project.Root,
		TempRoot:    s.tempRoot,
		Logger:      s.logger,
	})
	if err != nil {
		return fmt.Errorf("run codex launch service: prepare runtime: %w", err)
	}
	defer prepared.Close()
	s.emitNotices(resolved.profile, prepared.Warnings, codexArgs)

	request, err := s.buildRequest(resolved.workingDir, resolved.project, resolved.profile, prepared, codexArgs)
	if err != nil {
		return fmt.Errorf("run codex launch service: build docker request: %w", err)
	}
	s.debug(
		"launching codex container",
		"container_name", request.Name,
		"image", request.Image.String(),
		"args", request.Args,
		"tty", request.TTY,
		"interactive", request.Interactive,
		"init", request.Init,
		"user", request.User,
		"working_dir", request.WorkingDir,
		"profile_home", resolved.profile.HomePath,
		"shared_home", sharedHome,
		"env_passthrough", request.EnvPassthrough,
		"mount_count", len(request.Mounts),
	)

	if request.Interactive && request.TTY {
		if err := s.runAttached(ctx, resolved.project.Root, request); err != nil {
			return err
		}
		return nil
	}
	if err := s.executor.Run(ctx, request); err != nil {
		return fmt.Errorf("run codex launch service: execute docker request for project %q: %w", resolved.project.Root, err)
	}
	return nil
}

func (s Service) runAttached(ctx context.Context, projectRoot string, request docker.ContainerRunRequest) error {
	createRequest := request
	createRequest.Detached = false
	createRequest.Remove = false

	containerID, err := s.executor.Create(ctx, createRequest)
	if err != nil {
		return fmt.Errorf("run codex launch service: create docker request for project %q: %w", projectRoot, err)
	}
	containerRef := strings.TrimSpace(request.Name)
	if containerRef == "" {
		containerRef = strings.TrimSpace(containerID)
	}
	s.debug("created interactive codex container", "container_id", strings.TrimSpace(containerID), "container_ref", containerRef)

	remove := func() error {
		if strings.TrimSpace(containerRef) == "" {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := s.executor.RemoveContainer(ctx, docker.ContainerRemoveRequest{
			IDs:   []string{containerRef},
			Force: true,
		})
		if err != nil && !strings.Contains(err.Error(), "No such container") {
			return err
		}
		return nil
	}

	startErr := s.executor.Start(ctx, docker.ContainerStartRequest{
		ContainerID: containerRef,
		Attach:      true,
		Interactive: request.Interactive,
	})
	removeErr := remove()
	if startErr != nil && removeErr != nil {
		return errors.Join(
			fmt.Errorf("run codex launch service: start attached docker request for project %q: %w", projectRoot, startErr),
			fmt.Errorf("run codex launch service: cleanup interactive container %q: %w", containerRef, removeErr),
		)
	}
	if startErr != nil {
		return fmt.Errorf("run codex launch service: start attached docker request for project %q: %w", projectRoot, startErr)
	}
	if removeErr != nil {
		return fmt.Errorf("run codex launch service: cleanup interactive container %q: %w", containerRef, removeErr)
	}
	return nil
}

func (s Service) sharedCodexStateHome(profile domain.Profile) string {
	if profile.Provider != domain.ProviderCodex {
		return profile.HomePath
	}
	if strings.TrimSpace(s.realHome) == "" {
		return profile.HomePath
	}
	_, homePath, err := codexruntime.DefaultHostProfile(s.realHome)
	if err != nil {
		return profile.HomePath
	}
	return homePath
}

func (s Service) ValidateBinding(ctx context.Context, cwd string) error {
	_, err := s.resolveBinding(ctx, cwd)
	return err
}

type resolvedLaunchBinding struct {
	workingDir string
	project    domain.Project
	profile    domain.Profile
}

func (s Service) resolveBinding(ctx context.Context, cwd string) (resolvedLaunchBinding, error) {
	workingDir, err := pathutil.Normalize(cwd)
	if err != nil {
		return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: normalize working directory: %w", err)
	}

	projectResult, err := s.detect(workingDir)
	if err != nil {
		return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: detect project from %q: %w", workingDir, err)
	}
	s.debug("detected project", "cwd", workingDir, "root", projectResult.Root, "has_git_marker", projectResult.HasGitMarker)

	projectRecord, err := s.store.ProjectByRoot(ctx, projectResult.Root)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: project %q: %w", projectResult.Root, domain.ErrUnboundProject)
		}
		return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: lookup project %q: %w", projectResult.Root, err)
	}

	binding, err := s.store.BindingByProjectID(ctx, projectRecord.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: project %q: %w", projectRecord.Root, domain.ErrUnboundProject)
		}
		return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: lookup binding for project %q: %w", projectRecord.Root, err)
	}
	if binding.Provider != domain.ProviderCodex {
		return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: binding provider %q: expected %q", binding.Provider, domain.ProviderCodex)
	}

	profile, err := s.store.ProfileByID(ctx, binding.ProfileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: profile for project %q: %w", projectRecord.Root, domain.ErrUnboundProject)
		}
		return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: lookup profile %q: %w", binding.ProfileID, err)
	}
	if profile.Provider != domain.ProviderCodex {
		return resolvedLaunchBinding{}, fmt.Errorf("run codex launch service: profile provider %q: expected %q", profile.Provider, domain.ProviderCodex)
	}

	return resolvedLaunchBinding{
		workingDir: workingDir,
		project:    projectRecord,
		profile:    profile,
	}, nil
}

func (s Service) buildRequest(workingDir string, project domain.Project, profile domain.Profile, prepared codexruntime.PreparedRuntime, codexArgs []string) (docker.ContainerRunRequest, error) {
	withinRoot, err := withinProjectRoot(project.Root, workingDir)
	if err != nil {
		return docker.ContainerRunRequest{}, err
	}
	if !withinRoot {
		return docker.ContainerRunRequest{}, fmt.Errorf("working directory %q is outside project root %q", workingDir, project.Root)
	}

	request := docker.ContainerRunRequest{
		Name:           s.containerName(project),
		Image:          s.image,
		WorkingDir:     workingDir,
		Env:            prepared.Env,
		EnvPassthrough: prepared.EnvPassthrough,
		Labels: map[string]string{
			"io.valv.managed":    "true",
			"io.valv.provider":   "codex",
			"io.valv.scope":      "interactive",
			"io.valv.project_id": project.ID,
			"io.valv.profile_id": profile.ID,
		},
		Mounts:      append([]docker.MountSpec{docker.NewMountSpec(project.Root, project.Root, false)}, prepared.Mounts...),
		Args:        append([]string(nil), codexArgs...),
		Interactive: s.stdin,
		TTY:         s.tty,
		Init:        s.tty || s.stdin,
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

func (s Service) emitNotices(_ domain.Profile, warnings, _ []string) {
	if len(warnings) == 0 {
		return
	}
	for _, warning := range warnings {
		s.debug("codex runtime warning", "warning", warning)
	}
	if s.notices == nil || s.tty {
		return
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(s.notices, "Valv MCP note: %s\n", warning)
	}
}

func (s Service) containerName(project domain.Project) string {
	base := sanitizeContainerPart(project.Name)
	if base == "" {
		base = sanitizeContainerPart(filepath.Base(project.Root))
	}
	if base == "" {
		base = "project"
	}
	return fmt.Sprintf("valv-codex-interactive-%s-%d", base, s.now().UnixNano())
}

func sanitizeContainerPart(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == '.':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
