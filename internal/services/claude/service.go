package claude

import (
	"context"
	"encoding/json"
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
	clauderuntime "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/pathutil"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

// Store aggregates the repository interfaces required by the Claude launch service.
type Store interface {
	domain.ProjectRepository
	domain.BindingRepository
	domain.ProfileRepository
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
}

// Service launches Claude containers bound to a Valv project and profile.
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

// Run launches a Claude container bound to the project at cwd. The service
// resolves the project, binding, and profile, prepares the container runtime,
// and attaches or detaches depending on the TTY/Stdin flags.
func (s Service) Run(ctx context.Context, cwd string, claudeArgs []string) error {
	resolved, err := s.resolveBinding(ctx, cwd)
	if err != nil {
		return err
	}

	// Claude isolated-first model: pass profileHome directly as both profile and
	// shared home. SharedHome is intentionally empty — the adapter collapses
	// empty SharedHome to profileHome, skipping the temp-copy branch.
	prepared, err := clauderuntime.PrepareRuntime(ctx, clauderuntime.PrepareRequest{
		ProfileHome: resolved.profile.HomePath,
		SharedHome:  "",
		ProjectRoot: resolved.project.Root,
		TempRoot:    s.tempRoot,
		Logger:      s.logger,
	})
	if err != nil {
		return fmt.Errorf("run claude launch service: prepare runtime: %w", err)
	}
	defer prepared.Close()
	s.emitNotices(resolved.profile, prepared.Warnings, claudeArgs)

	request, err := s.buildRequest(resolved.workingDir, resolved.project, resolved.profile, prepared, claudeArgs)
	if err != nil {
		return fmt.Errorf("run claude launch service: build docker request: %w", err)
	}
	s.debug(
		"launching claude container",
		"container_name", request.Name,
		"image", request.Image.String(),
		"args", request.Args,
		"tty", request.TTY,
		"interactive", request.Interactive,
		"init", request.Init,
		"user", request.User,
		"working_dir", request.WorkingDir,
		"profile_home", resolved.profile.HomePath,
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
		return fmt.Errorf("run claude launch service: execute docker request for project %q: %w", resolved.project.Root, err)
	}
	return nil
}

func (s Service) runAttached(ctx context.Context, projectRoot string, request docker.ContainerRunRequest) error {
	request.Detached = false
	request.Remove = true
	s.debug("starting interactive claude container", "name", request.Name, "image", request.Image.String(), "working_dir", request.WorkingDir)

	if err := s.executor.Run(ctx, request); err != nil {
		return fmt.Errorf("run claude launch service: run attached docker request for project %q: %w", projectRoot, err)
	}
	return nil
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

func (s Service) buildRequest(workingDir string, project domain.Project, profile domain.Profile, prepared clauderuntime.PreparedRuntime, claudeArgs []string) (docker.ContainerRunRequest, error) {
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
			"io.valv.provider":   "claude",
			"io.valv.scope":      "interactive",
			"io.valv.project_id": project.ID,
			"io.valv.profile_id": profile.ID,
		},
		Mounts:      append([]docker.MountSpec{docker.NewMountSpec(project.Root, project.Root, false)}, prepared.Mounts...),
		Args:        append([]string(nil), claudeArgs...),
		Interactive: s.stdin,
		TTY:         s.tty,
		Init:        s.tty || s.stdin,
		Remove:      true,
		User:        s.user,
	}

	// Inject CLAUDE_CODE_OAUTH_TOKEN from the stored credentials file so the
	// container claude CLI authenticates without an interactive login prompt.
	// CLAUDE_CODE_OAUTH_TOKEN is passed as an environment variable and is
	// visible to ps(1). Acceptable for v0.1.0.
	if token, err := readClaudeAuthToken(profile.HomePath); err != nil {
		s.debug("claude auth token unreadable", "home", profile.HomePath, "err", err)
	} else if token != "" {
		request.Env["CLAUDE_CODE_OAUTH_TOKEN"] = token
	} else {
		s.debug("no claude credentials file found, container will run unauthed", "home", profile.HomePath)
	}

	return request, nil
}

// readClaudeAuthToken reads the CLAUDE_CODE_OAUTH_TOKEN value from the
// .credentials.json file stored in the managed profile home directory. The
// file is written by ensureClaudeAccountReady after a successful host-side
// claude setup-token run and keychain extraction.
//
// Returns the token string on success. Returns an empty string and nil error
// when the file does not exist (graceful — the caller decides what to do).
// Returns an empty string and a non-nil error only when the file exists but
// cannot be parsed, which indicates a corrupted credentials file.
func readClaudeAuthToken(homePath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(homePath, ".credentials.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read claude credentials: %w", err)
	}

	var creds struct {
		Token string `json:"claudeAiAccessToken"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", fmt.Errorf("parse claude credentials: %w", err)
	}
	return creds.Token, nil
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
		s.debug("claude runtime warning", "warning", warning)
	}
	if s.notices == nil || s.tty {
		return
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(s.notices, "Valv note: %s\n", warning)
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
	return fmt.Sprintf("valv-claude-interactive-%s-%d", base, s.now().UnixNano())
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
