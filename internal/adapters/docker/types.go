package docker

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrOutputUnsupported = errors.New("docker runner does not support output capture")

type ImageRef struct {
	Repository string
	Tag        string
}

func NewImageRef(repository, tag string) ImageRef {
	return ImageRef{Repository: strings.TrimSpace(repository), Tag: strings.TrimSpace(tag)}
}

func (r ImageRef) String() string {
	switch {
	case r.Repository == "":
		return ""
	case r.Tag == "":
		return r.Repository
	default:
		return fmt.Sprintf("%s:%s", r.Repository, r.Tag)
	}
}

type MountSpec struct {
	Source   string
	Target   string
	ReadOnly bool
}

func NewMountSpec(source, target string, readOnly bool) MountSpec {
	return MountSpec{Source: strings.TrimSpace(source), Target: strings.TrimSpace(target), ReadOnly: readOnly}
}

type ContainerRunRequest struct {
	Name           string
	Image          ImageRef
	WorkingDir     string
	Env            map[string]string
	EnvPassthrough []string
	Labels         map[string]string
	Mounts         []MountSpec
	Args           []string
	Detached       bool
	Interactive    bool
	TTY            bool
	Init           bool
	Remove         bool
	User           string
	Network        string
	Extra          []string
}

type ContainerExecRequest struct {
	ContainerID    string
	WorkingDir     string
	Env            map[string]string
	EnvPassthrough []string
	Args           []string
	Interactive    bool
	TTY            bool
	User           string
}

type ContainerStartRequest struct {
	ContainerID string
	Attach      bool
	Interactive bool
}

func (r ContainerExecRequest) Valid() error {
	if strings.TrimSpace(r.ContainerID) == "" {
		return fmt.Errorf("validate container exec request: container id is required")
	}
	if len(r.Args) == 0 {
		return fmt.Errorf("validate container exec request: args are required")
	}
	return nil
}

func (r ContainerStartRequest) Valid() error {
	if strings.TrimSpace(r.ContainerID) == "" {
		return fmt.Errorf("validate container start request: container id is required")
	}
	return nil
}

func (r ContainerRunRequest) Valid() error {
	if strings.TrimSpace(r.Image.Repository) == "" {
		return fmt.Errorf("validate container run request: image is required")
	}
	if len(r.Mounts) == 0 {
		return nil
	}
	for _, mount := range r.Mounts {
		if strings.TrimSpace(mount.Source) == "" || strings.TrimSpace(mount.Target) == "" {
			return fmt.Errorf("validate container run request: mount source and target are required")
		}
	}
	return nil
}

type CommandRunner interface {
	Run(ctx context.Context, args []string) error
}

type CommandRunnerFunc func(ctx context.Context, args []string) error

func (f CommandRunnerFunc) Run(ctx context.Context, args []string) error {
	return f(ctx, args)
}

func BuildRunArgs(request ContainerRunRequest) ([]string, error) {
	return buildRunLikeArgs("run", request, true)
}

func BuildCreateArgs(request ContainerRunRequest) ([]string, error) {
	return buildRunLikeArgs("create", request, false)
}

func buildRunLikeArgs(verb string, request ContainerRunRequest, includeRemove bool) ([]string, error) {
	if err := request.Valid(); err != nil {
		return nil, err
	}

	args := []string{verb}
	if request.Detached {
		args = append(args, "-d")
	}
	if includeRemove && request.Remove {
		args = append(args, "--rm")
	}
	if request.Interactive {
		args = append(args, "-i")
	}
	if request.TTY {
		args = append(args, "-t")
	}
	if request.Init {
		args = append(args, "--init")
	}
	if request.Name != "" {
		args = append(args, "--name", request.Name)
	}
	if strings.TrimSpace(request.WorkingDir) != "" {
		args = append(args, "--workdir", strings.TrimSpace(request.WorkingDir))
	}
	if strings.TrimSpace(request.User) != "" {
		args = append(args, "--user", strings.TrimSpace(request.User))
	}
	if strings.TrimSpace(request.Network) != "" {
		args = append(args, "--network", strings.TrimSpace(request.Network))
	}
	if len(request.Env) > 0 {
		keys := make([]string, 0, len(request.Env))
		for key := range request.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args = append(args, "-e", fmt.Sprintf("%s=%s", key, request.Env[key]))
		}
	}
	if len(request.EnvPassthrough) > 0 {
		keys := append([]string(nil), request.EnvPassthrough...)
		sort.Strings(keys)
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, exists := request.Env[key]; exists {
				continue
			}
			args = append(args, "-e", key)
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
	for _, mount := range request.Mounts {
		mountArg := fmt.Sprintf("type=bind,source=%s,target=%s", mount.Source, mount.Target)
		if mount.ReadOnly {
			mountArg += ",readonly"
		}
		args = append(args, "--mount", mountArg)
	}
	args = append(args, request.Extra...)
	args = append(args, request.Image.String())
	args = append(args, request.Args...)
	return args, nil
}

func BuildExecArgs(request ContainerExecRequest) ([]string, error) {
	if err := request.Valid(); err != nil {
		return nil, err
	}

	args := []string{"exec"}
	if request.Interactive {
		args = append(args, "-i")
	}
	if request.TTY {
		args = append(args, "-t")
	}
	if strings.TrimSpace(request.WorkingDir) != "" {
		args = append(args, "--workdir", strings.TrimSpace(request.WorkingDir))
	}
	if strings.TrimSpace(request.User) != "" {
		args = append(args, "--user", strings.TrimSpace(request.User))
	}
	if len(request.Env) > 0 {
		keys := make([]string, 0, len(request.Env))
		for key := range request.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args = append(args, "-e", fmt.Sprintf("%s=%s", key, request.Env[key]))
		}
	}
	if len(request.EnvPassthrough) > 0 {
		keys := append([]string(nil), request.EnvPassthrough...)
		sort.Strings(keys)
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, exists := request.Env[key]; exists {
				continue
			}
			args = append(args, "-e", key)
		}
	}
	args = append(args, strings.TrimSpace(request.ContainerID))
	args = append(args, request.Args...)
	return args, nil
}

func BuildInspectArgs(name string) ([]string, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("build inspect args: name is required")
	}
	return []string{"inspect", strings.TrimSpace(name)}, nil
}

func BuildStartArgs(request ContainerStartRequest) ([]string, error) {
	if err := request.Valid(); err != nil {
		return nil, err
	}
	args := []string{"start"}
	if request.Attach {
		args = append(args, "-a")
	}
	if request.Interactive {
		args = append(args, "-i")
	}
	args = append(args, strings.TrimSpace(request.ContainerID))
	return args, nil
}
