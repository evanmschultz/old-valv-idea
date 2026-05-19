package claude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	clauderuntime "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/domain"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

// fakeStore implements Store using field-level stubs for test control.
type fakeStore struct {
	project    domain.Project
	projectErr error
	binding    domain.ProjectBinding
	bindingErr error
	profile    domain.Profile
	profileErr error
}

func (f fakeStore) CreateProject(context.Context, domain.Project) (domain.Project, error) {
	panic("unexpected call")
}

func (f fakeStore) ProjectByRoot(_ context.Context, _ string) (domain.Project, error) {
	return f.project, f.projectErr
}

func (f fakeStore) ListProjects(context.Context) ([]domain.Project, error) {
	panic("unexpected call")
}

func (f fakeStore) CreateProfile(context.Context, domain.Profile) (domain.Profile, error) {
	panic("unexpected call")
}

func (f fakeStore) ProfileByID(_ context.Context, _ string) (domain.Profile, error) {
	return f.profile, f.profileErr
}

func (f fakeStore) ProfileByName(context.Context, domain.Provider, string) (domain.Profile, error) {
	panic("unexpected call")
}

func (f fakeStore) ListProfilesByProvider(context.Context, domain.Provider) ([]domain.Profile, error) {
	panic("unexpected call")
}

func (f fakeStore) UpdateProfileName(context.Context, domain.Provider, string, string) (domain.Profile, error) {
	panic("unexpected call")
}

func (f fakeStore) DeleteProfile(context.Context, domain.Provider, string) error {
	panic("unexpected call")
}

func (f fakeStore) UpsertProjectBinding(context.Context, domain.ProjectBinding) (domain.ProjectBinding, error) {
	panic("unexpected call")
}

func (f fakeStore) BindingByProjectID(context.Context, string, domain.Provider) (domain.ProjectBinding, error) {
	return f.binding, f.bindingErr
}

func (f fakeStore) ListBindings(context.Context) ([]domain.ProjectBinding, error) {
	panic("unexpected call")
}

// fakeExecutor records the most recent ContainerRunRequest for assertion.
type fakeExecutor struct {
	got       docker.ContainerRunRequest
	createID  string
	created   docker.ContainerRunRequest
	started   docker.ContainerStartRequest
	removed   docker.ContainerRemoveRequest
	err       error
	createErr error
	startErr  error
	removeErr error
	runErr    error
}

func (f *fakeExecutor) Run(_ context.Context, request docker.ContainerRunRequest) error {
	f.got = request
	f.started = docker.ContainerStartRequest{
		ContainerID: request.Name,
		Attach:      request.Interactive,
		Interactive: request.Interactive,
	}
	if f.runErr != nil {
		return f.runErr
	}
	if f.err != nil {
		return f.err
	}
	return nil
}

func (f *fakeExecutor) Create(_ context.Context, request docker.ContainerRunRequest) (string, error) {
	f.created = request
	if f.createErr != nil {
		return "", f.createErr
	}
	if f.createID == "" {
		return "container-123", nil
	}
	return f.createID, nil
}

func (f *fakeExecutor) Start(_ context.Context, request docker.ContainerStartRequest) error {
	f.started = request
	return f.startErr
}

func (f *fakeExecutor) RemoveContainer(_ context.Context, request docker.ContainerRemoveRequest) error {
	f.removed = request
	return f.removeErr
}

// boundClaudeStore returns a fakeStore pre-wired with a valid Claude project,
// binding, and profile for the given project.
func boundClaudeStore(project domain.Project, profileHome string) fakeStore {
	profile := domain.Profile{
		ID:       "profile-claude-1",
		Provider: domain.ProviderClaude,
		Name:     "default",
		HomePath: profileHome,
	}
	binding := domain.ProjectBinding{
		ProjectID: project.ID,
		ProfileID: profile.ID,
		Provider:  domain.ProviderClaude,
	}
	return fakeStore{
		project: project,
		binding: binding,
		profile: profile,
	}
}

func detectAlways(root string) DetectFunc {
	return func(start string) (projectdetect.Result, error) {
		return projectdetect.Result{Root: root, HasGitMarker: true}, nil
	}
}

// TestNewRequiresDependencies verifies New returns an error when required
// dependencies are absent (table-driven per acceptance criterion 5).
func TestNewRequiresDependencies(t *testing.T) {
	t.Parallel()

	validImage := docker.NewImageRef("valv-claude", "dev")
	validStore := fakeStore{
		project: domain.Project{ID: "p1", Root: "/tmp/project"},
		binding: domain.ProjectBinding{Provider: domain.ProviderClaude},
		profile: domain.Profile{Provider: domain.ProviderClaude},
	}
	validExecutor := &fakeExecutor{}

	cases := []struct {
		name    string
		options Options
	}{
		{
			name:    "nil store",
			options: Options{Store: nil, Executor: validExecutor, Image: validImage},
		},
		{
			name:    "nil executor",
			options: Options{Store: validStore, Executor: nil, Image: validImage},
		},
		{
			name:    "empty image repository",
			options: Options{Store: validStore, Executor: validExecutor, Image: docker.ImageRef{}},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tc.options)
			if err == nil {
				t.Fatalf("New(%s) error = nil, want non-nil", tc.name)
			}
		})
	}
}

// TestRunSucceedsWithBoundProject verifies the happy-path: a bound Claude
// project produces a ContainerRunRequest with the correct image, Claude-dir
// mount, and provider label.
func TestRunSucceedsWithBoundProject(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "project-1234567890", Root: "/tmp/project", Name: "project"}
	store := boundClaudeStore(project, t.TempDir())
	executor := &fakeExecutor{}

	service, err := New(Options{
		Store:    store,
		Executor: executor,
		Detect:   detectAlways(project.Root),
		Image:    docker.NewImageRef("valv-claude", "dev"),
		TTY:      true,
		Stdin:    true,
		TempRoot: t.TempDir(),
		Now:      func() time.Time { return time.Unix(0, 123456789) },
		Logger:   log.NewWithOptions(io.Discard, log.Options{Level: log.DebugLevel}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.Run(context.Background(), "/tmp/project", []string{"--resume"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Container name uses "claude"
	if !strings.Contains(executor.got.Name, "valv-claude-interactive-") {
		t.Fatalf("Run() container name = %q, want valv-claude-interactive-* prefix", executor.got.Name)
	}
	// Image is set
	if executor.got.Image.String() != "valv-claude:dev" {
		t.Fatalf("Run() image = %q, want valv-claude:dev", executor.got.Image.String())
	}
	// Provider label is "claude"
	if got := executor.got.Labels["io.valv.provider"]; got != "claude" {
		t.Fatalf("Run() io.valv.provider label = %q, want \"claude\"", got)
	}
	// Mount contains ContainerClaudeDir
	foundClaudeMount := false
	for _, m := range executor.got.Mounts {
		if m.Target == clauderuntime.ContainerClaudeDir {
			foundClaudeMount = true
			break
		}
	}
	if !foundClaudeMount {
		t.Fatalf("Run() mounts = %+v, want mount with target %q", executor.got.Mounts, clauderuntime.ContainerClaudeDir)
	}
	// Env contains CLAUDE_CONFIG_DIR
	if got := executor.got.Env["CLAUDE_CONFIG_DIR"]; got != clauderuntime.ContainerClaudeDir {
		t.Fatalf("Run() CLAUDE_CONFIG_DIR = %q, want %q", got, clauderuntime.ContainerClaudeDir)
	}
	// Managed label
	if got := executor.got.Labels["io.valv.managed"]; got != "true" {
		t.Fatalf("Run() io.valv.managed = %q, want true", got)
	}
}

// TestRunReturnsUnboundProjectWhenNoProject verifies that a missing project
// record surfaces as domain.ErrUnboundProject.
func TestRunReturnsUnboundProjectWhenNoProject(t *testing.T) {
	t.Parallel()

	executor := &fakeExecutor{}
	service, err := New(Options{
		Store: fakeStore{
			projectErr: fmt.Errorf("no project: %w", domain.ErrNotFound),
		},
		Executor: executor,
		Detect: func(start string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: start}, nil
		},
		Image: docker.NewImageRef("valv-claude", "dev"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/project", []string{"--resume"})
	if !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("Run() error = %v, want domain.ErrUnboundProject", err)
	}
}

// TestRunReturnsUnboundProjectWhenNoBinding verifies that a missing binding
// record surfaces as domain.ErrUnboundProject.
func TestRunReturnsUnboundProjectWhenNoBinding(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "p1", Root: "/tmp/project", Name: "proj"}
	executor := &fakeExecutor{}
	service, err := New(Options{
		Store: fakeStore{
			project:    project,
			bindingErr: fmt.Errorf("no binding: %w", domain.ErrNotFound),
		},
		Executor: executor,
		Detect:   detectAlways(project.Root),
		Image:    docker.NewImageRef("valv-claude", "dev"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/project", []string{"--resume"})
	if !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("Run() error = %v, want domain.ErrUnboundProject", err)
	}
}

// TestRunRejectsWrongBindingProvider verifies that a binding whose Provider is
// not ProviderClaude returns an error containing "expected".
func TestRunRejectsWrongBindingProvider(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "p1", Root: "/tmp/project", Name: "proj"}
	executor := &fakeExecutor{}
	service, err := New(Options{
		Store: fakeStore{
			project: project,
			binding: domain.ProjectBinding{
				ProjectID: project.ID,
				ProfileID: "profile-1",
				Provider:  domain.ProviderCodex, // wrong provider
			},
			profile: domain.Profile{
				ID:       "profile-1",
				Provider: domain.ProviderClaude,
				HomePath: t.TempDir(),
			},
		},
		Executor: executor,
		Detect:   detectAlways(project.Root),
		Image:    docker.NewImageRef("valv-claude", "dev"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/project", []string{"--resume"})
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("Run() error = %v, want error containing \"expected\"", err)
	}
}

// TestRunRejectsWrongProfileProvider verifies that a profile whose Provider is
// not ProviderClaude returns an error containing "expected".
func TestRunRejectsWrongProfileProvider(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "p1", Root: "/tmp/project", Name: "proj"}
	executor := &fakeExecutor{}
	service, err := New(Options{
		Store: fakeStore{
			project: project,
			binding: domain.ProjectBinding{
				ProjectID: project.ID,
				ProfileID: "profile-1",
				Provider:  domain.ProviderClaude,
			},
			profile: domain.Profile{
				ID:       "profile-1",
				Provider: domain.ProviderCodex, // wrong provider
				HomePath: t.TempDir(),
			},
		},
		Executor: executor,
		Detect:   detectAlways(project.Root),
		Image:    docker.NewImageRef("valv-claude", "dev"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/project", []string{"--resume"})
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("Run() error = %v, want error containing \"expected\"", err)
	}
}

// TestValidateBindingReturnsNilForBoundProject verifies ValidateBinding
// succeeds when the project, binding, and profile are all Claude-provider.
func TestValidateBindingReturnsNilForBoundProject(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "p1", Root: "/tmp/project", Name: "proj"}
	store := boundClaudeStore(project, t.TempDir())

	service, err := New(Options{
		Store:    store,
		Executor: &fakeExecutor{},
		Detect:   detectAlways(project.Root),
		Image:    docker.NewImageRef("valv-claude", "dev"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.ValidateBinding(context.Background(), "/tmp/project"); err != nil {
		t.Fatalf("ValidateBinding() error = %v, want nil", err)
	}
}

// TestRunRejectsWorkingDirectoryOutsideProjectRoot verifies that a working
// directory outside the detected project root returns an error.
func TestRunRejectsWorkingDirectoryOutsideProjectRoot(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "p1", Root: "/tmp/project", Name: "proj"}
	store := boundClaudeStore(project, t.TempDir())

	service, err := New(Options{
		Store:    store,
		Executor: &fakeExecutor{},
		Detect:   detectAlways(project.Root),
		Image:    docker.NewImageRef("valv-claude", "dev"),
		TempRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/other", []string{"--resume"})
	if err == nil || !strings.Contains(err.Error(), "outside project root") {
		t.Fatalf("Run() error = %v, want working dir validation", err)
	}
}

// TestRunRejectsSiblingPathThatSharesProjectPrefix verifies a sibling path
// (/tmp/project2 vs /tmp/project) is rejected as outside the root.
func TestRunRejectsSiblingPathThatSharesProjectPrefix(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "p1", Root: "/tmp/project", Name: "proj"}
	store := boundClaudeStore(project, t.TempDir())

	service, err := New(Options{
		Store:    store,
		Executor: &fakeExecutor{},
		Detect:   detectAlways(project.Root),
		Image:    docker.NewImageRef("valv-claude", "dev"),
		TempRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/project2", []string{"--resume"})
	if err == nil || !strings.Contains(err.Error(), "outside project root") {
		t.Fatalf("Run() error = %v, want sibling path rejection", err)
	}
}

// TestRunBuildsNonInteractiveDockerRequestWhenTTYDisabled verifies a
// non-interactive request is built when TTY and Stdin are false.
func TestRunBuildsNonInteractiveDockerRequestWhenTTYDisabled(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "p1", Root: "/tmp/project", Name: "proj"}
	store := boundClaudeStore(project, t.TempDir())
	executor := &fakeExecutor{}

	service, err := New(Options{
		Store:    store,
		Executor: executor,
		Detect:   detectAlways(project.Root),
		Image:    docker.NewImageRef("valv-claude", "dev"),
		TempRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.Run(context.Background(), "/tmp/project", []string{"--version"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if executor.got.Interactive || executor.got.TTY {
		t.Fatalf("Run() interactive flags = %+v, want non-interactive request", executor.got)
	}
}

// TestEmitNoticesSuppressesWarningsOnTTY verifies warnings are suppressed when
// TTY is enabled.
func TestEmitNoticesSuppressesWarningsOnTTY(t *testing.T) {
	t.Parallel()

	var notices strings.Builder
	service := Service{tty: true, notices: &notices}
	service.emitNotices(domain.Profile{}, []string{"runtime warning"}, nil)
	if notices.String() != "" {
		t.Fatalf("emitNotices() wrote %q, want no interactive notices", notices.String())
	}
}

// TestEmitNoticesWritesWarningsWithoutTTY verifies warnings are written to the
// notices writer when TTY is disabled.
func TestEmitNoticesWritesWarningsWithoutTTY(t *testing.T) {
	t.Parallel()

	var notices strings.Builder
	service := Service{notices: &notices}
	service.emitNotices(domain.Profile{}, []string{"runtime warning"}, nil)
	if !strings.Contains(notices.String(), "runtime warning") {
		t.Fatalf("emitNotices() output = %q, want warning text", notices.String())
	}
}

// TestContainerNameContainsClaude verifies containerName produces a name with
// the "claude" prefix, not "codex".
func TestContainerNameContainsClaude(t *testing.T) {
	t.Parallel()

	service := Service{now: func() time.Time { return time.Unix(0, 42) }}
	project := domain.Project{Name: "myapp", Root: "/tmp/myapp"}
	name := service.containerName(project)
	if !strings.HasPrefix(name, "valv-claude-interactive-") {
		t.Fatalf("containerName() = %q, want valv-claude-interactive-* prefix", name)
	}
	if strings.Contains(name, "codex") {
		t.Fatalf("containerName() = %q, must not contain \"codex\"", name)
	}
}

// TestRunBubblesExecutorErrors verifies executor errors are propagated
// upward with context.
func TestRunBubblesExecutorErrors(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "p1", Root: "/tmp/project", Name: "proj"}
	store := boundClaudeStore(project, t.TempDir())
	executor := &fakeExecutor{err: errors.New("docker failed")}

	service, err := New(Options{
		Store:    store,
		Executor: executor,
		Detect:   detectAlways(project.Root),
		Image:    docker.NewImageRef("valv-claude", "dev"),
		TempRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/project", []string{"--resume"})
	if err == nil || !strings.Contains(err.Error(), "docker failed") {
		t.Fatalf("Run() error = %v, want wrapped docker error", err)
	}
}

// TestRunUsesOverrideProfileHomePath verifies that when OverrideProfile is set,
// Run uses that profile's HomePath for the container mount instead of the bound
// profile's home. This exercises the C2 concern from 8.3 QA falsification.
func TestRunUsesOverrideProfileHomePath(t *testing.T) {
	t.Parallel()

	// Use os.MkdirTemp directly and EvalSymlinks to get the real path, which
	// avoids the /var vs /private/var discrepancy on macOS.
	overrideHomeRaw := t.TempDir()
	boundHomeRaw := t.TempDir()
	overrideHome, err := filepath.EvalSymlinks(overrideHomeRaw)
	if err != nil {
		t.Fatalf("EvalSymlinks(overrideHome) error = %v", err)
	}
	boundHome, err := filepath.EvalSymlinks(boundHomeRaw)
	if err != nil {
		t.Fatalf("EvalSymlinks(boundHome) error = %v", err)
	}

	project := domain.Project{ID: "project-1234567890", Root: "/tmp/project", Name: "proj"}
	// The store has a bound profile with a DIFFERENT home.
	store := boundClaudeStore(project, boundHome)
	executor := &fakeExecutor{}

	overrideProfile := domain.Profile{
		ID:       "override-profile",
		Provider: domain.ProviderClaude,
		Name:     "override",
		HomePath: overrideHome,
	}

	service, err := New(Options{
		Store:           store,
		Executor:        executor,
		Detect:          detectAlways(project.Root),
		Image:           docker.NewImageRef("valv-claude", "dev"),
		TempRoot:        t.TempDir(),
		Now:             func() time.Time { return time.Unix(0, 42) },
		OverrideProfile: &overrideProfile,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.Run(context.Background(), "/tmp/project", []string{"--prompt", "hello"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Assert the override profile's home is bind-mounted, not the bound profile's home.
	foundOverrideMount := false
	for _, m := range executor.got.Mounts {
		if m.Source == overrideHome {
			foundOverrideMount = true
		}
		if m.Source == boundHome {
			t.Fatalf("Run() mount source = %q, want override home %q not bound home", m.Source, overrideHome)
		}
	}
	if !foundOverrideMount {
		t.Fatalf("Run() mounts = %+v, want mount with source %q (override home)", executor.got.Mounts, overrideHome)
	}
}
