package docker

import (
	"context"
	"reflect"
	"testing"
)

func TestExecutorRunUsesBuiltArgs(t *testing.T) {
	t.Parallel()

	var got []string
	var gotCtx context.Context
	exec := NewExecutor(CommandRunnerFunc(func(ctx context.Context, args []string) error {
		gotCtx = ctx
		got = append([]string(nil), args...)
		return nil
	}))

	ctx := context.WithValue(context.Background(), struct{}{}, "run")
	req := ContainerRunRequest{
		Name:  "valv-test",
		Image: NewImageRef("ghcr.io/valv/codex", "dev"),
	}
	if err := exec.Run(ctx, req); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if gotCtx != ctx {
		t.Fatal("Run() context was not forwarded to the runner")
	}
	want := []string{"run", "--name", "valv-test", "ghcr.io/valv/codex:dev"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Run() args = %#v, want %#v", got, want)
	}
}

func TestExecutorInspectUsesBuiltArgs(t *testing.T) {
	t.Parallel()

	var got []string
	var gotCtx context.Context
	exec := NewExecutor(CommandRunnerFunc(func(ctx context.Context, args []string) error {
		gotCtx = ctx
		got = append([]string(nil), args...)
		return nil
	}))

	ctx := context.WithValue(context.Background(), struct{}{}, "inspect")
	if err := exec.Inspect(ctx, " valv-test "); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if gotCtx != ctx {
		t.Fatal("Inspect() context was not forwarded to the runner")
	}
	want := []string{"inspect", "valv-test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Inspect() args = %#v, want %#v", got, want)
	}
}

type outputRecorder struct {
	got []string
	out string
}

func (o *outputRecorder) Run(context.Context, []string) error {
	return nil
}

func (o *outputRecorder) Output(_ context.Context, args []string) (string, error) {
	o.got = append([]string(nil), args...)
	return o.out, nil
}

func TestExecutorCreateUsesBuiltArgs(t *testing.T) {
	t.Parallel()

	runner := &outputRecorder{out: "container-123"}
	exec := NewExecutor(runner)

	id, err := exec.Create(context.Background(), ContainerRunRequest{
		Name:        "valv-test",
		Image:       NewImageRef("ghcr.io/valv/codex", "dev"),
		Interactive: true,
		TTY:         true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id != "container-123" {
		t.Fatalf("Create() id = %q, want container-123", id)
	}
	want := []string{"create", "-i", "-t", "--name", "valv-test", "ghcr.io/valv/codex:dev"}
	if !reflect.DeepEqual(runner.got, want) {
		t.Fatalf("Create() args = %#v, want %#v", runner.got, want)
	}
}

func TestExecutorStartUsesBuiltArgs(t *testing.T) {
	t.Parallel()

	var got []string
	exec := NewExecutor(CommandRunnerFunc(func(ctx context.Context, args []string) error {
		got = append([]string(nil), args...)
		return nil
	}))

	if err := exec.Start(context.Background(), ContainerStartRequest{
		ContainerID: "valv-test",
		Attach:      true,
		Interactive: true,
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	want := []string{"start", "-a", "-i", "valv-test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Start() args = %#v, want %#v", got, want)
	}
}

func TestExecutorRemoveContainerUsesOutputWhenAvailable(t *testing.T) {
	t.Parallel()

	runner := &outputRecorder{out: "valv-test"}
	exec := NewExecutor(runner)

	if err := exec.RemoveContainer(context.Background(), ContainerRemoveRequest{
		IDs:   []string{"valv-test"},
		Force: true,
	}); err != nil {
		t.Fatalf("RemoveContainer() error = %v", err)
	}

	want := []string{"rm", "--force", "valv-test"}
	if !reflect.DeepEqual(runner.got, want) {
		t.Fatalf("RemoveContainer() args = %#v, want %#v", runner.got, want)
	}
}
