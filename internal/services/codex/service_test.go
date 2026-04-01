package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	codexruntime "github.com/evanmschultz/valv/internal/adapters/providers/codex"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/pathutil"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

type fakeStore struct {
	project     domain.Project
	projectErr  error
	binding     domain.ProjectBinding
	bindingErr  error
	profile     domain.Profile
	profileErr  error
	projectRoot string
	profileID   string
}

func (f fakeStore) CreateProject(context.Context, domain.Project) (domain.Project, error) {
	panic("unexpected call")
}

func (f fakeStore) ProjectByRoot(_ context.Context, root string) (domain.Project, error) {
	f.projectRoot = root
	return f.project, f.projectErr
}

func (f fakeStore) ListProjects(context.Context) ([]domain.Project, error) {
	panic("unexpected call")
}

func (f fakeStore) CreateProfile(context.Context, domain.Profile) (domain.Profile, error) {
	panic("unexpected call")
}

func (f fakeStore) ProfileByID(_ context.Context, id string) (domain.Profile, error) {
	f.profileID = id
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

func (f fakeStore) BindingByProjectID(context.Context, string) (domain.ProjectBinding, error) {
	return f.binding, f.bindingErr
}

func (f fakeStore) ListBindings(context.Context) ([]domain.ProjectBinding, error) {
	panic("unexpected call")
}

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
}

func (f *fakeExecutor) Run(_ context.Context, request docker.ContainerRunRequest) error {
	f.got = request
	return f.err
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

func TestNewRequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := New(Options{})
	if err == nil {
		t.Fatal("New() error = nil, want dependency failure")
	}
}

func TestRunReturnsUnboundProjectWhenProjectMissing(t *testing.T) {
	t.Parallel()

	executor := &fakeExecutor{}
	service, err := New(Options{
		Store: fakeStore{
			projectErr: fmt.Errorf("missing project: %w", domain.ErrNotFound),
		},
		Executor: executor,
		Detect: func(start string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: start}, nil
		},
		Image: docker.NewImageRef("valv-codex", "dev"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/project", []string{"resume", "abc"})
	if !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("Run() error = %v, want domain.ErrUnboundProject", err)
	}
}

func TestRunBuildsDockerRequestFromProjectBindingAndProfile(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "project-1234567890", Root: "/tmp/project", Name: "project"}
	binding := domain.ProjectBinding{ProjectID: project.ID, ProfileID: "profile-1", Provider: domain.ProviderCodex}
	profile := domain.Profile{ID: "profile-1", Provider: domain.ProviderCodex, Name: "dev", HomePath: "/tmp/valv/providers/codex/profiles/dev"}
	executor := &fakeExecutor{}
	service, err := New(Options{
		Store: fakeStore{
			project: project,
			binding: binding,
			profile: profile,
		},
		Executor: executor,
		Detect: func(start string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: project.Root, HasGitMarker: true}, nil
		},
		Image:    docker.NewImageRef("valv-codex", "dev"),
		TTY:      true,
		Stdin:    true,
		TempRoot: t.TempDir(),
		Now:      func() time.Time { return time.Unix(0, 123456789) },
		Logger:   log.NewWithOptions(io.Discard, log.Options{Level: log.DebugLevel}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	args := []string{"resume", "session-123", "--model", "gpt-5-codex"}
	if err := service.Run(context.Background(), "/tmp/project/subdir", args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got, want := executor.created.Name, "valv-codex-interactive-project-123456789"; got != want {
		t.Fatalf("Run() container name = %q, want %q", got, want)
	}
	if executor.created.WorkingDir != "/tmp/project/subdir" {
		t.Fatalf("Run() working dir = %q", executor.created.WorkingDir)
	}
	if executor.created.Image.String() != "valv-codex:dev" {
		t.Fatalf("Run() image = %q", executor.created.Image.String())
	}
	if executor.created.User != "" {
		t.Fatalf("Run() user = %q, want empty to use the image default user", executor.created.User)
	}
	if !executor.created.Interactive || !executor.created.TTY {
		t.Fatalf("Run() interactive flags = %+v", executor.created)
	}
	if executor.created.Remove {
		t.Fatalf("Run() create request remove = %t, want false for create/start lifecycle", executor.created.Remove)
	}
	if !executor.created.Init {
		t.Fatalf("Run() init = %t, want true", executor.created.Init)
	}
	if got := executor.created.Env["CODEX_HOME"]; got != codexruntime.ContainerCodexDir {
		t.Fatalf("Run() CODEX_HOME = %q, want %q", got, codexruntime.ContainerCodexDir)
	}
	if got := executor.created.Env["HOME"]; got != codexruntime.ContainerHomeDir {
		t.Fatalf("Run() HOME = %q, want %q", got, codexruntime.ContainerHomeDir)
	}
	if got := executor.created.Labels["io.valv.managed"]; got != "true" {
		t.Fatalf("Run() managed label = %q, want true", got)
	}
	if got := executor.created.Labels["io.valv.scope"]; got != "interactive" {
		t.Fatalf("Run() scope label = %q, want interactive", got)
	}
	if got := executor.created.Labels["io.valv.project_id"]; got != project.ID {
		t.Fatalf("Run() project label = %q, want %q", got, project.ID)
	}
	if got := executor.created.Labels["io.valv.profile_id"]; got != profile.ID {
		t.Fatalf("Run() profile label = %q, want %q", got, profile.ID)
	}
	if len(executor.created.Mounts) != 2 {
		t.Fatalf("Run() mounts len = %d, want 2", len(executor.created.Mounts))
	}
	if executor.created.Mounts[0] != docker.NewMountSpec(project.Root, project.Root, false) {
		t.Fatalf("Run() project mount = %+v", executor.created.Mounts[0])
	}
	wantProfileHome, err := pathutil.Normalize(profile.HomePath)
	if err != nil {
		t.Fatalf("Normalize(profile.HomePath) error = %v", err)
	}
	gotProfileHome, err := pathutil.Normalize(executor.created.Mounts[1].Source)
	if err != nil {
		t.Fatalf("Normalize(profile mount source) error = %v", err)
	}
	if got := docker.NewMountSpec(gotProfileHome, executor.created.Mounts[1].Target, executor.created.Mounts[1].ReadOnly); got != docker.NewMountSpec(wantProfileHome, codexruntime.ContainerCodexDir, false) {
		t.Fatalf("Run() profile mount = %+v", executor.created.Mounts[1])
	}
	for index, arg := range args {
		if executor.created.Args[index] != arg {
			t.Fatalf("Run() args[%d] = %q, want %q", index, executor.created.Args[index], arg)
		}
	}
	if got, want := executor.started.ContainerID, executor.created.Name; got != want {
		t.Fatalf("Run() start container = %q, want %q", got, want)
	}
	if executor.started.ContainerID != executor.created.Name {
		t.Fatalf("Run() start container = %q, want %q", executor.started.ContainerID, executor.created.Name)
	}
	if !executor.started.Attach {
		t.Fatalf("Run() start request = %+v, want attached start", executor.started)
	}
	if !executor.started.Interactive {
		t.Fatalf("Run() start request = %+v, want interactive start", executor.started)
	}
	if got := executor.removed.IDs; len(got) != 1 || got[0] != executor.created.Name {
		t.Fatalf("Run() cleanup ids = %#v, want [%q]", got, executor.created.Name)
	}
}

func TestRunBubblesExecutorErrors(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "project-1234567890", Root: "/tmp/project", Name: "project"}
	executor := &fakeExecutor{err: errors.New("docker failed")}
	service, err := New(Options{
		Store: fakeStore{
			project: project,
			binding: domain.ProjectBinding{ProjectID: project.ID, ProfileID: "profile-1", Provider: domain.ProviderCodex},
			profile: domain.Profile{ID: "profile-1", Provider: domain.ProviderCodex, HomePath: "/tmp/profile"},
		},
		Executor: executor,
		Detect: func(start string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: project.Root}, nil
		},
		Image:    docker.NewImageRef("valv-codex", "dev"),
		TempRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/project", []string{"resume", "abc"})
	if err == nil || !strings.Contains(err.Error(), "docker failed") {
		t.Fatalf("Run() error = %v, want wrapped docker error", err)
	}
}

func TestRunBuildsNonInteractiveDockerRequestWhenTTYDisabled(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "project-1234567890", Root: "/tmp/project", Name: "project"}
	executor := &fakeExecutor{}
	service, err := New(Options{
		Store: fakeStore{
			project: project,
			binding: domain.ProjectBinding{ProjectID: project.ID, ProfileID: "profile-1", Provider: domain.ProviderCodex},
			profile: domain.Profile{ID: "profile-1", Provider: domain.ProviderCodex, HomePath: "/tmp/profile"},
		},
		Executor: executor,
		Detect: func(start string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: project.Root}, nil
		},
		Image:    docker.NewImageRef("valv-codex", "dev"),
		TempRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.Run(context.Background(), "/tmp/project", []string{"exec", "status"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if executor.got.Interactive || executor.got.TTY {
		t.Fatalf("Run() interactive flags = %+v, want non-interactive request", executor.got)
	}
}

func TestRunRejectsWorkingDirectoryOutsideProjectRoot(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "project-1234567890", Root: "/tmp/project", Name: "project"}
	service, err := New(Options{
		Store: fakeStore{
			project: project,
			binding: domain.ProjectBinding{ProjectID: project.ID, ProfileID: "profile-1", Provider: domain.ProviderCodex},
			profile: domain.Profile{ID: "profile-1", Provider: domain.ProviderCodex, HomePath: "/tmp/profile"},
		},
		Executor: &fakeExecutor{},
		Detect: func(start string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: project.Root}, nil
		},
		Image:    docker.NewImageRef("valv-codex", "dev"),
		TempRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/other", []string{"resume", "abc"})
	if err == nil || !strings.Contains(err.Error(), "outside project root") {
		t.Fatalf("Run() error = %v, want working dir validation", err)
	}
}

func TestBuildRequestCarriesEnvPassthrough(t *testing.T) {
	t.Parallel()

	service := Service{
		image: docker.NewImageRef("valv-codex", "dev"),
		now:   func() time.Time { return time.Unix(0, 1) },
	}
	project := domain.Project{ID: "project-1234567890", Root: "/tmp/project", Name: "project"}
	profile := domain.Profile{ID: "profile-1", Provider: domain.ProviderCodex, HomePath: "/tmp/profile"}
	prepared := codexruntime.PreparedRuntime{
		Env: map[string]string{
			"CODEX_HOME": codexruntime.ContainerCodexDir,
			"HOME":       codexruntime.ContainerHomeDir,
		},
		EnvPassthrough: []string{"CONTEXT7_API_KEY"},
		Mounts:         []docker.MountSpec{docker.NewMountSpec("/tmp/profile", codexruntime.ContainerCodexDir, false)},
	}

	request, err := service.buildRequest("/tmp/project", project, profile, prepared, []string{"--help"})
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if !strings.Contains(strings.Join(request.EnvPassthrough, ","), "CONTEXT7_API_KEY") {
		t.Fatalf("EnvPassthrough = %#v, want CONTEXT7_API_KEY", request.EnvPassthrough)
	}
}

func TestRunUsesSharedHostHomeForCodexStateWhenRealHomeIsSet(t *testing.T) {
	t.Parallel()

	realHome := t.TempDir()
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(project.Root) error = %v", err)
	}
	normalizedProjectRoot, err := pathutil.Normalize(projectRoot)
	if err != nil {
		t.Fatalf("Normalize(projectRoot) error = %v", err)
	}
	project := domain.Project{ID: "project-1234567890", Root: normalizedProjectRoot, Name: "project"}
	hostCodexHome := filepath.Join(realHome, ".codex")
	if err := os.MkdirAll(hostCodexHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(hostCodexHome) error = %v", err)
	}
	profileHome := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"auth_mode":"chatgpt"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(auth.json) error = %v", err)
	}
	binding := domain.ProjectBinding{ProjectID: project.ID, ProfileID: "profile-1", Provider: domain.ProviderCodex}
	profile := domain.Profile{ID: "profile-1", Provider: domain.ProviderCodex, Name: "hylla", HomePath: profileHome}
	executor := &fakeExecutor{}
	service, err := New(Options{
		Store: fakeStore{
			project: project,
			binding: binding,
			profile: profile,
		},
		Executor: executor,
		Detect: func(start string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: project.Root, HasGitMarker: true}, nil
		},
		Image:    docker.NewImageRef("valv-codex", "dev"),
		TempRoot: t.TempDir(),
		RealHome: realHome,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.Run(context.Background(), project.Root, []string{"resume", "session-123"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	wantSharedHome, err := pathutil.Normalize(hostCodexHome)
	if err != nil {
		t.Fatalf("Normalize(hostCodexHome) error = %v", err)
	}
	if got := executor.got.Mounts[1]; got.Target != codexruntime.ContainerCodexDir || got.ReadOnly {
		t.Fatalf("shared codex mount = %+v, want writable target %q", got, codexruntime.ContainerCodexDir)
	}
	gotMountSource, err := pathutil.Normalize(executor.got.Mounts[1].Source)
	if err != nil {
		t.Fatalf("Normalize(runtime codex mount) error = %v", err)
	}
	if gotMountSource == wantSharedHome {
		t.Fatalf("runtime codex mount source = %q, want staged runtime dir distinct from shared home", gotMountSource)
	}
	if len(executor.got.Mounts) != 2 {
		t.Fatalf("mount count = %d, want 2 without nested auth mount", len(executor.got.Mounts))
	}
}

func TestRunRejectsSiblingPathThatSharesProjectPrefix(t *testing.T) {
	t.Parallel()

	project := domain.Project{ID: "project-1234567890", Root: "/tmp/project", Name: "project"}
	service, err := New(Options{
		Store: fakeStore{
			project: project,
			binding: domain.ProjectBinding{ProjectID: project.ID, ProfileID: "profile-1", Provider: domain.ProviderCodex},
			profile: domain.Profile{ID: "profile-1", Provider: domain.ProviderCodex, HomePath: "/tmp/profile"},
		},
		Executor: &fakeExecutor{},
		Detect: func(start string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: project.Root}, nil
		},
		Image:    docker.NewImageRef("valv-codex", "dev"),
		TempRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.Run(context.Background(), "/tmp/project2", []string{"resume", "abc"})
	if err == nil || !strings.Contains(err.Error(), "outside project root") {
		t.Fatalf("Run() error = %v, want sibling path rejection", err)
	}
}

func TestEmitNoticesSuppressesWarningsOnTTY(t *testing.T) {
	t.Parallel()

	var notices strings.Builder
	service := Service{tty: true, notices: &notices}
	service.emitNotices(domain.Profile{}, []string{"bridge warning"}, nil)
	if notices.String() != "" {
		t.Fatalf("emitNotices() wrote %q, want no interactive notices", notices.String())
	}
}

func TestEmitNoticesWritesWarningsWithoutTTY(t *testing.T) {
	t.Parallel()

	var notices strings.Builder
	service := Service{notices: &notices}
	service.emitNotices(domain.Profile{}, []string{"bridge warning"}, nil)
	if !strings.Contains(notices.String(), "Valv MCP note: bridge warning") {
		t.Fatalf("emitNotices() output = %q, want warning text", notices.String())
	}
}
