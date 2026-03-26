package cleanup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/pathutil"
)

type Runner interface {
	Run(context.Context, []string) error
}

type Options struct {
	Runner Runner
	Logger *log.Logger
}

type Service struct {
	runner Runner
	logger *log.Logger
}

type LocalCleanupRequest struct {
	Paths []string
}

type LocalCleanupResult struct {
	Removed []string
}

type DockerCleanupRequest struct {
	ContainerIDs    []string
	ImageRefs       []docker.ImageRef
	PruneBuilder    bool
	PruneBuilderAll bool
	BuilderFilters  map[string]string
	Force           bool
	Volumes         bool
}

type DockerCleanupResult struct {
	Commands [][]string
}

func New(options Options) (Service, error) {
	if options.Runner == nil {
		return Service{}, fmt.Errorf("new cleanup service: runner is required")
	}
	return Service{runner: options.Runner, logger: options.Logger}, nil
}

func DefaultLocalTargets(paths config.Paths) []string {
	return []string{
		paths.LogsDir,
		paths.BuildCacheDir,
		paths.TempCacheDir,
		paths.RuntimeTmpDir,
		paths.PIDsDir,
		paths.LocksDir,
		paths.SocketsDir,
	}
}

func (s Service) CleanLocal(ctx context.Context, request LocalCleanupRequest) (LocalCleanupResult, error) {
	if len(request.Paths) == 0 {
		return LocalCleanupResult{}, nil
	}

	seen := make(map[string]struct{}, len(request.Paths))
	resolved := make([]string, 0, len(request.Paths))
	for _, rawPath := range request.Paths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			return LocalCleanupResult{}, fmt.Errorf("clean local: path is required")
		}
		normalized, err := pathutil.Normalize(path)
		if err != nil {
			return LocalCleanupResult{}, fmt.Errorf("clean local %q: normalize path: %w", path, err)
		}
		if normalized == string(filepath.Separator) {
			return LocalCleanupResult{}, fmt.Errorf("clean local %q: refusing to remove filesystem root", normalized)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		resolved = append(resolved, normalized)
	}

	removed := make([]string, 0, len(resolved))
	for _, normalized := range resolved {
		if err := os.RemoveAll(normalized); err != nil {
			return LocalCleanupResult{}, fmt.Errorf("clean local %q: remove directory: %w", normalized, err)
		}
		removed = append(removed, normalized)
		s.debug("removed local path", "path", normalized)
	}
	return LocalCleanupResult{Removed: removed}, nil
}

func (s Service) CleanDocker(ctx context.Context, request DockerCleanupRequest) (DockerCleanupResult, error) {
	commands := make([][]string, 0, 3)

	if len(request.ContainerIDs) > 0 {
		args, err := docker.BuildContainerRemoveArgs(docker.ContainerRemoveRequest{
			IDs:     request.ContainerIDs,
			Force:   request.Force,
			Volumes: request.Volumes,
		})
		if err != nil {
			return DockerCleanupResult{}, fmt.Errorf("clean docker containers: %w", err)
		}
		if err := s.runner.Run(ctx, args); err != nil {
			return DockerCleanupResult{}, fmt.Errorf("clean docker containers: %w", err)
		}
		commands = append(commands, args)
		s.debug("removed docker containers", "count", len(request.ContainerIDs))
	}

	if len(request.ImageRefs) > 0 {
		args, err := docker.BuildImageRemoveArgs(docker.ImageRemoveRequest{
			Refs:  request.ImageRefs,
			Force: request.Force,
		})
		if err != nil {
			return DockerCleanupResult{}, fmt.Errorf("clean docker images: %w", err)
		}
		if err := s.runner.Run(ctx, args); err != nil {
			if isMissingImageError(err) {
				s.debug("ignored missing docker images during cleanup", "count", len(request.ImageRefs))
			} else {
				return DockerCleanupResult{}, fmt.Errorf("clean docker images: %w", err)
			}
		}
		commands = append(commands, args)
		s.debug("removed docker images", "count", len(request.ImageRefs))
	}

	if request.PruneBuilder {
		args, err := docker.BuildBuilderPruneArgs(docker.BuilderPruneRequest{
			All:     request.PruneBuilderAll,
			Filters: cloneMap(request.BuilderFilters),
		})
		if err != nil {
			return DockerCleanupResult{}, fmt.Errorf("clean docker builder cache: %w", err)
		}
		if err := s.runner.Run(ctx, args); err != nil {
			return DockerCleanupResult{}, fmt.Errorf("clean docker builder cache: %w", err)
		}
		commands = append(commands, args)
		s.debug("pruned docker builder cache")
	}

	return DockerCleanupResult{Commands: commands}, nil
}

func isMissingImageError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such image") ||
		strings.Contains(message, "image not known")
}

func (s Service) Clean(ctx context.Context, local LocalCleanupRequest, dockerRequest DockerCleanupRequest) (LocalCleanupResult, DockerCleanupResult, error) {
	localResult, err := s.CleanLocal(ctx, local)
	if err != nil {
		return LocalCleanupResult{}, DockerCleanupResult{}, err
	}
	dockerResult, err := s.CleanDocker(ctx, dockerRequest)
	if err != nil {
		return LocalCleanupResult{}, DockerCleanupResult{}, err
	}
	return localResult, dockerResult, nil
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cloned[key] = values[key]
	}
	return cloned
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}
