//go:build integration

package cleanup

import (
	"context"
	"io"
	"testing"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/testcontainers/testcontainers-go"
)

func TestCleanDockerRemovesRealContainer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "alpine:3.20",
			Cmd:   []string{"sh", "-c", "sleep 60"},
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(ctx)
	})

	runner := docker.NewSystemRunner("docker", nil, io.Discard, io.Discard)
	svc, err := New(Options{Runner: runner})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	id := container.GetContainerID()
	result, err := svc.CleanDocker(ctx, DockerCleanupRequest{ContainerIDs: []string{id}, Force: true})
	if err != nil {
		t.Fatalf("CleanDocker() error = %v", err)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(result.Commands))
	}

	if err := runner.Run(ctx, []string{"inspect", id}); err == nil {
		t.Fatal("inspect after cleanup succeeded, want removed container")
	}
}
