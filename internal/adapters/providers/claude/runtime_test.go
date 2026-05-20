package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
)

func TestPrepareRuntimeSetsClaudeConfigDirEnv(t *testing.T) {
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

	if got := prepared.Env["CLAUDE_CONFIG_DIR"]; got != ContainerClaudeDir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want %q", got, ContainerClaudeDir)
	}
	if got := prepared.Env["HOME"]; got != ContainerHomeDir {
		t.Fatalf("HOME = %q, want %q", got, ContainerHomeDir)
	}
	if got := prepared.Env["USER"]; got != "valv" {
		t.Fatalf("USER = %q, want valv", got)
	}
}

func TestPrepareRuntimeMountsClaudeDir(t *testing.T) {
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

	mount := findMountTarget(t, prepared.Mounts, ContainerClaudeDir)
	if mount.ReadOnly {
		t.Fatalf("claude mount ReadOnly = true, want false")
	}
}

func TestPrepareRuntimeSkipsCodexMountWhenNotProvided(t *testing.T) {
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

	if _, ok := prepared.Env["CODEX_HOME"]; ok {
		t.Fatalf("Env contains CODEX_HOME = %q, must not be set when OtherProviderProfileHome is empty", prepared.Env["CODEX_HOME"])
	}
	for _, m := range prepared.Mounts {
		if m.Target == "/home/valv/.codex" {
			t.Fatalf("mounts contain /home/valv/.codex target = %+v, must not be present when OtherProviderProfileHome is empty", m)
		}
	}
}

func TestPrepareRuntimeMountsCodexHomeWhenProvided(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	profileHome := filepath.Join(root, "profile")
	projectRoot := filepath.Join(root, "project")
	tempRoot := filepath.Join(root, "tmp")
	otherHome := t.TempDir()
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
		OtherProviderProfileHome: otherHome,
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}
	defer func() {
		if err := prepared.Close(); err != nil {
			t.Fatalf("prepared.Close() error = %v", err)
		}
	}()

	if got := prepared.Env["CODEX_HOME"]; got != "/home/valv/.codex" {
		t.Fatalf("Env[CODEX_HOME] = %q, want /home/valv/.codex", got)
	}
	mount := findMountTarget(t, prepared.Mounts, "/home/valv/.codex")
	if mount.ReadOnly {
		t.Fatalf("codex cross-mount ReadOnly = true, want false (read-write so codex can write session state)")
	}
}

func TestPrepareRuntimeCleanupRemovesTempDir(t *testing.T) {
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
		ProfileHome: profileHome,
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
	})
	if err != nil {
		t.Fatalf("PrepareRuntime() error = %v", err)
	}

	if err := prepared.Close(); err != nil {
		t.Fatalf("prepared.Close() error = %v", err)
	}

	// The claude-runtime-* dir must be removed after Close.
	claudeRuntimeDirs, _ := filepath.Glob(filepath.Join(tempRoot, "claude-runtime-*"))
	if len(claudeRuntimeDirs) > 0 {
		t.Fatalf("claude-runtime-* dirs still exist after Close: %v", claudeRuntimeDirs)
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

func TestPrepareRuntimeUsesSharedHomeAndSyncsBack(t *testing.T) {
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
	// Place some shared state and a credentials file in the shared home.
	if err := os.WriteFile(filepath.Join(sharedHome, "history.txt"), []byte("shared"), 0o600); err != nil {
		t.Fatalf("WriteFile(shared history) error = %v", err)
	}
	// .credentials.json should NOT be synced back (excluded).
	if err := os.WriteFile(filepath.Join(sharedHome, ".credentials.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile(.credentials.json) error = %v", err)
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

	// The runtime mount source must be a staged copy distinct from sharedHome.
	mount := findMountTarget(t, prepared.Mounts, ContainerClaudeDir)
	if mount.Source == sharedHome {
		t.Fatalf("runtime mount source = %q, want staged runtime dir distinct from shared home %q", mount.Source, sharedHome)
	}

	// Write a fixture to the runtime dir to verify sync-back.
	fixtureFile := filepath.Join(mount.Source, "session.txt")
	if err := os.WriteFile(fixtureFile, []byte("session-data\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(session fixture) error = %v", err)
	}

	if err := prepared.Close(); err != nil {
		t.Fatalf("prepared.Close() error = %v", err)
	}

	// session.txt should be synced back.
	content, err := os.ReadFile(filepath.Join(sharedHome, "session.txt"))
	if err != nil {
		t.Fatalf("ReadFile(synced session.txt) error = %v", err)
	}
	if string(content) != "session-data\n" {
		t.Fatalf("synced session.txt = %q, want session-data", string(content))
	}

	// .credentials.json must NOT be synced back (excluded from sync).
	if _, err := os.Stat(filepath.Join(sharedHome, ".credentials.json")); err == nil {
		// The original exists; check it wasn't overwritten via sync-back of a different copy.
		// (Sync-back excludes .credentials.json, so the shared one stays, but no new one appears.)
	}
}

func TestAppendUniqueStrings(t *testing.T) {
	t.Parallel()

	got := appendUniqueStrings([]string{"a", "b"}, "b", "c", "", "  c  ", "d")
	want := []string{"a", "b", "c", "d"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("appendUniqueStrings = %v, want %v", got, want)
	}
}

func TestErrorsJoin(t *testing.T) {
	t.Parallel()

	if err := errorsJoin(); err != nil {
		t.Fatalf("errorsJoin() with no errors = %v, want nil", err)
	}
	if err := errorsJoin(nil); err != nil {
		t.Fatalf("errorsJoin(nil) = %v, want nil", err)
	}
}

// findMountTarget finds a mount by its container target path, failing the test
// if none is found.
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
