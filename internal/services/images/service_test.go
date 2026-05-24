package images

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/tools"
)

type runnerRecorder struct {
	calls [][]string
	errs  map[string]error
	outs  map[string]string
}

func (r *runnerRecorder) Run(_ context.Context, args []string) error {
	r.calls = append(r.calls, append([]string(nil), args...))
	if r.errs != nil {
		if err := r.errs[strings.Join(args, " ")]; err != nil {
			return err
		}
	}
	return nil
}

func (r *runnerRecorder) Output(_ context.Context, args []string) (string, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	key := strings.Join(args, " ")
	if r.errs != nil {
		if err := r.errs[key]; err != nil {
			return "", err
		}
	}
	if r.outs != nil {
		if out, ok := r.outs[key]; ok {
			return out, nil
		}
	}
	return "", nil
}

type providerImageStateStore struct {
	state domain.ProviderImageState
	ok    bool
}

func (s *providerImageStateStore) ProviderImageState(_ context.Context, provider domain.Provider) (domain.ProviderImageState, error) {
	if !s.ok || s.state.Provider != provider {
		return domain.ProviderImageState{}, domain.ErrNotFound
	}
	return s.state, nil
}

func (s *providerImageStateStore) UpsertProviderImageState(_ context.Context, state domain.ProviderImageState) (domain.ProviderImageState, error) {
	s.state = state
	s.ok = true
	return state, nil
}

type staticResolver string

func (r staticResolver) LatestVersion(context.Context) (string, error) {
	return string(r), nil
}

type errResolver struct{ err error }

func (r errResolver) LatestVersion(context.Context) (string, error) { return "", r.err }

func TestServiceBuildAddsVersionAndUsesDefaultImageInfo(t *testing.T) {
	runner := &runnerRecorder{}
	uid := os.Getuid()
	gid := os.Getgid()
	svc, err := New(Options{
		Runner:     runner,
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		Dockerfile: "Dockerfile",
		DefaultTag: "dev",
		UserID:     uid,
		GroupID:    gid,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{Version: "0.117.0"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if got, want := result.Image.String(), "ghcr.io/valv/codex:dev"; got != want {
		t.Fatalf("Build() image = %q, want %q", got, want)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner call count = %d, want 1", len(runner.calls))
	}
	// Alphabetic build-arg order (sort.Strings on map keys):
	// CLAUDE_VERSION < CODEX_VERSION < VALV_GID < VALV_UID
	want := []string{
		"buildx", "build", "--load",
		"-f", "/tmp/codex-image/Dockerfile",
		"-t", "ghcr.io/valv/codex:dev",
		"--build-arg", "CLAUDE_VERSION=latest",
		"--build-arg", "CODEX_VERSION=0.117.0",
		"--build-arg", fmt.Sprintf("VALV_GID=%d", gid),
		"--build-arg", fmt.Sprintf("VALV_UID=%d", uid),
		"--label", "io.valv.managed=true",
		"--label", "io.valv.provider=codex",
		"--label", fmt.Sprintf("%s=%s", recipeHashLabel, svc.recipeHash()),
		"--label", "io.valv.scope=image",
		"--label", "io.valv.version=0.117.0",
		"/tmp/codex-image",
	}
	if !reflect.DeepEqual(runner.calls[0], want) {
		t.Fatalf("Build() args = %#v, want %#v", runner.calls[0], want)
	}
}

func TestEnsureLatestBuildsAndPersistsStateWhenImageIsMissing(t *testing.T) {
	oldFindDockerBinary := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFindDockerBinary })

	runner := &runnerRecorder{
		errs: map[string]error{
			"image inspect ghcr.io/valv/codex:dev": fmt.Errorf("run docker image inspect ghcr.io/valv/codex:dev: No such image"),
		},
	}
	stateStore := &providerImageStateStore{}
	svc, err := New(Options{
		Runner:     runner,
		StateStore: stateStore,
		Resolver:   staticResolver("0.117.0"),
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		CachePath:  filepath.Join(t.TempDir(), "version-cache.json"),
		UserID:     1000,
		GroupID:    1000,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}
	if got, want := result.Action, EnsureActionUpdated; got != want {
		t.Fatalf("EnsureLatest() action = %q, want %q", got, want)
	}
	if got, want := stateStore.state.InstalledVersion, "0.117.0"; got != want {
		t.Fatalf("installed version = %q, want %q", got, want)
	}
	if got, want := stateStore.state.InstalledVersionTag, "ghcr.io/valv/codex:0-117-0"; got != want {
		t.Fatalf("installed version tag = %q, want %q", got, want)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner call count = %d, want 2", len(runner.calls))
	}
}

func TestEnsureLatestSkipsBuildWhenStateIsCurrent(t *testing.T) {
	oldFindDockerBinary := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFindDockerBinary })

	stateStore := &providerImageStateStore{
		state: domain.ProviderImageState{
			Provider:            domain.ProviderCodex,
			LatestVersion:       "0.117.0",
			LatestCheckedAt:     time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
			InstalledVersion:    "0.117.0",
			InstalledImageRef:   "ghcr.io/valv/codex:dev",
			InstalledVersionTag: "ghcr.io/valv/codex:0-117-0",
			UpdatedAt:           time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
		},
		ok: true,
	}
	runner := &runnerRecorder{
		outs: map[string]string{
			`image inspect --format {{ index .Config.Labels "` + recipeHashLabel + `" }} ghcr.io/valv/codex:dev`: svcRecipeHashForTest("/tmp/codex-image", "Dockerfile"),
		},
	}
	svc, err := New(Options{
		Runner:     runner,
		StateStore: stateStore,
		Resolver:   staticResolver("0.117.0"),
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		CachePath:  filepath.Join(t.TempDir(), "version-cache.json"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}
	if got, want := result.Action, EnsureActionUpToDate; got != want {
		t.Fatalf("EnsureLatest() action = %q, want %q", got, want)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner call count = %d, want 2", len(runner.calls))
	}
	if got, want := runner.calls[0], []string{"image", "inspect", "ghcr.io/valv/codex:dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inspect args = %#v, want %#v", got, want)
	}
	if got, want := runner.calls[1], []string{"image", "inspect", "--format", "{{ index .Config.Labels \"" + recipeHashLabel + "\" }}", "ghcr.io/valv/codex:dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recipe inspect args = %#v, want %#v", got, want)
	}
}

func TestEnsureLatestRebuildsWhenRecipeHashDiffers(t *testing.T) {
	oldFindDockerBinary := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFindDockerBinary })

	stateStore := &providerImageStateStore{
		state: domain.ProviderImageState{
			Provider:            domain.ProviderCodex,
			LatestVersion:       "0.117.0",
			LatestCheckedAt:     time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
			InstalledVersion:    "0.117.0",
			InstalledImageRef:   "ghcr.io/valv/codex:dev",
			InstalledVersionTag: "ghcr.io/valv/codex:0-117-0",
			UpdatedAt:           time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
		},
		ok: true,
	}
	runner := &runnerRecorder{
		outs: map[string]string{
			`image inspect --format {{ index .Config.Labels "` + recipeHashLabel + `" }} ghcr.io/valv/codex:dev`: "stale-recipe",
		},
	}
	svc, err := New(Options{
		Runner:     runner,
		StateStore: stateStore,
		Resolver:   staticResolver("0.117.0"),
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		CachePath:  filepath.Join(t.TempDir(), "version-cache.json"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}
	if got, want := result.Action, EnsureActionUpdated; got != want {
		t.Fatalf("EnsureLatest() action = %q, want %q", got, want)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("runner call count = %d, want 3", len(runner.calls))
	}
	if got, want := runner.calls[1], []string{"image", "inspect", "--format", "{{ index .Config.Labels \"" + recipeHashLabel + "\" }}", "ghcr.io/valv/codex:dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recipe inspect args = %#v, want %#v", got, want)
	}
	if got := runner.calls[2]; len(got) < 2 || got[0] != "buildx" || got[1] != "build" {
		t.Fatalf("build args = %#v, want docker build invocation", got)
	}
}

func TestEnsureLatestFallsBackToExistingImageWhenVersionCheckFails(t *testing.T) {
	oldFindDockerBinary := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFindDockerBinary })

	stateStore := &providerImageStateStore{
		state: domain.ProviderImageState{
			Provider:            domain.ProviderCodex,
			LatestVersion:       "0.117.0",
			LatestCheckedAt:     time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
			InstalledVersion:    "0.117.0",
			InstalledImageRef:   "ghcr.io/valv/codex:dev",
			InstalledVersionTag: "ghcr.io/valv/codex:0-117-0",
			UpdatedAt:           time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
		},
		ok: true,
	}
	runner := &runnerRecorder{}
	svc, err := New(Options{
		Runner:     runner,
		StateStore: stateStore,
		Resolver:   errResolver{err: fmt.Errorf("boom")},
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		CachePath:  filepath.Join(t.TempDir(), "version-cache.json"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{AllowExistingOnCheckFail: true})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}
	if got, want := result.Action, EnsureActionUsingExistingImage; got != want {
		t.Fatalf("EnsureLatest() action = %q, want %q", got, want)
	}
}

func TestEnsureLatestRemovesPreviousVersionTagWhenUpdating(t *testing.T) {
	oldFindDockerBinary := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFindDockerBinary })

	stateStore := &providerImageStateStore{
		state: domain.ProviderImageState{
			Provider:            domain.ProviderCodex,
			LatestVersion:       "0.116.0",
			LatestCheckedAt:     time.Now().UTC(),
			InstalledVersion:    "0.116.0",
			InstalledImageRef:   "ghcr.io/valv/codex:dev",
			InstalledVersionTag: "ghcr.io/valv/codex:0-116-0",
			UpdatedAt:           time.Now().UTC(),
		},
		ok: true,
	}
	runner := &runnerRecorder{}
	svc, err := New(Options{
		Runner:     runner,
		StateStore: stateStore,
		Resolver:   staticResolver("0.117.0"),
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		CachePath:  filepath.Join(t.TempDir(), "version-cache.json"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}
	if got, want := result.PreviousVersion, "0.116.0"; got != want {
		t.Fatalf("previous version = %q, want %q", got, want)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("runner call count = %d, want 4", len(runner.calls))
	}
	if got, want := runner.calls[1], []string{"image", "inspect", "--format", "{{ index .Config.Labels \"" + recipeHashLabel + "\" }}", "ghcr.io/valv/codex:dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recipe inspect args = %#v, want %#v", got, want)
	}
	if got, want := runner.calls[3], []string{"image", "rm", "--force", "ghcr.io/valv/codex:0-116-0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("image remove args = %#v, want %#v", got, want)
	}
}

func TestNewRequiresRunnerAndRepository(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("New() error = nil, want dependency failure")
	}

	_, err := New(Options{Runner: &runnerRecorder{}, ContextDir: "/tmp/context"})
	if err == nil {
		t.Fatal("New() error = nil, want repository failure")
	}
}

func TestWriteDefaultCodexContextWritesDockerfile(t *testing.T) {
	root := t.TempDir()
	dockerfilePath, err := WriteDefaultCodexContext(root)
	if err != nil {
		t.Fatalf("WriteDefaultCodexContext() error = %v", err)
	}
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		// Primary CLI (codex)
		`@openai/codex@${CODEX_VERSION}`,
		// Cross-provider CLI (claude) — installed in the same image
		`@anthropic-ai/claude-code@${CLAUDE_VERSION}`,
		// Both config dirs are created at image build time
		`/home/valv/.codex`,
		`/home/valv/.claude`,
		// Standard env / user setup
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
		`ARG VALV_UID=1000`,
		`ARG VALV_GID=1000`,
		`apt-get install -y --no-install-recommends bubblewrap ca-certificates curl git ncurses-term`,
		`getent group "${VALV_GID}" >/dev/null || groupadd -g "${VALV_GID}" valv`,
		`useradd -o -m -u "${VALV_UID}" -g "${VALV_GID}" -s /bin/sh valv`,
		`chown -R "${VALV_UID}:${VALV_GID}" /home/valv /workspace`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("dockerfile missing %q: %q", want, string(content))
		}
	}
	if filepath.Base(dockerfilePath) != "Dockerfile" {
		t.Fatalf("dockerfile base = %q, want Dockerfile", filepath.Base(dockerfilePath))
	}
}

func TestBuildIncludesExtraTags(t *testing.T) {
	runner := &runnerRecorder{}
	uid := os.Getuid()
	gid := os.Getgid()
	svc, err := New(Options{Runner: runner, Repository: "ghcr.io/valv/codex", ContextDir: "/tmp/codex-image", UserID: uid, GroupID: gid})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{
		Version: "0.117.0",
		ExtraTags: []docker.ImageRef{
			docker.NewImageRef("ghcr.io/valv/codex", "0-117-0"),
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Tags) != 2 {
		t.Fatalf("result tags len = %d, want 2", len(result.Tags))
	}
	// Alphabetic build-arg order: CLAUDE_VERSION < CODEX_VERSION < VALV_GID < VALV_UID
	wantArgs := []string{
		"buildx", "build", "--load",
		"-f", "/tmp/codex-image/Dockerfile",
		"-t", "ghcr.io/valv/codex:dev",
		"-t", "ghcr.io/valv/codex:0-117-0",
		"--build-arg", "CLAUDE_VERSION=latest",
		"--build-arg", "CODEX_VERSION=0.117.0",
		"--build-arg", fmt.Sprintf("VALV_GID=%d", gid),
		"--build-arg", fmt.Sprintf("VALV_UID=%d", uid),
		"--label", "io.valv.managed=true",
		"--label", "io.valv.provider=codex",
		"--label", fmt.Sprintf("%s=%s", recipeHashLabel, svc.recipeHash()),
		"--label", "io.valv.scope=image",
		"--label", "io.valv.version=0.117.0",
		"/tmp/codex-image",
	}
	if !reflect.DeepEqual(runner.calls[0], wantArgs) {
		t.Fatalf("Build() args = %#v, want %#v", runner.calls[0], wantArgs)
	}
}

func TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable(t *testing.T) {
	uid := os.Getuid()
	gid := os.Getgid()
	recipeHash := svcRecipeHashForTest("/tmp/codex-image", "Dockerfile")
	// Alphabetic build-arg order: CLAUDE_VERSION < CODEX_VERSION < VALV_GID < VALV_UID.
	// The first call (buildx) triggers the error that forces the legacy fallback.
	firstCallParts := []string{
		"buildx", "build", "--load",
		"-f", "/tmp/codex-image/Dockerfile",
		"-t", "ghcr.io/valv/codex:dev",
		"--build-arg", "CLAUDE_VERSION=latest",
		"--build-arg", "CODEX_VERSION=0.117.0",
		"--build-arg", fmt.Sprintf("VALV_GID=%d", gid),
		"--build-arg", fmt.Sprintf("VALV_UID=%d", uid),
		"--label", "io.valv.managed=true",
		"--label", "io.valv.provider=codex",
		"--label", fmt.Sprintf("%s=%s", recipeHashLabel, recipeHash),
		"--label", "io.valv.scope=image",
		"--label", "io.valv.version=0.117.0",
		"/tmp/codex-image",
	}
	firstCall := strings.Join(firstCallParts, " ")
	runner := &runnerRecorder{
		errs: map[string]error{
			firstCall: fmt.Errorf("docker buildx is required but unavailable"),
		},
	}
	svc, err := New(Options{
		Runner:     runner,
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
		Dockerfile: "Dockerfile",
		DefaultTag: "dev",
		UserID:     uid,
		GroupID:    gid,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = svc.Build(context.Background(), BuildRequest{Version: "0.117.0"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner call count = %d, want 2", len(runner.calls))
	}
	// Legacy fallback uses "build" (not "buildx build --load") but same build-args.
	wantFallback := []string{
		"build",
		"-f", "/tmp/codex-image/Dockerfile",
		"-t", "ghcr.io/valv/codex:dev",
		"--build-arg", "CLAUDE_VERSION=latest",
		"--build-arg", "CODEX_VERSION=0.117.0",
		"--build-arg", fmt.Sprintf("VALV_GID=%d", gid),
		"--build-arg", fmt.Sprintf("VALV_UID=%d", uid),
		"--label", "io.valv.managed=true",
		"--label", "io.valv.provider=codex",
		"--label", fmt.Sprintf("%s=%s", recipeHashLabel, recipeHash),
		"--label", "io.valv.scope=image",
		"--label", "io.valv.version=0.117.0",
		"/tmp/codex-image",
	}
	if got := runner.calls[1]; !reflect.DeepEqual(got, wantFallback) {
		t.Fatalf("fallback args = %#v, want %#v", got, wantFallback)
	}
}

func TestCodexVersionResolverReadsLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"rust-v0.117.0","name":"0.117.0"}`))
	}))
	defer server.Close()

	resolver := codexVersionResolver{client: server.Client(), url: server.URL}
	version, err := resolver.LatestVersion(context.Background())
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if got, want := version, "0.117.0"; got != want {
		t.Fatalf("version = %q, want %q", got, want)
	}
}

func svcRecipeHashForTest(contextDir, dockerfile string) string {
	svc, _ := New(Options{
		Runner:     &runnerRecorder{},
		Repository: "ghcr.io/valv/codex",
		ContextDir: contextDir,
		Dockerfile: dockerfile,
	})
	return svc.recipeHash()
}

func TestWriteDefaultClaudeContextWritesDockerfile(t *testing.T) {
	root := t.TempDir()
	dockerfilePath, err := WriteDefaultClaudeContext(root)
	if err != nil {
		t.Fatalf("WriteDefaultClaudeContext() error = %v", err)
	}
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		// Primary CLI (claude)
		`@anthropic-ai/claude-code@${CLAUDE_VERSION}`,
		`CLAUDE_CONFIG_DIR=/home/valv/.claude`,
		`ENTRYPOINT ["claude"]`,
		// Cross-provider CLI (codex) — installed in the same image
		`@openai/codex@${CODEX_VERSION}`,
		// CODEX_HOME env so cross-provider codex calls find their config dir
		`CODEX_HOME=/home/valv/.codex`,
		// Both config dirs are created at image build time
		`/home/valv/.claude`,
		`/home/valv/.codex`,
		// Standard env / user setup
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
		`ARG VALV_UID=1000`,
		`ARG VALV_GID=1000`,
		`apt-get install -y --no-install-recommends bubblewrap ca-certificates curl git ncurses-term`,
		`getent group "${VALV_GID}" >/dev/null || groupadd -g "${VALV_GID}" valv`,
		`useradd -o -m -u "${VALV_UID}" -g "${VALV_GID}" -s /bin/sh valv`,
		`chown -R "${VALV_UID}:${VALV_GID}" /home/valv /workspace`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("dockerfile missing %q: %q", want, string(content))
		}
	}
	if filepath.Base(dockerfilePath) != "Dockerfile" {
		t.Fatalf("dockerfile base = %q, want Dockerfile", filepath.Base(dockerfilePath))
	}
}

// testClaudeCLIVersion is a fixed version string used in tests that need a
// concrete Claude CLI version but do not depend on any pinned-constant export.
const testClaudeCLIVersion = "2.1.143"

// TestDefaultProviderDockerfilesEmbedGoToolchain asserts the literal substrings
// required by DROP_12 Unit 12.0 acceptance: both default Dockerfiles must add
// `curl` to apt, declare `ARG TARGETARCH` inside the build stage, embed the
// go1.26.1 tarball URL with the in-stage ${TARGETARCH} expansion, verify it
// via `sha256sum -c`, extract into /usr/local/go and expose /usr/local/go/bin
// on PATH, and cover both amd64 and arm64 in the per-arch case dispatch. The
// literal sha256 hex values for both architectures are also asserted so a
// version bump cannot accidentally land with stale hashes.
func TestDefaultProviderDockerfilesEmbedGoToolchain(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "codex", body: DefaultCodexDockerfile()},
		{name: "claude", body: DefaultClaudeDockerfile()},
	}

	wantSubstrings := []string{
		"ARG TARGETARCH",
		"go1.26.1.linux-${TARGETARCH}.tar.gz",
		"sha256sum -c",
		"/usr/local/go/bin",
		"amd64",
		"arm64",
		"bubblewrap ca-certificates curl git ncurses-term",
		// Literal sha256 hex values from https://go.dev/dl/?mode=json for
		// go1.26.1 linux-amd64 and linux-arm64. See
		// drops/DROP_12_IMAGE_LAYERING/BUILDER_WORKLOG.md § Go Tarball Hashes.
		"031f088e5d955bab8657ede27ad4e3bc5b7c1ba281f05f245bcc304f327c987a",
		"a290581cfe4fe28ddd737dde3095f3dbeb7f2e4065cab4eae44dfc53b760c2f7",
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, want := range wantSubstrings {
				if !strings.Contains(tc.body, want) {
					t.Errorf("Dockerfile missing %q", want)
				}
			}

			// ARG TARGETARCH must be declared inside the build stage and
			// positioned BEFORE the go install RUN line. We approximate
			// "build stage" as "anywhere after the FROM line" since both
			// Dockerfiles are single-stage.
			fromIdx := strings.Index(tc.body, "FROM ")
			argIdx := strings.Index(tc.body, "ARG TARGETARCH")
			tarballIdx := strings.Index(tc.body, "go1.26.1.linux-${TARGETARCH}.tar.gz")
			if fromIdx < 0 || argIdx < 0 || tarballIdx < 0 {
				t.Fatalf("Dockerfile missing required markers (FROM=%d ARG=%d tarball=%d)", fromIdx, argIdx, tarballIdx)
			}
			if argIdx < fromIdx {
				t.Errorf("ARG TARGETARCH positioned before FROM (arg=%d, from=%d)", argIdx, fromIdx)
			}
			if argIdx > tarballIdx {
				t.Errorf("ARG TARGETARCH must precede the go tarball RUN (arg=%d, tarball=%d)", argIdx, tarballIdx)
			}
		})
	}
}

func TestServiceBuildRecipeHashMatchesProviderDockerfile(t *testing.T) {
	cases := []struct {
		name        string
		provider    domain.Provider
		writeCtx    func(string) (string, error)
		wantContent string
		version     string
	}{
		{
			name:        "codex",
			provider:    domain.ProviderCodex,
			writeCtx:    WriteDefaultCodexContext,
			wantContent: DefaultCodexDockerfile(),
			version:     "0.117.0",
		},
		{
			name:        "claude",
			provider:    domain.ProviderClaude,
			writeCtx:    WriteDefaultClaudeContext,
			wantContent: DefaultClaudeDockerfile(),
			version:     testClaudeCLIVersion,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contextDir := t.TempDir()
			if _, err := tc.writeCtx(contextDir); err != nil {
				t.Fatalf("writeCtx() error = %v", err)
			}
			runner := &runnerRecorder{}
			svc, err := New(Options{
				Runner:     runner,
				Provider:   tc.provider,
				Repository: "ghcr.io/valv/" + tc.name,
				ContextDir: contextDir,
				Dockerfile: "Dockerfile",
				DefaultTag: "dev",
				UserID:     1000,
				GroupID:    1000,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			if _, err := svc.Build(context.Background(), BuildRequest{Version: tc.version}); err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if len(runner.calls) != 1 {
				t.Fatalf("runner call count = %d, want 1", len(runner.calls))
			}

			sum := sha256.Sum256([]byte(tc.wantContent))
			wantLabel := recipeHashLabel + "=" + hex.EncodeToString(sum[:])

			var gotLabel string
			args := runner.calls[0]
			for i := 0; i < len(args)-1; i++ {
				if args[i] == "--label" && strings.HasPrefix(args[i+1], recipeHashLabel+"=") {
					gotLabel = args[i+1]
					break
				}
			}
			if gotLabel == "" {
				t.Fatalf("buildx args missing %s label: %#v", recipeHashLabel, args)
			}
			if gotLabel != wantLabel {
				t.Fatalf("recipe hash label = %q, want %q", gotLabel, wantLabel)
			}
		})
	}
}

func TestClaudeVersionResolverReadsLatestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"version":"2.1.200"}`))
	}))
	defer server.Close()

	resolver := claudeVersionResolver{client: server.Client(), url: server.URL}
	version, err := resolver.LatestVersion(context.Background())
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if got, want := version, "2.1.200"; got != want {
		t.Fatalf("version = %q, want %q", got, want)
	}
}

func TestClaudeVersionResolverNon200ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	resolver := claudeVersionResolver{client: server.Client(), url: server.URL}
	_, err := resolver.LatestVersion(context.Background())
	if err == nil {
		t.Fatal("LatestVersion() error = nil, want error for non-200 status")
	}
}

func TestClaudeVersionResolverBadJSONReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	resolver := claudeVersionResolver{client: server.Client(), url: server.URL}
	_, err := resolver.LatestVersion(context.Background())
	if err == nil {
		t.Fatal("LatestVersion() error = nil, want error for bad JSON")
	}
}

func TestClaudeVersionResolverEmptyVersionReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"version":""}`))
	}))
	defer server.Close()

	resolver := claudeVersionResolver{client: server.Client(), url: server.URL}
	_, err := resolver.LatestVersion(context.Background())
	if err == nil {
		t.Fatal("LatestVersion() error = nil, want error for empty version")
	}
}

func TestClaudeVersionResolverNetworkErrorReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"2.1.200"}`))
	}))
	serverURL := server.URL
	server.Close() // close before call to force network error

	resolver := claudeVersionResolver{client: server.Client(), url: serverURL}
	_, err := resolver.LatestVersion(context.Background())
	if err == nil {
		t.Fatal("LatestVersion() error = nil, want error for network failure")
	}
}

func TestNewWiresClaudeVersionResolverWhenProviderIsClaudeAndNilResolver(t *testing.T) {
	svc, err := New(Options{
		Runner:     &runnerRecorder{},
		Provider:   domain.ProviderClaude,
		Repository: "ghcr.io/valv/claude",
		ContextDir: "/tmp/claude-image",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if svc.resolver == nil {
		t.Fatal("New() with ProviderClaude and nil Resolver: resolver is nil, want auto-wired claudeVersionResolver")
	}
}

// newCacheTestService builds a minimal Service for cache-focused tests. It
// wires a providerImageStateStore so EnsureLatest can persist state, a
// runnerRecorder that simulates a missing image (forcing a build), and
// optionally injects a clock and cachePath.
func newCacheTestService(t *testing.T, resolver VersionResolver, cachePath string, clock func() time.Time) Service {
	t.Helper()
	runner := &runnerRecorder{
		errs: map[string]error{
			"image inspect ghcr.io/valv/claude:dev": fmt.Errorf("no such image"),
		},
	}
	svc, err := New(Options{
		Runner:     runner,
		StateStore: &providerImageStateStore{},
		Resolver:   resolver,
		Provider:   domain.ProviderClaude,
		Repository: "ghcr.io/valv/claude",
		ContextDir: t.TempDir(),
		CachePath:  cachePath,
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return svc
}

// writeCacheFile writes a versionCacheFile JSON directly to path so tests can
// pre-populate the cache without going through the production write path.
func writeCacheFile(t *testing.T, path string, c versionCacheFile) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll cache dir: %v", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent cache: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile cache: %v", err)
	}
}

// readCacheFile reads and parses the on-disk cache file for assertions.
func readCacheFile(t *testing.T, path string) versionCacheFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile cache: %v", err)
	}
	var c versionCacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("Unmarshal cache: %v", err)
	}
	return c
}

// resolverCallCounter wraps a staticResolver and counts invocations.
type resolverCallCounter struct {
	inner  VersionResolver
	called int
}

func (r *resolverCallCounter) LatestVersion(ctx context.Context) (string, error) {
	r.called++
	return r.inner.LatestVersion(ctx)
}

func TestEnsureLatestUsesCacheWhenFresh(t *testing.T) {
	oldFind := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFind })

	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "version-cache.json")
	fixedNow := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	cacheTime := fixedNow.Add(-1 * time.Hour) // 1 hour ago — within TTL

	writeCacheFile(t, cachePath, versionCacheFile{
		Providers: map[string]versionCacheEntry{
			"claude": {Version: "2.1.143", CheckedAt: cacheTime},
		},
	})

	counter := &resolverCallCounter{inner: staticResolver("should-not-be-called")}
	svc := newCacheTestService(t, counter, cachePath, func() time.Time { return fixedNow })

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}
	if counter.called != 0 {
		t.Fatalf("resolver called %d times, want 0 (cache should have been used)", counter.called)
	}
	if got, want := result.LatestVersion, "2.1.143"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
}

func TestEnsureLatestSkipsCacheWhenStale(t *testing.T) {
	oldFind := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFind })

	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "version-cache.json")
	fixedNow := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	staleTime := fixedNow.Add(-25 * time.Hour) // 25 hours ago — beyond TTL

	writeCacheFile(t, cachePath, versionCacheFile{
		Providers: map[string]versionCacheEntry{
			"claude": {Version: "2.1.100", CheckedAt: staleTime},
		},
	})

	counter := &resolverCallCounter{inner: staticResolver("2.1.200")}
	svc := newCacheTestService(t, counter, cachePath, func() time.Time { return fixedNow })

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}
	if counter.called != 1 {
		t.Fatalf("resolver called %d times, want 1 (stale cache should trigger resolver)", counter.called)
	}
	if got, want := result.LatestVersion, "2.1.200"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	// Cache file must be updated with the new version.
	c := readCacheFile(t, cachePath)
	if got := c.Providers["claude"].Version; got != "2.1.200" {
		t.Fatalf("cache claude.version = %q, want %q", got, "2.1.200")
	}
}

func TestEnsureLatestWritesCacheAfterResolverSuccess(t *testing.T) {
	oldFind := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFind })

	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "version-cache.json")
	fixedNow := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	// No pre-existing cache file — resolver must be called and cache created.
	svc := newCacheTestService(t, staticResolver("2.1.200"), cachePath, func() time.Time { return fixedNow })

	_, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}

	c := readCacheFile(t, cachePath)
	entry, ok := c.Providers["claude"]
	if !ok {
		t.Fatal("cache missing claude entry after resolver call")
	}
	if entry.Version != "2.1.200" {
		t.Fatalf("cache claude.version = %q, want %q", entry.Version, "2.1.200")
	}
	wantCheckedAt := fixedNow.UTC().Truncate(time.Second)
	if !entry.CheckedAt.Equal(wantCheckedAt) {
		t.Fatalf("cache claude.checked_at = %v, want %v", entry.CheckedAt, wantCheckedAt)
	}
}

func TestEnsureLatestPreservesOtherProviderEntries(t *testing.T) {
	oldFind := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFind })

	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "version-cache.json")
	fixedNow := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	// Pre-populate with a Codex entry.
	writeCacheFile(t, cachePath, versionCacheFile{
		Providers: map[string]versionCacheEntry{
			"codex": {Version: "0.50.0", CheckedAt: fixedNow},
		},
	})

	// Run EnsureLatest for Claude — should add Claude entry without touching Codex.
	svc := newCacheTestService(t, staticResolver("2.1.200"), cachePath, func() time.Time { return fixedNow })

	_, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}

	c := readCacheFile(t, cachePath)
	if got := c.Providers["codex"].Version; got != "0.50.0" {
		t.Fatalf("codex entry overwritten: got %q, want %q", got, "0.50.0")
	}
	if got := c.Providers["claude"].Version; got != "2.1.200" {
		t.Fatalf("claude entry = %q, want %q", got, "2.1.200")
	}
}

func TestEnsureLatestIgnoresMalformedCache(t *testing.T) {
	oldFind := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFind })

	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "version-cache.json")

	// Write invalid JSON to the cache file.
	if err := os.WriteFile(cachePath, []byte("not-json"), 0o644); err != nil {
		t.Fatalf("WriteFile(malformed cache): %v", err)
	}

	fixedNow := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	counter := &resolverCallCounter{inner: staticResolver("2.1.200")}
	svc := newCacheTestService(t, counter, cachePath, func() time.Time { return fixedNow })

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v (malformed cache must not propagate)", err)
	}
	if counter.called != 1 {
		t.Fatalf("resolver called %d times, want 1 (malformed cache should fall through to resolver)", counter.called)
	}
	if got, want := result.LatestVersion, "2.1.200"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	// Cache should now be rewritten with the fresh entry.
	c := readCacheFile(t, cachePath)
	if c.Providers["claude"].Version != "2.1.200" {
		t.Fatalf("cache not rewritten after malformed-cache fallback")
	}
}

func TestEnsureLatestSurvivesCacheWriteError(t *testing.T) {
	oldFind := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFind })

	cacheDir := t.TempDir()
	// Place a directory at the cache file path so WriteFile fails (is-a-dir error).
	cachePath := filepath.Join(cacheDir, "version-cache.json")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(cachePath-as-dir): %v", err)
	}

	fixedNow := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	svc := newCacheTestService(t, staticResolver("2.1.200"), cachePath, func() time.Time { return fixedNow })

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v (cache write error must not propagate)", err)
	}
	if got, want := result.LatestVersion, "2.1.200"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
}

// TestEnsureLatestReportsCachedCheckedAtOnCacheHit verifies that when the
// version is served from the on-disk cache (resolver not called), EnsureResult.LatestCheckedAt
// reflects the CACHED entry's checked_at timestamp rather than the current wall clock.
func TestEnsureLatestReportsCachedCheckedAtOnCacheHit(t *testing.T) {
	oldFind := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFind })

	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "version-cache.json")

	// Cache entry stamped at midnight; clock is 12 hours later (well within TTL).
	cachedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clockNow := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	writeCacheFile(t, cachePath, versionCacheFile{
		Providers: map[string]versionCacheEntry{
			"claude": {Version: "2.1.143", CheckedAt: cachedAt},
		},
	})

	counter := &resolverCallCounter{inner: staticResolver("should-not-be-called")}
	svc := newCacheTestService(t, counter, cachePath, func() time.Time { return clockNow })

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}
	if counter.called != 0 {
		t.Fatalf("resolver called %d times, want 0 (cache should have been used)", counter.called)
	}
	if got, want := result.LatestVersion, "2.1.143"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	// LatestCheckedAt must reflect when the version was ACTUALLY checked
	// (the cache entry's timestamp), not the current clock value.
	if !result.LatestCheckedAt.Equal(cachedAt) {
		t.Fatalf("LatestCheckedAt = %v, want cached value %v", result.LatestCheckedAt, cachedAt)
	}
}

// TestEnsureLatestRejectsCacheWithFutureTimestamp verifies that a cache entry
// whose checked_at is in the future (clock skew, manual edit, NTP correction)
// is rejected and treated as a cache miss, so the resolver is called and the
// cache is refreshed.
func TestEnsureLatestRejectsCacheWithFutureTimestamp(t *testing.T) {
	oldFind := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFind })

	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "version-cache.json")

	// Cache entry's timestamp is in year 2030; clock is in 2026 — negative delta.
	futureTime := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	clockNow := time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC)

	writeCacheFile(t, cachePath, versionCacheFile{
		Providers: map[string]versionCacheEntry{
			"claude": {Version: "2.1.999", CheckedAt: futureTime},
		},
	})

	counter := &resolverCallCounter{inner: staticResolver("2.1.200")}
	svc := newCacheTestService(t, counter, cachePath, func() time.Time { return clockNow })

	result, err := svc.EnsureLatest(context.Background(), EnsureRequest{})
	if err != nil {
		t.Fatalf("EnsureLatest() error = %v", err)
	}
	// Resolver must have been called — the future-timestamp entry is treated as a miss.
	if counter.called != 1 {
		t.Fatalf("resolver called %d times, want 1 (future-timestamp cache entry must be rejected)", counter.called)
	}
	if got, want := result.LatestVersion, "2.1.200"; got != want {
		t.Fatalf("LatestVersion = %q, want %q", got, want)
	}
	// Cache file must now contain the fresh resolver result, not the stale future timestamp.
	c := readCacheFile(t, cachePath)
	entry := c.Providers["claude"]
	if entry.Version != "2.1.200" {
		t.Fatalf("cache claude.version = %q, want %q after future-timestamp rejection", entry.Version, "2.1.200")
	}
	if entry.CheckedAt.After(clockNow.Add(time.Second)) {
		t.Fatalf("cache claude.checked_at = %v is later than expected current time %v", entry.CheckedAt, clockNow)
	}
}

// --- Unit 12.3 — EnsureProjectImage tests ---------------------------------
//
// Cache-matrix coverage uses runnerRecorder for both Output (label probes)
// and Run (buildx build). The freshness probe key format mirrors the live
// inspectLabel call: `image inspect --format {{ index .Config.Labels "<label>" }} <ref>`.
// projectInspectKey centralises that string so the table tests stay
// readable.

// sampleProjectManifest returns a manifest used across the Unit 12.3 tests.
// Two object-form tools — one go install, one npm install -g — exercise the
// full overlay generator path.
func sampleProjectManifest() tools.ToolManifest {
	return tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"ta": {Source: "github.com/evanmschultz/ta@latest", Install: "go install"},
			"cc": {Source: "@anthropic-ai/claude-code@1.0.0", Install: "npm install -g"},
		},
	}
}

// projectInspectKey returns the joined-arg key the runnerRecorder uses for
// a label-inspect call against ref. Mirrors inspectLabel's outputRunner call
// exactly.
func projectInspectKey(ref, label string) string {
	return strings.Join([]string{
		"image", "inspect", "--format",
		"{{ index .Config.Labels \"" + label + "\" }}",
		ref,
	}, " ")
}

// newProjectImageService wires a Service with the supplied runner. UID/GID
// are pinned to 1000 so the test does not depend on the host user.
func newProjectImageService(t *testing.T, runner Runner) Service {
	t.Helper()
	svc, err := New(Options{
		Runner:     runner,
		Repository: "ghcr.io/valv/codex",
		ContextDir: t.TempDir(),
		Dockerfile: "Dockerfile",
		DefaultTag: "dev",
		UserID:     1000,
		GroupID:    1000,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return svc
}

// expectedProjectTag computes the per-project tag for the sample manifest
// against the given base ref. Shares the production logic so a future
// short-hash-length tweak does not break the tests in a non-load-bearing way.
func expectedProjectTag(repo string, manifest tools.ToolManifest) string {
	hash := OverlayHash(manifest)
	return repo + ":" + tagPrefixProjectOverlay + shortOverlayHash(hash)
}

func TestEnsureProjectImage_EmptyManifestShortCircuits(t *testing.T) {
	t.Parallel()

	runner := &runnerRecorder{}
	svc := newProjectImageService(t, runner)
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  tools.ToolManifest{},
		BaseImage: baseRef,
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if result.Image != baseRef {
		t.Fatalf("Image = %q, want base ref %q", result.Image.String(), baseRef.String())
	}
	if result.Action != EnsureActionUsingExistingImage {
		t.Fatalf("Action = %q, want %q", result.Action, EnsureActionUsingExistingImage)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("docker calls = %d, want 0 for empty-manifest short-circuit", len(runner.calls))
	}
}

func TestEnsureProjectImage_TargetMissingTriggersBuild(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	runner := &runnerRecorder{
		outs: map[string]string{
			// Base image carries its recipe hash — captured verbatim.
			projectInspectKey(baseRef.String(), recipeHashLabel): "base-recipe-sha256-hex",
		},
		errs: map[string]error{
			// Target tag is genuinely missing — dockerImageMissingError path.
			projectInspectKey(wantTag, recipeHashLabel): fmt.Errorf("no such image"),
		},
	}
	svc := newProjectImageService(t, runner)

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if result.Action != EnsureActionUpdated {
		t.Fatalf("Action = %q, want %q", result.Action, EnsureActionUpdated)
	}
	if result.Image.String() != wantTag {
		t.Fatalf("Image = %q, want %q", result.Image.String(), wantTag)
	}
	if result.ToolsHash == "" || len(result.ToolsHash) != 64 {
		t.Fatalf("ToolsHash = %q, want 64-char hex", result.ToolsHash)
	}

	// Find the build invocation among the runner calls.
	buildCallIdx := -1
	for i, call := range runner.calls {
		if len(call) >= 2 && call[0] == "buildx" && call[1] == "build" {
			buildCallIdx = i
			break
		}
	}
	if buildCallIdx < 0 {
		t.Fatalf("no buildx build call recorded: %#v", runner.calls)
	}
	// Assert all five labels appear on the build call.
	args := runner.calls[buildCallIdx]
	wantLabels := []string{
		fmt.Sprintf("%s=%s", baseRecipeHashLabel, "base-recipe-sha256-hex"),
		fmt.Sprintf("%s=%s", managedLabel, "true"),
		// recipe_hash value is the sha256 of the generated overlay dockerfile;
		// compute it here to compare without re-hashing the manifest.
		"", // populated below
		fmt.Sprintf("%s=%s", scopeLabel, scopeValueProjectOverlay),
		fmt.Sprintf("%s=%s", toolsHashLabel, OverlayHash(manifest)),
	}
	overlayContent, err := BuildOverlayDockerfile(manifest, baseRef)
	if err != nil {
		t.Fatalf("BuildOverlayDockerfile error = %v", err)
	}
	wantLabels[2] = fmt.Sprintf("%s=%s", recipeHashLabel, sha256Hex(overlayContent))

	for _, want := range wantLabels {
		found := false
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "--label" && args[i+1] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("build args missing label %q in %#v", want, args)
		}
	}

	// Confirm the build call carries the expected target tag.
	foundTag := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-t" && args[i+1] == wantTag {
			foundTag = true
			break
		}
	}
	if !foundTag {
		t.Errorf("build args missing -t %s in %#v", wantTag, args)
	}
}

func TestEnsureProjectImage_AllLabelsMatchSkipsBuild(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	overlayContent, err := BuildOverlayDockerfile(manifest, baseRef)
	if err != nil {
		t.Fatalf("BuildOverlayDockerfile error = %v", err)
	}
	expectedRecipe := sha256Hex(overlayContent)
	expectedTools := OverlayHash(manifest)
	const expectedBaseRecipe = "base-recipe-sha256-hex"

	runner := &runnerRecorder{
		outs: map[string]string{
			projectInspectKey(baseRef.String(), recipeHashLabel): expectedBaseRecipe,
			projectInspectKey(wantTag, recipeHashLabel):          expectedRecipe,
			projectInspectKey(wantTag, toolsHashLabel):           expectedTools,
			projectInspectKey(wantTag, baseRecipeHashLabel):      expectedBaseRecipe,
		},
	}
	svc := newProjectImageService(t, runner)

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if result.Action != EnsureActionUpToDate {
		t.Fatalf("Action = %q, want %q (no rebuild expected when all labels match)", result.Action, EnsureActionUpToDate)
	}
	if result.Image.String() != wantTag {
		t.Fatalf("Image = %q, want %q", result.Image.String(), wantTag)
	}
	// No buildx call should have happened.
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "buildx" && call[1] == "build" {
			t.Fatalf("unexpected buildx build call when labels match: %#v", call)
		}
	}
}

func TestEnsureProjectImage_FreshnessMismatchTriggersRebuild(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	overlayContent, err := BuildOverlayDockerfile(manifest, baseRef)
	if err != nil {
		t.Fatalf("BuildOverlayDockerfile error = %v", err)
	}
	expectedRecipe := sha256Hex(overlayContent)
	expectedTools := OverlayHash(manifest)
	const expectedBaseRecipe = "base-recipe-sha256-hex"

	cases := []struct {
		name string
		// label → simulated stale value (replaces the expected match).
		staleLabel string
		staleValue string
	}{
		{name: "recipe_hash_mismatch", staleLabel: recipeHashLabel, staleValue: "stale-recipe"},
		{name: "tools_hash_mismatch", staleLabel: toolsHashLabel, staleValue: "stale-tools"},
		{name: "base_recipe_hash_mismatch", staleLabel: baseRecipeHashLabel, staleValue: "stale-base"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			outs := map[string]string{
				projectInspectKey(baseRef.String(), recipeHashLabel): expectedBaseRecipe,
				projectInspectKey(wantTag, recipeHashLabel):          expectedRecipe,
				projectInspectKey(wantTag, toolsHashLabel):           expectedTools,
				projectInspectKey(wantTag, baseRecipeHashLabel):      expectedBaseRecipe,
			}
			outs[projectInspectKey(wantTag, tc.staleLabel)] = tc.staleValue

			runner := &runnerRecorder{outs: outs}
			svc := newProjectImageService(t, runner)

			result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
				Manifest:  manifest,
				BaseImage: baseRef,
			})
			if err != nil {
				t.Fatalf("EnsureProjectImage() error = %v", err)
			}
			if result.Action != EnsureActionUpdated {
				t.Fatalf("Action = %q, want %q for %s", result.Action, EnsureActionUpdated, tc.name)
			}
			// Confirm a buildx build call landed.
			built := false
			for _, call := range runner.calls {
				if len(call) >= 2 && call[0] == "buildx" && call[1] == "build" {
					built = true
					break
				}
			}
			if !built {
				t.Fatalf("no buildx build call for %s: %#v", tc.name, runner.calls)
			}
		})
	}
}

// nonOutputRunner implements Runner but NOT outputRunner. It exercises the
// errLabelUnreadable typecast-failure path in inspectLabel and the
// conservative-rebuild policy of EnsureProjectImage.
type nonOutputRunner struct {
	calls [][]string
}

func (r *nonOutputRunner) Run(_ context.Context, args []string) error {
	r.calls = append(r.calls, append([]string(nil), args...))
	return nil
}

func TestEnsureProjectImage_TypecastFailureForcesRebuild(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")

	runner := &nonOutputRunner{}
	svc, err := New(Options{
		Runner:     runner,
		Repository: "ghcr.io/valv/codex",
		ContextDir: t.TempDir(),
		Dockerfile: "Dockerfile",
		DefaultTag: "dev",
		UserID:     1000,
		GroupID:    1000,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if result.Action != EnsureActionUpdated {
		t.Fatalf("Action = %q, want %q (typecast failure must rebuild)", result.Action, EnsureActionUpdated)
	}
	// Exactly one buildx call expected; no Output calls happened (the runner
	// has no Output method).
	if len(runner.calls) != 1 {
		t.Fatalf("runner.calls = %d, want 1 (only buildx build, no label probes)", len(runner.calls))
	}
	if runner.calls[0][0] != "buildx" || runner.calls[0][1] != "build" {
		t.Fatalf("call[0] = %#v, want buildx build", runner.calls[0])
	}
}

func TestEnsureProjectImage_InspectErrorForcesRebuild(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	runner := &runnerRecorder{
		outs: map[string]string{
			projectInspectKey(baseRef.String(), recipeHashLabel): "base-recipe-sha256-hex",
		},
		errs: map[string]error{
			// Generic inspect failure (not image-missing) on the target tag.
			// Policy: rebuild rather than bubble.
			projectInspectKey(wantTag, recipeHashLabel): fmt.Errorf("dockerd is not responding"),
		},
	}
	svc := newProjectImageService(t, runner)

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if result.Action != EnsureActionUpdated {
		t.Fatalf("Action = %q, want %q (non-missing inspect error must rebuild)", result.Action, EnsureActionUpdated)
	}
}

func TestEnsureProjectImage_NoCacheForcesRebuild(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	overlayContent, err := BuildOverlayDockerfile(manifest, baseRef)
	if err != nil {
		t.Fatalf("BuildOverlayDockerfile error = %v", err)
	}
	expectedRecipe := sha256Hex(overlayContent)
	expectedTools := OverlayHash(manifest)
	const expectedBaseRecipe = "base-recipe-sha256-hex"

	// Even though all labels match, NoCache forces rebuild.
	runner := &runnerRecorder{
		outs: map[string]string{
			projectInspectKey(baseRef.String(), recipeHashLabel): expectedBaseRecipe,
			projectInspectKey(wantTag, recipeHashLabel):          expectedRecipe,
			projectInspectKey(wantTag, toolsHashLabel):           expectedTools,
			projectInspectKey(wantTag, baseRecipeHashLabel):      expectedBaseRecipe,
		},
	}
	svc := newProjectImageService(t, runner)

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
		NoCache:   true,
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if result.Action != EnsureActionUpdated {
		t.Fatalf("Action = %q, want %q (NoCache must rebuild)", result.Action, EnsureActionUpdated)
	}
	// Build call must carry --no-cache.
	var buildCall []string
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "buildx" && call[1] == "build" {
			buildCall = call
			break
		}
	}
	if buildCall == nil {
		t.Fatalf("no buildx build call recorded: %#v", runner.calls)
	}
	hasNoCache := false
	for _, arg := range buildCall {
		if arg == "--no-cache" {
			hasNoCache = true
			break
		}
	}
	if !hasNoCache {
		t.Errorf("buildx build call missing --no-cache: %#v", buildCall)
	}
}

func TestEnsureProjectImage_BaseImageMissingReturnsError(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")

	runner := &runnerRecorder{
		errs: map[string]error{
			projectInspectKey(baseRef.String(), recipeHashLabel): fmt.Errorf("no such image: ghcr.io/valv/codex:dev"),
		},
	}
	svc := newProjectImageService(t, runner)

	_, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err == nil {
		t.Fatal("EnsureProjectImage() error = nil, want base-missing error")
	}
	if !strings.Contains(err.Error(), "base image") {
		t.Errorf("error message missing 'base image' context: %q", err.Error())
	}
}

func TestEnsureProjectImage_OverlayGeneratorErrorWraps(t *testing.T) {
	t.Parallel()

	// String-form spec is rejected by BuildOverlayDockerfile — surface the
	// wrapped error from EnsureProjectImage without touching docker.
	manifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"jq": {Version: "1.7"},
		},
	}
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")

	runner := &runnerRecorder{}
	svc := newProjectImageService(t, runner)

	_, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err == nil {
		t.Fatal("EnsureProjectImage() error = nil, want overlay-generator error")
	}
	if !strings.Contains(err.Error(), "ensure project image:") {
		t.Errorf("error not wrapped with 'ensure project image:': %q", err.Error())
	}
	if !strings.Contains(err.Error(), "string-form spec") {
		t.Errorf("error does not include underlying string-form rejection: %q", err.Error())
	}
	if len(runner.calls) != 0 {
		t.Fatalf("docker calls = %d, want 0 (overlay gen error must short-circuit)", len(runner.calls))
	}
}

func TestProjectImageRef_TagFormat(t *testing.T) {
	t.Parallel()

	runner := &runnerRecorder{}
	svc := newProjectImageService(t, runner)

	const hash = "9c3a7b1e8d4f0123456789abcdef0123456789abcdef0123456789abcdef0123"
	ref := svc.projectImageRef(hash)
	if ref.Repository != "ghcr.io/valv/codex" {
		t.Errorf("Repository = %q, want ghcr.io/valv/codex", ref.Repository)
	}
	if ref.Tag != "proj-9c3a7b1e8d4f" {
		t.Errorf("Tag = %q, want proj-9c3a7b1e8d4f", ref.Tag)
	}
	if got, want := ref.String(), "ghcr.io/valv/codex:proj-9c3a7b1e8d4f"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestEnsureProjectImage_GenericBaseInspectErrorFallsThroughToRebuild
// covers the Round 2 falsification fix to PLAN.md decision 5: a generic
// non-missing inspect error on the BASE image (e.g. "permission denied")
// MUST be treated as a label-read miss, populate baseRecipeHash = "", and
// fall through to the rebuild path — NOT abort with an error.
// Pre-fix behavior aborted with `"ensure project image: inspect base recipe
// hash: ..."`; post-fix behavior is to log and rebuild with the empty value.
func TestEnsureProjectImage_GenericBaseInspectErrorFallsThroughToRebuild(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	runner := &runnerRecorder{
		errs: map[string]error{
			// Generic, non-missing inspect failure on the BASE image. The
			// substring "no such image" must NOT appear, otherwise
			// dockerImageMissingError would (correctly) classify this as
			// fatal. "permission denied" is the canonical generic case.
			projectInspectKey(baseRef.String(), recipeHashLabel): fmt.Errorf("permission denied"),
			// Target tag is genuinely missing so the rebuild path activates
			// cleanly via the dockerImageMissingError short-circuit in
			// projectImageNeedsBuild.
			projectInspectKey(wantTag, recipeHashLabel): fmt.Errorf("no such image"),
		},
	}
	svc := newProjectImageService(t, runner)

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v, want nil (generic base-inspect error must fall through, not abort)", err)
	}
	if result.Action != EnsureActionUpdated {
		t.Fatalf("Action = %q, want %q (rebuild path must activate)", result.Action, EnsureActionUpdated)
	}
	if result.Image.String() != wantTag {
		t.Fatalf("Image = %q, want %q", result.Image.String(), wantTag)
	}

	// Find the build invocation and confirm base_recipe_hash label carries
	// the empty-string sentinel rather than a stale value.
	var buildCall []string
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "buildx" && call[1] == "build" {
			buildCall = call
			break
		}
	}
	if buildCall == nil {
		t.Fatalf("no buildx build call recorded — rebuild path did not activate: %#v", runner.calls)
	}

	// Confirm the build args carry base_recipe_hash with an empty value.
	// Format mirrors the live label-formatter: "<label>=<value>".
	wantBaseLabel := fmt.Sprintf("%s=", baseRecipeHashLabel)
	foundBaseLabel := false
	for i := 0; i < len(buildCall)-1; i++ {
		if buildCall[i] == "--label" && buildCall[i+1] == wantBaseLabel {
			foundBaseLabel = true
			break
		}
	}
	if !foundBaseLabel {
		t.Errorf("build call missing empty base_recipe_hash label %q in %#v", wantBaseLabel, buildCall)
	}
}

// --- Unit 15.2.5 — Network-policy injection on overlay build ----------------
//
// fakeNetworkPolicy is a minimal NetworkPolicy stub for the tests below. It
// records Provision calls + returns canned PolicyMaterial. Cleanup is
// counted so a test can prove the deferred cleanup ran.
type fakeNetworkPolicy struct {
	material      NetworkPolicyMaterial
	cleanupCount  int
	provisionErr  error
	cleanupErr    error
	lastRequest   NetworkPolicyRequest
	provisionHits int
}

func (f *fakeNetworkPolicy) Provision(_ context.Context, req NetworkPolicyRequest) (NetworkPolicyMaterial, NetworkPolicyCleanup, error) {
	f.provisionHits++
	f.lastRequest = req
	if f.provisionErr != nil {
		return NetworkPolicyMaterial{}, nil, f.provisionErr
	}
	cleanup := func(_ context.Context) error {
		f.cleanupCount++
		return f.cleanupErr
	}
	return f.material, cleanup, nil
}

func newProjectImageServiceWithPolicy(t *testing.T, runner Runner, policy NetworkPolicy, endpoint string) Service {
	t.Helper()
	svc, err := New(Options{
		Runner:        runner,
		Repository:    "ghcr.io/valv/codex",
		ContextDir:    t.TempDir(),
		Dockerfile:    "Dockerfile",
		DefaultTag:    "dev",
		UserID:        1000,
		GroupID:       1000,
		NetworkPolicy: policy,
		ProxyEndpoint: endpoint,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return svc
}

func TestEnsureProjectImage_NetworkPolicyInjectsProxyArgsAndNetwork(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	policy := &fakeNetworkPolicy{
		material: NetworkPolicyMaterial{
			HTTPProxyURL:  "http://host.docker.internal:18080",
			HTTPSProxyURL: "http://host.docker.internal:18080",
			NoProxy:       "github.com,objects.githubusercontent.com,proxy.golang.org,sum.golang.org",
			NetworkName:   "valv-netpol-aaaaaaaaaaaa",
		},
	}

	runner := &runnerRecorder{
		outs: map[string]string{
			projectInspectKey(baseRef.String(), recipeHashLabel): "base-recipe-sha256-hex",
		},
		errs: map[string]error{
			projectInspectKey(wantTag, recipeHashLabel): fmt.Errorf("no such image"),
		},
	}
	svc := newProjectImageServiceWithPolicy(t, runner, policy, "host.docker.internal:18080")

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if result.Action != EnsureActionUpdated {
		t.Fatalf("Action = %q, want %q", result.Action, EnsureActionUpdated)
	}

	if policy.provisionHits != 1 {
		t.Fatalf("policy.provisionHits = %d, want 1", policy.provisionHits)
	}
	// Effective allowlist must include the four built-in defaults even
	// though the manifest has no user-declared hosts.
	wantHosts := map[string]bool{
		"github.com": true, "objects.githubusercontent.com": true,
		"proxy.golang.org": true, "sum.golang.org": true,
	}
	for _, host := range policy.lastRequest.Allowlist {
		delete(wantHosts, host)
	}
	if len(wantHosts) != 0 {
		t.Errorf("Provision Allowlist missing built-in defaults: %#v (got %#v)", wantHosts, policy.lastRequest.Allowlist)
	}
	if policy.lastRequest.ProxyEndpoint != "host.docker.internal:18080" {
		t.Errorf("Provision ProxyEndpoint = %q, want %q", policy.lastRequest.ProxyEndpoint, "host.docker.internal:18080")
	}

	// Build args must include the three proxy values; --network must point
	// at the policy network.
	var buildCall []string
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "buildx" && call[1] == "build" {
			buildCall = call
			break
		}
	}
	if buildCall == nil {
		t.Fatalf("no buildx build call recorded: %#v", runner.calls)
	}

	wantArgs := []string{
		"HTTPS_PROXY=http://host.docker.internal:18080",
		"HTTP_PROXY=http://host.docker.internal:18080",
		"NO_PROXY=github.com,objects.githubusercontent.com,proxy.golang.org,sum.golang.org",
	}
	for _, want := range wantArgs {
		found := false
		for i := 0; i < len(buildCall)-1; i++ {
			if buildCall[i] == "--build-arg" && buildCall[i+1] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("build call missing --build-arg %q in %#v", want, buildCall)
		}
	}

	// --network <policy network>
	foundNetwork := false
	for i := 0; i < len(buildCall)-1; i++ {
		if buildCall[i] == "--network" && buildCall[i+1] == "valv-netpol-aaaaaaaaaaaa" {
			foundNetwork = true
			break
		}
	}
	if !foundNetwork {
		t.Errorf("build call missing --network valv-netpol-aaaaaaaaaaaa in %#v", buildCall)
	}

	// Deferred cleanup must have run.
	if policy.cleanupCount != 1 {
		t.Errorf("policy.cleanupCount = %d, want 1 (deferred cleanup must run)", policy.cleanupCount)
	}

	// Freshness label contract preserved: the five Unit 12.3 labels still
	// appear on the build call.
	overlayContent, err := BuildOverlayDockerfile(manifest, baseRef)
	if err != nil {
		t.Fatalf("BuildOverlayDockerfile error = %v", err)
	}
	wantLabels := []string{
		fmt.Sprintf("%s=%s", baseRecipeHashLabel, "base-recipe-sha256-hex"),
		fmt.Sprintf("%s=%s", managedLabel, "true"),
		fmt.Sprintf("%s=%s", recipeHashLabel, sha256Hex(overlayContent)),
		fmt.Sprintf("%s=%s", scopeLabel, scopeValueProjectOverlay),
		fmt.Sprintf("%s=%s", toolsHashLabel, OverlayHash(manifest)),
	}
	for _, want := range wantLabels {
		found := false
		for i := 0; i < len(buildCall)-1; i++ {
			if buildCall[i] == "--label" && buildCall[i+1] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("build call missing --label %q in %#v", want, buildCall)
		}
	}

	// Target tag identity preserved across policy on/off — the tag is
	// computed from OverlayHash(manifest) only, never the allowlist.
	if result.Image.String() != wantTag {
		t.Errorf("Image = %q, want %q (overlay tag must not change with policy on)", result.Image.String(), wantTag)
	}
}

func TestEnsureProjectImage_NetworkPolicyOff_NoProxyArgsNoNetwork(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	runner := &runnerRecorder{
		outs: map[string]string{
			projectInspectKey(baseRef.String(), recipeHashLabel): "base-recipe-sha256-hex",
		},
		errs: map[string]error{
			projectInspectKey(wantTag, recipeHashLabel): fmt.Errorf("no such image"),
		},
	}
	// No NetworkPolicy in Options — policy off.
	svc := newProjectImageService(t, runner)

	if _, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	}); err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}

	var buildCall []string
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "buildx" && call[1] == "build" {
			buildCall = call
			break
		}
	}
	if buildCall == nil {
		t.Fatalf("no buildx build call recorded: %#v", runner.calls)
	}

	// No proxy --build-arg pairs.
	for i := 0; i < len(buildCall)-1; i++ {
		if buildCall[i] != "--build-arg" {
			continue
		}
		val := buildCall[i+1]
		if strings.HasPrefix(val, "HTTP_PROXY=") || strings.HasPrefix(val, "HTTPS_PROXY=") || strings.HasPrefix(val, "NO_PROXY=") {
			t.Errorf("policy-off build call contained proxy build-arg %q: %#v", val, buildCall)
		}
	}
	// No --network flag.
	for i := 0; i < len(buildCall); i++ {
		if buildCall[i] == "--network" {
			t.Errorf("policy-off build call contained --network at index %d: %#v", i, buildCall)
		}
	}
}

func TestEnsureProjectImage_NetworkPolicyEmptyManifest_NoProvision(t *testing.T) {
	t.Parallel()

	policy := &fakeNetworkPolicy{}
	runner := &runnerRecorder{}
	svc := newProjectImageServiceWithPolicy(t, runner, policy, "host.docker.internal:18080")

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  tools.ToolManifest{},
		BaseImage: docker.NewImageRef("ghcr.io/valv/codex", "dev"),
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if result.Action != EnsureActionUsingExistingImage {
		t.Errorf("Action = %q, want %q", result.Action, EnsureActionUsingExistingImage)
	}
	if policy.provisionHits != 0 {
		t.Errorf("policy.provisionHits = %d, want 0 (empty manifest must short-circuit)", policy.provisionHits)
	}
	if policy.cleanupCount != 0 {
		t.Errorf("policy.cleanupCount = %d, want 0 (no cleanup when no provision)", policy.cleanupCount)
	}
}

func TestEnsureProjectImage_NetworkPolicyUpToDate_NoProvision(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	overlayContent, err := BuildOverlayDockerfile(manifest, baseRef)
	if err != nil {
		t.Fatalf("BuildOverlayDockerfile error = %v", err)
	}
	expectedRecipe := sha256Hex(overlayContent)
	expectedTools := OverlayHash(manifest)
	const expectedBaseRecipe = "base-recipe-sha256-hex"

	runner := &runnerRecorder{
		outs: map[string]string{
			projectInspectKey(baseRef.String(), recipeHashLabel): expectedBaseRecipe,
			projectInspectKey(wantTag, recipeHashLabel):          expectedRecipe,
			projectInspectKey(wantTag, toolsHashLabel):           expectedTools,
			projectInspectKey(wantTag, baseRecipeHashLabel):      expectedBaseRecipe,
		},
	}
	policy := &fakeNetworkPolicy{}
	svc := newProjectImageServiceWithPolicy(t, runner, policy, "host.docker.internal:18080")

	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if result.Action != EnsureActionUpToDate {
		t.Errorf("Action = %q, want %q (up-to-date overlay must skip rebuild)", result.Action, EnsureActionUpToDate)
	}
	if policy.provisionHits != 0 {
		t.Errorf("policy.provisionHits = %d, want 0 (up-to-date must not provision)", policy.provisionHits)
	}
	if policy.cleanupCount != 0 {
		t.Errorf("policy.cleanupCount = %d, want 0 (no cleanup when no provision)", policy.cleanupCount)
	}
}

func TestEnsureProjectImage_NetworkPolicyProvisionError_Wraps(t *testing.T) {
	t.Parallel()

	manifest := sampleProjectManifest()
	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	sentinel := errors.New("docker daemon unreachable")
	policy := &fakeNetworkPolicy{provisionErr: sentinel}

	runner := &runnerRecorder{
		outs: map[string]string{
			projectInspectKey(baseRef.String(), recipeHashLabel): "base-recipe-sha256-hex",
		},
		errs: map[string]error{
			projectInspectKey(wantTag, recipeHashLabel): fmt.Errorf("no such image"),
		},
	}
	svc := newProjectImageServiceWithPolicy(t, runner, policy, "host.docker.internal:18080")

	_, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	})
	if err == nil {
		t.Fatal("EnsureProjectImage() error = nil, want provision error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want errors.Is sentinel", err)
	}
	if !strings.Contains(err.Error(), "provision network policy") {
		t.Errorf("err = %q, want context containing 'provision network policy'", err.Error())
	}
}

func TestEnsureProjectImage_NetworkPolicyHonorsUserAllowlist(t *testing.T) {
	t.Parallel()

	baseRef := docker.NewImageRef("ghcr.io/valv/codex", "dev")
	manifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"ta": {Source: "github.com/evanmschultz/ta@latest", Install: "go install"},
		},
		Allowlist: tools.AllowlistConfig{
			Hosts: []string{"internal.example.com", "registry.example.org"},
		},
	}
	wantTag := expectedProjectTag("ghcr.io/valv/codex", manifest)

	policy := &fakeNetworkPolicy{
		material: NetworkPolicyMaterial{
			HTTPProxyURL:  "http://host.docker.internal:18080",
			HTTPSProxyURL: "http://host.docker.internal:18080",
			NoProxy:       "ignored-for-this-test",
			NetworkName:   "valv-netpol-aaaaaaaaaaaa",
		},
	}
	runner := &runnerRecorder{
		outs: map[string]string{
			projectInspectKey(baseRef.String(), recipeHashLabel): "base-recipe-sha256-hex",
		},
		errs: map[string]error{
			projectInspectKey(wantTag, recipeHashLabel): fmt.Errorf("no such image"),
		},
	}
	svc := newProjectImageServiceWithPolicy(t, runner, policy, "host.docker.internal:18080")

	if _, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
	}); err != nil {
		t.Fatalf("EnsureProjectImage() error = %v", err)
	}
	if policy.provisionHits != 1 {
		t.Fatalf("policy.provisionHits = %d, want 1", policy.provisionHits)
	}
	// Effective allowlist must be union of user hosts + four defaults.
	got := map[string]bool{}
	for _, h := range policy.lastRequest.Allowlist {
		got[h] = true
	}
	required := []string{
		"github.com", "objects.githubusercontent.com",
		"proxy.golang.org", "sum.golang.org",
		"internal.example.com", "registry.example.org",
	}
	for _, want := range required {
		if !got[want] {
			t.Errorf("Provision Allowlist missing %q in %#v", want, policy.lastRequest.Allowlist)
		}
	}
}
