package images

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
)

type Runner interface {
	Run(context.Context, []string) error
}

type Options struct {
	Runner     Runner
	Repository string
	ContextDir string
	Dockerfile string
	DefaultTag string
	Logger     *log.Logger
}

type Service struct {
	runner     Runner
	repository string
	contextDir string
	dockerfile string
	defaultTag string
	logger     *log.Logger
}

type BuildRequest struct {
	Tag        string
	Version    string
	ContextDir string
	Dockerfile string
	BuildArgs  map[string]string
	Labels     map[string]string
	Target     string
	Network    string
	Extra      []string
	Pull       bool
	NoCache    bool
}

type BuildResult struct {
	Image docker.ImageRef
	Args  []string
}

type UpdateRequest struct {
	BuildRequest
	PreviousImages []docker.ImageRef
	RemovePrevious bool
}

func New(options Options) (Service, error) {
	if options.Runner == nil {
		return Service{}, fmt.Errorf("new image service: runner is required")
	}
	if strings.TrimSpace(options.Repository) == "" {
		return Service{}, fmt.Errorf("new image service: repository is required")
	}
	if strings.TrimSpace(options.ContextDir) == "" {
		return Service{}, fmt.Errorf("new image service: context dir is required")
	}
	defaultTag := strings.TrimSpace(options.DefaultTag)
	if defaultTag == "" {
		defaultTag = "dev"
	}
	return Service{
		runner:     options.Runner,
		repository: strings.TrimSpace(options.Repository),
		contextDir: strings.TrimSpace(options.ContextDir),
		dockerfile: strings.TrimSpace(options.Dockerfile),
		defaultTag: defaultTag,
		logger:     options.Logger,
	}, nil
}

func (s Service) Build(ctx context.Context, request BuildRequest) (BuildResult, error) {
	buildRequest, image := s.resolveBuildRequest(request)
	args, err := docker.BuildImageArgs(buildRequest)
	if err != nil {
		return BuildResult{}, fmt.Errorf("build codex image: %w", err)
	}

	s.debug("building codex image", "image", image.String(), "context_dir", buildRequest.ContextDir, "dockerfile", buildRequest.Dockerfile)
	if err := s.runner.Run(ctx, args); err != nil {
		return BuildResult{}, fmt.Errorf("build codex image %q: %w", image.String(), err)
	}
	return BuildResult{Image: image, Args: args}, nil
}

func (s Service) Update(ctx context.Context, request UpdateRequest) (BuildResult, error) {
	result, err := s.Build(ctx, request.BuildRequest)
	if err != nil {
		return BuildResult{}, err
	}
	if !request.RemovePrevious || len(request.PreviousImages) == 0 {
		return result, nil
	}
	if err := s.Remove(ctx, request.PreviousImages...); err != nil {
		return BuildResult{}, fmt.Errorf("update codex image %q: remove previous images: %w", result.Image.String(), err)
	}
	return result, nil
}

func (s Service) Remove(ctx context.Context, refs ...docker.ImageRef) error {
	if len(refs) == 0 {
		return nil
	}
	args, err := docker.BuildImageRemoveArgs(docker.ImageRemoveRequest{Refs: refs, Force: true})
	if err != nil {
		return fmt.Errorf("remove codex image: %w", err)
	}
	s.debug("removing codex image", "images", refs)
	if err := s.runner.Run(ctx, args); err != nil {
		return fmt.Errorf("remove codex image: %w", err)
	}
	return nil
}

func (s Service) resolveBuildRequest(request BuildRequest) (docker.ImageBuildRequest, docker.ImageRef) {
	contextDir := strings.TrimSpace(request.ContextDir)
	if contextDir == "" {
		contextDir = s.contextDir
	}
	dockerfile := strings.TrimSpace(request.Dockerfile)
	if dockerfile == "" {
		dockerfile = s.dockerfile
	}
	if dockerfile != "" && !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(contextDir, dockerfile)
	}
	tag := strings.TrimSpace(request.Tag)
	if tag == "" {
		tag = s.defaultTag
	}
	image := docker.NewImageRef(s.repository, tag)
	buildArgs := cloneMap(request.BuildArgs)
	if request.Version != "" {
		if buildArgs == nil {
			buildArgs = make(map[string]string)
		}
		if _, ok := buildArgs["CODEX_VERSION"]; !ok {
			buildArgs["CODEX_VERSION"] = request.Version
		}
	}
	return docker.ImageBuildRequest{
		ContextDir: contextDir,
		Dockerfile: dockerfile,
		Tags:       []docker.ImageRef{image},
		BuildArgs:  buildArgs,
		Labels:     cloneMap(request.Labels),
		Target:     strings.TrimSpace(request.Target),
		Network:    strings.TrimSpace(request.Network),
		Extra:      append([]string(nil), request.Extra...),
		Pull:       request.Pull,
		NoCache:    request.NoCache,
	}, image
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}
