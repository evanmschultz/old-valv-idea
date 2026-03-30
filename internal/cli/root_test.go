package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/logging"
)

func newTestRootCommand(t *testing.T, stdout, stderr *bytes.Buffer) *cobra.Command {
	t.Helper()

	cmd, err := newRootCommandWithPaths(context.Background(), stdout, stderr, testCodexPaths(t))
	if err != nil {
		t.Fatalf("newRootCommandWithPaths() error = %v", err)
	}
	return cmd
}

func TestPathsCommandPlain(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"paths", "--format", "plain"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "database=") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestNewRootCommandUsesDefaultPathsOnDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		return
	}
	t.Setenv("HOME", t.TempDir())

	cmd, err := NewRootCommand(context.Background(), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("NewRootCommand() error = %v", err)
	}
	if cmd.Name() != "valv" {
		t.Fatalf("NewRootCommand().Name() = %q, want valv", cmd.Name())
	}
}

func TestNewRootCommandReturnsUnsupportedOSOnNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		return
	}

	_, err := NewRootCommand(context.Background(), &bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, domain.ErrUnsupportedOS) {
		t.Fatalf("NewRootCommand() error = %v, want domain.ErrUnsupportedOS", err)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"version", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"version": "dev"`) {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestHelpAliasDisplaysRootHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"h"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Usage:", "help", "manage"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected help output %q missing %q", stdout.String(), want)
		}
	}
}

func TestManageAliasWorks(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"m"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Operator workflows", "account", "cleanup"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected manage alias output %q missing %q", stdout.String(), want)
		}
	}
}

func TestGlobalAliasWorks(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"g"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Host-global convenience commands", "switch"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected global alias output %q missing %q", stdout.String(), want)
		}
	}
}

func TestManageHelpSubcommandWorks(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"manage", "help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Operator workflows", "bind", "update"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected manage help output %q missing %q", stdout.String(), want)
		}
	}
}

func TestGlobalHelpAliasWorks(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"g", "h"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Host-global convenience commands", "switch"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected global help output %q missing %q", stdout.String(), want)
		}
	}
}

func TestVisibleCommandsDefineLongAndExample(t *testing.T) {
	cmd := newTestRootCommand(t, &bytes.Buffer{}, &bytes.Buffer{})

	var visit func(*cobra.Command)
	visit = func(current *cobra.Command) {
		if current.Name() != "help" {
			if strings.TrimSpace(current.Long) == "" {
				t.Fatalf("command %q missing Long help text", current.CommandPath())
			}
			if strings.TrimSpace(current.Example) == "" {
				t.Fatalf("command %q missing Example help text", current.CommandPath())
			}
		}
		for _, child := range current.Commands() {
			if !child.IsAvailableCommand() || child.IsAdditionalHelpTopicCommand() {
				continue
			}
			visit(child)
		}
	}
	visit(cmd)
}

func TestOutputFlagsConflict(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"version", "--style", "--no-style"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoggerFromContext(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	logger, err := logging.New(logging.Options{Writer: &stderr, Level: "debug", Prefix: "valv"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := context.WithValue(context.Background(), loggerKey{}, logger)
	if LoggerFromContext(ctx) == nil {
		t.Fatal("expected logger in context")
	}
}

func TestStubCommandReturnsError(t *testing.T) {
	t.Parallel()

	stub := newStubCommand("codex", "stub")
	err := stub.RunE(stub, nil)
	if err == nil {
		t.Fatal("expected stub error")
	}
}

func TestOutputPolicyUsesConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := []byte("[output]\nformat='json'\nstyle='never'\n[logging]\nlevel='debug'\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"--config", configPath, "version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"version": "dev"`) {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestOutputPolicyFlagsOverrideConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := []byte("[output]\nformat='json'\nstyle='never'\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"--config", configPath, "version", "--format", "plain"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "version=dev") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestLoadEffectiveConfigUsesErrorsIsForMissingConfig(t *testing.T) {
	t.Parallel()

	_, _, err := loadEffectiveConfig(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("loadEffectiveConfig() error = %v", err)
	}

	if !errors.Is(fmt.Errorf("wrapped: %w", domain.ErrConfigNotFound), domain.ErrConfigNotFound) {
		t.Fatal("expected errors.Is sentinel behavior")
	}
}

func TestEffectiveConfigFromContext(t *testing.T) {
	t.Parallel()

	want := config.EffectiveConfig{
		Output: config.EffectiveOutputConfig{
			Format: domain.OutputFormatJSON,
			Style:  domain.OutputStyleNever,
		},
		Logging: config.EffectiveLoggingConfig{Level: "debug"},
	}
	ctx := context.WithValue(context.Background(), effectiveConfigKey{}, want)
	got, ok := EffectiveConfigFromContext(ctx)
	if !ok {
		t.Fatal("expected effective config in context")
	}
	if got.Output.Format != want.Output.Format || got.Output.Style != want.Output.Style || got.Logging.Level != want.Logging.Level {
		t.Fatalf("EffectiveConfigFromContext() = %+v, want %+v", got, want)
	}
}

func TestRootCommandCreatesDurableLogFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	paths := testCodexPaths(t)
	cmd, err := newRootCommandWithPaths(context.Background(), &stdout, &stderr, paths)
	if err != nil {
		t.Fatalf("newRootCommandWithPaths() error = %v", err)
	}
	cmd.SetArgs([]string{"--debug", "version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(paths.LogsDir, "valv.log"))
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	if !strings.Contains(string(content), "initialized valv logger") {
		t.Fatalf("unexpected log content: %q", string(content))
	}
}
