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

// TestResolveProjectImageClaudeEmptyManifestReturnsBaseRef pins decision 12:
// when tools.Resolve reports an empty manifest (no .valv/tools.toml in
// workingDir), resolveProjectImage returns the base ref unchanged and
// performs zero docker calls. No stderr warning is emitted.
func TestResolveProjectImageClaudeEmptyManifestReturnsBaseRef(t *testing.T) {
	paths := testCodexPaths(t)
	workingDir := t.TempDir() // no .valv/tools.toml inside

	logPath := installFakeDocker(t)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)

	baseRef := claudeImageRef()
	got, err := resolveProjectImage(cmd, paths, domain.ProviderClaude, workingDir, baseRef)
	if err != nil {
		t.Fatalf("resolveProjectImage() error = %v", err)
	}
	if got.String() != baseRef.String() {
		t.Fatalf("resolveProjectImage() = %q, want base ref %q", got.String(), baseRef.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr %q, want no warning for empty manifest", stderr.String())
	}

	// No docker calls should have been recorded — empty manifest short-
	// circuits before openImagesService is reached. The fake docker log
	// either does not exist (script never ran) or is empty.
	if data, err := os.ReadFile(logPath); err == nil && len(data) > 0 {
		t.Fatalf("unexpected docker calls for empty manifest: %q", data)
	}
}

// TestResolveProjectImageClaudeOverlayBuildInvoked pins decision 12 + the
// happy path of decision 4: a non-empty manifest WITHOUT a VALV_CLAUDE_IMAGE
// override triggers EnsureProjectImage, which fires docker buildx build with
// the project-overlay labels and returns a `proj-<short-hash>` tag.
func TestResolveProjectImageClaudeOverlayBuildInvoked(t *testing.T) {
	// VALV_CLAUDE_IMAGE intentionally NOT set — the EnsureProjectImage
	// path requires the env override to be empty so the images service is
	// constructed instead of the env-bypass.
	t.Setenv("VALV_CLAUDE_IMAGE", "")
	paths := testCodexPaths(t)
	workingDir := writeClaudeToolsManifest(t)

	// The fake docker binary returns empty output for `image inspect
	// --format`, so all freshness-label reads return "" and trigger a
	// rebuild. That is the assertion target: a buildx build call is
	// recorded.
	logPath := installFakeDocker(t)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)

	baseRef := claudeImageRef()
	got, err := resolveProjectImage(cmd, paths, domain.ProviderClaude, workingDir, baseRef)
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
	// The five overlay labels must appear in the build args. We assert a
	// couple of load-bearing ones — managed=true and scope=project-overlay
	// — to pin decision 4.
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

// TestResolveProjectImageClaudeOverrideSkipsOverlay pins decision 13: when
// VALV_CLAUDE_IMAGE is set AND a non-empty manifest is present, the overlay
// build is skipped, a single stderr warning is emitted, and the function
// returns the supplied baseRef (which upstream resolved to the override).
func TestResolveProjectImageClaudeOverrideSkipsOverlay(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "test/claude:override")
	paths := testCodexPaths(t)
	workingDir := writeClaudeToolsManifest(t)

	logPath := installFakeDocker(t)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)

	baseRef := claudeImageRef()
	got, err := resolveProjectImage(cmd, paths, domain.ProviderClaude, workingDir, baseRef)
	if err != nil {
		t.Fatalf("resolveProjectImage() error = %v", err)
	}
	if got.String() != baseRef.String() {
		t.Fatalf("resolveProjectImage() = %q, want base ref %q (override path)", got.String(), baseRef.String())
	}

	const wantWarning = "warning: VALV_CLAUDE_IMAGE override active; .valv/tools.toml overlay skipped"
	if !strings.Contains(stderr.String(), wantWarning) {
		t.Fatalf("stderr %q missing override warning %q", stderr.String(), wantWarning)
	}
	if got, want := strings.Count(stderr.String(), wantWarning), 1; got != want {
		t.Fatalf("override warning emitted %d times, want %d", got, want)
	}

	// The override path must not invoke any docker calls — openImagesService
	// (and therefore EnsureProjectImage) is short-circuited before it runs.
	if data, err := os.ReadFile(logPath); err == nil && len(data) > 0 {
		t.Fatalf("unexpected docker calls under VALV_CLAUDE_IMAGE override: %q", data)
	}
}

// writeClaudeToolsManifest writes a minimal `.valv/tools.toml` into a fresh
// temp directory so tools.Resolve returns a non-empty manifest. The manifest
// declares one object-form `go install` tool — the same shape required by
// BuildOverlayDockerfile in Unit 12.2.
func writeClaudeToolsManifest(t *testing.T) string {
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
