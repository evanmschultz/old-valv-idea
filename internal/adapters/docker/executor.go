package docker

import (
	"context"
	"fmt"
	"strings"
)

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

// RunContainerDetached shells out `docker run -d` for the supplied request and
// returns the container ID printed by Docker on stdout. The request must have
// Detached set to true by the caller; BuildRunArgs handles the -d flag. The
// underlying runner must implement Output; if it does not,
// ErrOutputUnsupported is returned. DROP_15 uses this to launch the proxy
// sidecar container (Schema Decision 5).
func (e Executor) RunContainerDetached(ctx context.Context, request ContainerRunRequest) (string, error) {
	outputter, ok := e.runner.(interface {
		Output(context.Context, []string) (string, error)
	})
	if !ok {
		return "", ErrOutputUnsupported
	}
	args, err := BuildRunArgs(request)
	if err != nil {
		return "", err
	}
	out, err := outputter.Output(ctx, args)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ConnectNetwork shells out `docker network connect` to attach a container
// to a network with optional aliases. The sidecar-proxy topology uses this to
// attach the proxy to both the internal network and bridge (Schema Decision 5).
func (e Executor) ConnectNetwork(ctx context.Context, request NetworkConnectRequest) error {
	args, err := BuildNetworkConnectArgs(request)
	if err != nil {
		return err
	}
	return e.runner.Run(ctx, args)
}

// ContainerRunning queries the Docker daemon for the running state of the
// named container by executing `docker inspect --format {{.State.Running}}
// <containerID>`. It returns true when the daemon reports "true", false when
// the state is any other non-error value (e.g. "false", ""), and an error
// when the underlying runner fails.
//
// Requires the underlying runner to implement
// Output(context.Context, []string) (string, error); a non-outputting runner
// returns ErrOutputUnsupported.
//
// DROP_15 Unit 15.2.5.D.2 uses this as the production-side readiness probe
// for the valv-proxy sidecar container.
func (e Executor) ContainerRunning(ctx context.Context, containerID string) (bool, error) {
	outputter, ok := e.runner.(interface {
		Output(context.Context, []string) (string, error)
	})
	if !ok {
		return false, ErrOutputUnsupported
	}
	args := []string{"inspect", "--format", "{{.State.Running}}", containerID}
	out, err := outputter.Output(ctx, args)
	if err != nil {
		return false, fmt.Errorf("docker inspect container %s: %w", containerID, err)
	}
	return strings.TrimSpace(out) == "true", nil
}

// ListContainersByLabel shells out `docker ps -a --filter label=<label>
// --format {{.ID}}` and returns one container ID per line. The label filter
// accepts either a key (e.g. "valv") or a key=value pair (e.g.
// "valv=network-policy"); Docker treats both forms uniformly.
//
// Returns an empty slice when no containers match. Requires the underlying
// runner to implement Output(context.Context, []string) (string, error); a
// non-outputting runner returns ErrOutputUnsupported.
//
// DROP_15 Unit 15.2.5.E.2 uses this to enumerate stale proxy sidecar
// containers so they can be removed before a fresh sidecar is launched.
func (e Executor) ListContainersByLabel(ctx context.Context, label string) ([]string, error) {
	outputter, ok := e.runner.(interface {
		Output(context.Context, []string) (string, error)
	})
	if !ok {
		return nil, ErrOutputUnsupported
	}
	args := []string{"ps", "-a", "--filter", "label=" + label, "--format", "{{.ID}}"}
	out, err := outputter.Output(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	lines := strings.Split(out, "\n")
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		id := strings.TrimSpace(line)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ListNetworks shells out `docker network ls --filter label=<label>
// --format {{.Name}}` and returns one network name per line. The label
// filter accepts either a key (e.g. "valv") or a key=value pair (e.g.
// "valv=network-policy"); Docker treats both forms uniformly.
//
// Returns an empty slice when no networks match. Requires the underlying
// runner to implement Output(context.Context, []string) (string, error); a
// non-outputting runner returns ErrOutputUnsupported.
func (e Executor) ListNetworks(ctx context.Context, label string) ([]string, error) {
	outputter, ok := e.runner.(interface {
		Output(context.Context, []string) (string, error)
	})
	if !ok {
		return nil, ErrOutputUnsupported
	}
	args := []string{"network", "ls", "--filter", "label=" + label, "--format", "{{.Name}}"}
	out, err := outputter.Output(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("docker network ls: %w", err)
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	lines := strings.Split(out, "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}
