//go:build integration

package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/creack/pty/v2"
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

	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "profile-name")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	wantWorkDir, err := pathutil.Normalize(workDir)
	if err != nil {
		t.Fatalf("Normalize(workDir) error = %v", err)
	}
	imageRef := buildFixtureImage(t)
	t.Setenv("VALV_CODEX_IMAGE", imageRef)
	t.Setenv("VALV_REAL_HOME", paths.HomeDir)
	runManageForIntegration(t, paths, projectRoot, []string{"account", "add", "codex", "profile-name", "--home", profileHome, "--skip-login"})
	runManageForIntegration(t, paths, workDir, []string{"bind", "codex", "profile-name"})

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
	if got := runResult["codex_home"]; got != "/home/valv/.codex" {
		t.Fatalf("fixture CODEX_HOME = %q, want /home/valv/.codex", got)
	}
	if got := runResult["home"]; got != "/home/valv" {
		t.Fatalf("fixture HOME = %q, want /home/valv", got)
	}
	if got := runResult["user"]; got != "valv" {
		t.Fatalf("fixture USER = %q, want valv", got)
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

	sharedCodexHome := filepath.Join(paths.HomeDir, ".codex")
	homeResult := readKeyValueFile(t, filepath.Join(sharedCodexHome, ".valv-fixture-home.txt"))
	if got := homeResult["pwd"]; got != wantWorkDir {
		t.Fatalf("fixture home pwd = %q, want %q", got, wantWorkDir)
	}
}

func TestCodexCommandRunsFixtureImageWithTTYEndToEnd(t *testing.T) {
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

	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "profile-name")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	repoRoot := findGoModuleRoot(t, prevWD)
	binaryPath := filepath.Join(t.TempDir(), "valv")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/valv")
	buildCmd.Dir = repoRoot
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v\n%s", err, output)
	}

	imageRef := buildFixtureImage(t)
	t.Setenv("VALV_CODEX_IMAGE", imageRef)
	runValvBinaryCommand(t, binaryPath, paths.HomeDir, projectRoot, imageRef, "manage", "account", "add", "codex", "profile-name", "--home", profileHome, "--skip-login")
	runValvBinaryCommand(t, binaryPath, paths.HomeDir, workDir, imageRef, "manage", "bind", "codex", "profile-name")

	runCmd := exec.Command(binaryPath, "codex", "resume", "session-tty")
	runCmd.Dir = workDir
	runCmd.Env = append(os.Environ(),
		"HOME="+paths.HomeDir,
		"VALV_REAL_HOME="+paths.HomeDir,
		"VALV_TEST_HOME_DIR="+paths.HomeDir,
		"VALV_CODEX_IMAGE="+imageRef,
		valvTestSkipHostCodexLoginEnv+"=1",
	)

	master, err := pty.StartWithSize(runCmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatalf("pty.StartWithSize() error = %v", err)
	}
	defer func() { _ = master.Close() }()

	var stream bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stream, master)
		close(done)
	}()
	if err := runCmd.Wait(); err != nil {
		t.Fatalf("valv codex resume error = %v\nstream=%s", err, stream.String())
	}
	_ = master.Close()
	<-done

	runResult := readKeyValueFile(t, filepath.Join(workDir, ".valv-fixture", "codex-run.txt"))
	if got := runResult["stdin_tty"]; got != "true" {
		t.Fatalf("fixture stdin_tty = %q, want true", got)
	}
	if got := runResult["stdout_tty"]; got != "true" {
		t.Fatalf("fixture stdout_tty = %q, want true", got)
	}
	if got := runResult["codex_home"]; got != "/home/valv/.codex" {
		t.Fatalf("fixture CODEX_HOME = %q, want /home/valv/.codex", got)
	}
}

func TestCodexInteractiveMCPGolden(t *testing.T) {
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

	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "profile-name")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	repoRoot := findGoModuleRoot(t, prevWD)
	fakeMCPPath := buildFixtureMCPServer(t, repoRoot)
	configText := strings.TrimSpace(`
[mcp_servers.context7-mcp]
url = "https://mcp.context7.com/mcp"

[mcp_servers.context7-mcp.env_http_headers]
CONTEXT7_API_KEY = "CONTEXT7_API_KEY"

[mcp_servers.gopls]
command = "`+fakeMCPPath+`"

[mcp_servers.hylla]
url = "http://127.0.0.1:7389/mcp"

[mcp_servers.tillsyn]
command = "`+fakeMCPPath+`"
`) + "\n"
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte(configText), 0o600); err != nil {
		t.Fatalf("WriteFile(profile config) error = %v", err)
	}
	binaryPath := filepath.Join(t.TempDir(), "valv")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/valv")
	buildCmd.Dir = repoRoot
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v\n%s", err, output)
	}

	imageRef := buildFixtureImage(t)
	t.Setenv("VALV_CODEX_IMAGE", imageRef)
	runValvBinaryCommand(t, binaryPath, paths.HomeDir, workDir, imageRef, "manage", "account", "add", "codex", "profile-name", "--home", profileHome, "--skip-login")

	runCmd := exec.Command(binaryPath, "codex", "--no-alt-screen")
	runCmd.Dir = workDir
	runCmd.Env = append(os.Environ(),
		"HOME="+paths.HomeDir,
		"VALV_REAL_HOME="+paths.HomeDir,
		"VALV_TEST_HOME_DIR="+paths.HomeDir,
		"VALV_CODEX_IMAGE="+imageRef,
		valvTestSkipHostCodexLoginEnv+"=1",
	)

	master, err := pty.StartWithSize(runCmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatalf("pty.StartWithSize() error = %v", err)
	}
	defer func() { _ = master.Close() }()

	var stream bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stream, master)
		close(done)
	}()

	waitForTranscript(t, &stream, "Fixture ready.")
	configSnapshot, err := os.ReadFile(filepath.Join(workDir, ".valv-fixture", "codex-config.toml"))
	if err != nil {
		t.Fatalf("ReadFile(codex-config.toml) error = %v", err)
	}
	if !strings.Contains(string(configSnapshot), "mcp_servers") {
		t.Fatalf("fixture codex config missing mcp servers\n%s", configSnapshot)
	}
	if _, err := master.Write([]byte("/mcp\n")); err != nil {
		t.Fatalf("Write(/mcp) error = %v", err)
	}
	waitForTranscript(t, &stream, "context7-mcp")
	waitForTranscript(t, &stream, "gopls")
	waitForTranscript(t, &stream, "tillsyn")
	if _, err := master.Write([]byte("/quit\n")); err != nil {
		t.Fatalf("Write(/quit) error = %v", err)
	}

	if err := runCmd.Wait(); err != nil {
		t.Fatalf("valv codex --no-alt-screen error = %v\nstream=%s", err, stream.String())
	}
	_ = master.Close()
	<-done

	golden.RequireEqual(t, normalizeTranscript(stream.Bytes()))
}

func buildFixtureImage(t *testing.T) string {
	t.Helper()

	repo := "valv-codex"
	tag := strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	imageRef := repo + ":" + tag
	ctx := context.Background()
	if err := exec.CommandContext(ctx, "docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}

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
	cleanupManagedFixtureContainers(t)
	t.Cleanup(func() {
		cleanupManagedFixtureContainers(t)
		_ = container.Terminate(ctx)
	})

	return imageRef
}

func buildFixtureMCPServer(t *testing.T, repoRoot string) string {
	t.Helper()

	binaryPath := filepath.Join(t.TempDir(), "fixture-mcp")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./internal/cli/testdata/fake-mcp")
	buildCmd.Dir = repoRoot
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build fake mcp error = %v\n%s", err, output)
	}
	return binaryPath
}

func runManageForIntegration(t *testing.T, paths config.Paths, workingDir string, args []string) {
	t.Helper()

	t.Setenv(valvTestSkipHostCodexLoginEnv, "1")

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

func findGoModuleRoot(t *testing.T, start string) string {
	t.Helper()

	current := start
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("go.mod not found from %q upward", start)
		}
		current = parent
	}
}

func runValvBinaryCommand(t *testing.T, binaryPath, homeDir, workingDir, imageRef string, args ...string) {
	t.Helper()

	command := exec.Command(binaryPath, args...)
	command.Dir = workingDir
	command.Env = append(os.Environ(),
		"HOME="+homeDir,
		"VALV_REAL_HOME="+homeDir,
		"VALV_TEST_HOME_DIR="+homeDir,
		"VALV_CODEX_IMAGE="+imageRef,
		valvTestSkipHostCodexLoginEnv+"=1",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("valv binary %v error = %v\n%s", args, err, output)
	}
}

func waitForTranscript(t *testing.T, stream *bytes.Buffer, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stream.String(), want) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for transcript to contain %q\nstream=%s", want, stream.String())
}

func cleanupManagedFixtureContainers(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	listCmd := exec.CommandContext(ctx, "docker", "ps", "-aq",
		"--filter", "label=io.valv.managed=true",
		"--filter", "name=^valv-codex-interactive-",
	)
	output, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker ps for fixture cleanup error = %v\n%s", err, output)
	}
	ids := strings.Fields(string(output))
	if len(ids) == 0 {
		return
	}

	inspectArgs := append([]string{"inspect", "--format", "{{.Id}} {{.Config.Image}}"}, ids...)
	inspectCmd := exec.CommandContext(ctx, "docker", inspectArgs...)
	inspectOutput, err := inspectCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker inspect for fixture cleanup error = %v\n%s", err, inspectOutput)
	}
	removeIDs := make([]string, 0, len(ids))
	for _, line := range strings.Split(strings.TrimSpace(string(inspectOutput)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.HasPrefix(fields[1], "valv-codex:test") {
			removeIDs = append(removeIDs, fields[0])
		}
	}
	if len(removeIDs) == 0 {
		return
	}
	args := append([]string{"rm", "-f"}, removeIDs...)
	removeCmd := exec.CommandContext(ctx, "docker", args...)
	if output, err := removeCmd.CombinedOutput(); err != nil {
		t.Fatalf("docker rm fixture cleanup error = %v\n%s", err, output)
	}
}

var (
	ansiPattern      = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	bridgeURLPattern = regexp.MustCompile(`http://host\.docker\.internal:\d+/mcp/([a-z0-9-]+)-\d+`)
)

func normalizeTranscript(input []byte) []byte {
	text := string(input)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = ansiPattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\x1b]0;", "")
	text = strings.ReplaceAll(text, "\x1b\\", "")
	text = bridgeURLPattern.ReplaceAllString(text, "http://host.docker.internal:<port>/mcp/$1-<token>")
	return []byte(strings.TrimSpace(text) + "\n")
}
