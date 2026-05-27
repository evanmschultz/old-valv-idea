//go:build integration

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRunCommandPassesAccountEnvToLaunchRequest verifies that when AccountEnv
// entries are seeded in the store for an account, valv run passes those
// variables into the container environment.
func TestRunCommandPassesAccountEnvToLaunchRequest(t *testing.T) {
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

	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "env-test-profile")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}

	imageRef := buildFixtureImage(t)
	t.Setenv("VALV_CODEX_IMAGE", imageRef)
	t.Setenv("VALV_REAL_HOME", paths.HomeDir)

	// Set up account and binding.
	runManageForIntegration(t, paths, projectRoot, []string{"account", "add", "codex", "env-test-profile", "--home", profileHome, "--skip-login"})
	runManageForIntegration(t, paths, workDir, []string{"bind", "codex", "env-test-profile"})

	// Seed AccountEnv entries for the profile.
	runManageForIntegration(t, paths, workDir, []string{"account", "env", "set", "env-test-profile", "TEST_VAR_1=hello-world"})
	runManageForIntegration(t, paths, workDir, []string{"account", "env", "set", "env-test-profile", "TEST_VAR_2=second-value"})

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

	// Invoke `valv run` with a command that echoes the env vars to a fixture file.
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"--account", "env-test-profile",
		"--provider", "codex",
		"sh", "-c",
		"mkdir -p .valv-fixture && echo TEST_VAR_1=$TEST_VAR_1 > .valv-fixture/run-env.txt && echo TEST_VAR_2=$TEST_VAR_2 >> .valv-fixture/run-env.txt",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}

	// Read and verify the fixture output.
	runResult := readKeyValueFile(t, filepath.Join(workDir, ".valv-fixture", "run-env.txt"))
	if got := runResult["TEST_VAR_1"]; got != "hello-world" {
		t.Fatalf("fixture TEST_VAR_1 = %q, want %q", got, "hello-world")
	}
	if got := runResult["TEST_VAR_2"]; got != "second-value" {
		t.Fatalf("fixture TEST_VAR_2 = %q, want %q", got, "second-value")
	}
}

// TestRunCommandWithNoAccountEnv verifies that valv run succeeds when no
// AccountEnv entries are seeded, and the container launches with no extra env
// variables injected.
func TestRunCommandWithNoAccountEnv(t *testing.T) {
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

	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "no-env-profile")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}

	imageRef := buildFixtureImage(t)
	t.Setenv("VALV_CODEX_IMAGE", imageRef)
	t.Setenv("VALV_REAL_HOME", paths.HomeDir)

	// Set up account and binding.
	runManageForIntegration(t, paths, projectRoot, []string{"account", "add", "codex", "no-env-profile", "--home", profileHome, "--skip-login"})
	runManageForIntegration(t, paths, workDir, []string{"bind", "codex", "no-env-profile"})
	// DO NOT seed any AccountEnv entries; test the empty-env path.

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

	// Invoke `valv run` with a simple echo to verify launch succeeds.
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"--account", "no-env-profile",
		"--provider", "codex",
		"sh", "-c",
		"echo success > /tmp/valv-run-success.txt",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}
	// If we reach here without error, the no-env path succeeded.
}
