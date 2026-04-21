package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/domain"
)

func TestClaudeImageRefDefaults(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "")
	if got := claudeImageRef().String(); got != "valv-claude:dev" {
		t.Fatalf("claudeImageRef() = %q, want %q", got, "valv-claude:dev")
	}
}

func TestClaudeImageRefParsesOverrideWithTag(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "ghcr.io/example/claude:test-fixture")
	if got := claudeImageRef().String(); got != "ghcr.io/example/claude:test-fixture" {
		t.Fatalf("claudeImageRef() = %q, want override image", got)
	}
}

func TestClaudeImageRefParsesOverrideWithoutTag(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "ghcr.io/example/claude")
	ref := claudeImageRef()
	if ref.Repository != "ghcr.io/example/claude" {
		t.Fatalf("claudeImageRef().Repository = %q, want %q", ref.Repository, "ghcr.io/example/claude")
	}
	if ref.Tag != "" {
		t.Fatalf("claudeImageRef().Tag = %q, want empty", ref.Tag)
	}
}

func TestClaudeImageRepositoryUsesDefault(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "")
	if got := claudeImageRepository(); got != "valv-claude" {
		t.Fatalf("claudeImageRepository() = %q, want %q", got, "valv-claude")
	}
}

func TestClaudeImageRepositoryUsesOverride(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "ghcr.io/example/claude:stable")
	if got := claudeImageRepository(); got != "ghcr.io/example/claude" {
		t.Fatalf("claudeImageRepository() = %q, want %q", got, "ghcr.io/example/claude")
	}
}

func TestClaudeImageTagDefaults(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "")
	if got := claudeImageTag(); got != "dev" {
		t.Fatalf("claudeImageTag() = %q, want %q", got, "dev")
	}
}

func TestClaudeImageTagUsesOverride(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "ghcr.io/example/claude:1.2.3")
	if got := claudeImageTag(); got != "1.2.3" {
		t.Fatalf("claudeImageTag() = %q, want %q", got, "1.2.3")
	}
}

func TestClaudeImageTagFallsBackWhenOverrideHasNoTag(t *testing.T) {
	t.Setenv("VALV_CLAUDE_IMAGE", "ghcr.io/example/claude")
	if got := claudeImageTag(); got != "dev" {
		t.Fatalf("claudeImageTag() = %q, want %q", got, "dev")
	}
}

func TestOpenImagesServiceClaudeContextWritesDockerfile(t *testing.T) {
	paths := testCodexPaths(t)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	_, closer, err := openImagesService(cmd, paths, domain.ProviderClaude)
	if err != nil {
		t.Fatalf("openImagesService() error = %v", err)
	}
	defer closer()

	dockerfilePath := filepath.Join(paths.BuildCacheDir, "claude", "Dockerfile")
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", dockerfilePath, err)
	}
	if !strings.Contains(string(content), "@anthropic-ai/claude-code") {
		t.Fatalf("Dockerfile at %q missing @anthropic-ai/claude-code install line; got:\n%s", dockerfilePath, content)
	}
}

func TestOpenImagesServiceCodexContextWritesToCodexSubdir(t *testing.T) {
	paths := testCodexPaths(t)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	_, closer, err := openImagesService(cmd, paths, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("openImagesService() error = %v", err)
	}
	defer closer()

	codexDockerfile := filepath.Join(paths.BuildCacheDir, "codex", "Dockerfile")
	if _, err := os.Stat(codexDockerfile); err != nil {
		t.Fatalf("Stat(%q) error = %v", codexDockerfile, err)
	}
	claudeDockerfile := filepath.Join(paths.BuildCacheDir, "claude", "Dockerfile")
	if _, err := os.Stat(claudeDockerfile); !os.IsNotExist(err) {
		t.Fatalf("expected no Claude Dockerfile under %q, got err=%v", claudeDockerfile, err)
	}
}

func TestOpenImagesServiceUnsupportedProvider(t *testing.T) {
	paths := testCodexPaths(t)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	_, closer, err := openImagesService(cmd, paths, domain.Provider("nonsense"))
	if closer != nil {
		closer()
	}
	if err == nil {
		t.Fatalf("openImagesService() error = nil, want unsupported provider error")
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Fatalf("openImagesService() error = %q, want substring %q", err.Error(), "unsupported provider")
	}
}
