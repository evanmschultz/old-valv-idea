package cleanup

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/pathutil"
)

type cleanupRunnerRecorder struct {
	calls [][]string
}

func (r *cleanupRunnerRecorder) Run(_ context.Context, args []string) error {
	r.calls = append(r.calls, append([]string(nil), args...))
	return nil
}

func TestDefaultLocalTargets(t *testing.T) {
	t.Parallel()

	paths := config.Paths{
		BuildCacheDir: "/tmp/valv/cache/build",
		TempCacheDir:  "/tmp/valv/cache/tmp",
		RuntimeTmpDir: "/tmp/valv-runtime",
		PIDsDir:       "/tmp/valv-runtime/pids",
		LocksDir:      "/tmp/valv-runtime/locks",
		SocketsDir:    "/tmp/valv-runtime/sockets",
	}

	got := DefaultLocalTargets(paths)
	want := []string{
		"/tmp/valv/cache/build",
		"/tmp/valv/cache/tmp",
		"/tmp/valv-runtime",
		"/tmp/valv-runtime/pids",
		"/tmp/valv-runtime/locks",
		"/tmp/valv-runtime/sockets",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultLocalTargets() = %#v, want %#v", got, want)
	}
}

func TestCleanLocalRemovesTargets(t *testing.T) {
	t.Parallel()

	runner := &cleanupRunnerRecorder{}
	svc, err := New(Options{Runner: runner})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	root := t.TempDir()
	targetA := filepath.Join(root, "a")
	targetB := filepath.Join(root, "b")
	if err := os.MkdirAll(targetA, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetB), []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	expectedA, err := pathutil.Normalize(targetA)
	if err != nil {
		t.Fatalf("Normalize(targetA) error = %v", err)
	}
	expectedB, err := pathutil.Normalize(targetB)
	if err != nil {
		t.Fatalf("Normalize(targetB) error = %v", err)
	}

	result, err := svc.CleanLocal(context.Background(), LocalCleanupRequest{Paths: []string{targetA, targetB, targetA}})
	if err != nil {
		t.Fatalf("CleanLocal() error = %v", err)
	}
	if len(result.Removed) < 2 {
		t.Fatalf("Removed len = %d, want at least 2", len(result.Removed))
	}
	removed := make(map[string]struct{}, len(result.Removed))
	for _, path := range result.Removed {
		removed[path] = struct{}{}
	}
	if _, ok := removed[expectedA]; !ok {
		t.Fatalf("Removed paths %v missing targetA %q", result.Removed, expectedA)
	}
	if _, ok := removed[expectedB]; !ok {
		t.Fatalf("Removed paths %v missing targetB %q", result.Removed, expectedB)
	}
	if _, err := os.Stat(targetA); !os.IsNotExist(err) {
		t.Fatalf("targetA stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(targetB); !os.IsNotExist(err) {
		t.Fatalf("targetB stat err = %v, want not exist", err)
	}
}

func TestCleanDockerBuildsRemovalCommands(t *testing.T) {
	t.Parallel()

	runner := &cleanupRunnerRecorder{}
	svc, err := New(Options{Runner: runner})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.CleanDocker(context.Background(), DockerCleanupRequest{
		ContainerIDs: []string{"abc123"},
		ImageRefs:    []docker.ImageRef{docker.NewImageRef("ghcr.io/valv/codex", "dev")},
		PruneBuilder: true,
		Force:        true,
	})
	if err != nil {
		t.Fatalf("CleanDocker() error = %v", err)
	}
	if len(result.Commands) != 3 {
		t.Fatalf("command count = %d, want 3", len(result.Commands))
	}
	if got, want := runner.calls[0], []string{"rm", "--force", "abc123"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("container rm args = %#v, want %#v", got, want)
	}
	if got, want := runner.calls[1], []string{"image", "rm", "--force", "ghcr.io/valv/codex:dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("image rm args = %#v, want %#v", got, want)
	}
	if got, want := runner.calls[2], []string{"builder", "prune", "--force"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("builder prune args = %#v, want %#v", got, want)
	}
}
