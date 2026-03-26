package docker

import (
	"context"
	"reflect"
	"testing"
)

func TestBuildImageArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildImageArgs(ImageBuildRequest{
		ContextDir: "/tmp/context",
		Dockerfile: "Dockerfile.dev",
		Tags:       []ImageRef{NewImageRef("ghcr.io/valv/codex", "dev")},
		BuildArgs:  map[string]string{"B": "2", "A": "1"},
		Labels:     map[string]string{"z": "last", "a": "first"},
		Target:     "release",
		Network:    "none",
		Extra:      []string{"--progress=plain"},
		Pull:       true,
		NoCache:    true,
	})
	if err != nil {
		t.Fatalf("BuildImageArgs() error = %v", err)
	}

	want := []string{
		"buildx",
		"build",
		"--load",
		"--pull",
		"--no-cache",
		"-f", "Dockerfile.dev",
		"--target", "release",
		"--network", "none",
		"-t", "ghcr.io/valv/codex:dev",
		"--build-arg", "A=1",
		"--build-arg", "B=2",
		"--label", "a=first",
		"--label", "z=last",
		"--progress=plain",
		"/tmp/context",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildImageArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildImageRemoveArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildImageRemoveArgs(ImageRemoveRequest{
		Refs:  []ImageRef{NewImageRef("ghcr.io/valv/codex", "dev"), NewImageRef("ghcr.io/valv/codex", "0.1.0")},
		Force: true,
	})
	if err != nil {
		t.Fatalf("BuildImageRemoveArgs() error = %v", err)
	}

	want := []string{"image", "rm", "--force", "ghcr.io/valv/codex:dev", "ghcr.io/valv/codex:0.1.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildImageRemoveArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildContainerRemoveArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildContainerRemoveArgs(ContainerRemoveRequest{
		IDs:     []string{"abc123", "def456"},
		Force:   true,
		Volumes: true,
	})
	if err != nil {
		t.Fatalf("BuildContainerRemoveArgs() error = %v", err)
	}

	want := []string{"rm", "--force", "--volumes", "abc123", "def456"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildContainerRemoveArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildBuilderPruneArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildBuilderPruneArgs(BuilderPruneRequest{All: true, Filters: map[string]string{"until": "24h", "type": "regular"}})
	if err != nil {
		t.Fatalf("BuildBuilderPruneArgs() error = %v", err)
	}

	want := []string{"builder", "prune", "--force", "--all", "--filter", "type=regular", "--filter", "until=24h"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildBuilderPruneArgs() = %#v, want %#v", got, want)
	}
}

func TestExecutorBuildForwardsArgs(t *testing.T) {
	t.Parallel()

	var got []string
	exec := NewExecutor(CommandRunnerFunc(func(ctx context.Context, args []string) error {
		got = append([]string(nil), args...)
		return nil
	}))

	if err := exec.Build(context.Background(), ImageBuildRequest{
		ContextDir: "/tmp/context",
		Tags:       []ImageRef{NewImageRef("ghcr.io/valv/codex", "dev")},
	}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	want := []string{"buildx", "build", "--load", "-t", "ghcr.io/valv/codex:dev", "/tmp/context"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build() args = %#v, want %#v", got, want)
	}
}
