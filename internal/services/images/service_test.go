package images

import (
	"context"
	"reflect"
	"testing"

	"github.com/evanmschultz/valv/internal/adapters/docker"
)

type runnerRecorder struct {
	calls [][]string
}

func (r *runnerRecorder) Run(_ context.Context, args []string) error {
	r.calls = append(r.calls, append([]string(nil), args...))
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
	want := []string{"build", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "--build-arg", "CODEX_VERSION=0.116.0", "/tmp/codex-image"}
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
