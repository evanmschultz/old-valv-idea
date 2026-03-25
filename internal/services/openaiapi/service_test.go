package openaiapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	sqliteadapter "github.com/evanmschultz/valv/internal/adapters/sqlite"
	openaiapi "github.com/evanmschultz/valv/internal/api/openai"
	"github.com/evanmschultz/valv/internal/domain"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

func TestRenderPromptIncludesRolesAndNames(t *testing.T) {
	t.Parallel()

	prompt := renderPrompt(openaiapi.Request{
		Model:    "gpt-5.2",
		Messages: []openaiapi.Message{{Role: openaiapi.RoleSystem, Content: "You are precise."}, {Role: openaiapi.RoleUser, Name: "evan", Content: "Say hi."}},
	})
	for _, want := range []string{"SYSTEM:", "You are precise.", "USER (evan):", "Say hi."} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q in %q", want, prompt)
		}
	}
}

func TestNewRequiresDependencies(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatal("New() error = nil, want dependency failure")
	}
}

func TestBuildRuntimeRequestOmitsWorkspaceByDefault(t *testing.T) {
	t.Parallel()

	service := Service{image: dockeradapter.NewImageRef("valv-codex", "dev"), tempRoot: "/tmp/valv-api"}
	project := domain.Project{Root: "/tmp/project"}
	profile := domain.Profile{HomePath: "/tmp/profile"}
	record := domain.RuntimeRecord{ContainerID: "valv-api-codex-nowork-123"}

	request, err := service.buildRuntimeRequest(record, project, profile)
	if err != nil {
		t.Fatalf("buildRuntimeRequest() error = %v", err)
	}
	if request.WorkingDir != "/tmp" {
		t.Fatalf("WorkingDir = %q, want /tmp", request.WorkingDir)
	}
	if !request.Detached {
		t.Fatal("Detached = false, want true")
	}
	if len(request.Mounts) != 2 {
		t.Fatalf("mount count = %d, want 2", len(request.Mounts))
	}
	if got := request.Env["CODEX_HOME"]; got != "/tmp/profile" {
		t.Fatalf("CODEX_HOME = %q, want /tmp/profile", got)
	}
}

func TestBuildRuntimeRequestIncludesWorkspaceWhenEnabled(t *testing.T) {
	t.Parallel()

	service := Service{image: dockeradapter.NewImageRef("valv-codex", "dev"), tempRoot: "/tmp/valv-api", workspaceAccess: true}
	project := domain.Project{Root: "/tmp/project"}
	profile := domain.Profile{HomePath: "/tmp/profile"}
	record := domain.RuntimeRecord{ContainerID: "valv-api-codex-ws-123"}

	request, err := service.buildRuntimeRequest(record, project, profile)
	if err != nil {
		t.Fatalf("buildRuntimeRequest() error = %v", err)
	}
	if request.WorkingDir != "/tmp/project" {
		t.Fatalf("WorkingDir = %q, want /tmp/project", request.WorkingDir)
	}
	if len(request.Mounts) != 3 {
		t.Fatalf("mount count = %d, want 3", len(request.Mounts))
	}
}

func TestBuildExecRequestOmitsWorkspaceByDefault(t *testing.T) {
	t.Parallel()

	service := Service{image: dockeradapter.NewImageRef("valv-codex", "dev")}
	project := domain.Project{Root: "/tmp/project"}
	profile := domain.Profile{HomePath: "/tmp/profile"}
	record := domain.RuntimeRecord{ContainerID: "valv-api-codex-nowork-123"}

	request, err := service.buildExecRequest(openaiapi.Request{Model: "gpt-5.2", Messages: []openaiapi.Message{{Role: openaiapi.RoleUser, Content: "hello"}}}, record, project, profile, "/tmp/result", "/tmp/result/out.txt")
	if err != nil {
		t.Fatalf("buildExecRequest() error = %v", err)
	}
	if request.WorkingDir != "/tmp" {
		t.Fatalf("WorkingDir = %q, want /tmp", request.WorkingDir)
	}
	if strings.Contains(strings.Join(request.Args, " "), "--cd /tmp/project") {
		t.Fatalf("args unexpectedly include project cd: %#v", request.Args)
	}
	if request.ContainerID != record.ContainerID {
		t.Fatalf("ContainerID = %q, want %q", request.ContainerID, record.ContainerID)
	}
}

func TestBuildExecRequestIncludesWorkspaceWhenEnabled(t *testing.T) {
	t.Parallel()

	service := Service{image: dockeradapter.NewImageRef("valv-codex", "dev"), workspaceAccess: true}
	project := domain.Project{Root: "/tmp/project"}
	profile := domain.Profile{HomePath: "/tmp/profile"}
	record := domain.RuntimeRecord{ContainerID: "valv-api-codex-ws-123"}

	request, err := service.buildExecRequest(openaiapi.Request{Model: "gpt-5.2", Messages: []openaiapi.Message{{Role: openaiapi.RoleUser, Content: "hello"}}}, record, project, profile, "/tmp/result", "/tmp/result/out.txt")
	if err != nil {
		t.Fatalf("buildExecRequest() error = %v", err)
	}
	if request.WorkingDir != "/tmp/project" {
		t.Fatalf("WorkingDir = %q, want /tmp/project", request.WorkingDir)
	}
	if !strings.Contains(strings.Join(request.Args, " "), "--cd /tmp/project") {
		t.Fatalf("args missing project cd: %#v", request.Args)
	}
}

func TestValidateBindingReturnsUnboundProject(t *testing.T) {
	t.Parallel()

	store := newOpenAIStore(t)
	service, err := New(Options{
		Store:     store,
		Executor:  &recordingExecutor{},
		Image:     dockeradapter.NewImageRef("valv-codex", "dev"),
		TempRoot:  t.TempDir(),
		StartPath: "/tmp/project",
		Detect: func(string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: "/tmp/project"}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.ValidateBinding(context.Background()); !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("ValidateBinding() error = %v, want ErrUnboundProject", err)
	}
}

func TestCompleteCreatesAndReusesWarmRuntimeUsingRealStore(t *testing.T) {
	t.Parallel()

	store, project, profile := seededOpenAIStore(t, "/tmp/project", "/tmp/profile")
	now := time.Date(2026, 3, 25, 15, 0, 0, 0, time.UTC)
	executor := &recordingExecutor{content: "fixture response"}
	service, err := New(Options{
		Store:     store,
		Executor:  executor,
		Image:     dockeradapter.NewImageRef("valv-codex", "dev"),
		TempRoot:  t.TempDir(),
		StartPath: "/tmp/project",
		IdleTTL:   time.Minute,
		Now:       func() time.Time { return now },
		Detect: func(string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: "/tmp/project"}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first, err := service.Complete(context.Background(), openaiapi.Request{
		Model:    "gpt-5.2",
		Messages: []openaiapi.Message{{Role: openaiapi.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete() first error = %v", err)
	}
	now = now.Add(30 * time.Second)
	second, err := service.Complete(context.Background(), openaiapi.Request{
		Model:    "gpt-5.2",
		Messages: []openaiapi.Message{{Role: openaiapi.RoleUser, Content: "hello again"}},
	})
	if err != nil {
		t.Fatalf("Complete() second error = %v", err)
	}

	if first.Content != "fixture response" || second.Content != "fixture response" {
		t.Fatalf("unexpected content: first=%q second=%q", first.Content, second.Content)
	}
	if len(executor.runRequests) != 1 {
		t.Fatalf("runRequests len = %d, want 1", len(executor.runRequests))
	}
	if len(executor.execRequests) != 2 {
		t.Fatalf("execRequests len = %d, want 2", len(executor.execRequests))
	}
	if len(executor.inspectCalls) != 1 {
		t.Fatalf("inspectCalls len = %d, want 1", len(executor.inspectCalls))
	}
	if executor.execRequests[0].ContainerID != executor.execRequests[1].ContainerID {
		t.Fatalf("warm runtime not reused: %q vs %q", executor.execRequests[0].ContainerID, executor.execRequests[1].ContainerID)
	}

	records, err := store.ListRuntimesByProjectID(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListRuntimesByProjectID() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("runtime record count = %d, want 1", len(records))
	}
	if records[0].ProfileID != profile.ID {
		t.Fatalf("runtime profile id = %q, want %q", records[0].ProfileID, profile.ID)
	}
	if records[0].Status != "running" {
		t.Fatalf("runtime status = %q, want running", records[0].Status)
	}
}

func TestCompleteExpiresWarmRuntimeAndStartsReplacement(t *testing.T) {
	t.Parallel()

	store, project, _ := seededOpenAIStore(t, "/tmp/project", "/tmp/profile")
	now := time.Date(2026, 3, 25, 15, 0, 0, 0, time.UTC)
	executor := &recordingExecutor{content: "fixture response"}
	service, err := New(Options{
		Store:     store,
		Executor:  executor,
		Image:     dockeradapter.NewImageRef("valv-codex", "dev"),
		TempRoot:  t.TempDir(),
		StartPath: "/tmp/project",
		IdleTTL:   time.Minute,
		Now:       func() time.Time { return now },
		Detect: func(string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: "/tmp/project"}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := service.Complete(context.Background(), openaiapi.Request{Model: "gpt-5.2", Messages: []openaiapi.Message{{Role: openaiapi.RoleUser, Content: "hello"}}}); err != nil {
		t.Fatalf("Complete() first error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := service.Complete(context.Background(), openaiapi.Request{Model: "gpt-5.2", Messages: []openaiapi.Message{{Role: openaiapi.RoleUser, Content: "hello again"}}}); err != nil {
		t.Fatalf("Complete() second error = %v", err)
	}

	if len(executor.removedRequests) != 1 {
		t.Fatalf("removedRequests len = %d, want 1", len(executor.removedRequests))
	}
	if len(executor.runRequests) != 2 {
		t.Fatalf("runRequests len = %d, want 2", len(executor.runRequests))
	}

	records, err := store.ListRuntimesByProjectID(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListRuntimesByProjectID() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("runtime record count = %d, want 2", len(records))
	}
}

type recordingExecutor struct {
	runRequests     []dockeradapter.ContainerRunRequest
	execRequests    []dockeradapter.ContainerExecRequest
	removedRequests []dockeradapter.ContainerRemoveRequest
	inspectCalls    []string
	content         string
	inspectErr      error
}

func (e *recordingExecutor) Run(_ context.Context, request dockeradapter.ContainerRunRequest) error {
	e.runRequests = append(e.runRequests, request)
	return nil
}

func (e *recordingExecutor) Exec(_ context.Context, request dockeradapter.ContainerExecRequest) error {
	e.execRequests = append(e.execRequests, request)
	for i := 0; i < len(request.Args); i++ {
		if request.Args[i] == "--output-last-message" || request.Args[i] == "-o" {
			if i+1 < len(request.Args) {
				if err := os.MkdirAll(filepath.Dir(request.Args[i+1]), 0o755); err != nil {
					return err
				}
				return os.WriteFile(request.Args[i+1], []byte(e.content), 0o644)
			}
		}
	}
	return fmt.Errorf("output-last-message argument not found")
}

func (e *recordingExecutor) Inspect(_ context.Context, name string) error {
	e.inspectCalls = append(e.inspectCalls, name)
	return e.inspectErr
}

func (e *recordingExecutor) RemoveContainer(_ context.Context, request dockeradapter.ContainerRemoveRequest) error {
	e.removedRequests = append(e.removedRequests, request)
	return nil
}

func newOpenAIStore(t *testing.T) *sqliteadapter.Store {
	t.Helper()
	store, err := sqliteadapter.NewStore(filepath.Join(t.TempDir(), "valv.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seededOpenAIStore(t *testing.T, projectRoot, profileHome string) (*sqliteadapter.Store, domain.Project, domain.Profile) {
	t.Helper()

	store := newOpenAIStore(t)
	project := mustOpenAIProject(t, projectRoot)
	profile := mustOpenAIProfile(t, "dev", profileHome)
	if _, err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	binding, err := domain.NewProjectBinding(project.ID, profile.ID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("NewProjectBinding() error = %v", err)
	}
	if _, err := store.UpsertProjectBinding(context.Background(), binding); err != nil {
		t.Fatalf("UpsertProjectBinding() error = %v", err)
	}
	return store, project, profile
}

func mustOpenAIProject(t *testing.T, root string) domain.Project {
	t.Helper()
	project, err := domain.NewProject(root)
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	return project
}

func mustOpenAIProfile(t *testing.T, name, home string) domain.Profile {
	t.Helper()
	profile, err := domain.NewProfile(domain.ProviderCodex, name, home)
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	return profile
}
