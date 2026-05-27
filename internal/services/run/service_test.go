package run

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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

// isCaseInsensitiveFS detects whether the filesystem hosting dir treats names
// in a case-insensitive way. It creates a lowercase probe file and then
// attempts to Stat its uppercase spelling: a successful stat means the volume
// folds case (e.g. the default macOS APFS / HFS+ configuration on
// `/private/tmp`).
func isCaseInsensitiveFS(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "case_probe")
	if err := os.WriteFile(probe, []byte("x"), 0o600); err != nil {
		t.Fatalf("isCaseInsensitiveFS: write probe: %v", err)
	}
	defer os.Remove(probe)
	upper := filepath.Join(dir, "CASE_PROBE")
	if _, err := os.Stat(upper); err == nil {
		return true
	}
	return false
}

// TestRunNormalizesCaseVariantProjectRootBeforeGuard verifies that when the
// caller hands the service a ProjectRoot whose spelling differs only by case
// from a prefix of WorkingDir on a case-insensitive filesystem, the shared
// service normalizes both paths before the within-project guard and the launch
// succeeds rather than being false-rejected.
//
// Repros A1 from BUILDER_QA_FALSIFICATION.md Round 1. Pre-fix: this test
// fails with "outside project root" because withinProjectRoot is lexical-only.
func TestRunNormalizesCaseVariantProjectRootBeforeGuard(t *testing.T) {
	t.Parallel()

	tempBase := t.TempDir()
	if !isCaseInsensitiveFS(t, tempBase) {
		t.Skipf("filesystem at %q is case-sensitive; skipping case-variant guard test", tempBase)
	}

	// Create the canonical lowercase project directory and a nested subdir.
	canonical := filepath.Join(tempBase, "project")
	subdir := filepath.Join(canonical, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", subdir, err)
	}
	// Reference the same project root using a different case spelling for
	// ProjectRoot (lowercase) vs WorkingDir (uppercase prefix). On a
	// case-insensitive filesystem both paths resolve to the same physical
	// directory but lexical filepath.Rel returns "../PROJECT/subdir" and
	// rejects the launch.
	projectRoot := canonical
	workingDir := filepath.Join(tempBase, "PROJECT", "subdir")

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			prepared := newPreparedFixture(nil, nil, nil, nil)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: projectRoot,
				WorkingDir:  workingDir,
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v, want nil (case-variant paths must resolve to same project)", err)
			}
		})
	}
}

// TestRunNormalizesSymlinkedProjectRootBeforeGuard verifies that when
// ProjectRoot is a symlink that resolves to the same physical directory as a
// prefix of WorkingDir, the shared service normalizes both paths before the
// within-project guard and the launch succeeds.
//
// Repros A1 from BUILDER_QA_FALSIFICATION.md Round 1 (symlink variant).
// Pre-fix: this test fails with "outside project root" because
// withinProjectRoot is lexical-only.
func TestRunNormalizesSymlinkedProjectRootBeforeGuard(t *testing.T) {
	t.Parallel()

	tempBase := t.TempDir()

	// Create the real on-disk project directory and a nested subdir.
	realProject := filepath.Join(tempBase, "real-project")
	subdir := filepath.Join(realProject, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", subdir, err)
	}

	// Point a symlink at the real project directory and reference the project
	// root through the symlink spelling. The working directory references the
	// real (non-symlinked) path so the two spellings differ.
	projectLink := filepath.Join(tempBase, "project-link")
	if err := os.Symlink(realProject, projectLink); err != nil {
		t.Skipf("symlink unsupported on this filesystem: %v", err)
	}
	workingDir := filepath.Join(realProject, "subdir")

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			prepared := newPreparedFixture(nil, nil, nil, nil)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: projectLink,
				WorkingDir:  workingDir,
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v, want nil (symlinked project root must resolve to same project)", err)
			}
		})
	}
}

// TestRunNormalizesSymlinkedProjectRootWithMissingLeaf verifies the inode-walk
// fallback peels trailing missing components when the working dir spelling
// goes through a symlink alias whose final component does not yet exist on
// disk. Repros A2 from BUILDER_QA_FALSIFICATION.md Round 2: `pathutil.Normalize`
// falls back to the raw absolute path on `fs.ErrNotExist`, so the symlink alias
// in the working-dir prefix is never collapsed, and the lexical filepath.Rel
// rejects. Pre-fix the inode walk also aborted on the first os.Stat ENOENT
// instead of peeling the missing leaf and matching the existing ancestor.
func TestRunNormalizesSymlinkedProjectRootWithMissingLeaf(t *testing.T) {
	t.Parallel()

	tempBase := t.TempDir()

	// Create the real on-disk project directory. No subdir is created — the
	// working-dir leaf is intentionally missing for this test.
	realProject := filepath.Join(tempBase, "real-project")
	if err := os.MkdirAll(realProject, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", realProject, err)
	}

	// Point a symlink at the real project directory.
	projectLink := filepath.Join(tempBase, "project-link")
	if err := os.Symlink(realProject, projectLink); err != nil {
		t.Skipf("symlink unsupported on this filesystem: %v", err)
	}

	// Working dir spells the project root through the symlink AND adds a
	// missing-leaf component that does not exist on disk.
	workingDir := filepath.Join(projectLink, "missing-child")

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			prepared := newPreparedFixture(nil, nil, nil, nil)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: realProject,
				WorkingDir:  workingDir,
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v, want nil (symlinked project root with missing leaf must resolve to same project)", err)
			}

			// Verify the canonical working dir was rebuilt under the project
			// root spelling, not left as the symlink-aliased input. The
			// in-container working dir must be reachable inside the
			// project-root bind mount Docker receives. macOS resolves
			// /var to /private/var via EvalSymlinks inside the service, so
			// compute the expected path through EvalSymlinks(realProject) as
			// the canonical projectRoot spelling — this is exactly the
			// spelling buildRequest uses for the bind mount source.
			canonicalRoot, err := filepath.EvalSymlinks(realProject)
			if err != nil {
				t.Fatalf("EvalSymlinks(%q): %v", realProject, err)
			}
			wantWorkingDir := filepath.Join(canonicalRoot, "missing-child")
			if executor.got.WorkingDir != wantWorkingDir {
				t.Fatalf("Run() WorkingDir = %q, want %q (canonical rebuild under projectRoot)", executor.got.WorkingDir, wantWorkingDir)
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

// TestRunMergesAccountEnvIntoContainerEnv verifies that when AccountEnv is
// populated with ordinary environment variables, they appear in the container
// request environment.
func TestRunMergesAccountEnvIntoContainerEnv(t *testing.T) {
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

			accountEnv := map[string]string{"API_KEY": "secret"}

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				AccountEnv:  accountEnv,
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if executor.got.Env["API_KEY"] != "secret" {
				t.Fatalf("Run() Env[API_KEY] = %q, want %q", executor.got.Env["API_KEY"], "secret")
			}
			if executor.got.Env["HOME"] != "/home/valv" {
				t.Fatalf("Run() Env[HOME] = %q, want %q", executor.got.Env["HOME"], "/home/valv")
			}
		})
	}
}

// TestRunPreservesRuntimeOwnedEnvOnAccountEnvCollision verifies that when
// AccountEnv and Prepared.Env both define the same key (especially the
// runtime-owned keys CODEX_HOME, CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER),
// Prepared.Env wins.
func TestRunPreservesRuntimeOwnedEnvOnAccountEnvCollision(t *testing.T) {
	t.Parallel()

	runtimeKeys := []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "HOME", "LOGNAME", "TERM", "USER"}

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			for _, key := range runtimeKeys {
				key := key
				t.Run(key, func(t *testing.T) {
					t.Parallel()

					executor := &fakeExecutor{}
					service := newServiceForTest(t, provider, executor, nil)

					prepared := newPreparedFixture(
						map[string]string{key: "runtime"},
						nil,
						nil,
						nil,
					)

					accountEnv := map[string]string{key: "account"}

					err := service.Run(context.Background(), LaunchRequest{
						ProjectRoot: "/tmp/project",
						WorkingDir:  "/tmp/project",
						ProjectID:   "proj-1",
						ProfileID:   "profile-1",
						Prepared:    &prepared.runtime,
						AccountEnv:  accountEnv,
						Args:        nil,
					})
					if err != nil {
						t.Fatalf("Run() error = %v", err)
					}

					if executor.got.Env[key] != "runtime" {
						t.Fatalf("Run() Env[%s] = %q, want %q (prepared must win on collision)", key, executor.got.Env[key], "runtime")
					}
				})
			}
		})
	}
}

// TestRunHandlesNilAccountEnv verifies that when AccountEnv is nil, the
// resulting container environment equals Prepared.Env (existing behavior).
func TestRunHandlesNilAccountEnv(t *testing.T) {
	t.Parallel()

	for _, provider := range providerDescriptors() {
		provider := provider
		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			executor := &fakeExecutor{}
			service := newServiceForTest(t, provider, executor, nil)

			preparedEnv := map[string]string{"HOME": "/home/valv", "USER": "valv"}
			prepared := newPreparedFixture(preparedEnv, nil, nil, nil)

			err := service.Run(context.Background(), LaunchRequest{
				ProjectRoot: "/tmp/project",
				WorkingDir:  "/tmp/project",
				ProjectID:   "proj-1",
				ProfileID:   "profile-1",
				Prepared:    &prepared.runtime,
				AccountEnv:  nil,
				Args:        nil,
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if !reflect.DeepEqual(executor.got.Env, preparedEnv) {
				t.Fatalf("Run() Env = %v, want exact match with Prepared.Env %v", executor.got.Env, preparedEnv)
			}
		})
	}
}
