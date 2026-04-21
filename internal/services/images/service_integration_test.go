//go:build integration

package images

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/domain"
)

func TestServiceBuildRealDockerImage(t *testing.T) {
	t.Parallel()

	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}

	root := t.TempDir()
	dockerfile := filepath.Join(root, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.20\nARG CODEX_VERSION\nLABEL valv.codex.version=$CODEX_VERSION\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := docker.NewSystemRunner("docker", nil, &stdout, &stderr)
	svc, err := New(Options{
		Runner:     runner,
		Repository: "valv-test/codex",
		ContextDir: root,
		Dockerfile: "Dockerfile",
		DefaultTag: "it",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{Version: "0.116.0"})
	if err != nil {
		t.Fatalf("Build() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	t.Cleanup(func() {
		_ = runner.Run(context.Background(), []string{"image", "rm", "--force", result.Image.String()})
	})

	if err := runner.Run(context.Background(), []string{"image", "inspect", result.Image.String()}); err != nil {
		t.Fatalf("inspect built image: %v", err)
	}
}

func TestWriteDefaultCodexContextBuildsWithExistingUIDAndGID(t *testing.T) {
	t.Parallel()

	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}

	root := t.TempDir()
	if _, err := WriteDefaultCodexContext(root); err != nil {
		t.Fatalf("WriteDefaultCodexContext() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := docker.NewSystemRunner("docker", nil, &stdout, &stderr)
	svc, err := New(Options{
		Runner:     runner,
		Repository: "valv-test/codex-default",
		ContextDir: root,
		Dockerfile: "Dockerfile",
		DefaultTag: "it",
		UserID:     1000,
		GroupID:    1000,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{Version: "0.116.0"})
	if err != nil {
		t.Fatalf("Build() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	t.Cleanup(func() {
		_ = runner.Run(context.Background(), []string{"image", "rm", "--force", result.Image.String()})
	})

	if err := runner.Run(context.Background(), []string{"image", "inspect", result.Image.String()}); err != nil {
		t.Fatalf("inspect built image: %v", err)
	}
}

func TestWriteDefaultClaudeContextBuildsWithExistingUIDAndGID(t *testing.T) {
	t.Parallel()

	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}

	root := t.TempDir()
	if _, err := WriteDefaultClaudeContext(root); err != nil {
		t.Fatalf("WriteDefaultClaudeContext() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := docker.NewSystemRunner("docker", nil, &stdout, &stderr)
	svc, err := New(Options{
		Runner:     runner,
		Provider:   domain.ProviderClaude,
		Repository: "valv-test/claude-default",
		ContextDir: root,
		Dockerfile: "Dockerfile",
		DefaultTag: "it",
		UserID:     1000,
		GroupID:    1000,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{Version: DefaultClaudeCLIVersion})
	if err != nil {
		t.Fatalf("Build() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	t.Cleanup(func() {
		_ = runner.Run(context.Background(), []string{"image", "rm", "--force", result.Image.String()})
	})

	if err := runner.Run(context.Background(), []string{"image", "inspect", result.Image.String()}); err != nil {
		t.Fatalf("inspect built image: %v", err)
	}
}
