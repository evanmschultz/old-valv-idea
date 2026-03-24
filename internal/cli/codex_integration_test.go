//go:build integration

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	testcontainers "github.com/testcontainers/testcontainers-go"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/pathutil"
)

func TestCodexCommandRunsFixtureImageEndToEnd(t *testing.T) {
	paths := testCodexPaths(t)
	if err := paths.Ensure(); err != nil {
		t.Fatalf("paths.Ensure() error = %v", err)
	}
	projectRoot := filepath.Join(t.TempDir(), "project")
	workDir := filepath.Join(projectRoot, "subdir")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workDir) error = %v", err)
	}

	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	wantWorkDir, err := pathutil.Normalize(workDir)
	if err != nil {
		t.Fatalf("Normalize(workDir) error = %v", err)
	}
	wantProfileHome, err := pathutil.Normalize(profileHome)
	if err != nil {
		t.Fatalf("Normalize(profileHome) error = %v", err)
	}

	imageRef := buildFixtureImage(t)
	t.Setenv("VALV_CODEX_IMAGE", imageRef)
	runManageForIntegration(t, paths, projectRoot, []string{"profile", "add", "codex", "dev", "--home", profileHome})
	runManageForIntegration(t, paths, workDir, []string{"bind", "codex", "dev"})

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(prevWD)
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"resume", "session-123", "--model", "gpt-5-codex"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}

	runResult := readKeyValueFile(t, filepath.Join(workDir, ".valv-fixture", "codex-run.txt"))
	if got := runResult["pwd"]; got != wantWorkDir {
		t.Fatalf("fixture pwd = %q, want %q", got, wantWorkDir)
	}
	if got := runResult["codex_home"]; got != wantProfileHome {
		t.Fatalf("fixture CODEX_HOME = %q, want %q", got, wantProfileHome)
	}
	if got := runResult["stdin_tty"]; got != "false" {
		t.Fatalf("fixture stdin_tty = %q, want false", got)
	}
	if got := runResult["stdout_tty"]; got != "false" {
		t.Fatalf("fixture stdout_tty = %q, want false", got)
	}
	wantArgs := []string{"resume", "session-123", "--model", "gpt-5-codex"}
	for index, want := range wantArgs {
		key := "arg_" + strconv.Itoa(index)
		if got := runResult[key]; got != want {
			t.Fatalf("fixture %s = %q, want %q", key, got, want)
		}
	}

	homeResult := readKeyValueFile(t, filepath.Join(profileHome, ".valv-fixture-home.txt"))
	if got := homeResult["pwd"]; got != wantWorkDir {
		t.Fatalf("fixture home pwd = %q, want %q", got, wantWorkDir)
	}
}

func buildFixtureImage(t *testing.T) string {
	t.Helper()

	repo := "valv-codex"
	tag := strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	ctx := context.Background()

	container, err := testcontainers.Run(
		ctx,
		"",
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context:    filepath.Join("testdata", "codex-fixture"),
			Dockerfile: "Dockerfile",
			Repo:       repo,
			Tag:        tag,
			KeepImage:  true,
		}),
		testcontainers.WithEntrypoint("sleep", "600"),
	)
	if err != nil {
		t.Fatalf("testcontainers.Run() error = %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	return repo + ":" + tag
}

func runManageForIntegration(t *testing.T, paths config.Paths, workingDir string, args []string) {
	t.Helper()

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", workingDir, err)
	}
	defer func() {
		_ = os.Chdir(prevWD)
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("manage command %v error = %v\nstderr=%s", args, err, stderr.String())
	}
}

func readKeyValueFile(t *testing.T, path string) map[string]string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	values := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for _, line := range lines {
		key, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("malformed key/value line %q in %q", line, path)
		}
		values[key] = value
	}
	return values
}
