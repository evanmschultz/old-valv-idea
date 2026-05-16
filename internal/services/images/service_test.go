package images

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	want := []string{"buildx", "build", "--load", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "--build-arg", "CODEX_VERSION=0.117.0", "--build-arg", fmt.Sprintf("VALV_GID=%d", gid), "--build-arg", fmt.Sprintf("VALV_UID=%d", uid), "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", fmt.Sprintf("%s=%s", recipeHashLabel, svc.recipeHash()), "--label", "io.valv.scope=image", "--label", "io.valv.version=0.117.0", "/tmp/codex-image"}
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
	if !strings.Contains(string(content), `@openai/codex@${CODEX_VERSION}`) {
		t.Fatalf("dockerfile missing package install: %q", string(content))
	}
	for _, want := range []string{
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
		`ARG VALV_UID=1000`,
		`ARG VALV_GID=1000`,
		`apt-get install -y --no-install-recommends bubblewrap ca-certificates git ncurses-term`,
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
	if !reflect.DeepEqual(runner.calls[0], []string{"buildx", "build", "--load", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "-t", "ghcr.io/valv/codex:0-117-0", "--build-arg", "CODEX_VERSION=0.117.0", "--build-arg", fmt.Sprintf("VALV_GID=%d", gid), "--build-arg", fmt.Sprintf("VALV_UID=%d", uid), "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", fmt.Sprintf("%s=%s", recipeHashLabel, svc.recipeHash()), "--label", "io.valv.scope=image", "--label", "io.valv.version=0.117.0", "/tmp/codex-image"}) {
		t.Fatalf("Build() args = %#v", runner.calls[0])
	}
}

func TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable(t *testing.T) {
	uid := os.Getuid()
	gid := os.Getgid()
	firstCall := strings.Join([]string{"buildx", "build", "--load", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "--build-arg", "CODEX_VERSION=0.117.0", "--build-arg", fmt.Sprintf("VALV_GID=%d", gid), "--build-arg", fmt.Sprintf("VALV_UID=%d", uid), "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", fmt.Sprintf("%s=%s", recipeHashLabel, svcRecipeHashForTest("/tmp/codex-image", "Dockerfile")), "--label", "io.valv.scope=image", "--label", "io.valv.version=0.117.0", "/tmp/codex-image"}, " ")
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
	if got, want := runner.calls[1], []string{"build", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "--build-arg", "CODEX_VERSION=0.117.0", "--build-arg", fmt.Sprintf("VALV_GID=%d", gid), "--build-arg", fmt.Sprintf("VALV_UID=%d", uid), "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", fmt.Sprintf("%s=%s", recipeHashLabel, svcRecipeHashForTest("/tmp/codex-image", "Dockerfile")), "--label", "io.valv.scope=image", "--label", "io.valv.version=0.117.0", "/tmp/codex-image"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback args = %#v, want %#v", got, want)
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
		`@anthropic-ai/claude-code@${CLAUDE_VERSION}`,
		`CLAUDE_CONFIG_DIR=/home/valv/.claude`,
		`ENTRYPOINT ["claude"]`,
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
		`ARG VALV_UID=1000`,
		`ARG VALV_GID=1000`,
		`apt-get install -y --no-install-recommends bubblewrap ca-certificates git ncurses-term`,
		`getent group "${VALV_GID}" >/dev/null || groupadd -g "${VALV_GID}" valv`,
		`useradd -o -m -u "${VALV_UID}" -g "${VALV_GID}" -s /bin/sh valv`,
		`mkdir -p /home/valv/.claude /workspace`,
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
