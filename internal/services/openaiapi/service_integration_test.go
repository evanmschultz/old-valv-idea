//go:build integration

package openaiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	testcontainers "github.com/testcontainers/testcontainers-go"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	sqliteadapter "github.com/evanmschultz/valv/internal/adapters/sqlite"
	openaihandler "github.com/evanmschultz/valv/internal/api/openai"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/pathutil"
	manageservice "github.com/evanmschultz/valv/internal/services/manage"
)

func TestHandlerServesChatCompletionsViaRealDockerFixture(t *testing.T) {
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}
	paths := testOpenAIPaths(t)
	if err := paths.Ensure(); err != nil {
		t.Fatalf("paths.Ensure() error = %v", err)
	}
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	store, err := sqliteadapter.NewStore(paths.DatabasePath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	manageSvc, err := manageservice.New(manageservice.Options{Store: store, ProviderRoot: paths.ProviderRoot})
	if err != nil {
		t.Fatalf("manage.New() error = %v", err)
	}
	if _, err := manageSvc.CreateProfile(context.Background(), domain.ProviderCodex, "dev", profileHome); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if _, err := manageSvc.BindProject(context.Background(), domain.ProviderCodex, "dev", projectRoot); err != nil {
		t.Fatalf("BindProject() error = %v", err)
	}
	normalizedProjectRoot, err := pathutil.Normalize(projectRoot)
	if err != nil {
		t.Fatalf("Normalize(projectRoot) error = %v", err)
	}
	project, err := store.ProjectByRoot(context.Background(), normalizedProjectRoot)
	if err != nil {
		t.Fatalf("ProjectByRoot() error = %v", err)
	}

	imageRef := buildFixtureImage(t)
	executor := dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", nil, &bytes.Buffer{}, &bytes.Buffer{}))
	t.Cleanup(func() {
		records, err := store.ListRuntimesByProjectID(context.Background(), project.ID)
		if err != nil {
			return
		}
		for _, record := range records {
			_ = executor.RemoveContainer(context.Background(), dockeradapter.ContainerRemoveRequest{IDs: []string{record.ContainerID}, Force: true, Volumes: true})
		}
	})
	service, err := New(Options{Store: store, Executor: executor, Image: dockeradapter.NewImageRef(strings.Split(imageRef, ":")[0], strings.Split(imageRef, ":")[1]), TempRoot: paths.TempCacheDir, StartPath: projectRoot})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	handler, err := openaihandler.NewHandler(service, openaihandler.Options{})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	body := bytes.NewBufferString(`{"model":"gpt-5.2","messages":[{"role":"user","content":"say hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, openaihandler.ChatCompletionsPath, body)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if got, want := rr.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body=%s", got, want, rr.Body.String())
	}
	var response openaihandler.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !strings.Contains(response.Choices[0].Message.Content, "fixture exec response") {
		t.Fatalf("response content = %q, want fixture exec response", response.Choices[0].Message.Content)
	}
}

func buildFixtureImage(t *testing.T) string {
	t.Helper()
	repo := "valv-codex"
	tag := strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	ctx := context.Background()
	_, thisFile, _, _ := runtime.Caller(0)
	fixtureDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "cli", "testdata", "codex-fixture")
	container, err := testcontainers.Run(
		ctx,
		"",
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{Context: fixtureDir, Dockerfile: "Dockerfile", Repo: repo, Tag: tag, KeepImage: true}),
		testcontainers.WithEntrypoint("sleep", "600"),
	)
	if err != nil {
		t.Fatalf("testcontainers.Run() error = %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	return repo + ":" + tag
}

func testOpenAIPaths(t *testing.T) config.Paths {
	t.Helper()
	root := t.TempDir()
	return config.Paths{
		HomeDir:        root,
		AppSupportRoot: root,
		DatabaseDir:    filepath.Join(root, "db"),
		DatabasePath:   filepath.Join(root, "db", "valv.sqlite3"),
		ProviderRoot:   filepath.Join(root, "providers"),
		StateDir:       filepath.Join(root, "state"),
		ConfigDir:      filepath.Join(root, "config"),
		LogsDir:        filepath.Join(root, "logs"),
		CachesDir:      filepath.Join(root, "caches"),
		BuildCacheDir:  filepath.Join(root, "caches", "build"),
		TempCacheDir:   filepath.Join(root, "caches", "tmp"),
		RuntimeTmpDir:  filepath.Join(root, "runtime"),
		PIDsDir:        filepath.Join(root, "runtime", "pids"),
		LocksDir:       filepath.Join(root, "runtime", "locks"),
		SocketsDir:     filepath.Join(root, "runtime", "sockets"),
	}
}
