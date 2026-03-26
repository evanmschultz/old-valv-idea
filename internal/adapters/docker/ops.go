package docker

import (
	"fmt"
	"sort"
	"strings"
)

type ImageBuildRequest struct {
	ContextDir string
	Dockerfile string
	Tags       []ImageRef
	Builder    string
	BuildArgs  map[string]string
	Labels     map[string]string
	Target     string
	Network    string
	Extra      []string
	Pull       bool
	NoCache    bool
}

func (r ImageBuildRequest) Valid() error {
	if strings.TrimSpace(r.ContextDir) == "" {
		return fmt.Errorf("validate image build request: context dir is required")
	}
	if len(r.Tags) == 0 {
		return fmt.Errorf("validate image build request: at least one tag is required")
	}
	for _, tag := range r.Tags {
		if strings.TrimSpace(tag.Repository) == "" {
			return fmt.Errorf("validate image build request: tag repository is required")
		}
	}
	return nil
}

func BuildImageArgs(request ImageBuildRequest) ([]string, error) {
	if err := request.Valid(); err != nil {
		return nil, err
	}

	builder := strings.ToLower(strings.TrimSpace(request.Builder))
	if builder == "" || builder == "auto" {
		builder = "buildx"
	}

	args := []string{}
	switch builder {
	case "buildx":
		args = append(args, "buildx", "build", "--load")
	case "build", "legacy":
		args = append(args, "build")
	default:
		return nil, fmt.Errorf("build image args: unsupported builder %q", request.Builder)
	}

	if request.Pull {
		args = append(args, "--pull")
	}
	if request.NoCache {
		args = append(args, "--no-cache")
	}
	if strings.TrimSpace(request.Dockerfile) != "" {
		args = append(args, "-f", strings.TrimSpace(request.Dockerfile))
	}
	if strings.TrimSpace(request.Target) != "" {
		args = append(args, "--target", strings.TrimSpace(request.Target))
	}
	if strings.TrimSpace(request.Network) != "" {
		args = append(args, "--network", strings.TrimSpace(request.Network))
	}

	for _, tag := range request.Tags {
		args = append(args, "-t", tag.String())
	}

	if len(request.BuildArgs) > 0 {
		keys := make([]string, 0, len(request.BuildArgs))
		for key := range request.BuildArgs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, request.BuildArgs[key]))
		}
	}

	if len(request.Labels) > 0 {
		keys := make([]string, 0, len(request.Labels))
		for key := range request.Labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args = append(args, "--label", fmt.Sprintf("%s=%s", key, request.Labels[key]))
		}
	}

	args = append(args, request.Extra...)
	args = append(args, strings.TrimSpace(request.ContextDir))
	return args, nil
}

type ImageRemoveRequest struct {
	Refs  []ImageRef
	Force bool
}

func (r ImageRemoveRequest) Valid() error {
	if len(r.Refs) == 0 {
		return fmt.Errorf("validate image remove request: at least one image ref is required")
	}
	for _, ref := range r.Refs {
		if strings.TrimSpace(ref.Repository) == "" {
			return fmt.Errorf("validate image remove request: image repository is required")
		}
	}
	return nil
}

func BuildImageRemoveArgs(request ImageRemoveRequest) ([]string, error) {
	if err := request.Valid(); err != nil {
		return nil, err
	}
	args := []string{"image", "rm"}
	if request.Force {
		args = append(args, "--force")
	}
	for _, ref := range request.Refs {
		args = append(args, ref.String())
	}
	return args, nil
}

type ContainerRemoveRequest struct {
	IDs     []string
	Force   bool
	Volumes bool
}

func (r ContainerRemoveRequest) Valid() error {
	if len(r.IDs) == 0 {
		return fmt.Errorf("validate container remove request: at least one container id is required")
	}
	for _, id := range r.IDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("validate container remove request: container id is required")
		}
	}
	return nil
}

func BuildContainerRemoveArgs(request ContainerRemoveRequest) ([]string, error) {
	if err := request.Valid(); err != nil {
		return nil, err
	}
	args := []string{"rm"}
	if request.Force {
		args = append(args, "--force")
	}
	if request.Volumes {
		args = append(args, "--volumes")
	}
	for _, id := range request.IDs {
		args = append(args, strings.TrimSpace(id))
	}
	return args, nil
}

type ContainerListRequest struct {
	All     bool
	Quiet   bool
	Filters map[string]string
}

func BuildContainerListArgs(request ContainerListRequest) ([]string, error) {
	args := []string{"ps"}
	if request.All {
		args = append(args, "-a")
	}
	if request.Quiet {
		args = append(args, "-q")
	}
	if len(request.Filters) > 0 {
		keys := make([]string, 0, len(request.Filters))
		for key := range request.Filters {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args = append(args, "--filter", fmt.Sprintf("%s=%s", key, request.Filters[key]))
		}
	}
	return args, nil
}

type BuilderPruneRequest struct {
	All     bool
	Filters map[string]string
}

func BuildBuilderPruneArgs(request BuilderPruneRequest) ([]string, error) {
	args := []string{"builder", "prune", "--force"}
	if request.All {
		args = append(args, "--all")
	}
	if len(request.Filters) > 0 {
		keys := make([]string, 0, len(request.Filters))
		for key := range request.Filters {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args = append(args, "--filter", fmt.Sprintf("%s=%s", key, request.Filters[key]))
		}
	}
	return args, nil
}
