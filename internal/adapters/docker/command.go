package docker

import "context"

type Executor struct {
	runner CommandRunner
}

func NewExecutor(runner CommandRunner) Executor {
	return Executor{runner: runner}
}

func (e Executor) Run(ctx context.Context, request ContainerRunRequest) error {
	args, err := BuildRunArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}

func (e Executor) Inspect(ctx context.Context, name string) error {
	args, err := BuildInspectArgs(name)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}

func (e Executor) Exec(ctx context.Context, request ContainerExecRequest) error {
	args, err := BuildExecArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}
