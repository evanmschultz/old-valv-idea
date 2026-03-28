package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/pathutil"
)

func TestPrepareRuntimeNormalizesEnvAndTranslatesConfig(t *testing.T) {
	t.Setenv("CONTEXT7_API_KEY", "test-key")

	root := t.TempDir()
	profileHome := filepath.Join(root, "profile")
	projectRoot := filepath.Join(root, "project")
	tempRoot := filepath.Join(root, "tmp")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatalf("MkdirAll(project config dir) error = %v", err)
	}
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte(strings.TrimSpace(`
[mcp_servers.loopback]
url = "http://127.0.0.1:7389/mcp"

[mcp_servers.context7]
url = "https://mcp.context7.com/mcp"

[mcp_servers.context7.env_http_headers]
CONTEXT7_API_KEY = "CONTEXT7_API_KEY"

[mcp_servers.bridge]
command = "`+os.Args[0]+`"
args = ["-test.run=TestBridgeHelperProcess"]

[mcp_servers.bridge.env]
VALV_MCP_HELPER_PROCESS = "1"
`)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(profile config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "config.toml"), []byte(strings.TrimSpace(`
[mcp_servers.project_loopback]
url = "http://localhost:4242/mcp"
`)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(project config) error = %v", err)
	}

	prepared, err := PrepareRuntime(context.Background(), PrepareRequest{
		ProfileHome: profileHome,
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Fatalf("prepared.Close() error = %v", err)
		}
	}()

	if got := prepared.Env["CODEX_HOME"]; got != ContainerCodexDir {
		t.Fatalf("CODEX_HOME = %q, want %q", got, ContainerCodexDir)
	}
	if got := prepared.Env["HOME"]; got != ContainerHomeDir {
		t.Fatalf("HOME = %q, want %q", got, ContainerHomeDir)
	}
	if got := prepared.Env["USER"]; got != "valv" {
		t.Fatalf("USER = %q, want valv", got)
	}
	if !containsString(prepared.EnvPassthrough, "CONTEXT7_API_KEY") {
		t.Fatalf("EnvPassthrough = %#v, want CONTEXT7_API_KEY", prepared.EnvPassthrough)
	}

	if len(prepared.Mounts) != 3 {
		t.Fatalf("mount count = %d, want 3", len(prepared.Mounts))
	}
	wantProfileHome, err := pathutil.Normalize(profileHome)
	if err != nil {
		t.Fatalf("Normalize(profileHome) error = %v", err)
	}
	wantProjectRoot, err := pathutil.Normalize(projectRoot)
	if err != nil {
		t.Fatalf("Normalize(projectRoot) error = %v", err)
	}
	if got := prepared.Mounts[0]; got != newProfileMount(wantProfileHome) {
		t.Fatalf("profile mount = %+v, want %+v", got, newProfileMount(wantProfileHome))
	}

	profileOverlay := findMountTarget(t, prepared.Mounts, filepath.Join(ContainerCodexDir, "config.toml"))
	projectOverlay := findMountTarget(t, prepared.Mounts, filepath.Join(wantProjectRoot, ".codex", "config.toml"))

	profileContent, err := os.ReadFile(profileOverlay.Source)
	if err != nil {
		t.Fatalf("ReadFile(profile overlay) error = %v", err)
	}
	profileText := string(profileContent)
	if !strings.Contains(profileText, "http://host.docker.internal:7389/mcp") {
		t.Fatalf("profile overlay missing loopback rewrite: %s", profileText)
	}
	if !strings.Contains(profileText, "[mcp_servers.context7.env_http_headers]") {
		t.Fatalf("profile overlay missing env_http_headers table: %s", profileText)
	}
	if !strings.Contains(profileText, `url = "http://host.docker.internal:`) {
		t.Fatalf("profile overlay missing bridge URL: %s", profileText)
	}
	if strings.Contains(profileText, "command =") {
		t.Fatalf("profile overlay still contains stdio command: %s", profileText)
	}

	projectContent, err := os.ReadFile(projectOverlay.Source)
	if err != nil {
		t.Fatalf("ReadFile(project overlay) error = %v", err)
	}
	if !strings.Contains(string(projectContent), "http://host.docker.internal:4242/mcp") {
		t.Fatalf("project overlay missing loopback rewrite: %s", string(projectContent))
	}
}

func TestPrepareRuntimeOmitsBrokenHostCommandBridgeEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	profileHome := filepath.Join(root, "profile")
	projectRoot := filepath.Join(root, "project")
	tempRoot := filepath.Join(root, "tmp")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0o755); err != nil {
		t.Fatalf("MkdirAll(project config dir) error = %v", err)
	}
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte(strings.TrimSpace(`
[mcp_servers.good]
url = "http://127.0.0.1:7389/mcp"

[mcp_servers.bad]
command = "/path/that/does/not/exist"
`)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(profile config) error = %v", err)
	}

	prepared, err := PrepareRuntime(context.Background(), PrepareRequest{
		ProfileHome: profileHome,
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Fatalf("prepared.Close() error = %v", err)
		}
	}()

	profileOverlay := findMountTarget(t, prepared.Mounts, filepath.Join(ContainerCodexDir, "config.toml"))
	profileContent, err := os.ReadFile(profileOverlay.Source)
	if err != nil {
		t.Fatalf("ReadFile(profile overlay) error = %v", err)
	}
	profileText := string(profileContent)
	if strings.Contains(profileText, "[mcp_servers.bad]") {
		t.Fatalf("profile overlay kept broken bridged server: %s", profileText)
	}
	if !strings.Contains(profileText, "[mcp_servers.good]") {
		t.Fatalf("profile overlay removed working server unexpectedly: %s", profileText)
	}
	if len(prepared.Warnings) == 0 || !strings.Contains(strings.Join(prepared.Warnings, "\n"), "bad:") {
		t.Fatalf("prepared.Warnings = %#v, want broken bridge warning", prepared.Warnings)
	}
}

func TestPrepareRuntimePassesThroughRemoteMCPHeaderEnvWithoutOverlay(t *testing.T) {
	t.Setenv("CONTEXT7_API_KEY", "test-key")

	root := t.TempDir()
	profileHome := filepath.Join(root, "profile")
	projectRoot := filepath.Join(root, "project")
	tempRoot := filepath.Join(root, "tmp")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte(strings.TrimSpace(`
[mcp_servers.context7]
url = "https://mcp.context7.com/mcp"

[mcp_servers.context7.env_http_headers]
CONTEXT7_API_KEY = "CONTEXT7_API_KEY"
`)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(profile config) error = %v", err)
	}

	prepared, err := PrepareRuntime(context.Background(), PrepareRequest{
		ProfileHome: profileHome,
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Fatalf("prepared.Close() error = %v", err)
		}
	}()

	if len(prepared.Mounts) != 1 {
		t.Fatalf("mount count = %d, want 1 profile mount only", len(prepared.Mounts))
	}
	if !containsString(prepared.EnvPassthrough, "CONTEXT7_API_KEY") {
		t.Fatalf("EnvPassthrough = %#v, want CONTEXT7_API_KEY", prepared.EnvPassthrough)
	}
}

func TestPrepareRuntimePassesThroughTerminalEnv(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")

	root := t.TempDir()
	profileHome := filepath.Join(root, "profile")
	projectRoot := filepath.Join(root, "project")
	tempRoot := filepath.Join(root, "tmp")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot) error = %v", err)
	}

	prepared, err := PrepareRuntime(context.Background(), PrepareRequest{
		ProfileHome: profileHome,
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Fatalf("prepared.Close() error = %v", err)
		}
	}()

	if got := prepared.Env["TERM"]; got != "xterm-256color" {
		t.Fatalf("Env TERM = %q, want xterm-256color", got)
	}
	for _, name := range []string{"COLORTERM", "TERM_PROGRAM"} {
		if !containsString(prepared.EnvPassthrough, name) {
			t.Fatalf("EnvPassthrough = %#v, want %s", prepared.EnvPassthrough, name)
		}
	}
	if containsString(prepared.EnvPassthrough, "TERM") {
		t.Fatalf("EnvPassthrough = %#v, TERM should not be passed through directly", prepared.EnvPassthrough)
	}
}

func TestPrepareRuntimeNormalizesUnsupportedTERM(t *testing.T) {
	t.Setenv("TERM", "xterm-ghostty")

	root := t.TempDir()
	profileHome := filepath.Join(root, "profile")
	projectRoot := filepath.Join(root, "project")
	tempRoot := filepath.Join(root, "tmp")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot) error = %v", err)
	}

	prepared, err := PrepareRuntime(context.Background(), PrepareRequest{
		ProfileHome: profileHome,
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Fatalf("prepared.Close() error = %v", err)
		}
	}()

	if got := prepared.Env["TERM"]; got != "xterm-256color" {
		t.Fatalf("Env TERM = %q, want xterm-256color", got)
	}
}

func newProfileMount(profileHome string) dockeradapter.MountSpec {
	return dockeradapter.MountSpec{
		Source:   profileHome,
		Target:   ContainerCodexDir,
		ReadOnly: false,
	}
}

func findMountTarget(t *testing.T, mounts []dockeradapter.MountSpec, target string) dockeradapter.MountSpec {
	t.Helper()
	for _, mount := range mounts {
		if mount.Target == target {
			return mount
		}
	}
	t.Fatalf("target %q not found in mounts: %+v", target, mounts)
	return dockeradapter.MountSpec{}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
