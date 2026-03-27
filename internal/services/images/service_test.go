package images

import (
	"context"
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
	want := []string{"buildx", "build", "--load", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "--build-arg", "CODEX_VERSION=0.117.0", "--build-arg", fmt.Sprintf("VALV_GID=%d", gid), "--build-arg", fmt.Sprintf("VALV_UID=%d", uid), "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", "io.valv.scope=image", "--label", "io.valv.version=0.117.0", "/tmp/codex-image"}
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
	runner := &runnerRecorder{}
	svc, err := New(Options{
		Runner:     runner,
		StateStore: stateStore,
		Resolver:   staticResolver("0.117.0"),
		Repository: "ghcr.io/valv/codex",
		ContextDir: "/tmp/codex-image",
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
	if len(runner.calls) != 1 {
		t.Fatalf("runner call count = %d, want 1", len(runner.calls))
	}
	if got, want := runner.calls[0], []string{"image", "inspect", "ghcr.io/valv/codex:dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inspect args = %#v, want %#v", got, want)
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
	if len(runner.calls) != 3 {
		t.Fatalf("runner call count = %d, want 3", len(runner.calls))
	}
	if got, want := runner.calls[2], []string{"image", "rm", "--force", "ghcr.io/valv/codex:0-116-0"}; !reflect.DeepEqual(got, want) {
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
	if !reflect.DeepEqual(runner.calls[0], []string{"buildx", "build", "--load", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "-t", "ghcr.io/valv/codex:0-117-0", "--build-arg", "CODEX_VERSION=0.117.0", "--build-arg", fmt.Sprintf("VALV_GID=%d", gid), "--build-arg", fmt.Sprintf("VALV_UID=%d", uid), "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", "io.valv.scope=image", "--label", "io.valv.version=0.117.0", "/tmp/codex-image"}) {
		t.Fatalf("Build() args = %#v", runner.calls[0])
	}
}

func TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable(t *testing.T) {

	uid := os.Getuid()
	gid := os.Getgid()
	firstCall := strings.Join([]string{"buildx", "build", "--load", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "--build-arg", "CODEX_VERSION=0.117.0", "--build-arg", fmt.Sprintf("VALV_GID=%d", gid), "--build-arg", fmt.Sprintf("VALV_UID=%d", uid), "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", "io.valv.scope=image", "--label", "io.valv.version=0.117.0", "/tmp/codex-image"}, " ")
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
	if got, want := runner.calls[1], []string{"build", "-f", "/tmp/codex-image/Dockerfile", "-t", "ghcr.io/valv/codex:dev", "--build-arg", "CODEX_VERSION=0.117.0", "--build-arg", fmt.Sprintf("VALV_GID=%d", gid), "--build-arg", fmt.Sprintf("VALV_UID=%d", uid), "--label", "io.valv.managed=true", "--label", "io.valv.provider=codex", "--label", "io.valv.scope=image", "--label", "io.valv.version=0.117.0", "/tmp/codex-image"}; !reflect.DeepEqual(got, want) {
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
