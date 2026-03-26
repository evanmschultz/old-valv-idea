package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

func TestManageCommandWithoutTTYShowsHelp(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Operator workflows", "profile", "cleanup"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected manage help output %q missing %q", stdout.String(), want)
		}
	}
}

func TestManageProfileListOutputsStoredProfiles(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	profileHomeA := filepath.Join(paths.ProviderRoot, "codex", "profiles", "alpha")
	profileHomeB := filepath.Join(paths.ProviderRoot, "codex", "profiles", "beta")

	runManage(t, paths, []string{"profile", "add", "codex", "alpha", "--home", profileHomeA})
	runManage(t, paths, []string{"profile", "add", "codex", "beta", "--home", profileHomeB})

	output := runManage(t, paths, []string{"profile", "list", "codex"})
	for _, want := range []string{"codex profiles", "- alpha", "- beta", "home="} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected profile list output %q missing %q", output, want)
		}
	}
}

func TestManageProfileListShowsEmptyState(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	output := runManage(t, paths, []string{"profile", "list", "codex"})
	for _, want := range []string{"codex profiles", "(none)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected empty profile list output %q missing %q", output, want)
		}
	}
}

func TestRunManageBindInteractiveShowsGuidanceWhenNoProfilesExist(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runManageBindInteractive(cmd, paths, &rootOptions{})
	if err == nil || !strings.Contains(err.Error(), "run `valv manage profile add codex <name>` first") {
		t.Fatalf("runManageBindInteractive() error = %v, want profile guidance", err)
	}
}

func TestManageUpdateUsesFakeDockerAndWritesBuildContext(t *testing.T) {
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"update"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Provider image updated") {
		t.Fatalf("unexpected update output: %q", stdout.String())
	}

	dockerfilePath := filepath.Join(paths.BuildCacheDir, string(domain.ProviderCodex), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		t.Fatalf("Stat(%q) error = %v", dockerfilePath, err)
	}

	logContent := mustReadFile(t, logPath)
	for _, want := range []string{"build", "--build-arg CODEX_VERSION=0.116.0", "-t valv-codex:dev", "-t valv-codex:0-116-0"} {
		if !strings.Contains(logContent, want) {
			t.Fatalf("unexpected docker log %q missing %q", logContent, want)
		}
	}
}

func TestManageUpdateUsesOverrideImageRepository(t *testing.T) {
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)
	t.Setenv("VALV_CODEX_IMAGE", "valv-codex-dev:dev")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"update"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}

	logContent := mustReadFile(t, logPath)
	for _, want := range []string{"-t valv-codex-dev:dev", "-t valv-codex-dev:0-116-0"} {
		if !strings.Contains(logContent, want) {
			t.Fatalf("unexpected docker log %q missing %q", logContent, want)
		}
	}
}

func TestManageCleanupAllRemovesLocalStateAndInvokesDocker(t *testing.T) {
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)
	for _, path := range []string{
		paths.LogsDir,
		paths.BuildCacheDir,
		paths.TempCacheDir,
		paths.RuntimeTmpDir,
		paths.PIDsDir,
		paths.LocksDir,
		paths.SocketsDir,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(path, "marker.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"cleanup"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Cleanup completed") {
		t.Fatalf("unexpected cleanup output: %q", stdout.String())
	}
	for _, path := range cleanupPaths(paths) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("Stat(%q) error = %v, want not exist", path, err)
		}
	}

	logContent := mustReadFile(t, logPath)
	if !strings.Contains(logContent, "builder prune --force --all") {
		t.Fatalf("unexpected docker cleanup log: %q", logContent)
	}
}

func TestGlobalSwitchCreatesHostSymlink(t *testing.T) {
	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	runManage(t, paths, []string{"profile", "add", "codex", "dev", "--home", profileHome})
	installFakePgrep(t, 1)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newGlobalCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"switch", "codex", "dev"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Global profile switched") {
		t.Fatalf("unexpected global switch output: %q", stdout.String())
	}

	target := filepath.Join(paths.HomeDir, ".codex")
	linkTarget, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink(%q) error = %v", target, err)
	}
	wantTarget, err := filepath.EvalSymlinks(profileHome)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", profileHome, err)
	}
	if linkTarget != wantTarget {
		t.Fatalf("Readlink(%q) = %q, want %q", target, linkTarget, wantTarget)
	}
}

func TestPickProfileRequiresTTY(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err := pickProfile(cmd, domain.ProviderCodex, []domain.Profile{{Name: "dev", HomePath: "/tmp/dev", Provider: domain.ProviderCodex}})
	if err == nil || !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("pickProfile() error = %v, want TTY failure", err)
	}
}

func TestRunManageBindInteractiveRequiresTTYWhenProfilesExist(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	runManage(t, paths, []string{"profile", "add", "codex", "dev", "--home", profileHome})

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runManageBindInteractive(cmd, paths, &rootOptions{})
	if err == nil || !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("runManageBindInteractive() error = %v, want TTY failure", err)
	}
}

func TestRunGlobalSwitchWithoutProfileRequiresTTY(t *testing.T) {
	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	runManage(t, paths, []string{"profile", "add", "codex", "dev", "--home", profileHome})
	installFakePgrep(t, 1)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runGlobalSwitch(cmd, paths, &rootOptions{}, domain.ProviderCodex, "")
	if err == nil || !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("runGlobalSwitch() error = %v, want TTY failure", err)
	}
}

func TestListItemsForProfilesIncludesHomeAndProvider(t *testing.T) {
	t.Parallel()

	items := listItemsForProfiles([]domain.Profile{{Provider: domain.ProviderCodex, Name: "dev", HomePath: "/tmp/dev"}})
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Title != "dev" {
		t.Fatalf("Title = %q, want dev", items[0].Title)
	}
	if len(items[0].Fields) != 2 {
		t.Fatalf("len(fields) = %d, want 2", len(items[0].Fields))
	}
}

func TestParseOptionalProviderAndRequireProjectPath(t *testing.T) {
	t.Parallel()

	provider, err := parseOptionalProvider(nil, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("parseOptionalProvider(nil) error = %v", err)
	}
	if provider != domain.ProviderCodex {
		t.Fatalf("provider = %q, want codex", provider)
	}
	if _, err := parseOptionalProvider([]string{"unsupported"}, domain.ProviderCodex); err == nil {
		t.Fatal("parseOptionalProvider() error = nil, want unsupported provider")
	}
	if got := requireProjectPath("  /tmp/project  "); got != "/tmp/project" {
		t.Fatalf("requireProjectPath() = %q, want /tmp/project", got)
	}
}

func TestRunAPIServeStartsAndStopsCleanly(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := t.TempDir()
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	runManage(t, paths, []string{"profile", "add", "codex", "dev", "--home", profileHome})
	runManage(t, paths, []string{"bind", "codex", "dev", "--project", projectRoot})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	done := make(chan error, 1)
	go func() {
		done <- runAPIServe(cmd, paths, &rootOptions{}, "127.0.0.1:0", projectRoot, false, time.Minute)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			if strings.Contains(err.Error(), "bind: operation not permitted") {
				t.Skipf("sandbox blocked listener bind: %v", err)
			}
			t.Fatalf("runAPIServe() error = %v\nstderr=%s", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for api server shutdown")
	}

	if !strings.Contains(stdout.String(), "API server starting") {
		t.Fatalf("unexpected api output: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "runtime_ttl=1m0s") {
		t.Fatalf("unexpected api output: %q", stdout.String())
	}
}

func cleanupPaths(paths config.Paths) []string {
	return []string{
		paths.LogsDir,
		paths.BuildCacheDir,
		paths.TempCacheDir,
		paths.RuntimeTmpDir,
		paths.PIDsDir,
		paths.LocksDir,
		paths.SocketsDir,
	}
}

func installFakeDocker(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "docker.log")
	scriptPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$VALV_DOCKER_LOG\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", scriptPath, err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)
	t.Setenv("VALV_DOCKER_LOG", logPath)
	return logPath
}

func installFakePgrep(t *testing.T, exitCode int) {
	t.Helper()

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "pgrep")
	script := fmt.Sprintf("#!/bin/sh\nexit %d\n", exitCode)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", scriptPath, err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}
