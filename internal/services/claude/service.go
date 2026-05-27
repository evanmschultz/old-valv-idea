package claude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	clauderuntime "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/pathutil"
	projectdetect "github.com/evanmschultz/valv/internal/project"
	runservice "github.com/evanmschultz/valv/internal/services/run"
)

// Store aggregates the repository interfaces required by the Claude launch service.
type Store interface {
	domain.AccountEnvRepository
	domain.BindingRepository
	domain.ProfileRepository
	domain.ProjectRepository
}

// Executor runs and manages Docker containers for the Claude launch service.
type Executor interface {
	Run(context.Context, docker.ContainerRunRequest) error
	Create(context.Context, docker.ContainerRunRequest) (string, error)
	Start(context.Context, docker.ContainerStartRequest) error
	RemoveContainer(context.Context, docker.ContainerRemoveRequest) error
}

// DetectFunc detects the Valv project rooted at or above the given directory.
type DetectFunc func(string) (projectdetect.Result, error)

// Options holds the configurable dependencies for the Claude launch service.
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
	// OverrideProfile, when non-nil, causes Run to use this profile directly
	// instead of resolving the binding from the store. The profile's HomePath
	// is bind-mounted into the container. Used when --account is supplied or
	// when ensureClaudeBindingReady has already resolved the profile (auto-bind
	// or picker). When nil, binding resolution proceeds normally via the store.
	OverrideProfile *domain.Profile
}

// Service launches Claude containers bound to a Valv project and profile.
type Service struct {
	store           Store
	executor        Executor
	detect          DetectFunc
	image           docker.ImageRef
	user            string
	tty             bool
	stdin           bool
	tempRoot        string
	now             func() time.Time
	realHome        string
	logger          *log.Logger
	notices         io.Writer
	overrideProfile *domain.Profile
}

// New constructs a Service from Options. Returns an error if Store, Executor,
// or Image.Repository are missing.
func New(options Options) (Service, error) {
	if options.Store == nil {
		return Service{}, fmt.Errorf("new claude launch service: store is required")
	}
	if options.Executor == nil {
		return Service{}, fmt.Errorf("new claude launch service: executor is required")
	}
	if strings.TrimSpace(options.Image.Repository) == "" {
		return Service{}, fmt.Errorf("new claude launch service: image repository is required")
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
		store:           options.Store,
		executor:        options.Executor,
		detect:          detect,
		image:           options.Image,
		user:            strings.TrimSpace(options.User),
		tty:             options.TTY,
		stdin:           options.Stdin,
		tempRoot:        tempRoot,
		now:             now,
		realHome:        strings.TrimSpace(options.RealHome),
		logger:          options.Logger,
		notices:         options.Notices,
		overrideProfile: options.OverrideProfile,
	}, nil
}

// Run launches a Claude container bound to the project at cwd. When
// OverrideProfile is set in Options, that profile is used directly instead of
// resolving the binding from the store — binding check was already done by the
// caller (ensureClaudeBindingReady). When OverrideProfile is nil, the service
// resolves the binding from the store as normal.
func (s Service) Run(ctx context.Context, cwd string, claudeArgs []string) error {
	var resolved resolvedLaunchBinding
	if s.overrideProfile != nil {
		if s.overrideProfile.Provider != domain.ProviderClaude {
			return fmt.Errorf("claude service: override profile provider mismatch: got %q, want claude", s.overrideProfile.Provider)
		}
		workingDir, err := pathutil.Normalize(cwd)
		if err != nil {
			return fmt.Errorf("run claude launch service: normalize working directory: %w", err)
		}
		projectResult, err := s.detect(workingDir)
		if err != nil {
			return fmt.Errorf("run claude launch service: detect project from %q: %w", workingDir, err)
		}
		projectRecord, err := s.store.ProjectByRoot(ctx, projectResult.Root)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("run claude launch service: project %q: %w", projectResult.Root, domain.ErrUnboundProject)
			}
			return fmt.Errorf("run claude launch service: lookup project %q: %w", projectResult.Root, err)
		}
		resolved = resolvedLaunchBinding{
			workingDir: workingDir,
			project:    projectRecord,
			profile:    *s.overrideProfile,
		}
	} else {
		var err error
		resolved, err = s.resolveBinding(ctx, cwd)
		if err != nil {
			return err
		}
	}

	// Cross-provider binding lookup: when a codex profile is bound to this
	// project, pass its home path so PrepareRuntime can mount it at
	// /home/valv/.codex with CODEX_HOME set. ErrNotFound means codex is not
	// bound — skip silently. Any other store error is fatal.
	var otherProfileHome string
	otherBinding, err := s.store.BindingByProjectID(ctx, resolved.project.ID, domain.ProviderCodex)
	if err == nil {
		otherProfile, profileErr := s.store.ProfileByID(ctx, otherBinding.ProfileID)
		if profileErr == nil {
			otherProfileHome = otherProfile.HomePath
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("run claude launch service: lookup codex binding for project %q: %w", resolved.project.Root, err)
	}

	// Claude isolated-first model: pass profileHome directly as both profile and
	// shared home. SharedHome is intentionally empty — the adapter collapses
	// empty SharedHome to profileHome, skipping the temp-copy branch.
	prepared, err := clauderuntime.PrepareRuntime(ctx, clauderuntime.PrepareRequest{
		ProfileHome:              resolved.profile.HomePath,
		SharedHome:               "",
		ProjectRoot:              resolved.project.Root,
		TempRoot:                 s.tempRoot,
		OtherProviderProfileHome: otherProfileHome,
		Logger:                   s.logger,
	})
	if err != nil {
		return fmt.Errorf("run claude launch service: prepare runtime: %w", err)
	}

	// Adapt Claude's PreparedRuntime to the shared run service contract.
	// The shared run service will invoke the Cleanup func via defer on both
	// success and failure paths.
	runPrepared := &runservice.PreparedRuntime{
		Env:            prepared.Env,
		EnvPassthrough: prepared.EnvPassthrough,
		Mounts:         prepared.Mounts,
		Warnings:       prepared.Warnings,
		Cleanup: func() error {
			return prepared.Close()
		},
	}

	// Load account-specific environment variables for this profile and merge them
	// into the LaunchRequest.
	entries, err := s.store.ListAccountEnv(ctx, resolved.profile.ID)
	if err != nil {
		return fmt.Errorf("run claude launch service: load account env for profile %q: %w", resolved.profile.ID, err)
	}
	accountEnv := domain.AccountEnvEntriesToMap(entries)

	// Delegate to the shared run service with no command override (Claude
	// entrypoint is baked into the image).
	sharedService, err := runservice.New(runservice.Options{
		Executor: s.executor,
		Image:    s.image,
		User:     s.user,
		TTY:      s.tty,
		Stdin:    s.stdin,
		Logger:   s.logger,
		Notices:  s.notices,
		Now:      s.now,
		Provider: runservice.Provider{
			Name:                "claude",
			ContainerNamePrefix: "valv-claude-interactive",
			NoticePrefix:        "Valv note",
		},
	})
	if err != nil {
		return fmt.Errorf("run claude launch service: initialize shared run service: %w", err)
	}

	return sharedService.Run(ctx, runservice.LaunchRequest{
		ProjectRoot: resolved.project.Root,
		WorkingDir:  resolved.workingDir,
		ProjectID:   resolved.project.ID,
		ProfileID:   resolved.profile.ID,
		ProjectName: resolved.project.Name,
		Prepared:    runPrepared,
		Args:        append([]string(nil), claudeArgs...),
		Command:     nil, // No entrypoint override for Claude.
		AccountEnv:  accountEnv,
	})
}

// ValidateBinding resolves the project binding at cwd and returns an error if
// the project is unbound or the binding/profile provider is wrong.
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
		return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: normalize working directory: %w", err)
	}

	projectResult, err := s.detect(workingDir)
	if err != nil {
		return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: detect project from %q: %w", workingDir, err)
	}
	s.debug("detected project", "cwd", workingDir, "root", projectResult.Root, "has_git_marker", projectResult.HasGitMarker)

	projectRecord, err := s.store.ProjectByRoot(ctx, projectResult.Root)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: project %q: %w", projectResult.Root, domain.ErrUnboundProject)
		}
		return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: lookup project %q: %w", projectResult.Root, err)
	}

	binding, err := s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderClaude)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: project %q: %w", projectRecord.Root, domain.ErrUnboundProject)
		}
		return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: lookup binding for project %q: %w", projectRecord.Root, err)
	}
	if binding.Provider != domain.ProviderClaude {
		return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: binding provider %q: expected %q", binding.Provider, domain.ProviderClaude)
	}

	profile, err := s.store.ProfileByID(ctx, binding.ProfileID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: profile for project %q: %w", projectRecord.Root, domain.ErrUnboundProject)
		}
		return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: lookup profile %q: %w", binding.ProfileID, err)
	}
	if profile.Provider != domain.ProviderClaude {
		return resolvedLaunchBinding{}, fmt.Errorf("run claude launch service: profile provider %q: expected %q", profile.Provider, domain.ProviderClaude)
	}

	return resolvedLaunchBinding{
		workingDir: workingDir,
		project:    projectRecord,
		profile:    profile,
	}, nil
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}
