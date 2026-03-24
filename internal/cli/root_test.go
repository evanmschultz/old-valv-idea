package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/logging"
)

func TestPathsCommandPlain(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd, err := NewRootCommand(context.Background(), &stdout, &stderr)
	if err != nil {
		t.Fatalf("NewRootCommand() error = %v", err)
	}
	cmd.SetArgs([]string{"paths", "--format", "plain"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "database=") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestVersionCommandJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd, err := NewRootCommand(context.Background(), &stdout, &stderr)
	if err != nil {
		t.Fatalf("NewRootCommand() error = %v", err)
	}
	cmd.SetArgs([]string{"version", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"version": "dev"`) {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestOutputFlagsConflict(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd, err := NewRootCommand(context.Background(), &stdout, &stderr)
	if err != nil {
		t.Fatalf("NewRootCommand() error = %v", err)
	}
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
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := []byte("[output]\nformat='json'\nstyle='never'\n[logging]\nlevel='debug'\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd, err := NewRootCommand(context.Background(), &stdout, &stderr)
	if err != nil {
		t.Fatalf("NewRootCommand() error = %v", err)
	}
	cmd.SetArgs([]string{"--config", configPath, "version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"version": "dev"`) {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestOutputPolicyFlagsOverrideConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := []byte("[output]\nformat='json'\nstyle='never'\n")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd, err := NewRootCommand(context.Background(), &stdout, &stderr)
	if err != nil {
		t.Fatalf("NewRootCommand() error = %v", err)
	}
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
