package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/domain"
)

// TestResolveProjectImageCodexEmptyManifestReturnsBaseRef mirrors the claude
// empty-manifest case for codex. Pins decision 12: empty manifest →
// resolveProjectImage returns baseRef with zero docker calls and no warning.
func TestResolveProjectImageCodexEmptyManifestReturnsBaseRef(t *testing.T) {
	paths := testCodexPaths(t)
	workingDir := t.TempDir()

	logPath := installFakeDocker(t)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)

	baseRef := codexImageRef()
	got, err := resolveProjectImage(cmd, paths, domain.ProviderCodex, workingDir, baseRef)
	if err != nil {
		t.Fatalf("resolveProjectImage() error = %v", err)
	}
	if got.String() != baseRef.String() {
		t.Fatalf("resolveProjectImage() = %q, want base ref %q", got.String(), baseRef.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr %q, want no warning for empty manifest", stderr.String())
	}

	if data, err := os.ReadFile(logPath); err == nil && len(data) > 0 {
		t.Fatalf("unexpected docker calls for empty manifest: %q", data)
	}
}

// TestResolveProjectImageCodexOverlayBuildInvoked mirrors the claude overlay-
// build case for codex. With a non-empty manifest and no override, the
// EnsureProjectImage path runs and docker buildx build is invoked with the
// five project-overlay labels.
func TestResolveProjectImageCodexOverlayBuildInvoked(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "")
	paths := testCodexPaths(t)
	workingDir := writeCodexToolsManifest(t)

	logPath := installFakeDocker(t)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)

	baseRef := codexImageRef()
	got, err := resolveProjectImage(cmd, paths, domain.ProviderCodex, workingDir, baseRef)
	if err != nil {
		t.Fatalf("resolveProjectImage() error = %v", err)
	}
	if got.String() == baseRef.String() {
		t.Fatalf("resolveProjectImage() = %q, want resolved per-project ref distinct from base %q", got.String(), baseRef.String())
	}
	if !strings.HasPrefix(got.Tag, "proj-") {
		t.Fatalf("resolveProjectImage() tag = %q, want proj- prefix", got.Tag)
	}
	if got.Repository != baseRef.Repository {
		t.Fatalf("resolveProjectImage() repo = %q, want base repo %q", got.Repository, baseRef.Repository)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr %q, want no warning when override unset", stderr.String())
	}

	logContent := mustReadFile(t, logPath)
	if !strings.Contains(logContent, "buildx build --load") {
		t.Fatalf("docker log %q missing buildx build invocation", logContent)
	}
	for _, want := range []string{
		"io.valv.managed=true",
		"io.valv.scope=project-overlay",
		"io.valv.tools_hash=",
	} {
		if !strings.Contains(logContent, want) {
			t.Fatalf("docker log %q missing label %q", logContent, want)
		}
	}
}

// TestResolveProjectImageCodexOverrideSkipsOverlay pins decision 13 for codex:
// VALV_CODEX_IMAGE set + non-empty manifest → one stderr warning, baseRef
// returned, zero docker calls.
func TestResolveProjectImageCodexOverrideSkipsOverlay(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "test/codex:override")
	paths := testCodexPaths(t)
	workingDir := writeCodexToolsManifest(t)

	logPath := installFakeDocker(t)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)

	baseRef := codexImageRef()
	got, err := resolveProjectImage(cmd, paths, domain.ProviderCodex, workingDir, baseRef)
	if err != nil {
		t.Fatalf("resolveProjectImage() error = %v", err)
	}
	if got.String() != baseRef.String() {
		t.Fatalf("resolveProjectImage() = %q, want base ref %q (override path)", got.String(), baseRef.String())
	}

	const wantWarning = "warning: VALV_CODEX_IMAGE override active; .valv/tools.toml overlay skipped"
	if !strings.Contains(stderr.String(), wantWarning) {
		t.Fatalf("stderr %q missing override warning %q", stderr.String(), wantWarning)
	}
	if got, want := strings.Count(stderr.String(), wantWarning), 1; got != want {
		t.Fatalf("override warning emitted %d times, want %d", got, want)
	}

	if data, err := os.ReadFile(logPath); err == nil && len(data) > 0 {
		t.Fatalf("unexpected docker calls under VALV_CODEX_IMAGE override: %q", data)
	}
}

// writeCodexToolsManifest writes a minimal `.valv/tools.toml` for the codex
// overlay tests. Single object-form go-install tool — the same minimal shape
// BuildOverlayDockerfile accepts.
func writeCodexToolsManifest(t *testing.T) string {
	t.Helper()
	workingDir := t.TempDir()
	valvDir := filepath.Join(workingDir, ".valv")
	if err := os.MkdirAll(valvDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", valvDir, err)
	}
	manifest := strings.TrimSpace(`
[tools]
ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }
`) + "\n"
	manifestPath := filepath.Join(valvDir, "tools.toml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", manifestPath, err)
	}
	return workingDir
}
