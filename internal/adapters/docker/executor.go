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
