package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

// TestNewClaudeCommandHelp verifies that newClaudeCommand produces a command
// whose metadata includes "claude" and "Docker", and that the command's
// help text is accessible (exits 0). Since DisableFlagParsing is set, cobra
// passes --help through to RunE; we verify the command description directly.
func TestNewClaudeCommandHelp(t *testing.T) {
	t.Parallel()

	cmd := newClaudeCommand(testCodexPaths(t), func(_ *cobra.Command, _ []string) error {
		return nil
	})

	if cmd.Use != "claude" {
		t.Fatalf("cmd.Use = %q, want %q", cmd.Use, "claude")
	}
	for _, want := range []string{"claude", "Docker"} {
		if !strings.Contains(cmd.Long, want) {
			t.Fatalf("cmd.Long = %q, want substring %q", cmd.Long, want)
		}
	}
	// Verify help is accessible via cobra: call Help() directly.
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Help(); err != nil {
		t.Fatalf("cmd.Help() error = %v", err)
	}
}

// TestNewClaudeCommandVersion verifies that `valv claude --version` routes
// through runClaudeImageOnlyCommand. With the VALV_CLAUDE_IMAGE env set to a
// fake image and a fake docker script, it confirms the docker run call is
// invoked.
func TestNewClaudeCommandVersion(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "valv-claude-dev:dev")

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker-run.txt")
	scriptPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\n" +
		"set -eu\n" +
		"case \"$1 $2\" in\n" +
		"  \"image inspect\") exit 0 ;;\n" +
		"  \"run --rm\") printf '%s\\n' \"$@\" > " + shellQuote(logPath) + "; exit 0 ;;\n" +
		"esac\n" +
		"printf '%s\\n' \"$@\" > " + shellQuote(logPath) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newClaudeCommand(testCodexPaths(t), nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := runClaudeImageOnlyCommand(cmd, testCodexPaths(t), []string{"--version"}); err != nil {
		t.Fatalf("runClaudeImageOnlyCommand(--version) error = %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath) error = %v", err)
	}
	got := string(content)
	for _, want := range []string{
		"run",
		"--rm",
		"--name",
		"valv-claude-info-",
		"-e",
		"CLAUDE_CONFIG_DIR=/home/valv/.claude",
		"HOME=/home/valv",
		"USER=valv",
		"valv-claude-dev:dev",
		"--version",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("docker run args missing %q in %q", want, got)
		}
	}
}

// TestRunClaudeCommandUnboundProject verifies that runClaudeCommand returns an
// error wrapping domain.ErrUnboundProject when the working directory has no
// associated project record.
func TestRunClaudeCommandUnboundProject(t *testing.T) {
	paths := testCodexPaths(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	projectDir := t.TempDir()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	cmd := newClaudeCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err = cmd.RunE(cmd, []string{"--prompt", "hello"})
	if !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("RunE() error = %v, want domain.ErrUnboundProject", err)
	}
}

// TestRunClaudeCommandRejectsWrongProvider verifies that runClaudeCommand
// returns an error when the project has a Codex binding (not Claude). Since no
// Claude binding exists, the service returns ErrUnboundProject.
func TestRunClaudeCommandRejectsWrongProvider(t *testing.T) {
	paths := testCodexPaths(t)

	// Set up a project with a Codex binding — Claude service will not find a
	// Claude binding and will surface ErrUnboundProject.
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	runManage(t, paths, []string{"account", "add", "codex", "--project", projectRoot})

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	cmd := newClaudeCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err = cmd.RunE(cmd, []string{"--prompt", "hello"})
	if !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("RunE() error = %v, want domain.ErrUnboundProject (no Claude binding)", err)
	}
}

// TestClaudeArgsSkipProjectBinding is a table-driven test verifying the
// claudeArgsSkipProjectBinding predicate for all documented flag patterns.
func TestClaudeArgsSkipProjectBinding(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "empty", args: nil, want: false},
		{name: "help flag", args: []string{"--help"}, want: true},
		{name: "short help flag", args: []string{"-h"}, want: true},
		{name: "help subcommand", args: []string{"help"}, want: true},
		{name: "nested help flag", args: []string{"resume", "--help"}, want: true},
		{name: "version flag", args: []string{"--version"}, want: true},
		{name: "short version flag", args: []string{"-V"}, want: true},
		{name: "prompt arg", args: []string{"--prompt", "foo"}, want: false},
		{name: "interactive no args", args: []string{"run"}, want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := claudeArgsSkipProjectBinding(tc.args); got != tc.want {
				t.Fatalf("claudeArgsSkipProjectBinding(%v) = %t, want %t", tc.args, got, tc.want)
			}
		})
	}
}

// TestClaudeCommandPassesArgsThroughUnchanged verifies that newClaudeCommand
// does not modify args when a custom run function intercepts them.
func TestClaudeCommandPassesArgsThroughUnchanged(t *testing.T) {
	t.Parallel()

	var got []string
	cmd := newClaudeCommand(config.Paths{}, func(_ *cobra.Command, args []string) error {
		got = append([]string(nil), args...)
		return nil
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--resume", "session-123", "--model", "claude-opus-4-5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := []string{"--resume", "session-123", "--model", "claude-opus-4-5"}
	if len(got) != len(want) {
		t.Fatalf("Execute() args len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Execute() args[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
