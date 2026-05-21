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

func TestPrepareRuntimePreservesHostTERM(t *testing.T) {
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

	if got := prepared.Env["TERM"]; got != "xterm-ghostty" {
		t.Fatalf("Env TERM = %q, want xterm-ghostty", got)
	}
}

func TestPrepareRuntimeFallsBackWhenTERMEmpty(t *testing.T) {
	t.Setenv("TERM", "")

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

func TestPrepareRuntimeUsesSharedHomeAndOverlaysAccountAuth(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sharedHome := filepath.Join(root, "shared")
	profileHome := filepath.Join(root, "profile")
	projectRoot := filepath.Join(root, "project")
	tempRoot := filepath.Join(root, "tmp")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	if err := os.MkdirAll(sharedHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(sharedHome) error = %v", err)
	}
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sharedHome, "history.txt"), []byte("shared"), 0o600); err != nil {
		t.Fatalf("WriteFile(shared history) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(`{"auth_mode":"chatgpt"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(auth.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte("[mcp_servers.test]\nurl = \"http://127.0.0.1:7389/mcp\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	prepared, err := PrepareRuntime(context.Background(), PrepareRequest{
		ProfileHome: profileHome,
		SharedHome:  sharedHome,
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	gotShared, err := pathutil.Normalize(prepared.Mounts[0].Source)
	if err != nil {
		t.Fatalf("Normalize(shared mount source) error = %v", err)
	}
	if got := dockeradapter.NewMountSpec(gotShared, prepared.Mounts[0].Target, prepared.Mounts[0].ReadOnly); got.Target != ContainerCodexDir || got.ReadOnly {
		t.Fatalf("runtime mount = %+v, want writable mount to %q", got, ContainerCodexDir)
	}
	if gotShared == sharedHome {
		t.Fatalf("runtime mount source = %q, want staged runtime dir distinct from shared home %q", gotShared, sharedHome)
	}
	runtimeAuthPath := filepath.Join(prepared.Mounts[0].Source, "auth.json")
	wantAuth, err := pathutil.Normalize(runtimeAuthPath)
	if err != nil {
		t.Fatalf("Normalize(runtime auth path) error = %v", err)
	}
	gotAuth, err := pathutil.Normalize(runtimeAuthPath)
	if err != nil {
		t.Fatalf("Normalize(got auth path) error = %v", err)
	}
	if gotAuth != wantAuth {
		t.Fatalf("runtime auth path = %q, want %q", gotAuth, wantAuth)
	}
	authContent, err := os.ReadFile(runtimeAuthPath)
	if err != nil {
		t.Fatalf("ReadFile(runtime auth) error = %v", err)
	}
	if string(authContent) != `{"auth_mode":"chatgpt"}` {
		t.Fatalf("runtime auth = %q, want staged account auth", string(authContent))
	}
	configContent, err := os.ReadFile(filepath.Join(prepared.Mounts[0].Source, "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile(runtime config) error = %v", err)
	}
	if !strings.Contains(string(configContent), "host.docker.internal") {
		t.Fatalf("runtime config missing translated overlay: %s", string(configContent))
	}
	if err := os.WriteFile(filepath.Join(prepared.Mounts[0].Source, ".valv-fixture-home.txt"), []byte("pwd=/workspace\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(runtime shared state) error = %v", err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatalf("prepared.Close() error = %v", err)
	}
	if got := string(mustReadFile(t, filepath.Join(sharedHome, ".valv-fixture-home.txt"))); got != "pwd=/workspace\n" {
		t.Fatalf("shared home sync = %q, want persisted runtime state", got)
	}
	if _, err := os.Stat(filepath.Join(sharedHome, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("shared home auth.json should not be synced back, stat err = %v", err)
	}
}

func TestPrepareRuntimeSkipsClaudeMountWhenNotProvided(t *testing.T) {
	t.Parallel()

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
		ProfileHome:              profileHome,
		ProjectRoot:              projectRoot,
		TempRoot:                 tempRoot,
		OtherProviderProfileHome: "",
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Fatalf("prepared.Close() error = %v", err)
		}
	}()

	if _, ok := prepared.Env["CLAUDE_CONFIG_DIR"]; ok {
		t.Fatalf("Env[CLAUDE_CONFIG_DIR] = %q, want absent when OtherProviderProfileHome is empty", prepared.Env["CLAUDE_CONFIG_DIR"])
	}
	for _, mount := range prepared.Mounts {
		if mount.Target == "/home/valv/.claude" {
			t.Fatalf("found unexpected mount with target /home/valv/.claude: %+v", mount)
		}
	}
}

func TestPrepareRuntimeMountsClaudeHomeWhenProvided(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	profileHome := filepath.Join(root, "profile")
	projectRoot := filepath.Join(root, "project")
	tempRoot := filepath.Join(root, "tmp")
	claudeHome := filepath.Join(root, "claude-profile")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot) error = %v", err)
	}
	if err := os.MkdirAll(claudeHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(claudeHome) error = %v", err)
	}

	prepared, err := PrepareRuntime(context.Background(), PrepareRequest{
		ProfileHome:              profileHome,
		ProjectRoot:              projectRoot,
		TempRoot:                 tempRoot,
		OtherProviderProfileHome: claudeHome,
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Fatalf("prepared.Close() error = %v", err)
		}
	}()

	if got := prepared.Env["CLAUDE_CONFIG_DIR"]; got != "/home/valv/.claude" {
		t.Fatalf("Env[CLAUDE_CONFIG_DIR] = %q, want /home/valv/.claude", got)
	}
	wantClaudeHome, err := pathutil.Normalize(claudeHome)
	if err != nil {
		t.Fatalf("Normalize(claudeHome) error = %v", err)
	}
	claudeMount := findMountTarget(t, prepared.Mounts, "/home/valv/.claude")
	if claudeMount.Source != wantClaudeHome {
		t.Fatalf("claude mount source = %q, want %q", claudeMount.Source, wantClaudeHome)
	}
	if claudeMount.ReadOnly {
		t.Fatalf("claude mount should be read-write, got ReadOnly=true")
	}
}

func TestPrepareRuntimeMountsWorktreeGitDir(t *testing.T) {
	t.Parallel()

	// Layout:
	//   <tmp>/bare/             ← bare repo (common git dir)
	//   <tmp>/worktrees/main/   ← worktree gitdir
	//   <tmp>/project/.git      ← linkfile -> <tmp>/worktrees/main
	root := t.TempDir()
	bareDir := filepath.Join(root, "bare")
	worktreeGitDir := filepath.Join(root, "worktrees", "main")
	projectDir := filepath.Join(root, "project")
	profileHome := filepath.Join(root, "profile")
	tempRoot := filepath.Join(root, "tmp")

	for _, d := range []string{bareDir, worktreeGitDir, projectDir, profileHome} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", d, err)
		}
	}

	linkContent := "gitdir: " + worktreeGitDir + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".git"), []byte(linkContent), 0o644); err != nil {
		t.Fatalf("WriteFile(.git linkfile) error = %v", err)
	}
	rel, err := filepath.Rel(worktreeGitDir, bareDir)
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDir, "commondir"), []byte(rel+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(commondir) error = %v", err)
	}

	prepared, err := PrepareRuntime(context.Background(), PrepareRequest{
		ProfileHome: profileHome,
		ProjectRoot: projectDir,
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

	// The bare repo dir must appear as a mount with source == target == bareDir.
	found := false
	for _, m := range prepared.Mounts {
		if m.Source == bareDir {
			found = true
			if m.Target != bareDir {
				t.Fatalf("worktree mount Target = %q, want %q (path-transparent)", m.Target, bareDir)
			}
			if m.ReadOnly {
				t.Fatalf("worktree mount ReadOnly = true, want false")
			}
		}
	}
	if !found {
		t.Fatalf("mounts = %+v; missing bind mount for bare repo dir %q", prepared.Mounts, bareDir)
	}
}

func TestPrepareRuntimeSkipsWorktreeMountForRegularRepo(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	profileHome := filepath.Join(root, "profile")
	projectDir := filepath.Join(root, "project")
	gitDir := filepath.Join(projectDir, ".git")
	tempRoot := filepath.Join(root, "tmp")

	for _, d := range []string{profileHome, gitDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", d, err)
		}
	}

	prepared, err := PrepareRuntime(context.Background(), PrepareRequest{
		ProfileHome: profileHome,
		ProjectRoot: projectDir,
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

	// No extra mount should be added; only the codex dir mount (1 mount).
	if len(prepared.Mounts) != 1 {
		t.Fatalf("mount count = %d, want 1 (only codex dir, no worktree mount)", len(prepared.Mounts))
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return content
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
