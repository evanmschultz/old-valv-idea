package images

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/domain"
)

const (
	defaultCodexDockerfile   = "Dockerfile"
	defaultCodexLatestURL    = "https://api.github.com/repos/openai/codex/releases/latest"
	defaultVersionRequestTTL = 10 * time.Second
	recipeHashLabel          = "io.valv.recipe_hash"
)

// DefaultClaudeCLIVersion is the pinned version of the @anthropic-ai/claude-code
// npm package baked into the default Claude provider image. Verified against
// Context7 /anthropics/claude-code at build time; see drop BUILDER_WORKLOG.md
// for the timestamped re-verification record.
const DefaultClaudeCLIVersion = "2.1.89"

var (
	findDockerBinary = exec.LookPath
	versionPattern   = regexp.MustCompile(`\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?`)
)

type Runner interface {
	Run(context.Context, []string) error
}

type outputRunner interface {
	Output(context.Context, []string) (string, error)
}

type StateStore interface {
	domain.ProviderImageRepository
}

type VersionResolver interface {
	LatestVersion(context.Context) (string, error)
}

type Options struct {
	Runner     Runner
	StateStore StateStore
	Resolver   VersionResolver
	Provider   domain.Provider
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
	stateStore StateStore
	resolver   VersionResolver
	provider   domain.Provider
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

type EnsureRequest struct {
	Pull                     bool
	NoCache                  bool
	AllowExistingOnCheckFail bool
}

type EnsureAction string

const (
	EnsureActionUpdated            EnsureAction = "updated"
	EnsureActionUpToDate           EnsureAction = "up_to_date"
	EnsureActionUsingExistingImage EnsureAction = "using_existing_image"
)

type EnsureResult struct {
	BuildResult
	Action          EnsureAction
	LatestVersion   string
	PreviousVersion string
	LatestCheckedAt time.Time
}

type codexReleasePayload struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

type codexVersionResolver struct {
	client *http.Client
	url    string
}

func NewCodexVersionResolver(client *http.Client) VersionResolver {
	if client == nil {
		client = &http.Client{Timeout: defaultVersionRequestTTL}
	}
	return codexVersionResolver{client: client, url: defaultCodexLatestURL}
}

func (r codexVersionResolver) LatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return "", fmt.Errorf("latest codex version: new request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "valv")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("latest codex version: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("latest codex version: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload codexReleasePayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("latest codex version: decode response: %w", err)
	}
	for _, candidate := range []string{payload.Name, payload.TagName} {
		if version := normalizeCodexVersion(candidate); version != "" {
			return version, nil
		}
	}
	return "", fmt.Errorf("latest codex version: no version found in release payload")
}

func normalizeCodexVersion(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "rust-v")
	trimmed = strings.TrimPrefix(trimmed, "v")
	return versionPattern.FindString(trimmed)
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
	provider := options.Provider
	if provider == "" {
		provider = domain.ProviderCodex
	}
	resolver := options.Resolver
	if resolver == nil && provider == domain.ProviderCodex {
		resolver = NewCodexVersionResolver(nil)
	}
	return Service{
		runner:     options.Runner,
		stateStore: options.StateStore,
		resolver:   resolver,
		provider:   provider,
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
		return BuildResult{}, fmt.Errorf("build image: version is required")
	}

	tags := []docker.ImageRef{docker.NewImageRef(s.repository, s.defaultTag)}
	seenTags := map[string]struct{}{tags[0].String(): {}}
	for _, extra := range request.ExtraTags {
		if strings.TrimSpace(extra.Repository) == "" {
			continue
		}
		if _, ok := seenTags[extra.String()]; ok {
			continue
		}
		seenTags[extra.String()] = struct{}{}
		tags = append(tags, extra)
	}

	buildRequest := docker.ImageBuildRequest{
		ContextDir: s.contextDir,
		Dockerfile: filepath.Join(s.contextDir, s.dockerfile),
		Tags:       tags,
		Builder:    "auto",
		BuildArgs: map[string]string{
			s.providerVersionBuildArg(): version,
			"VALV_GID":                  fmt.Sprintf("%d", s.groupID),
			"VALV_UID":                  fmt.Sprintf("%d", s.userID),
		},
		Labels: map[string]string{
			"io.valv.managed":  "true",
			"io.valv.provider": string(s.provider),
			"io.valv.scope":    "image",
			"io.valv.version":  version,
			recipeHashLabel:    s.recipeHash(),
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
	s.debug("built provider image", "provider", s.provider, "image", result.Image.String(), "version", version, "context_dir", s.contextDir)
	return result, nil
}

func (s Service) EnsureLatest(ctx context.Context, request EnsureRequest) (EnsureResult, error) {
	if s.resolver == nil {
		return EnsureResult{}, fmt.Errorf("ensure latest image: latest-version resolver is required")
	}
	if s.stateStore == nil {
		return EnsureResult{}, fmt.Errorf("ensure latest image: state store is required")
	}

	state, stateFound, err := s.currentState(ctx)
	if err != nil {
		return EnsureResult{}, err
	}

	latestVersion, err := s.resolver.LatestVersion(ctx)
	if err != nil {
		if request.AllowExistingOnCheckFail {
			available, inspectErr := s.imageAvailable(ctx, s.defaultImageRef())
			if inspectErr != nil {
				return EnsureResult{}, inspectErr
			}
			if available {
				return EnsureResult{
					BuildResult: BuildResult{
						Image:      s.defaultImageRef(),
						Tags:       existingTags(state, s.defaultImageRef()),
						ContextDir: s.contextDir,
						Dockerfile: filepath.Join(s.contextDir, s.dockerfile),
						Version:    state.InstalledVersion,
					},
					Action:          EnsureActionUsingExistingImage,
					LatestVersion:   state.LatestVersion,
					PreviousVersion: state.InstalledVersion,
					LatestCheckedAt: state.LatestCheckedAt,
				}, nil
			}
		}
		return EnsureResult{}, fmt.Errorf("ensure latest image: resolve latest %s version: %w", s.provider, err)
	}

	checkedAt := time.Now().UTC()
	versionTag := s.versionImageRef(latestVersion)
	defaultRef := s.defaultImageRef()
	available, err := s.imageAvailable(ctx, defaultRef)
	if err != nil {
		return EnsureResult{}, err
	}
	recipeMatches := false
	if available {
		recipeMatches, err = s.imageRecipeMatches(ctx, defaultRef)
		if err != nil {
			return EnsureResult{}, err
		}
	}

	if available && stateFound && state.InstalledVersion == latestVersion && recipeMatches {
		state.LatestVersion = latestVersion
		state.LatestCheckedAt = checkedAt
		state.UpdatedAt = time.Now().UTC()
		if _, err := s.stateStore.UpsertProviderImageState(ctx, state); err != nil {
			return EnsureResult{}, fmt.Errorf("ensure latest image: persist provider image state: %w", err)
		}
		return EnsureResult{
			BuildResult: BuildResult{
				Image:      defaultRef,
				Tags:       []docker.ImageRef{defaultRef, versionTag},
				ContextDir: s.contextDir,
				Dockerfile: filepath.Join(s.contextDir, s.dockerfile),
				Version:    latestVersion,
			},
			Action:          EnsureActionUpToDate,
			LatestVersion:   latestVersion,
			PreviousVersion: state.InstalledVersion,
			LatestCheckedAt: checkedAt,
		}, nil
	}

	buildResult, err := s.Build(ctx, BuildRequest{
		Version:   latestVersion,
		Pull:      request.Pull,
		NoCache:   request.NoCache,
		ExtraTags: []docker.ImageRef{versionTag},
	})
	if err != nil {
		return EnsureResult{}, err
	}

	previousVersion := state.InstalledVersion
	if stateFound && state.InstalledVersionTag != "" && state.InstalledVersionTag != versionTag.String() {
		args, err := docker.BuildImageRemoveArgs(docker.ImageRemoveRequest{
			Refs:  []docker.ImageRef{parseImageRef(state.InstalledVersionTag)},
			Force: true,
		})
		if err != nil {
			return EnsureResult{}, fmt.Errorf("ensure latest image: remove previous image tag: %w", err)
		}
		if err := s.runner.Run(ctx, args); err != nil && !isMissingImageError(err) {
			return EnsureResult{}, fmt.Errorf("ensure latest image: remove previous image tag: %w", err)
		}
	}

	state = domain.ProviderImageState{
		Provider:            s.provider,
		LatestVersion:       latestVersion,
		LatestCheckedAt:     checkedAt,
		InstalledVersion:    latestVersion,
		InstalledImageRef:   buildResult.Image.String(),
		InstalledVersionTag: versionTag.String(),
		UpdatedAt:           time.Now().UTC(),
	}
	if _, err := s.stateStore.UpsertProviderImageState(ctx, state); err != nil {
		return EnsureResult{}, fmt.Errorf("ensure latest image: persist provider image state: %w", err)
	}

	return EnsureResult{
		BuildResult:     buildResult,
		Action:          EnsureActionUpdated,
		LatestVersion:   latestVersion,
		PreviousVersion: previousVersion,
		LatestCheckedAt: checkedAt,
	}, nil
}

func (s Service) CurrentState(ctx context.Context) (domain.ProviderImageState, error) {
	if s.stateStore == nil {
		return domain.ProviderImageState{}, fmt.Errorf("provider image state: state store is required")
	}
	state, _, err := s.currentState(ctx)
	return state, err
}

func (s Service) currentState(ctx context.Context) (domain.ProviderImageState, bool, error) {
	if s.stateStore == nil {
		return domain.ProviderImageState{}, false, nil
	}
	state, err := s.stateStore.ProviderImageState(ctx, s.provider)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ProviderImageState{}, false, nil
		}
		return domain.ProviderImageState{}, false, fmt.Errorf("provider image state: load %s image state: %w", s.provider, err)
	}
	return state, true, nil
}

func (s Service) defaultImageRef() docker.ImageRef {
	return docker.NewImageRef(s.repository, s.defaultTag)
}

func (s Service) recipeHash() string {
	content := s.providerDockerfileContent()
	if filepath.Base(s.dockerfile) != defaultCodexDockerfile {
		path := filepath.Join(s.contextDir, s.dockerfile)
		if fileContent, err := os.ReadFile(path); err == nil {
			content = string(fileContent)
		}
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// providerDockerfileContent returns the default-Dockerfile text for the service's
// provider. It is the single source of truth the recipeHash default branch
// consults; adding a new provider means adding a new case here.
func (s Service) providerDockerfileContent() string {
	if s.provider == domain.ProviderClaude {
		return DefaultClaudeDockerfile()
	}
	return DefaultCodexDockerfile()
}

// providerVersionBuildArg returns the Docker build-arg name that the provider's
// default Dockerfile expects for its CLI version pin. Codex uses CODEX_VERSION;
// Claude uses CLAUDE_VERSION.
func (s Service) providerVersionBuildArg() string {
	if s.provider == domain.ProviderClaude {
		return "CLAUDE_VERSION"
	}
	return "CODEX_VERSION"
}

func (s Service) versionImageRef(version string) docker.ImageRef {
	return docker.NewImageRef(s.repository, strings.ReplaceAll(strings.TrimSpace(version), ".", "-"))
}

func (s Service) imageAvailable(ctx context.Context, image docker.ImageRef) (bool, error) {
	if _, err := findDockerBinary("docker"); err != nil {
		return false, nil
	}
	if err := s.runner.Run(ctx, []string{"image", "inspect", image.String()}); err != nil {
		if dockerImageMissingError(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect image %q: %w", image.String(), err)
	}
	return true, nil
}

func (s Service) imageRecipeMatches(ctx context.Context, image docker.ImageRef) (bool, error) {
	runner, ok := s.runner.(outputRunner)
	if !ok {
		return true, nil
	}
	output, err := runner.Output(ctx, []string{"image", "inspect", "--format", "{{ index .Config.Labels \"" + recipeHashLabel + "\" }}", image.String()})
	if err != nil {
		if dockerImageMissingError(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect image %q recipe hash: %w", image.String(), err)
	}
	return strings.TrimSpace(output) == s.recipeHash(), nil
}

func existingTags(state domain.ProviderImageState, defaultRef docker.ImageRef) []docker.ImageRef {
	tags := []docker.ImageRef{defaultRef}
	if strings.TrimSpace(state.InstalledVersionTag) != "" {
		tags = append(tags, parseImageRef(state.InstalledVersionTag))
	}
	return tags
}

func parseImageRef(value string) docker.ImageRef {
	trimmed := strings.TrimSpace(value)
	lastSlash := strings.LastIndex(trimmed, "/")
	lastColon := strings.LastIndex(trimmed, ":")
	if lastColon > lastSlash {
		return docker.NewImageRef(trimmed[:lastColon], trimmed[lastColon+1:])
	}
	return docker.NewImageRef(trimmed, "")
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
    && apt-get install -y --no-install-recommends bubblewrap ca-certificates git ncurses-term \
    && rm -rf /var/lib/apt/lists/*

RUN getent group "${VALV_GID}" >/dev/null || groupadd -g "${VALV_GID}" valv \
    && useradd -o -m -u "${VALV_UID}" -g "${VALV_GID}" -s /bin/sh valv \
    && mkdir -p /home/valv/.codex /workspace \
    && chown -R "${VALV_UID}:${VALV_GID}" /home/valv /workspace

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

// WriteDefaultClaudeContext writes the default Claude build context (a single
// Dockerfile) under root and returns the absolute Dockerfile path. Mirrors
// WriteDefaultCodexContext — same directory permissions, same file permissions,
// same error wrapping shape.
func WriteDefaultClaudeContext(root string) (string, error) {
	contextDir := strings.TrimSpace(root)
	if contextDir == "" {
		return "", fmt.Errorf("write default claude context: root is required")
	}
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return "", fmt.Errorf("write default claude context: ensure context dir %q: %w", contextDir, err)
	}
	dockerfilePath := filepath.Join(contextDir, defaultCodexDockerfile)
	if err := os.WriteFile(dockerfilePath, []byte(DefaultClaudeDockerfile()), 0o644); err != nil {
		return "", fmt.Errorf("write default claude context: write dockerfile %q: %w", dockerfilePath, err)
	}
	return dockerfilePath, nil
}

// DefaultClaudeDockerfile returns the default Claude provider Dockerfile text.
// The recipe mirrors DefaultCodexDockerfile — same base image, same apt
// packages, same valv user creation, same NPM_CONFIG env — and swaps in
// Claude-specific bits: the @anthropic-ai/claude-code npm install, a
// /home/valv/.claude config dir, a CLAUDE_CONFIG_DIR env var, a CLAUDE_VERSION
// build arg, and a `claude` entrypoint.
func DefaultClaudeDockerfile() string {
	return strings.TrimSpace(`
FROM node:22-bookworm-slim

ARG VALV_UID=1000
ARG VALV_GID=1000

RUN apt-get update \
    && apt-get install -y --no-install-recommends bubblewrap ca-certificates git ncurses-term \
    && rm -rf /var/lib/apt/lists/*

RUN getent group "${VALV_GID}" >/dev/null || groupadd -g "${VALV_GID}" valv \
    && useradd -o -m -u "${VALV_UID}" -g "${VALV_GID}" -s /bin/sh valv \
    && mkdir -p /home/valv/.claude /workspace \
    && chown -R "${VALV_UID}:${VALV_GID}" /home/valv /workspace

ENV NPM_CONFIG_UPDATE_NOTIFIER=false \
    NPM_CONFIG_FUND=false \
    NPM_CONFIG_AUDIT=false \
    HOME=/home/valv \
    LOGNAME=valv \
    USER=valv \
    CLAUDE_CONFIG_DIR=/home/valv/.claude

ARG CLAUDE_VERSION
RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"

USER valv
WORKDIR /workspace
ENTRYPOINT ["claude"]
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

func dockerImageMissingError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such image") ||
		strings.Contains(message, "no such object") ||
		strings.Contains(message, "pull access denied")
}

func isMissingImageError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such image") ||
		strings.Contains(message, "image not known")
}
