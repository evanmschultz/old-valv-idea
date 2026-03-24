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
