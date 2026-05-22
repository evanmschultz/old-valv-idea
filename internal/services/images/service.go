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
	"github.com/evanmschultz/valv/internal/tools"
)

const (
	defaultCodexDockerfile   = "Dockerfile"
	defaultCodexLatestURL    = "https://api.github.com/repos/openai/codex/releases/latest"
	defaultClaudeLatestURL   = "https://registry.npmjs.org/@anthropic-ai/claude-code/latest"
	defaultVersionRequestTTL = 10 * time.Second
	recipeHashLabel          = "io.valv.recipe_hash"

	// DROP_12 per-project overlay image constants.
	tagPrefixProjectOverlay  = "proj-"
	toolsHashLabel           = "io.valv.tools_hash"
	baseRecipeHashLabel      = "io.valv.base_recipe_hash"
	managedLabel             = "io.valv.managed"
	scopeLabel               = "io.valv.scope"
	scopeValueProjectOverlay = "project-overlay"
)

// errLabelUnreadable is returned by inspectLabel when the underlying runner
// does not implement outputRunner (label capture impossible without it). Per
// PLAN.md decision 5 the caller (EnsureProjectImage) treats this as a
// freshness-label mismatch and forces a rebuild — the conservative-opposite
// of imageRecipeMatches's safe-skip behavior, because for overlays an
// unreadable label means we cannot prove freshness.
var errLabelUnreadable = errors.New("inspect image label: runner does not support output capture")

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
	// CachePath is the path to the on-disk version cache file.  If empty,
	// the platform-default cache path is used (see defaultCachePath).
	CachePath string
	// Clock overrides time.Now for deterministic testing.  If nil, time.Now
	// is used.
	Clock func() time.Time
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
	cachePath  string
	clock      func() time.Time
}

type BuildRequest struct {
	Version string
	// CrossProviderVersion is the version of the OTHER provider CLI to install in
	// the image (e.g. the claude version when building a codex image, and vice
	// versa). When empty, "latest" is used so the cross-provider npm install
	// always succeeds even when the caller does not pin the secondary version.
	CrossProviderVersion string
	Pull                 bool
	NoCache              bool
	ExtraTags            []docker.ImageRef
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

// EnsureProjectRequest is the input to Service.EnsureProjectImage. Manifest
// drives both the overlay tools-hash and the per-tool RUN lines emitted by
// BuildOverlayDockerfile. BaseImage is the already-resolved provider image
// (e.g. valv-codex:dev) that the overlay layers on top of. NoCache forces a
// rebuild even when freshness labels match. There is intentionally no Pull
// field — base images are locally built, never pulled from a registry (F3).
type EnsureProjectRequest struct {
	Manifest  tools.ToolManifest
	BaseImage docker.ImageRef
	NoCache   bool
}

// EnsureProjectResult is the output of Service.EnsureProjectImage. Image is
// the resolved per-project tag (or the base image when the manifest has no
// tools). Action reports whether the overlay was built, found up-to-date, or
// the base image was used directly. ToolsHash is the full sha256 hex digest
// from OverlayHash; Unit 12.4's CLI caller can log the unambiguous unprefixed
// value alongside the resolved tag.
type EnsureProjectResult struct {
	Image     docker.ImageRef
	Action    EnsureAction
	ToolsHash string
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

type claudeNPMPayload struct {
	Version string `json:"version"`
}

type claudeVersionResolver struct {
	client *http.Client
	url    string
}

// NewClaudeVersionResolver returns a VersionResolver that queries the npm
// registry for the latest @anthropic-ai/claude-code version. If client is
// nil, a default client with a 10-second timeout is used.
func NewClaudeVersionResolver(client *http.Client) VersionResolver {
	if client == nil {
		client = &http.Client{Timeout: defaultVersionRequestTTL}
	}
	return claudeVersionResolver{client: client, url: defaultClaudeLatestURL}
}

func (r claudeVersionResolver) LatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return "", fmt.Errorf("latest claude version: new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "valv")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("latest claude version: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("latest claude version: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload claudeNPMPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("latest claude version: decode response: %w", err)
	}
	version := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(payload.Version), "v"))
	if version == "" || !versionPattern.MatchString(version) {
		return "", fmt.Errorf("latest claude version: no valid version in npm response")
	}
	return versionPattern.FindString(version), nil
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
	if resolver == nil && provider == domain.ProviderClaude {
		resolver = NewClaudeVersionResolver(nil)
	}
	cachePath := strings.TrimSpace(options.CachePath)
	if cachePath == "" {
		cachePath = defaultCachePath()
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
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
		cachePath:  cachePath,
		clock:      clock,
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

	crossVersion := strings.TrimSpace(request.CrossProviderVersion)
	if crossVersion == "" {
		crossVersion = "latest"
	}
	buildRequest := docker.ImageBuildRequest{
		ContextDir: s.contextDir,
		Dockerfile: filepath.Join(s.contextDir, s.dockerfile),
		Tags:       tags,
		Builder:    "auto",
		BuildArgs: map[string]string{
			s.providerVersionBuildArg():      version,
			s.crossProviderVersionBuildArg(): crossVersion,
			"VALV_GID":                       fmt.Sprintf("%d", s.groupID),
			"VALV_UID":                       fmt.Sprintf("%d", s.userID),
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

	now := s.clock()
	cacheData := readVersionCache(s.cachePath)
	latestVersion, cachedCheckedAt, fromCache := cachedVersion(cacheData, s.provider, now)
	if !fromCache {
		latestVersion, err = s.resolver.LatestVersion(ctx)
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
		if writeErr := writeVersionCache(s.cachePath, latestVersion, s.provider, now); writeErr != nil {
			s.debug("version cache write failed", "error", writeErr)
		}
	}

	// On a cache hit, report when the version was actually last checked (the
	// cached timestamp).  On a fresh resolver call, report the current time.
	var checkedAt time.Time
	if fromCache {
		checkedAt = cachedCheckedAt
	} else {
		checkedAt = now.UTC()
	}
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
		state.UpdatedAt = s.clock().UTC()
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
		UpdatedAt:           s.clock().UTC(),
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

// crossProviderVersionBuildArg returns the Docker build-arg name for the OTHER
// provider's CLI version pin. Both Dockerfiles install both CLIs, so a build-arg
// for the secondary CLI must always be present. Codex image needs CLAUDE_VERSION;
// Claude image needs CODEX_VERSION.
func (s Service) crossProviderVersionBuildArg() string {
	if s.provider == domain.ProviderClaude {
		return "CODEX_VERSION"
	}
	return "CLAUDE_VERSION"
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

// projectImageRef returns the docker.ImageRef for the per-project overlay
// image keyed by the given full tools hash. The tag is `proj-<short>` where
// <short> is the first 12 hex chars of the supplied OverlayHash result (the
// caller is responsible for computing the hash so projectImageRef never
// re-hashes). The repository segment matches the base image so all of a
// project's images live under the same `valv-<provider>` repo.
func (s Service) projectImageRef(toolsHash string) docker.ImageRef {
	return docker.NewImageRef(s.repository, tagPrefixProjectOverlay+shortOverlayHash(toolsHash))
}

// inspectLabel reads a single label from a docker image via `docker image
// inspect --format '{{ index .Config.Labels "<label>" }}'`. It reuses the
// outputRunner typecast pattern from imageRecipeMatches but returns the raw
// trimmed label value rather than a match-bool. Three error modes:
//
//   - runner does not implement outputRunner: returns errLabelUnreadable. The
//     caller (EnsureProjectImage) treats this as a freshness mismatch and
//     rebuilds — PLAN.md decision 5 conservative-opposite policy.
//   - docker image missing: returns dockerImageMissingError-wrapped error so
//     the caller can detect missing-image via dockerImageMissingError and
//     skip straight to build.
//   - any other inspect error: returned wrapped; the caller treats it as a
//     mismatch and rebuilds.
//
// The empty-output case (label not set on the image) returns the empty string
// with nil error — the caller compares against the expected value and treats
// "" != expected as a mismatch.
func (s Service) inspectLabel(ctx context.Context, ref docker.ImageRef, label string) (string, error) {
	runner, ok := s.runner.(outputRunner)
	if !ok {
		return "", errLabelUnreadable
	}
	output, err := runner.Output(ctx, []string{"image", "inspect", "--format", "{{ index .Config.Labels \"" + label + "\" }}", ref.String()})
	if err != nil {
		if dockerImageMissingError(err) {
			return "", fmt.Errorf("inspect image %q label %q: %w", ref.String(), label, err)
		}
		return "", fmt.Errorf("inspect image %q label %q: %w", ref.String(), label, err)
	}
	return strings.TrimSpace(output), nil
}

// EnsureProjectImage resolves the per-project overlay image for the supplied
// tools manifest, building it on top of request.BaseImage when missing or
// stale. The freshness contract is a three-label set on the per-project
// image:
//
//   - recipeHashLabel       — sha256 of overlay Dockerfile content (forces
//     rebuild when BuildOverlayDockerfile's output drifts).
//   - baseRecipeHashLabel   — verbatim copy of the base image's recipe-hash
//     label, captured at build time. The label value is already a sha256
//     hex string per PLAN.md decision 4; it is NOT re-hashed.
//   - toolsHashLabel        — full OverlayHash digest of the canonical
//     manifest (forces rebuild when manifest content changes).
//
// Any mismatch — or any unreadable label (typecast failure / non-missing
// inspect error) per PLAN.md decision 5 — triggers a rebuild. When
// request.Manifest has zero tools, EnsureProjectImage returns the base ref
// untouched with EnsureActionUsingExistingImage and performs zero docker
// calls.
//
// The returned EnsureProjectResult.Image is the resolved tag (either the
// freshly-built `valv-<provider>:proj-<short>` or, for the empty-manifest
// case, request.BaseImage). ToolsHash carries the full digest so the CLI
// caller (Unit 12.4) can log it unambiguously alongside the resolved tag.
func (s Service) EnsureProjectImage(ctx context.Context, request EnsureProjectRequest) (EnsureProjectResult, error) {
	// Step 1: empty-manifest short-circuit. No docker calls, no overlay
	// build, no per-project tag.
	if len(request.Manifest.Tools) == 0 {
		return EnsureProjectResult{
			Image:  request.BaseImage,
			Action: EnsureActionUsingExistingImage,
		}, nil
	}

	// Step 2: compute expected freshness values up front. The overlay
	// dockerfile content is generated here once so we know (a) the exact
	// recipe-hash an up-to-date image would carry and (b) we have a byte-
	// identical payload ready for the rebuild path. Generating the overlay
	// also surfaces manifest-shape errors (string-form / unsupported verb /
	// empty source-install) before we touch docker.
	toolsHash := OverlayHash(request.Manifest)
	dockerfileContent, err := BuildOverlayDockerfile(request.Manifest, request.BaseImage)
	if err != nil {
		return EnsureProjectResult{}, fmt.Errorf("ensure project image: %w", err)
	}
	expectedRecipeHash := sha256Hex(dockerfileContent)

	// Step 2b: capture the base image's recipe-hash label VERBATIM (no
	// re-hashing — the label is already a sha256 hex string per PLAN.md
	// decision 4 + Notes line 312). If the base image is genuinely missing
	// we cannot build a sensible overlay on top of it, so surface the error.
	// Per PLAN.md decision 5 (clarified Unit 12.3 Round 2): ANY other
	// base-inspect failure — typecast (errLabelUnreadable) OR a generic
	// non-missing inspect error (e.g. "permission denied", "dockerd is not
	// responding") — is treated as a label-read miss. We set
	// baseRecipeHash = "" and fall through to the rebuild path. The empty
	// value flows into the rebuild path's label set and the next launch
	// (with a healthy daemon) sees a hash mismatch and rebuilds — safe-but-
	// wasteful, the conservative-opposite of imageRecipeMatches's base-image
	// safe-skip behavior. Only dockerImageMissingError on the BASE image
	// remains fatal: there is no overlay to build on top of a non-existent
	// base.
	baseRecipeHash, baseErr := s.inspectLabel(ctx, request.BaseImage, recipeHashLabel)
	if baseErr != nil {
		if dockerImageMissingError(baseErr) {
			return EnsureProjectResult{}, fmt.Errorf("ensure project image: base image %q missing: %w", request.BaseImage.String(), baseErr)
		}
		s.debug("base recipe hash unreadable; falling through to rebuild", "base", request.BaseImage.String(), "err", baseErr)
		baseRecipeHash = ""
	}

	// Step 3: construct the target tag from the tools hash.
	targetRef := s.projectImageRef(toolsHash)

	// Step 4: freshness probe. NoCache forces rebuild even on a perfect
	// label match. Otherwise compare all three labels; any mismatch or
	// read failure means rebuild.
	rebuild := request.NoCache
	if !rebuild {
		rebuild = s.projectImageNeedsBuild(ctx, targetRef, expectedRecipeHash, toolsHash, baseRecipeHash)
	}

	if !rebuild {
		return EnsureProjectResult{
			Image:     targetRef,
			Action:    EnsureActionUpToDate,
			ToolsHash: toolsHash,
		}, nil
	}

	// Step 5: rebuild path. Write the already-generated dockerfile content
	// to an ephemeral build context (cleaned up on return), invoke
	// docker.BuildImageArgs with the five labels (three freshness + managed
	// + scope) per PLAN.md decision 4.
	tempDir, err := os.MkdirTemp("", "valv-overlay-*")
	if err != nil {
		return EnsureProjectResult{}, fmt.Errorf("ensure project image: create build context: %w", err)
	}
	defer os.RemoveAll(tempDir)

	dockerfilePath := filepath.Join(tempDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0o644); err != nil {
		return EnsureProjectResult{}, fmt.Errorf("ensure project image: write overlay dockerfile: %w", err)
	}

	buildRequest := docker.ImageBuildRequest{
		ContextDir: tempDir,
		Dockerfile: dockerfilePath,
		Tags:       []docker.ImageRef{targetRef},
		Builder:    "auto",
		Labels: map[string]string{
			recipeHashLabel:     expectedRecipeHash,
			baseRecipeHashLabel: baseRecipeHash,
			toolsHashLabel:      toolsHash,
			managedLabel:        "true",
			scopeLabel:          scopeValueProjectOverlay,
		},
		NoCache: request.NoCache,
	}
	args, err := docker.BuildImageArgs(buildRequest)
	if err != nil {
		return EnsureProjectResult{}, fmt.Errorf("ensure project image: %w", err)
	}
	if err := s.runner.Run(ctx, args); err != nil {
		if isBuildxUnavailable(err) {
			s.debug("docker buildx unavailable for project overlay, falling back to legacy build", "tag", targetRef.String())
			buildRequest.Builder = "legacy"
			fallbackArgs, fallbackErr := docker.BuildImageArgs(buildRequest)
			if fallbackErr != nil {
				return EnsureProjectResult{}, fmt.Errorf("ensure project image fallback: %w", fallbackErr)
			}
			if err := s.runner.Run(ctx, fallbackArgs); err != nil {
				return EnsureProjectResult{}, fmt.Errorf("ensure project image fallback: %w", err)
			}
		} else {
			return EnsureProjectResult{}, fmt.Errorf("ensure project image: %w", err)
		}
	}

	s.debug("built project overlay image", "tag", targetRef.String(), "tools_hash", toolsHash, "base_recipe_hash", baseRecipeHash)
	return EnsureProjectResult{
		Image:     targetRef,
		Action:    EnsureActionUpdated,
		ToolsHash: toolsHash,
	}, nil
}

// projectImageNeedsBuild returns true when the per-project image at targetRef
// is missing, has any freshness label that mismatches the expected values,
// or has unreadable labels (per PLAN.md decision 5 conservative-opposite
// policy). It deliberately swallows inspect errors and returns the rebuild
// decision because the caller cannot meaningfully recover from a freshness
// probe failure — the only sane response is to rebuild.
func (s Service) projectImageNeedsBuild(ctx context.Context, targetRef docker.ImageRef, expectedRecipeHash, expectedToolsHash, expectedBaseRecipeHash string) bool {
	// recipe_hash probe. dockerImageMissing → build; any other read failure
	// → conservative rebuild.
	gotRecipe, err := s.inspectLabel(ctx, targetRef, recipeHashLabel)
	if err != nil {
		return true
	}
	if gotRecipe != expectedRecipeHash {
		return true
	}

	gotTools, err := s.inspectLabel(ctx, targetRef, toolsHashLabel)
	if err != nil {
		return true
	}
	if gotTools != expectedToolsHash {
		return true
	}

	gotBase, err := s.inspectLabel(ctx, targetRef, baseRecipeHashLabel)
	if err != nil {
		return true
	}
	if gotBase != expectedBaseRecipeHash {
		return true
	}

	return false
}

// sha256Hex computes the sha256 hex digest of s — small helper that mirrors
// recipeHash's existing pattern without exposing crypto/sha256 at the call
// site.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
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

// goInstallStep is the shared Go 1.26.1 install snippet embedded into both
// DefaultCodexDockerfile and DefaultClaudeDockerfile so DROP_12 overlay layers
// can run `go install <source>` against a tools.toml manifest. Shell-form RUN
// is used because the multi-step pipeline (per-arch case dispatch + curl +
// sha256 verify + tar extract + cleanup) requires shell control flow and
// ${TARGETARCH} variable expansion. Per Docker BuildKit docs the automatic
// platform ARG TARGETARCH lives in global scope only and is NOT auto-injected
// into build stages — the explicit `ARG TARGETARCH` redeclaration immediately
// before this RUN is mandatory. The literal sha256 hex values for amd64 and
// arm64 are fetched from https://go.dev/dl/?mode=json and verified at build
// time via `sha256sum -c`; see drops/DROP_12_IMAGE_LAYERING/BUILDER_WORKLOG.md
// § "Go Tarball Hashes" for the source of truth.
const goInstallStep = `ARG TARGETARCH
RUN set -eu \
    && case "${TARGETARCH}" in \
        amd64) GO_SHA256=031f088e5d955bab8657ede27ad4e3bc5b7c1ba281f05f245bcc304f327c987a ;; \
        arm64) GO_SHA256=a290581cfe4fe28ddd737dde3095f3dbeb7f2e4065cab4eae44dfc53b760c2f7 ;; \
        *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl -fSL "https://go.dev/dl/go1.26.1.linux-${TARGETARCH}.tar.gz" -o /tmp/go.tar.gz \
    && echo "${GO_SHA256}  /tmp/go.tar.gz" | sha256sum -c - \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz
ENV PATH=/usr/local/go/bin:$PATH`

func DefaultCodexDockerfile() string {
	return strings.TrimSpace(`
FROM node:22-bookworm-slim

ARG VALV_UID=1000
ARG VALV_GID=1000

RUN apt-get update \
    && apt-get install -y --no-install-recommends bubblewrap ca-certificates curl git ncurses-term \
    && rm -rf /var/lib/apt/lists/*

`+goInstallStep+`

RUN getent group "${VALV_GID}" >/dev/null || groupadd -g "${VALV_GID}" valv \
    && useradd -o -m -u "${VALV_UID}" -g "${VALV_GID}" -s /bin/sh valv \
    && mkdir -p /home/valv/.codex /home/valv/.claude /workspace \
    && chown -R "${VALV_UID}:${VALV_GID}" /home/valv /workspace

ENV NPM_CONFIG_UPDATE_NOTIFIER=false \
    NPM_CONFIG_FUND=false \
    NPM_CONFIG_AUDIT=false \
    HOME=/home/valv \
    LOGNAME=valv \
    USER=valv

ARG CODEX_VERSION
RUN npm install --global "@openai/codex@${CODEX_VERSION}"

ARG CLAUDE_VERSION
RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"

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
// build arg, and a `claude` entrypoint. Both CLIs (@openai/codex and
// @anthropic-ai/claude-code) are installed so that a Claude Code agent running
// inside this container can invoke `codex exec` directly.
func DefaultClaudeDockerfile() string {
	return strings.TrimSpace(`
FROM node:22-bookworm-slim

ARG VALV_UID=1000
ARG VALV_GID=1000

RUN apt-get update \
    && apt-get install -y --no-install-recommends bubblewrap ca-certificates curl git ncurses-term \
    && rm -rf /var/lib/apt/lists/*

`+goInstallStep+`

RUN getent group "${VALV_GID}" >/dev/null || groupadd -g "${VALV_GID}" valv \
    && useradd -o -m -u "${VALV_UID}" -g "${VALV_GID}" -s /bin/sh valv \
    && mkdir -p /home/valv/.claude /home/valv/.codex /workspace \
    && chown -R "${VALV_UID}:${VALV_GID}" /home/valv /workspace

ENV NPM_CONFIG_UPDATE_NOTIFIER=false \
    NPM_CONFIG_FUND=false \
    NPM_CONFIG_AUDIT=false \
    HOME=/home/valv \
    LOGNAME=valv \
    USER=valv \
    CLAUDE_CONFIG_DIR=/home/valv/.claude \
    CODEX_HOME=/home/valv/.codex

ARG CLAUDE_VERSION
RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"

ARG CODEX_VERSION
RUN npm install --global "@openai/codex@${CODEX_VERSION}"

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
