package docker

import "context"

func (e Executor) Build(ctx context.Context, request ImageBuildRequest) error {
	args, err := BuildImageArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}

func (e Executor) RemoveImage(ctx context.Context, request ImageRemoveRequest) error {
	args, err := BuildImageRemoveArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}

func (e Executor) RemoveContainer(ctx context.Context, request ContainerRemoveRequest) error {
	args, err := BuildContainerRemoveArgs(request)
	if err != nil {
		return err
	}
	if outputter, ok := e.runner.(interface {
		Output(context.Context, []string) (string, error)
	}); ok {
		_, err := outputter.Output(ctx, args)
		return err
	}
	return e.runner.Run(ctx, args)
}

func (e Executor) PruneBuilder(ctx context.Context, request BuilderPruneRequest) error {
	args, err := BuildBuilderPruneArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}

// CreateNetwork shells out `docker network create` with the request's flags
// and labels. DROP_15 uses this to provision the single internal network that
// backs closed-default network policy (per Schema Decision 5).
func (e Executor) CreateNetwork(ctx context.Context, request NetworkCreateRequest) error {
	args, err := BuildNetworkCreateArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}

// RemoveNetwork shells out `docker network rm` for the named network. Callers
// are expected to remove only Valv-managed networks (identified by the
// `valv=network-policy` label applied at create time).
func (e Executor) RemoveNetwork(ctx context.Context, request NetworkRemoveRequest) error {
	args, err := BuildNetworkRemoveArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}
