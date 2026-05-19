package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/config"
)

func TestCodexCommandPassesArgsThroughUnchanged(t *testing.T) {
	t.Parallel()

	var got []string
	cmd := newCodexCommand(config.Paths{}, func(_ *cobra.Command, args []string) error {
		got = append([]string(nil), args...)
		return nil
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"resume", "session-123", "--model", "gpt-5-codex"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := []string{"resume", "session-123", "--model", "gpt-5-codex"}
	if len(got) != len(want) {
		t.Fatalf("Execute() args len = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("Execute() args[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestRunCodexCommandReturnsUnboundProjectWhenNoProjectRecordExists(t *testing.T) {
	paths := testCodexPaths(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	projectDir := t.TempDir()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err = cmd.RunE(cmd, []string{"resume", "session-123"})
	// After 8.5, the 0-accounts path returns unboundProjectNoAccountsError which
	// does not wrap ErrUnboundProject — same behavior as Claude (8.4).
	if err == nil {
		t.Fatal("RunE() error = nil, want error for unbound project with no accounts")
	}
	if !strings.Contains(err.Error(), "project is not bound") {
		t.Fatalf("RunE() error = %q, want 'project is not bound' guidance", err.Error())
	}
}

func TestRunCodexCommandReturnsEnsureError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blockingFile := filepath.Join(root, "blocking")
	if err := os.WriteFile(blockingFile, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	paths := config.Paths{
		DatabaseDir:    blockingFile,
		ProviderRoot:   filepath.Join(root, "providers"),
		StateDir:       filepath.Join(root, "state"),
		ConfigDir:      filepath.Join(root, "config"),
		LogsDir:        filepath.Join(root, "logs"),
		CachesDir:      filepath.Join(root, "caches"),
		BuildCacheDir:  filepath.Join(root, "caches", "build"),
		TempCacheDir:   filepath.Join(root, "caches", "tmp"),
		RuntimeTmpDir:  filepath.Join(root, "runtime"),
		PIDsDir:        filepath.Join(root, "runtime", "pids"),
		LocksDir:       filepath.Join(root, "runtime", "locks"),
		SocketsDir:     filepath.Join(root, "runtime", "sockets"),
		DatabasePath:   filepath.Join(root, "db", "valv.sqlite3"),
		AppSupportRoot: root,
	}

	err := runCodexCommand(newCodexCommand(paths, nil), paths, nil)
	if err == nil {
		t.Fatal("runCodexCommand() error = nil, want ensure failure")
	}
}

func TestCodexImageRefDefaults(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "")
	if got := codexImageRef().String(); got != "valv-codex:dev" {
		t.Fatalf("codexImageRef() = %q, want %q", got, "valv-codex:dev")
	}
}

func TestCodexImageRefParsesOverrideWithTag(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "ghcr.io/example/codex:test-fixture")
	if got := codexImageRef().String(); got != "ghcr.io/example/codex:test-fixture" {
		t.Fatalf("codexImageRef() = %q, want override image", got)
	}
}

func TestCodexImageVersionRefUsesRepositoryAndDashedVersionTag(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "ghcr.io/example/codex:test-fixture")
	if got := codexImageVersionRef("0.117.0").String(); got != "ghcr.io/example/codex:0-117-0" {
		t.Fatalf("codexImageVersionRef() = %q, want dashed version tag", got)
	}
}

func TestCodexArgsSkipProjectBinding(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "empty", args: nil, want: false},
		{name: "help flag", args: []string{"--help"}, want: true},
		{name: "help subcommand", args: []string{"help"}, want: true},
		{name: "nested help flag", args: []string{"resume", "--help"}, want: true},
		{name: "version flag", args: []string{"--version"}, want: true},
		{name: "interactive no args", args: []string{"resume", "session-123"}, want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := codexArgsSkipProjectBinding(tc.args); got != tc.want {
				t.Fatalf("codexArgsSkipProjectBinding(%v) = %t, want %t", tc.args, got, tc.want)
			}
		})
	}
}

func TestCodexArgsSkipAccountReady(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "empty", args: nil, want: false},
		{name: "help", args: []string{"help"}, want: true},
		{name: "login", args: []string{"login"}, want: true},
		{name: "logout", args: []string{"logout"}, want: true},
		{name: "resume", args: []string{"resume", "--last"}, want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := codexArgsSkipAccountReady(tc.args); got != tc.want {
				t.Fatalf("codexArgsSkipAccountReady(%v) = %t, want %t", tc.args, got, tc.want)
			}
		})
	}
}

func TestEnsureCodexAccountReadyForLaunchUsesBoundAccount(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "codex", "work", "--project", projectRoot})

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	stub := installStubCodexAccountAuth(t, cmd, true)

	profile, err := ensureCodexAccountReadyForLaunch(cmd, paths, projectRoot, "", []string{"resume", "--last"})
	if err != nil {
		t.Fatalf("ensureCodexAccountReadyForLaunch() error = %v", err)
	}
	if stub.statusHits == 0 {
		t.Fatal("LoginStatus() hits = 0, want account readiness check")
	}
	if got := stub.homePaths[0]; !strings.Contains(got, "profiles/work") {
		t.Fatalf("LoginStatus() home path = %q, want isolated work profile home", got)
	}
	if profile.Name != "work" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "work")
	}
}

func TestEnsureCodexAccountReadyForLaunchSkipsAccountCheckForLoginCommand(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "codex", "--project", projectRoot})

	cmd := newCodexCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	stub := installStubCodexAccountAuth(t, cmd, true)

	profile, err := ensureCodexAccountReadyForLaunch(cmd, paths, projectRoot, "", []string{"login"})
	if err != nil {
		t.Fatalf("ensureCodexAccountReadyForLaunch(login) error = %v", err)
	}
	if stub.statusHits != 0 {
		t.Fatalf("LoginStatus() hits = %d, want 0 for login passthrough", stub.statusHits)
	}
	if profile.Name == "" {
		t.Fatal("profile.Name empty, want bound account name")
	}
}

func TestEnsureCodexImageAvailableReturnsActionableMessageWhenMissing(t *testing.T) {
	oldFindDockerBinary := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFindDockerBinary })

	err := ensureCodexImageAvailable(context.Background(), stubDockerRunner{
		err: errors.New("run docker image inspect valv-codex-dev:dev: exit status 1: Error response from daemon: No such image: valv-codex-dev:dev"),
	}, dockeradapter.NewImageRef("valv-codex-dev", "dev"))
	if err == nil {
		t.Fatal("ensureCodexImageAvailable() error = nil, want missing image failure")
	}
	for _, want := range []string{"valv-codex-dev:dev", "valv manage update"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ensureCodexImageAvailable() error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestEnsureCodexImageAvailableWrapsUnexpectedInspectErrors(t *testing.T) {
	oldFindDockerBinary := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "/usr/bin/docker", nil }
	t.Cleanup(func() { findDockerBinary = oldFindDockerBinary })

	err := ensureCodexImageAvailable(context.Background(), stubDockerRunner{
		err: errors.New("run docker image inspect valv-codex:dev: permission denied"),
	}, dockeradapter.NewImageRef("valv-codex", "dev"))
	if err == nil {
		t.Fatal("ensureCodexImageAvailable() error = nil, want inspect failure")
	}
	for _, want := range []string{"inspect codex image", "permission denied"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ensureCodexImageAvailable() error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestEnsureCodexImageAvailableSkipsWhenDockerIsUnavailable(t *testing.T) {
	oldFindDockerBinary := findDockerBinary
	findDockerBinary = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { findDockerBinary = oldFindDockerBinary })

	if err := ensureCodexImageAvailable(context.Background(), stubDockerRunner{
		err: errors.New("should not be called"),
	}, dockeradapter.NewImageRef("valv-codex", "dev")); err != nil {
		t.Fatalf("ensureCodexImageAvailable() error = %v, want nil when docker is unavailable", err)
	}
}

func TestRunCodexImageOnlyCommandPassesThroughArgs(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "valv-codex-dev:dev")

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker-run.txt")
	scriptPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\n" +
		"set -eu\n" +
		"case \"$1 $2\" in\n" +
		"  \"image inspect\") exit 0 ;;\n" +
		"  \"run --rm\") printf '%s\\n' \"$@\" > " + shellQuote(logPath) + "; exit 0 ;;\n" +
		"esac\n" +
		"printf '%s\\n' \"$@\" > " + shellQuote(logPath) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newCodexCommand(testCodexPaths(t), nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := runCodexImageOnlyCommand(cmd, testCodexPaths(t), []string{"resume", "--help"}); err != nil {
		t.Fatalf("runCodexImageOnlyCommand() error = %v", err)
	}
	for _, want := range []string{"Checking Codex image", "Codex image ready"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want substring %q", stderr.String(), want)
		}
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath) error = %v", err)
	}
	got := string(content)
	for _, want := range []string{
		"run",
		"--rm",
		"--name",
		"valv-codex-info-",
		"-e",
		"CODEX_HOME=/home/valv/.codex",
		"HOME=/home/valv",
		"USER=valv",
		"--label",
		"io.valv.scope=info",
		"valv-codex-dev:dev",
		"resume",
		"--help",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("docker run args missing %q in %q", want, got)
		}
	}
}

func TestRootDebugFlagIsNotPassedThroughToCodex(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "valv-codex-dev:dev")

	paths := testCodexPaths(t)
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker-run.txt")
	scriptPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\n" +
		"set -eu\n" +
		"case \"$1 $2\" in\n" +
		"  \"image inspect\") exit 0 ;;\n" +
		"  \"run --rm\") printf '%s\\n' \"$@\" > " + shellQuote(logPath) + "; exit 0 ;;\n" +
		"esac\n" +
		"printf '%s\\n' \"$@\" > " + shellQuote(logPath) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd, err := newRootCommandWithPaths(context.Background(), &stdout, &stderr, paths)
	if err != nil {
		t.Fatalf("newRootCommandWithPaths() error = %v", err)
	}
	cmd.SetArgs([]string{"--debug", "codex", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath) error = %v", err)
	}
	if strings.Contains(string(content), "--debug") {
		t.Fatalf("docker args unexpectedly contained root debug flag: %q", string(content))
	}
}

func TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch(t *testing.T) {
	t.Setenv("VALV_CODEX_IMAGE", "valv-codex-dev:dev")
	t.Setenv(valvTestSkipHostCodexLoginEnv, "1")
	// VALV_REAL_HOME controls the shared-home staging path used by codexservice.
	// Without this override, sharedCodexStateHome resolves to $HOME/.codex and
	// PrepareRuntime copies the developer's entire ~/.codex to a tmpfs-backed
	// t.TempDir(), which fails with "no space left on device" when the dev's
	// ~/.codex is large. Pointing VALV_REAL_HOME at a fresh empty temp dir
	// ensures the staging step copies nothing, eliminating the disk-space
	// dependency entirely.
	t.Setenv("VALV_REAL_HOME", t.TempDir())

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Join(projectRoot, ".git"), err)
	}
	runManage(t, paths, []string{"account", "add", "codex", "--project", projectRoot, "--skip-login"})

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "docker-run.txt")
	scriptPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\n" +
		"set -eu\n" +
		"case \"$1 $2\" in\n" +
		"  \"image inspect\") exit 0 ;;\n" +
		"  \"run --rm\") printf '%s\\n' \"$@\" > " + shellQuote(logPath) + "; exit 0 ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(docker) error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Chdir(%q) error = %v", projectRoot, err)
	}
	defer func() {
		_ = os.Chdir(prevWD)
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd, err := newRootCommandWithPaths(context.Background(), &stdout, &stderr, paths)
	if err != nil {
		t.Fatalf("newRootCommandWithPaths() error = %v", err)
	}
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--debug", "codex"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", logPath, err)
	}
	if bytes.Contains(got, []byte("--debug")) {
		t.Fatalf("docker run args unexpectedly contained root debug flag: %q", string(got))
	}
}

func shellQuote(value string) string {
	replacer := strings.NewReplacer("'", "'\"'\"'")
	return "'" + replacer.Replace(value) + "'"
}

type stubDockerRunner struct {
	err error
}

func (s stubDockerRunner) Run(context.Context, []string) error {
	return s.err
}

func testCodexPaths(t *testing.T) config.Paths {
	t.Helper()

	root := t.TempDir()
	return config.Paths{
		HomeDir:        root,
		AppSupportRoot: root,
		DatabaseDir:    filepath.Join(root, "db"),
		DatabasePath:   filepath.Join(root, "db", "valv.sqlite3"),
		ProviderRoot:   filepath.Join(root, "providers"),
		StateDir:       filepath.Join(root, "state"),
		ConfigDir:      filepath.Join(root, "config"),
		LogsDir:        filepath.Join(root, "logs"),
		CachesDir:      filepath.Join(root, "caches"),
		BuildCacheDir:  filepath.Join(root, "caches", "build"),
		TempCacheDir:   filepath.Join(root, "caches", "tmp"),
		RuntimeTmpDir:  filepath.Join(root, "runtime"),
		PIDsDir:        filepath.Join(root, "runtime", "pids"),
		LocksDir:       filepath.Join(root, "runtime", "locks"),
		SocketsDir:     filepath.Join(root, "runtime", "sockets"),
	}
}
