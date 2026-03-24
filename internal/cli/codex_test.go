package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
)

func TestCodexCommandPassesArgsThroughUnchanged(t *testing.T) {
	t.Parallel()

	var got []string
	cmd := newCodexCommand(config.Paths{}, func(_ *cobra.Command, args []string) error {
		got = append([]string(nil), args...)
		return nil
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"resume", "session-123", "--model", "gpt-5-codex"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := []string{"resume", "session-123", "--model", "gpt-5-codex"}
	if len(got) != len(want) {
		t.Fatalf("Execute() args len = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("Execute() args[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestRunCodexCommandReturnsUnboundProjectWhenNoProjectRecordExists(t *testing.T) {
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

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err = cmd.RunE(cmd, []string{"resume", "session-123"})
	if !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("RunE() error = %v, want domain.ErrUnboundProject", err)
	}
}

func TestRunCodexCommandReturnsEnsureError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blockingFile := filepath.Join(root, "blocking")
	if err := os.WriteFile(blockingFile, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	paths := config.Paths{
		DatabaseDir:    blockingFile,
		ProviderRoot:   filepath.Join(root, "providers"),
		StateDir:       filepath.Join(root, "state"),
		ConfigDir:      filepath.Join(root, "config"),
		LogsDir:        filepath.Join(root, "logs"),
		CachesDir:      filepath.Join(root, "caches"),
		BuildCacheDir:  filepath.Join(root, "caches", "build"),
		TempCacheDir:   filepath.Join(root, "caches", "tmp"),
		RuntimeTmpDir:  filepath.Join(root, "runtime"),
		PIDsDir:        filepath.Join(root, "runtime", "pids"),
		LocksDir:       filepath.Join(root, "runtime", "locks"),
		SocketsDir:     filepath.Join(root, "runtime", "sockets"),
		DatabasePath:   filepath.Join(root, "db", "valv.sqlite3"),
		AppSupportRoot: root,
	}

	err := runCodexCommand(newCodexCommand(paths, nil), paths, nil)
	if err == nil {
		t.Fatal("runCodexCommand() error = nil, want ensure failure")
	}
}

func TestCodexImageRefDefaults(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "")
	if got := codexImageRef().String(); got != "valv-codex:dev" {
		t.Fatalf("codexImageRef() = %q, want %q", got, "valv-codex:dev")
	}
}

func TestCodexImageRefParsesOverrideWithTag(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "ghcr.io/example/codex:test-fixture")
	if got := codexImageRef().String(); got != "ghcr.io/example/codex:test-fixture" {
		t.Fatalf("codexImageRef() = %q, want override image", got)
	}
}

func testCodexPaths(t *testing.T) config.Paths {
	t.Helper()

	root := t.TempDir()
	return config.Paths{
		HomeDir:        root,
		AppSupportRoot: root,
		DatabaseDir:    filepath.Join(root, "db"),
		DatabasePath:   filepath.Join(root, "db", "valv.sqlite3"),
		ProviderRoot:   filepath.Join(root, "providers"),
		StateDir:       filepath.Join(root, "state"),
		ConfigDir:      filepath.Join(root, "config"),
		LogsDir:        filepath.Join(root, "logs"),
		CachesDir:      filepath.Join(root, "caches"),
		BuildCacheDir:  filepath.Join(root, "caches", "build"),
		TempCacheDir:   filepath.Join(root, "caches", "tmp"),
		RuntimeTmpDir:  filepath.Join(root, "runtime"),
		PIDsDir:        filepath.Join(root, "runtime", "pids"),
		LocksDir:       filepath.Join(root, "runtime", "locks"),
		SocketsDir:     filepath.Join(root, "runtime", "sockets"),
	}
}
