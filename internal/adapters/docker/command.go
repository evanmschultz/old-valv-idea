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

func (e Executor) Create(ctx context.Context, request ContainerRunRequest) (string, error) {
	outputter, ok := e.runner.(interface {
		Output(context.Context, []string) (string, error)
	})
	if !ok {
		return "", ErrOutputUnsupported
	}
	args, err := BuildCreateArgs(request)
	if err != nil {
		return "", err
	}
	return outputter.Output(ctx, args)
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

func (e Executor) Start(ctx context.Context, request ContainerStartRequest) error {
	args, err := BuildStartArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}

func (e Executor) Attach(ctx context.Context, request ContainerAttachRequest) error {
	args, err := BuildAttachArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}
