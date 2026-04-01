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

func TestBuildContainerListArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildContainerListArgs(ContainerListRequest{
		All:   true,
		Quiet: true,
		Filters: map[string]string{
			"label": "io.valv.managed=true",
			"name":  "valv-",
		},
	})
	if err != nil {
		t.Fatalf("BuildContainerListArgs() error = %v", err)
	}

	want := []string{"ps", "-a", "-q", "--filter", "label=io.valv.managed=true", "--filter", "name=valv-"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildContainerListArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildCreateArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildCreateArgs(ContainerRunRequest{
		Name:        "valv-test",
		Image:       NewImageRef("ghcr.io/valv/codex", "dev"),
		WorkingDir:  "/workspace",
		Interactive: true,
		TTY:         true,
		Init:        true,
		Remove:      true,
		Env:         map[string]string{"HOME": "/home/valv"},
		Args:        []string{"resume", "abc"},
	})
	if err != nil {
		t.Fatalf("BuildCreateArgs() error = %v", err)
	}

	want := []string{
		"create",
		"-i",
		"-t",
		"--init",
		"--name", "valv-test",
		"--workdir", "/workspace",
		"-e", "HOME=/home/valv",
		"ghcr.io/valv/codex:dev",
		"resume", "abc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildCreateArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildStartArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildStartArgs(ContainerStartRequest{
		ContainerID: "valv-test",
		Attach:      true,
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("BuildStartArgs() error = %v", err)
	}

	want := []string{"start", "-a", "-i", "valv-test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildStartArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildAttachArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildAttachArgs(ContainerAttachRequest{
		ContainerID: "valv-test",
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("BuildAttachArgs() error = %v", err)
	}

	want := []string{"attach", "valv-test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildAttachArgs() = %#v, want %#v", got, want)
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
