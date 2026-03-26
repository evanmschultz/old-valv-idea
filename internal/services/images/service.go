package images

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
)

const (
	DefaultCodexVersion    = "0.116.0"
	defaultCodexDockerfile = "Dockerfile"
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
	UserID     int
	GroupID    int
	Logger     *log.Logger
}

type Service struct {
	runner     Runner
	repository string
	contextDir string
	dockerfile string
	defaultTag string
	userID     int
	groupID    int
	logger     *log.Logger
}

type BuildRequest struct {
	Version   string
	Pull      bool
	NoCache   bool
	ExtraTags []docker.ImageRef
}

type BuildResult struct {
	Image      docker.ImageRef
	Tags       []docker.ImageRef
	ContextDir string
	Dockerfile string
	Version    string
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

	dockerfile := strings.TrimSpace(options.Dockerfile)
	if dockerfile == "" {
		dockerfile = defaultCodexDockerfile
	}
	defaultTag := strings.TrimSpace(options.DefaultTag)
	if defaultTag == "" {
		defaultTag = "dev"
	}
	userID := options.UserID
	if userID == 0 {
		userID = os.Getuid()
	}
	groupID := options.GroupID
	if groupID == 0 {
		groupID = os.Getgid()
	}

	return Service{
		runner:     options.Runner,
		repository: strings.TrimSpace(options.Repository),
		contextDir: strings.TrimSpace(options.ContextDir),
		dockerfile: dockerfile,
		defaultTag: defaultTag,
		userID:     userID,
		groupID:    groupID,
		logger:     options.Logger,
	}, nil
}

func (s Service) Build(ctx context.Context, request BuildRequest) (BuildResult, error) {
	version := strings.TrimSpace(request.Version)
	if version == "" {
		version = DefaultCodexVersion
	}

	tags := []docker.ImageRef{docker.NewImageRef(s.repository, s.defaultTag)}
	for _, extra := range request.ExtraTags {
		if strings.TrimSpace(extra.Repository) == "" {
			continue
		}
		tags = append(tags, extra)
	}

	buildRequest := docker.ImageBuildRequest{
		ContextDir: s.contextDir,
		Dockerfile: filepath.Join(s.contextDir, s.dockerfile),
		Tags:       tags,
		Builder:    "auto",
		BuildArgs: map[string]string{
			"CODEX_VERSION": version,
			"VALV_GID":      fmt.Sprintf("%d", s.groupID),
			"VALV_UID":      fmt.Sprintf("%d", s.userID),
		},
		Labels: map[string]string{
			"io.valv.managed":  "true",
			"io.valv.provider": "codex",
			"io.valv.scope":    "image",
			"io.valv.version":  version,
		},
		Pull:    request.Pull,
		NoCache: request.NoCache,
	}
	args, err := docker.BuildImageArgs(buildRequest)
	if err != nil {
		return BuildResult{}, fmt.Errorf("build image: %w", err)
	}
	if err := s.runner.Run(ctx, args); err != nil {
		if isBuildxUnavailable(err) {
			s.debug("docker buildx unavailable, falling back to legacy docker build", "context_dir", s.contextDir)
			buildRequest.Builder = "legacy"
			fallbackArgs, fallbackErr := docker.BuildImageArgs(buildRequest)
			if fallbackErr != nil {
				return BuildResult{}, fmt.Errorf("build image fallback: %w", fallbackErr)
			}
			if err := s.runner.Run(ctx, fallbackArgs); err != nil {
				return BuildResult{}, fmt.Errorf("build image fallback: %w", err)
			}
		} else {
			return BuildResult{}, fmt.Errorf("build image: %w", err)
		}
	}

	result := BuildResult{
		Image:      tags[0],
		Tags:       tags,
		ContextDir: s.contextDir,
		Dockerfile: filepath.Join(s.contextDir, s.dockerfile),
		Version:    version,
	}
	s.debug("built provider image", "image", result.Image.String(), "version", version, "context_dir", s.contextDir)
	return result, nil
}

func (s Service) Update(ctx context.Context, request UpdateRequest) (BuildResult, error) {
	result, err := s.Build(ctx, request.BuildRequest)
	if err != nil {
		return BuildResult{}, err
	}

	if request.RemovePrevious && len(request.PreviousImages) > 0 {
		args, err := docker.BuildImageRemoveArgs(docker.ImageRemoveRequest{
			Refs:  request.PreviousImages,
			Force: true,
		})
		if err != nil {
			return BuildResult{}, fmt.Errorf("remove previous images: %w", err)
		}
		if err := s.runner.Run(ctx, args); err != nil {
			return BuildResult{}, fmt.Errorf("remove previous images: %w", err)
		}
		s.debug("removed previous images", "count", len(request.PreviousImages))
	}

	return result, nil
}

func WriteDefaultCodexContext(root string) (string, error) {
	contextDir := strings.TrimSpace(root)
	if contextDir == "" {
		return "", fmt.Errorf("write default codex context: root is required")
	}
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return "", fmt.Errorf("write default codex context: ensure context dir %q: %w", contextDir, err)
	}
	dockerfilePath := filepath.Join(contextDir, defaultCodexDockerfile)
	if err := os.WriteFile(dockerfilePath, []byte(DefaultCodexDockerfile()), 0o644); err != nil {
		return "", fmt.Errorf("write default codex context: write dockerfile %q: %w", dockerfilePath, err)
	}
	return dockerfilePath, nil
}

func DefaultCodexDockerfile() string {
	return strings.TrimSpace(`
FROM node:22-bookworm-slim

ARG VALV_UID=1000
ARG VALV_GID=1000

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -g "${VALV_GID}" valv \
    && useradd -m -u "${VALV_UID}" -g "${VALV_GID}" -s /bin/sh valv \
    && mkdir -p /home/valv/.codex /workspace \
    && chown -R valv:valv /home/valv /workspace

ENV NPM_CONFIG_UPDATE_NOTIFIER=false \
    NPM_CONFIG_FUND=false \
    NPM_CONFIG_AUDIT=false \
    HOME=/home/valv \
    LOGNAME=valv \
    USER=valv

ARG CODEX_VERSION
RUN npm install --global "@openai/codex@${CODEX_VERSION}"

USER valv
WORKDIR /workspace
ENTRYPOINT ["codex"]
`) + "\n"
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}

func isBuildxUnavailable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "docker buildx is required but unavailable") ||
		strings.Contains(message, "docker buildx is required")
}
