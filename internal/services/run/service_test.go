package run

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/adapters/docker"
)

// fakeExecutor records the most recent ContainerRunRequest for assertion.
type fakeExecutor struct {
	got    docker.ContainerRunRequest
	runErr error
}

func (f *fakeExecutor) Run(_ context.Context, request docker.ContainerRunRequest) error {
	f.got = request
	return f.runErr
}

// recordingNotices captures Fprintf writes.
type recordingNotices struct {
	buf strings.Builder
}

func (r *recordingNotices) Write(p []byte) (int, error) {
	return r.buf.Write(p)
}

// preparedRuntimeFixture builds a non-trivial PreparedRuntime with a recorded
// Close counter so tests can assert cleanup ran.
type preparedRuntimeFixture struct {
	closeCount int32
	runtime    PreparedRuntime
}

func newPreparedFixture(env map[string]string, passthrough []string, mounts []docker.MountSpec, warnings []string) *preparedRuntimeFixture {
	fixture := &preparedRuntimeFixture{}
	fixture.runtime = PreparedRuntime{
		Env:            env,
		EnvPassthrough: passthrough,
		Mounts:         mounts,
		Warnings:       warnings,
		Cleanup: func() error {
			atomic.AddInt32(&fixture.closeCount, 1)
			return nil
		},
	}
	return fixture
}

func (p *preparedRuntimeFixture) Closed() int32 {
	return atomic.LoadInt32(&p.closeCount)
}

// providerDescriptors returns the canonical claude + codex descriptors so we
// can parameterize the shared service tests over both providers.
func providerDescriptors() []Provider {
	return []Provider{
		{
			Name:                "claude",
			ContainerNamePrefix: "valv-claude-interactive",
			NoticePrefix:        "Valv note",
		},
		{
			Name:                "codex",
			ContainerNamePrefix: "valv-codex-interactive",
			NoticePrefix:        "Valv MCP note",
		},
	}
}

func newServiceForTest(t *testing.T, provider Provider, executor *fakeExecutor, notices io.Writer) Service {
	t.Helper()
	service, err := New(Options{
		Executor: executor,
		Image:    docker.NewImageRef("valv-"+provider.Name, "dev"),
		TTY:      false,
		Stdin:    false,
		Logger:   log.NewWithOptions(io.Discard, log.Options{Level: log.DebugLevel}),
		Notices:  notices,
		Now:      func() time.Time { return time.Unix(0, 42) },
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

// TestNewRequiresDependencies verifies New rejects missing required deps.
func TestNewRequiresDependencies(t *testing.T) {
	t.Parallel()

	validImage := docker.NewImageRef("valv-claude", "dev")
	validExecutor := &fakeExecutor{}
	validProvider := Provider{
		Name:                "claude",
		ContainerNamePrefix: "valv-claude-interactive",
		NoticePrefix:        "Valv note",
	}

	cases := []struct {
		name    string
		options Options
	}{
		{
			name:    "nil executor",
			options: Options{Executor: nil, Image: validImage, Provider: validProvider},
		},
		{
			name:    "empty image repository",
			options: Options{Executor: validExecutor, Image: docker.ImageRef{}, Provider: validProvider},
		},
		{
			name:    "empty provider name",
			options: Options{Executor: validExecutor, Image: validImage, Provider: Provider{}},
		},
		{
			name: "empty container name prefix",
			options: Options{
				Executor: validExecutor,
				Image:    validImage,
				Provider: Provider{Name: "claude"},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tc.options)
			if err == nil {
				t.Fatalf("New(%s) error = nil, want non-nil", tc.name)
			}
		})
	}
}

// TestRunNoCommandOverridePreservesEntrypoint verifies that when no explicit
// command override is supplied, the resulting ContainerRunRequest carries no
// `--entrypoint` argument in Extra. The provider image's baked-in entrypoint
// must be preserved.
func TestRunNoCommandOverridePreservesEntrypoint(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			prepared := newPreparedFixture(
				map[string]string{"HOME": "/home/valv"},
				[]string{"LANG"},
				[]docker.MountSpec{docker.NewMountSpec("/host/.provider", "/home/valv/.provider", false)},
				nil,
			)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        []string{"--resume"},
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			for _, token := range executor.got.Extra {
				if token == "--entrypoint" {
					t.Fatalf("Run() Extra = %v, want no --entrypoint token without command override", executor.got.Extra)
				}
			}
			if !reflect.DeepEqual(executor.got.Args, []string{"--resume"}) {
				t.Fatalf("Run() Args = %v, want [--resume]", executor.got.Args)
			}
			if got := prepared.Closed(); got != 1 {
				t.Fatalf("Closed() = %d, want 1 (close on success)", got)
			}
		})
	}
}

// TestRunWithCommandOverrideInjectsEntrypoint verifies that an explicit command
// override produces `--entrypoint <command[0]>` in Extra and forwards remaining
// tokens as container args.
func TestRunWithCommandOverrideInjectsEntrypoint(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			prepared := newPreparedFixture(
				map[string]string{"HOME": "/home/valv"},
				nil,
				nil,
				nil,
			)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Command:     []string{"bash", "-lc", "echo hi"},
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			wantExtra := []string{"--entrypoint", "bash"}
			if !reflect.DeepEqual(executor.got.Extra, wantExtra) {
				t.Fatalf("Run() Extra = %v, want %v", executor.got.Extra, wantExtra)
			}
			wantArgs := []string{"-lc", "echo hi"}
			if !reflect.DeepEqual(executor.got.Args, wantArgs) {
				t.Fatalf("Run() Args = %v, want %v", executor.got.Args, wantArgs)
			}
		})
	}
}

// TestRunEnvPassthroughFlowsFromPrepared verifies that EnvPassthrough names
// from the prepared runtime flow into ContainerRunRequest.EnvPassthrough
// unchanged.
func TestRunEnvPassthroughFlowsFromPrepared(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			passthrough := []string{"COLORTERM", "LANG", "LC_CTYPE"}
			prepared := newPreparedFixture(nil, passthrough, nil, nil)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if !reflect.DeepEqual(executor.got.EnvPassthrough, passthrough) {
				t.Fatalf("Run() EnvPassthrough = %v, want %v", executor.got.EnvPassthrough, passthrough)
			}
		})
	}
}

// TestRunPropagatesWarningsToNotices verifies that prepared.Warnings are
// written to the notices sink with the provider-specific prefix.
func TestRunPropagatesWarningsToNotices(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			notices := &recordingNotices{}
			service := newServiceForTest(t, provider, executor, notices)

			warnings := []string{"first warning", "second warning"}
			prepared := newPreparedFixture(nil, nil, nil, warnings)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			output := notices.buf.String()
			for _, warning := range warnings {
				if !strings.Contains(output, warning) {
					t.Fatalf("notices output = %q, want to contain %q", output, warning)
				}
			}
			if !strings.Contains(output, provider.NoticePrefix) {
				t.Fatalf("notices output = %q, want to contain prefix %q", output, provider.NoticePrefix)
			}
		})
	}
}

// TestRunMountsExactEqualityProjectRootThenPrepared verifies that
// request.Mounts equals projectRootMount followed by prepared.Mounts in
// byte-for-byte order. No extra default mounts are inserted by the shared
// service.
func TestRunMountsExactEqualityProjectRootThenPrepared(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			preparedMounts := []docker.MountSpec{
				docker.NewMountSpec("/host/.provider", "/home/valv/.provider", false),
				docker.NewMountSpec("/host/.other", "/home/valv/.other", false),
				docker.NewMountSpec("/host/worktree-gitdir", "/host/worktree-gitdir", false),
			}
			prepared := newPreparedFixture(nil, nil, preparedMounts, nil)

			projectRoot := "/tmp/project"
			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: projectRoot,
				WorkingDir:  projectRoot,
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			want := append(
				[]docker.MountSpec{docker.NewMountSpec(projectRoot, projectRoot, false)},
				preparedMounts...,
			)
			if !reflect.DeepEqual(executor.got.Mounts, want) {
				t.Fatalf("Run() Mounts = %+v, want exact equality %+v", executor.got.Mounts, want)
			}
		})
	}
}

// TestRunRejectsWorkingDirectoryOutsideProjectRoot verifies the within-project
// guard rejects a working directory that does not nest under the project root.
func TestRunRejectsWorkingDirectoryOutsideProjectRoot(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			prepared := newPreparedFixture(nil, nil, nil, nil)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/other",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err == nil || !strings.Contains(err.Error(), "outside project root") {
				t.Fatalf("Run() error = %v, want 'outside project root' rejection", err)
			}
		})
	}
}

// TestRunRejectsSiblingPathThatSharesProjectPrefix verifies a sibling path
// (/tmp/project2 vs /tmp/project) is also rejected.
func TestRunRejectsSiblingPathThatSharesProjectPrefix(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			prepared := newPreparedFixture(nil, nil, nil, nil)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project2",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err == nil || !strings.Contains(err.Error(), "outside project root") {
				t.Fatalf("Run() error = %v, want sibling rejection", err)
			}
		})
	}
}

// TestRunPreparedCloseRunsOnExecutorFailure verifies that prepared.Close()
// fires even when the executor returns an error (defer-style cleanup).
func TestRunPreparedCloseRunsOnExecutorFailure(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{runErr: errors.New("docker exploded")}
			service := newServiceForTest(t, provider, executor, nil)

			prepared := newPreparedFixture(nil, nil, nil, nil)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err == nil {
				t.Fatalf("Run() error = nil, want executor failure to bubble")
			}
			if !strings.Contains(err.Error(), "docker exploded") {
				t.Fatalf("Run() error = %v, want wrapped docker failure", err)
			}
			if got := prepared.Closed(); got != 1 {
				t.Fatalf("Closed() = %d, want 1 (close on failure)", got)
			}
		})
	}
}

// TestRunPropagatesProviderLabels verifies the provider, managed, project, and
// profile labels survive the shared service path.
func TestRunPropagatesProviderLabels(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			prepared := newPreparedFixture(nil, nil, nil, nil)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-id-9",
				ProfileID:   "profile-id-3",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			labels := executor.got.Labels
			if labels["io.valv.managed"] != "true" {
				t.Errorf("io.valv.managed = %q, want true", labels["io.valv.managed"])
			}
			if labels["io.valv.provider"] != provider.Name {
				t.Errorf("io.valv.provider = %q, want %q", labels["io.valv.provider"], provider.Name)
			}
			if labels["io.valv.scope"] != "interactive" {
				t.Errorf("io.valv.scope = %q, want interactive", labels["io.valv.scope"])
			}
			if labels["io.valv.project_id"] != "proj-id-9" {
				t.Errorf("io.valv.project_id = %q, want proj-id-9", labels["io.valv.project_id"])
			}
			if labels["io.valv.profile_id"] != "profile-id-3" {
				t.Errorf("io.valv.profile_id = %q, want profile-id-3", labels["io.valv.profile_id"])
			}
			if !strings.HasPrefix(executor.got.Name, provider.ContainerNamePrefix+"-") {
				t.Errorf("container name = %q, want prefix %q-", executor.got.Name, provider.ContainerNamePrefix)
			}
		})
	}
}

// TestRunSuppressesNoticesOnTTY verifies that warnings are NOT written to the
// notices sink when TTY is enabled, mirroring the current claude/codex
// emitNotices behavior. Warnings are still recorded in logger debug calls but
// the visible-output sink stays clean for the steady-state attached path.
func TestRunSuppressesNoticesOnTTY(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			notices := &recordingNotices{}
			service, err := New(Options{
				Executor: executor,
				Image:    docker.NewImageRef("valv-"+provider.Name, "dev"),
				TTY:      true,
				Stdin:    true,
				Logger:   log.NewWithOptions(io.Discard, log.Options{Level: log.DebugLevel}),
				Notices:  notices,
				Now:      func() time.Time { return time.Unix(0, 42) },
				Provider: provider,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			prepared := newPreparedFixture(nil, nil, nil, []string{"runtime warning"})

			if err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if notices.buf.Len() != 0 {
				t.Fatalf("notices output = %q, want empty under TTY", notices.buf.String())
			}
		})
	}
}

// TestRunBuildsInteractiveFlagsFromOptions verifies TTY/Stdin/Init are wired
// into the request from Options.
func TestRunBuildsInteractiveFlagsFromOptions(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service, err := New(Options{
				Executor: executor,
				Image:    docker.NewImageRef("valv-"+provider.Name, "dev"),
				TTY:      true,
				Stdin:    true,
				Logger:   log.NewWithOptions(io.Discard, log.Options{Level: log.DebugLevel}),
				Now:      func() time.Time { return time.Unix(0, 42) },
				Provider: provider,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			prepared := newPreparedFixture(nil, nil, nil, nil)

			if err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if !executor.got.TTY {
				t.Errorf("TTY = false, want true")
			}
			if !executor.got.Interactive {
				t.Errorf("Interactive = false, want true")
			}
			if !executor.got.Init {
				t.Errorf("Init = false, want true")
			}
			if !executor.got.Remove {
				t.Errorf("Remove = false, want true")
			}
		})
	}
}

// TestRunValidatesLaunchRequest verifies missing required LaunchRequest fields
// produce errors rather than nil-deref or silent success.
func TestRunValidatesLaunchRequest(t *testing.T) {
	t.Parallel()

	provider := providerDescriptors()[0]
	executor := &fakeExecutor{}
	service := newServiceForTest(t, provider, executor, nil)

	prepared := newPreparedFixture(nil, nil, nil, nil)

	cases := []struct {
		name    string
		request LaunchRequest
	}{
		{
			name: "empty project root",
			request: LaunchRequest{
				WorkingDir: "/tmp/project",
				ProjectID:  "p1",
				ProfileID:  "f1",
				Prepared:   &prepared.runtime,
			},
		},
		{
			name: "empty working dir",
			request: LaunchRequest{
				ProjectRoot: "/tmp/project",
				ProjectID:   "p1",
				ProfileID:   "f1",
				Prepared:    &prepared.runtime,
			},
		},
		{
			name: "nil prepared",
			request: LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "p1",
				ProfileID:   "f1",
				Prepared:    nil,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := service.Run(context.Background(), tc.request)
			if err == nil {
				t.Fatalf("Run() error = nil, want validation error")
			}
		})
	}
}
