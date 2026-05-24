// Package run hosts the shared provider-agnostic launch primitive. Both the
// Claude and Codex service wrappers (and the new top-level `valv run` command)
// delegate to a single Service in this package so that container-request
// orchestration — within-project guarding, exact mount sequencing, label
// stamping, warning-to-notice propagation, prepared-runtime cleanup — exists in
// one place instead of being copy-pasted per provider.
//
// The Service is intentionally provider-agnostic. Provider-specific runtime
// preparation (credential homes, MCP overlays, cross-provider mount targets,
// Codex shared-home derivation) stays on the provider wrappers; the wrappers
// hand the Service a PreparedRuntime value whose minimum contract is `Env`,
// `EnvPassthrough`, `Mounts`, `Warnings`, and a `Cleanup` func. The Service
// owns nothing provider-specific beyond a small Provider descriptor (display
// name, container-name prefix, notice copy).
package run

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
	"github.com/evanmschultz/valv/internal/pathutil"
)

// Executor runs Docker containers. The shared launch service requires only
// the `Run` shape; richer create/start/remove operations remain a concern of
// the provider wrappers if/when they need them.
type Executor interface {
	Run(ctx context.Context, request docker.ContainerRunRequest) error
}

// Provider describes the provider-specific labels and copy the shared service
// needs in order to stamp container metadata and write user-visible notices.
// It deliberately carries no runtime-prep concerns — those stay in the
// provider wrappers per the DROP_13 Schema Decisions.
type Provider struct {
	// Name is the lowercase provider identifier ("claude" / "codex"). It is
	// emitted as the `io.valv.provider` label value.
	Name string
	// ContainerNamePrefix is the prefix used to derive a unique container name
	// per launch, e.g. "valv-claude-interactive". The service appends a
	// sanitized project token plus a timestamp.
	ContainerNamePrefix string
	// NoticePrefix is the leading text used when writing prepared-runtime
	// warnings to the notices sink, e.g. "Valv note" or "Valv MCP note". The
	// service appends ": <warning>\n".
	NoticePrefix string
}

// PreparedRuntime is the minimum shape the shared launch service consumes
// from a provider wrapper. The wrappers' own PreparedRuntime values (in
// `internal/adapters/providers/claude/runtime.go` and
// `internal/adapters/providers/codex/runtime.go`) carry the same fields plus
// provider-private extras; they assemble a `run.PreparedRuntime` value and
// pass it into Service.Run.
//
// Cleanup, when non-nil, is invoked by the service via defer on both success
// and failure paths. Provider wrappers wire Cleanup to their own Close
// functions (state sync-back, temp-dir removal, bridge teardown).
type PreparedRuntime struct {
	Env            map[string]string
	EnvPassthrough []string
	Mounts         []docker.MountSpec
	Warnings       []string
	Cleanup        func() error
}

// Close runs the prepared-runtime cleanup if one was wired by the provider
// wrapper. Safe to call on a zero-value PreparedRuntime.
func (p *PreparedRuntime) Close() error {
	if p == nil || p.Cleanup == nil {
		return nil
	}
	return p.Cleanup()
}

// Options configures a Service. All fields except Notices and Logger are
// required; New validates the required ones.
type Options struct {
	Executor Executor
	Image    docker.ImageRef
	User     string
	TTY      bool
	Stdin    bool
	Logger   *log.Logger
	Notices  io.Writer
	Now      func() time.Time
	Provider Provider
}

// Service is the shared provider-agnostic launch primitive. It is safe for
// concurrent use after construction; per-launch state lives entirely in the
// LaunchRequest passed to Run.
type Service struct {
	executor Executor
	image    docker.ImageRef
	user     string
	tty      bool
	stdin    bool
	logger   *log.Logger
	notices  io.Writer
	now      func() time.Time
	provider Provider
}

// LaunchRequest is the per-call payload Run accepts. ProjectRoot, WorkingDir,
// ProjectID, ProfileID, and Prepared are required; Command and Args are
// optional. ProjectName, when set, drives the sanitized portion of the
// container name; when empty the service falls back to the project root's
// basename.
type LaunchRequest struct {
	ProjectRoot string
	WorkingDir  string
	ProjectID   string
	ProfileID   string
	ProjectName string
	Prepared    *PreparedRuntime
	// Args are passed as container args after the image token, mirroring how
	// the provider-baked entrypoint receives its CLI arguments today.
	Args []string
	// Command, when non-empty, overrides the image entrypoint. The first token
	// is emitted as `--entrypoint <token>` via ContainerRunRequest.Extra so
	// that no Docker adapter widening is required; remaining tokens become the
	// container args (replacing Args).
	Command []string
}

// New constructs a Service from Options. Returns an error if any required
// dependency is missing.
func New(options Options) (Service, error) {
	if options.Executor == nil {
		return Service{}, fmt.Errorf("new run service: executor is required")
	}
	if strings.TrimSpace(options.Image.Repository) == "" {
		return Service{}, fmt.Errorf("new run service: image repository is required")
	}
	if strings.TrimSpace(options.Provider.Name) == "" {
		return Service{}, fmt.Errorf("new run service: provider name is required")
	}
	if strings.TrimSpace(options.Provider.ContainerNamePrefix) == "" {
		return Service{}, fmt.Errorf("new run service: provider container name prefix is required")
	}

	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	return Service{
		executor: options.Executor,
		image:    options.Image,
		user:     strings.TrimSpace(options.User),
		tty:      options.TTY,
		stdin:    options.Stdin,
		logger:   options.Logger,
		notices:  options.Notices,
		now:      now,
		provider: options.Provider,
	}, nil
}

// Run launches a container for the given LaunchRequest. The prepared-runtime
// Cleanup runs via defer on both success and failure. Mounts are emitted in
// strict order: the project-root mount first, followed by prepared.Mounts
// unchanged.
func (s Service) Run(ctx context.Context, request LaunchRequest) error {
	if err := s.validate(request); err != nil {
		return err
	}

	defer func() {
		if err := request.Prepared.Close(); err != nil {
			s.debug("prepared runtime cleanup error", "err", err)
		}
	}()

	s.emitNotices(request.Prepared.Warnings)

	runRequest, err := s.buildRequest(request)
	if err != nil {
		return fmt.Errorf("run %s launch service: build docker request: %w", s.provider.Name, err)
	}

	s.debug(
		"launching container",
		"provider", s.provider.Name,
		"container_name", runRequest.Name,
		"image", runRequest.Image.String(),
		"args", runRequest.Args,
		"extra", runRequest.Extra,
		"tty", runRequest.TTY,
		"interactive", runRequest.Interactive,
		"init", runRequest.Init,
		"user", runRequest.User,
		"working_dir", runRequest.WorkingDir,
		"env_passthrough", runRequest.EnvPassthrough,
		"mount_count", len(runRequest.Mounts),
	)

	if err := s.executor.Run(ctx, runRequest); err != nil {
		return fmt.Errorf("run %s launch service: execute docker request for project %q: %w", s.provider.Name, request.ProjectRoot, err)
	}
	return nil
}

func (s Service) validate(request LaunchRequest) error {
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return errors.New("run launch service: project root is required")
	}
	if strings.TrimSpace(request.WorkingDir) == "" {
		return errors.New("run launch service: working directory is required")
	}
	if request.Prepared == nil {
		return errors.New("run launch service: prepared runtime is required")
	}
	return nil
}

func (s Service) buildRequest(launch LaunchRequest) (docker.ContainerRunRequest, error) {
	// Normalize both paths before the within-project ancestry check so that
	// callers passing equivalent-but-differently-spelled paths (case-variant
	// on case-insensitive filesystems, symlinked project root) are not
	// false-rejected by the lexical guard. The provider services upstream
	// already normalize via pathutil.Normalize; the shared seam must do the
	// same so the contract holds regardless of which caller routes through.
	normalizedProjectRoot, err := pathutil.Normalize(launch.ProjectRoot)
	if err != nil {
		return docker.ContainerRunRequest{}, fmt.Errorf("normalize project root %q: %w", launch.ProjectRoot, err)
	}
	normalizedWorkingDir, err := pathutil.Normalize(launch.WorkingDir)
	if err != nil {
		return docker.ContainerRunRequest{}, fmt.Errorf("normalize working directory %q: %w", launch.WorkingDir, err)
	}

	withinRoot, canonicalWorkingDir, err := resolveWithinProjectRoot(normalizedProjectRoot, normalizedWorkingDir)
	if err != nil {
		return docker.ContainerRunRequest{}, err
	}
	if !withinRoot {
		return docker.ContainerRunRequest{}, fmt.Errorf("working directory %q is outside project root %q", normalizedWorkingDir, normalizedProjectRoot)
	}

	mounts := append(
		[]docker.MountSpec{docker.NewMountSpec(normalizedProjectRoot, normalizedProjectRoot, false)},
		launch.Prepared.Mounts...,
	)

	args, extra := s.applyCommandOverride(launch.Command, launch.Args)

	request := docker.ContainerRunRequest{
		Name:           s.containerName(launch),
		Image:          s.image,
		WorkingDir:     canonicalWorkingDir,
		Env:            launch.Prepared.Env,
		EnvPassthrough: launch.Prepared.EnvPassthrough,
		Labels: map[string]string{
			"io.valv.managed":    "true",
			"io.valv.provider":   s.provider.Name,
			"io.valv.scope":      "interactive",
			"io.valv.project_id": launch.ProjectID,
			"io.valv.profile_id": launch.ProfileID,
		},
		Mounts:      mounts,
		Args:        args,
		Interactive: s.stdin,
		TTY:         s.tty,
		Init:        s.tty || s.stdin,
		Remove:      true,
		User:        s.user,
		Extra:       extra,
	}
	return request, nil
}

// applyCommandOverride produces the (args, extra) pair from the optional
// explicit command override. When no override is supplied, args is a copy of
// the original launch.Args and extra is nil so the image's baked entrypoint
// stays intact. When an override is supplied, the first token is emitted as
// `--entrypoint <token>` via Extra and the remaining tokens become container
// args (replacing launch.Args).
func (s Service) applyCommandOverride(command, originalArgs []string) ([]string, []string) {
	if len(command) == 0 {
		return append([]string(nil), originalArgs...), nil
	}
	extra := []string{"--entrypoint", command[0]}
	args := append([]string(nil), command[1:]...)
	return args, extra
}

func (s Service) emitNotices(warnings []string) {
	if len(warnings) == 0 {
		return
	}
	for _, warning := range warnings {
		s.debug(s.provider.Name+" runtime warning", "warning", warning)
	}
	if s.notices == nil || s.tty {
		return
	}
	prefix := strings.TrimSpace(s.provider.NoticePrefix)
	if prefix == "" {
		prefix = "Valv note"
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(s.notices, "%s: %s\n", prefix, warning)
	}
}

func (s Service) containerName(launch LaunchRequest) string {
	base := sanitizeContainerPart(launch.ProjectName)
	if base == "" {
		base = sanitizeContainerPart(filepath.Base(launch.ProjectRoot))
	}
	if base == "" {
		base = "project"
	}
	return fmt.Sprintf("%s-%s-%d", s.provider.ContainerNamePrefix, base, s.now().UnixNano())
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}

// resolveWithinProjectRoot reports whether workingDir is the project root or
// nested under it AND returns the working-directory spelling that is safe to
// use as the in-container WorkingDir.
//
// It first tries the cheap lexical Rel-based comparison; when that passes, the
// caller-supplied workingDir is returned unchanged. When the lexical check
// rejects, the function falls back to an inode-based ancestry walk that
// catches case-folded equivalents on case-insensitive filesystems and any
// symlink spellings Normalize did not collapse. When the inode walk succeeds,
// the working-directory spelling is rewritten to use projectRoot's spelling
// joined with the subpath collected during the walk so the resulting path is
// consistent with the project-root bind mount that Docker receives.
//
// Both inputs MUST already be absolute + symlink-resolved via pathutil.Normalize
// at the caller boundary.
func resolveWithinProjectRoot(projectRoot, workingDir string) (bool, string, error) {
	rel, err := filepath.Rel(projectRoot, workingDir)
	if err != nil {
		return false, "", fmt.Errorf("compare working directory %q to project root %q: %w", workingDir, projectRoot, err)
	}
	if rel == "." {
		return true, workingDir, nil
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true, workingDir, nil
	}

	// Lexical check rejected. Fall back to inode-based ancestry comparison so
	// case-folded and symlinked-but-equivalent paths are not false-rejected.
	// Non-existent paths fail os.Stat — treat that as the original lexical
	// rejection (we cannot prove same-project for paths that don't exist).
	rootInfo, err := os.Stat(projectRoot)
	if err != nil {
		return false, "", nil
	}
	current := workingDir
	var subparts []string
	for {
		info, err := os.Stat(current)
		if err != nil {
			return false, "", nil
		}
		if os.SameFile(rootInfo, info) {
			// Rebuild the working dir spelling using projectRoot's spelling so
			// the result is consistent with the bind mount Docker receives.
			canonical := projectRoot
			for i := len(subparts) - 1; i >= 0; i-- {
				canonical = filepath.Join(canonical, subparts[i])
			}
			return true, canonical, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false, "", nil
		}
		subparts = append(subparts, filepath.Base(current))
		current = parent
	}
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
