package images

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/evanmschultz/valv/internal/adapters/docker"
)

type runnerRecorder struct {
	calls [][]string
	errs  map[string]error
}

func (r *runnerRecorder) Run(_ context.Context, args []string) error {
	r.calls = append(r.calls, append([]string(nil), args...))
	if r.errs != nil {
		if err := r.errs[strings.Join(args, " ")]; err != nil {
			return err
		}
	}
	return nil
}

func TestServiceBuildAddsVersionAndUsesDefaultImageInfo(t *testing.T) {
	t.Parallel()

	runner := &runnerRecorder{}
	svc, err := New(Options{
		Runner:     runner,
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		Dockerfile: "Dockerfile",
		DefaultTag: "dev",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{Version: "0.116.0"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if got, want := result.Image.String(), "ghcr.io/valv/codex:dev"; got != want {
		t.Fatalf("Build() image = %q, want %q", got, want)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner call count = %d, want 1", len(runner.calls))
	}
	want := []string{"buildx", "build", "--load", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "--build-arg", "CODEX_VERSION=0.116.0", "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", "io.valv.scope=image", "--label", "io.valv.version=0.116.0", "/tmp/codex-image"}
	if !reflect.DeepEqual(runner.calls[0], want) {
		t.Fatalf("Build() args = %#v, want %#v", runner.calls[0], want)
	}
}

func TestServiceUpdateRemovesPreviousImages(t *testing.T) {
	t.Parallel()

	runner := &runnerRecorder{}
	svc, err := New(Options{
		Runner:     runner,
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		Dockerfile: "Dockerfile",
		DefaultTag: "dev",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = svc.Update(context.Background(), UpdateRequest{
		BuildRequest: BuildRequest{Version: "0.117.0"},
		PreviousImages: []docker.ImageRef{
			docker.NewImageRef("ghcr.io/valv/codex", "0.116.0"),
		},
		RemovePrevious: true,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("runner call count = %d, want 2", len(runner.calls))
	}
	if got, want := runner.calls[1], []string{"image", "rm", "--force", "ghcr.io/valv/codex:0.116.0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Remove() args = %#v, want %#v", got, want)
	}
}

func TestNewRequiresRunnerAndRepository(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatal("New() error = nil, want dependency failure")
	}

	_, err := New(Options{Runner: &runnerRecorder{}, ContextDir: "/tmp/context"})
	if err == nil {
		t.Fatal("New() error = nil, want repository failure")
	}
}

func TestWriteDefaultCodexContextWritesDockerfile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dockerfilePath, err := WriteDefaultCodexContext(root)
	if err != nil {
		t.Fatalf("WriteDefaultCodexContext() error = %v", err)
	}
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), `@openai/codex@${CODEX_VERSION}`) {
		t.Fatalf("dockerfile missing package install: %q", string(content))
	}
	for _, want := range []string{
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("dockerfile missing %q: %q", want, string(content))
		}
	}
	if filepath.Base(dockerfilePath) != "Dockerfile" {
		t.Fatalf("dockerfile base = %q, want Dockerfile", filepath.Base(dockerfilePath))
	}
}

func TestBuildIncludesExtraTags(t *testing.T) {
	t.Parallel()

	runner := &runnerRecorder{}
	svc, err := New(Options{Runner: runner, Repository: "ghcr.io/valv/codex", ContextDir: "/tmp/codex-image"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{
		Version: "0.116.0",
		ExtraTags: []docker.ImageRef{
			docker.NewImageRef("ghcr.io/valv/codex", "0-116-0"),
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Tags) != 2 {
		t.Fatalf("result tags len = %d, want 2", len(result.Tags))
	}
	if !reflect.DeepEqual(runner.calls[0], []string{"buildx", "build", "--load", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "-t", "ghcr.io/valv/codex:0-116-0", "--build-arg", "CODEX_VERSION=0.116.0", "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", "io.valv.scope=image", "--label", "io.valv.version=0.116.0", "/tmp/codex-image"}) {
		t.Fatalf("Build() args = %#v", runner.calls[0])
	}
}

func TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable(t *testing.T) {
	t.Parallel()

	runner := &runnerRecorder{
		errs: map[string]error{
			"buildx build --load -f /tmp/codex-image/Dockerfile -t ghcr.io/valv/codex:dev --build-arg CODEX_VERSION=0.116.0 --label io.valv.managed=true --label io.valv.provider=codex --label io.valv.scope=image --label io.valv.version=0.116.0 /tmp/codex-image": fmt.Errorf("docker buildx is required but unavailable"),
		},
	}
	svc, err := New(Options{
		Runner:     runner,
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		Dockerfile: "Dockerfile",
		DefaultTag: "dev",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = svc.Build(context.Background(), BuildRequest{Version: "0.116.0"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner call count = %d, want 2", len(runner.calls))
	}
	if got, want := runner.calls[1], []string{"build", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "--build-arg", "CODEX_VERSION=0.116.0", "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", "io.valv.scope=image", "--label", "io.valv.version=0.116.0", "/tmp/codex-image"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback args = %#v, want %#v", got, want)
	}
}
