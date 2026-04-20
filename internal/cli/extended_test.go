package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	sqliteadapter "github.com/evanmschultz/valv/internal/adapters/sqlite"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	imagesservice "github.com/evanmschultz/valv/internal/services/images"
	manageservice "github.com/evanmschultz/valv/internal/services/manage"
)

type staticCLIResolver string

func (r staticCLIResolver) LatestVersion(context.Context) (string, error) {
	return string(r), nil
}

func stubCodexVersionResolver(t *testing.T, version string) {
	t.Helper()
	previous := codexVersionResolverFactory
	codexVersionResolverFactory = func(*http.Client) imagesservice.VersionResolver {
		return staticCLIResolver(version)
	}
	t.Cleanup(func() {
		codexVersionResolverFactory = previous
	})
}

type statusStubService struct {
	status manageservice.StatusResult
	err    error
}

func (s statusStubService) Status(context.Context, string) (manageservice.StatusResult, error) {
	return s.status, s.err
}

func TestManageCommandWithoutTTYShowsHelp(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Operator workflows", "account", "cleanup"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected manage help output %q missing %q", stdout.String(), want)
		}
	}
}

func TestRunManageHomeWithoutTTYShowsHelp(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	var stdout bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := runManageHome(cmd, paths, &rootOptions{}); err != nil {
		t.Fatalf("runManageHome() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Operator workflows") {
		t.Fatalf("unexpected manage home help output: %q", stdout.String())
	}
}

func TestManageAccountListOutputsStoredAccounts(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	profileHomeA := filepath.Join(paths.ProviderRoot, "codex", "profiles", "alpha")
	profileHomeB := filepath.Join(paths.ProviderRoot, "codex", "profiles", "beta")

	runManage(t, paths, []string{"account", "add", "codex", "alpha", "--home", profileHomeA})
	runManage(t, paths, []string{"account", "add", "codex", "beta", "--home", profileHomeB})
	writeTestCodexAuth(t, profileHomeA, "alpha@example.com", "Alpha Example")
	writeTestCodexAuth(t, profileHomeB, "beta@example.com", "Beta Example")

	output := runManage(t, paths, []string{"account", "list", "codex"})
	for _, want := range []string{"codex accounts", "- alpha", "- beta", "auth=ChatGPT", "email=alpha@example.com", "email=beta@example.com", "home="} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected account list output %q missing %q", output, want)
		}
	}
}

func TestManageAccountListWithoutProviderGroupsByProvider(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "alpha")
	runManage(t, paths, []string{"account", "add", "codex", "alpha", "--home", profileHome})
	writeTestCodexAuth(t, profileHome, "alpha@example.com", "Alpha Example")

	output := runManage(t, paths, []string{"account", "list"})
	for _, want := range []string{"codex accounts", "- alpha", "auth=ChatGPT", "email=alpha@example.com", "home="} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected grouped account list output %q missing %q", output, want)
		}
	}
}

func TestManageAccountListJSONUsesCommandKey(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "alpha")
	runManage(t, paths, []string{"account", "add", "codex", "alpha", "--home", profileHome})
	writeTestCodexAuth(t, profileHome, "alpha@example.com", "Alpha Example")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	opts := &rootOptions{format: "json"}
	cmd := newManageCommand(paths, opts)
	cmd.PersistentFlags().StringVar(&opts.format, "format", "json", "output format: auto, human, plain, json")
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--format", "json", "account", "list", "codex"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	wantHome, err := filepath.EvalSymlinks(profileHome)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", profileHome, err)
	}
	want := fmt.Sprintf("{\n  \"accounts\": [\n    {\n      \"title\": \"alpha\",\n      \"fields\": [\n        {\n          \"label\": \"provider\",\n          \"value\": \"codex\",\n          \"muted\": true\n        },\n        {\n          \"label\": \"auth\",\n          \"value\": \"ChatGPT\",\n          \"muted\": true\n        },\n        {\n          \"label\": \"email\",\n          \"value\": \"alpha@example.com\"\n        },\n        {\n          \"label\": \"home\",\n          \"value\": %q,\n          \"identifier\": true\n        }\n      ]\n    }\n  ]\n}\n", wantHome)
	if output != want {
		t.Fatalf("unexpected account list json output:\n got: %q\nwant: %q", output, want)
	}
}

func TestManageAccountListShowsEmptyState(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	output := runManage(t, paths, []string{"account", "list", "codex"})
	for _, want := range []string{"codex accounts", "(none)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected empty account list output %q missing %q", output, want)
		}
	}
}

func TestManageAccountSwitchRebindsCurrentProject(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	alphaHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "alpha")
	betaHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "beta")
	runManage(t, paths, []string{"account", "add", "codex", "alpha", "--home", alphaHome})
	runManage(t, paths, []string{"account", "add", "codex", "beta", "--home", betaHome})
	runManage(t, paths, []string{"bind", "codex", "alpha", "--project", projectRoot})

	output := runManage(t, paths, []string{"account", "switch", "beta", "--project", projectRoot})
	if !strings.Contains(output, "account=beta") {
		t.Fatalf("unexpected account switch output: %q", output)
	}

	status := runManage(t, paths, []string{"status", "--project", projectRoot})
	if !strings.Contains(status, "account=beta") {
		t.Fatalf("unexpected status after account switch: %q", status)
	}
}

func TestManageAccountSwitchMissingAccountShowsActionableGuidance(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	runManage(t, paths, []string{"account", "add", "codex", "--project", projectRoot})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"account", "switch", "work", "--project", projectRoot})
	installStubCodexAccountAuth(t, cmd, true)

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "run `valv manage account add codex work`") {
		t.Fatalf("Execute() error = %v, want missing-account guidance", err)
	}
}

func TestManageAccountInspectShowsIdentityAndBindingCount(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "personal")
	runManage(t, paths, []string{"account", "add", "codex", "personal", "--home", profileHome, "--project", projectRoot})
	writeTestCodexAuth(t, profileHome, "person@example.com", "Person Example")

	output := runManage(t, paths, []string{"account", "inspect", "personal", "--project", projectRoot})
	for _, want := range []string{"Account details", "account=personal", "auth=ChatGPT", "email=person@example.com", "bound_projects=1"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected account inspect output %q missing %q", output, want)
		}
	}
}

func TestManageAccountRenameKeepsBindingByProfileID(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "personal")
	runManage(t, paths, []string{"account", "add", "codex", "personal", "--home", profileHome, "--project", projectRoot})

	output := runManage(t, paths, []string{"account", "rename", "personal", "hylla"})
	if !strings.Contains(output, "account=hylla") {
		t.Fatalf("unexpected account rename output: %q", output)
	}

	status := runManage(t, paths, []string{"status", "--project", projectRoot})
	if !strings.Contains(status, "account=hylla") {
		t.Fatalf("unexpected status after rename: %q", status)
	}
}

func TestManageAccountDeleteRejectsBoundAccount(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "personal")
	runManage(t, paths, []string{"account", "add", "codex", "personal", "--home", profileHome, "--project", projectRoot})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"account", "delete", "personal"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "still bound to project paths") {
		t.Fatalf("Execute() error = %v, want bound-project failure", err)
	}
}

func TestManageProjectListShowsBoundProjects(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "personal")
	runManage(t, paths, []string{"account", "add", "codex", "personal", "--home", profileHome, "--project", projectRoot})
	writeTestCodexAuth(t, profileHome, "person@example.com", "Person Example")

	output := runManage(t, paths, []string{"project", "list", "codex"})
	for _, want := range []string{projectRoot, "account=personal", "auth=ChatGPT", "email=person@example.com"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected project list output %q missing %q", output, want)
		}
	}
}

func TestManageAccountLoginUsesExistingAccount(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "hylla")
	runManage(t, paths, []string{"account", "add", "codex", "hylla", "--home", profileHome, "--skip-login", "--no-bind"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"account", "login", "hylla"})
	stub := installStubCodexAccountAuth(t, cmd, false)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stub.loginHits != 1 {
		t.Fatalf("Login() hits = %d, want 1", stub.loginHits)
	}
}

func TestManageAccountLogoutUsesExistingAccount(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "hylla")
	runManage(t, paths, []string{"account", "add", "codex", "hylla", "--home", profileHome, "--skip-login", "--no-bind"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"account", "logout", "hylla"})
	stub := installStubCodexAccountAuth(t, cmd, true)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stub.loggedIn {
		t.Fatal("loggedIn = true, want false after logout")
	}
}

func TestManageAccountCleanupRemovesLegacyAlias(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	hostHome := filepath.Join(paths.HomeDir, ".codex")
	runManage(t, paths, []string{"account", "add", "codex", "personal", "--home", hostHome, "--skip-login", "--no-bind"})
	store, err := sqliteadapter.NewStore(paths.DatabasePath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	hostAlias, err := domain.NewProfile(domain.ProviderCodex, "host", hostHome)
	if err != nil {
		t.Fatalf("NewProfile(host) error = %v", err)
	}
	if _, err := store.CreateProfile(context.Background(), hostAlias); err != nil {
		t.Fatalf("CreateProfile(host) error = %v", err)
	}

	output := runManage(t, paths, []string{"account", "cleanup", "codex"})
	if !strings.Contains(output, "deleted=host") {
		t.Fatalf("unexpected account cleanup output: %q", output)
	}

	listOutput := runManage(t, paths, []string{"account", "list", "codex"})
	if strings.Contains(listOutput, "- host") {
		t.Fatalf("unexpected account list output after cleanup: %q", listOutput)
	}
}

func TestRunManageBindInteractiveShowsGuidanceWhenNoAccountsExist(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runManageBindInteractive(cmd, paths, &rootOptions{})
	if err == nil || !strings.Contains(err.Error(), "run `valv manage account add codex` for the default host-backed account") {
		t.Fatalf("runManageBindInteractive() error = %v, want account guidance", err)
	}
}

func TestPickProfileWithoutTTYRequiresExplicitAccount(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err := pickProfile(cmd, domain.ProviderCodex, []domain.Profile{{Name: "account-name", Provider: domain.ProviderCodex}})
	if err == nil || !strings.Contains(err.Error(), "account is required when not running in a TTY") {
		t.Fatalf("pickProfile() error = %v, want non-tty guidance", err)
	}
}

func TestManageUpdateUsesFakeDockerAndWritesBuildContext(t *testing.T) {
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)
	stubCodexVersionResolver(t, "0.117.0")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"update"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Provider image updated") {
		t.Fatalf("unexpected update output: %q", stdout.String())
	}
	for _, want := range []string{"Checking provider image", "Provider image check complete"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want substring %q", stderr.String(), want)
		}
	}

	dockerfilePath := filepath.Join(paths.BuildCacheDir, string(domain.ProviderCodex), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		t.Fatalf("Stat(%q) error = %v", dockerfilePath, err)
	}

	logContent := mustReadFile(t, logPath)
	for _, want := range []string{"build", "--build-arg CODEX_VERSION=0.117.0", "-t valv-codex:dev", "-t valv-codex:0-117-0"} {
		if !strings.Contains(logContent, want) {
			t.Fatalf("unexpected docker log %q missing %q", logContent, want)
		}
	}
	for _, want := range []string{"--label io.valv.managed=true", "--label io.valv.provider=codex", "--label io.valv.scope=image", "--label io.valv.version=0.117.0"} {
		if !strings.Contains(logContent, want) {
			t.Fatalf("unexpected docker log %q missing %q", logContent, want)
		}
	}
}

func TestManageUpdateUsesOverrideImageRepository(t *testing.T) {
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)
	stubCodexVersionResolver(t, "0.117.0")
	t.Setenv("VALV_CODEX_IMAGE", "valv-codex-dev:dev")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"update"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}

	logContent := mustReadFile(t, logPath)
	for _, want := range []string{"-t valv-codex-dev:dev", "-t valv-codex-dev:0-117-0"} {
		if !strings.Contains(logContent, want) {
			t.Fatalf("unexpected docker log %q missing %q", logContent, want)
		}
	}
}

func TestManageUpdateSecondRunReportsUpToDate(t *testing.T) {
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)
	stubCodexVersionResolver(t, "0.117.0")
	t.Setenv("VALV_DOCKER_IMAGE_INSPECT_OUTPUT", fakeCodexRecipeHash())

	runManage(t, paths, []string{"update"})
	output := runManage(t, paths, []string{"update"})
	if !strings.Contains(output, "Provider image up to date") {
		t.Fatalf("unexpected second update output: %q", output)
	}

	logContent := mustReadFile(t, logPath)
	if got, want := strings.Count(logContent, "buildx build --load"), 1; got != want {
		t.Fatalf("build count = %d, want %d in log %q", got, want, logContent)
	}
}

func TestManageCleanupAllRemovesLocalStateAndInvokesDocker(t *testing.T) {
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)
	t.Setenv("VALV_DOCKER_PS_OUTPUT", "valv-api-1\nvalv-api-2\n")
	t.Setenv("VALV_DOCKER_IMAGE_LS_OUTPUT", "valv-codex:dev\nvalv-codex:0-117-0\n")
	for _, path := range []string{
		paths.LogsDir,
		paths.BuildCacheDir,
		paths.TempCacheDir,
		paths.RuntimeTmpDir,
		paths.PIDsDir,
		paths.LocksDir,
		paths.SocketsDir,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(path, "marker.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"cleanup"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Cleanup completed") {
		t.Fatalf("unexpected cleanup output: %q", stdout.String())
	}
	for _, want := range []string{"Running all cleanup", "Cleanup complete"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want substring %q", stderr.String(), want)
		}
	}
	for _, want := range []string{"scope=all", "containers=2 removed", "images=2 refs"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected cleanup summary %q missing %q", stdout.String(), want)
		}
	}
	for _, path := range cleanupPaths(paths) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("Stat(%q) error = %v, want not exist", path, err)
		}
	}

	logContent := mustReadFile(t, logPath)
	if !strings.Contains(logContent, "ps -a -q --filter label=io.valv.managed=true") {
		t.Fatalf("unexpected docker cleanup log: %q", logContent)
	}
	if strings.Contains(logContent, "ps -a -q --filter name=valv-") {
		t.Fatalf("unexpected prefix-based docker cleanup log: %q", logContent)
	}
	if !strings.Contains(logContent, "rm --force valv-api-1 valv-api-2") {
		t.Fatalf("unexpected docker cleanup log: %q", logContent)
	}
	if strings.Contains(logContent, "--volumes") {
		t.Fatalf("unexpected destructive volume cleanup log: %q", logContent)
	}
	if !strings.Contains(logContent, "image ls --format {{.Repository}}:{{.Tag}} --filter label=io.valv.managed=true") {
		t.Fatalf("unexpected docker cleanup log: %q", logContent)
	}
	if !strings.Contains(logContent, "image rm --force valv-codex:dev valv-codex:0-117-0") {
		t.Fatalf("unexpected docker cleanup log: %q", logContent)
	}
}

func TestManageCleanupImagesRemovesProviderImagesOnly(t *testing.T) {
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)
	t.Setenv("VALV_DOCKER_IMAGE_LS_OUTPUT", "valv-codex:dev\nvalv-codex:0-117-0\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"cleanup", "images"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "scope=images") {
		t.Fatalf("unexpected cleanup output: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "images=2 refs") {
		t.Fatalf("unexpected cleanup output: %q", stdout.String())
	}

	logContent := mustReadFile(t, logPath)
	if !strings.Contains(logContent, "image rm --force valv-codex:dev valv-codex:0-117-0") {
		t.Fatalf("unexpected docker cleanup log: %q", logContent)
	}
	if strings.Contains(logContent, "builder prune") {
		t.Fatalf("unexpected builder prune in images-only cleanup log: %q", logContent)
	}
}

func TestGlobalSwitchCreatesHostSymlink(t *testing.T) {
	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	runManage(t, paths, []string{"account", "add", "codex", "dev", "--home", profileHome})
	installFakePgrep(t, 1)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newGlobalCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"switch", "codex", "dev"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Global account switched") {
		t.Fatalf("unexpected global switch output: %q", stdout.String())
	}

	target := filepath.Join(paths.HomeDir, ".codex")
	linkTarget, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink(%q) error = %v", target, err)
	}
	wantTarget, err := filepath.EvalSymlinks(profileHome)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", profileHome, err)
	}
	if linkTarget != wantTarget {
		t.Fatalf("Readlink(%q) = %q, want %q", target, linkTarget, wantTarget)
	}
}

func TestEnsureCodexImageCurrentAutoUpdatesWhenNoOverrideIsSet(t *testing.T) {
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)
	stubCodexVersionResolver(t, "0.117.0")

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := ensureCodexImageCurrent(cmd, paths); err != nil {
		t.Fatalf("ensureCodexImageCurrent() error = %v", err)
	}

	logContent := mustReadFile(t, logPath)
	for _, want := range []string{"image inspect valv-codex:dev", "buildx build --load", "--build-arg CODEX_VERSION=0.117.0"} {
		if !strings.Contains(logContent, want) {
			t.Fatalf("unexpected docker log %q missing %q", logContent, want)
		}
	}
}

func TestWriteNoOpRecordOutputsReason(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	effective, err := config.Default().Effective()
	if err != nil {
		t.Fatalf("Default().Effective() error = %v", err)
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), effectiveConfigKey{}, effective))
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := writeNoOpRecord(cmd, &rootOptions{}, "No account switch made", "no account selected"); err != nil {
		t.Fatalf("writeNoOpRecord() error = %v", err)
	}
	for _, want := range []string{"No account switch made", "reason=no account selected"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected no-op output %q missing %q", stdout.String(), want)
		}
	}
}

func TestResolveProfileSwitchTargetUsesCurrentProviderFallback(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	provider, profile, err := resolveProfileSwitchTarget(cmd, statusStubService{
		status: manageservice.StatusResult{
			Binding: domain.ProjectBinding{Provider: domain.ProviderCodex},
		},
	}, "/tmp/project", []string{"alternate-profile"})
	if err != nil {
		t.Fatalf("resolveProfileSwitchTarget() error = %v", err)
	}
	if got, want := provider, domain.ProviderCodex; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
	if got, want := profile, "alternate-profile"; got != want {
		t.Fatalf("profile = %q, want %q", got, want)
	}
}

func TestNewRootCommandWithPathsReturnsCommand(t *testing.T) {
	t.Parallel()

	cmd, err := NewRootCommandWithPaths(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, testCodexPaths(t))
	if err != nil {
		t.Fatalf("NewRootCommandWithPaths() error = %v", err)
	}
	if got, want := cmd.Use, "valv"; got != want {
		t.Fatalf("cmd.Use = %q, want %q", got, want)
	}
}

func TestPickProfileRequiresTTY(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err := pickProfile(cmd, domain.ProviderCodex, []domain.Profile{{Name: "dev", HomePath: "/tmp/dev", Provider: domain.ProviderCodex}})
	if err == nil || !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("pickProfile() error = %v, want TTY failure", err)
	}
}

func TestRunManageBindInteractiveRequiresTTYWhenProfilesExist(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	runManage(t, paths, []string{"account", "add", "codex", "dev", "--home", profileHome})

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runManageBindInteractive(cmd, paths, &rootOptions{})
	if err == nil || !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("runManageBindInteractive() error = %v, want TTY failure", err)
	}
}

func TestRunGlobalSwitchWithoutProfileRequiresTTY(t *testing.T) {
	paths := testCodexPaths(t)
	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	runManage(t, paths, []string{"account", "add", "codex", "dev", "--home", profileHome})
	installFakePgrep(t, 1)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runGlobalSwitch(cmd, paths, &rootOptions{}, domain.ProviderCodex, "")
	if err == nil || !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("runGlobalSwitch() error = %v, want TTY failure", err)
	}
}

func TestListItemsForAccountsIncludesHomeAndProvider(t *testing.T) {
	t.Parallel()

	items := listItemsForAccounts([]domain.Profile{{Provider: domain.ProviderCodex, Name: "dev", HomePath: "/tmp/dev"}})
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Title != "dev" {
		t.Fatalf("Title = %q, want dev", items[0].Title)
	}
	if len(items[0].Fields) != 4 {
		t.Fatalf("len(fields) = %d, want 4", len(items[0].Fields))
	}
}

func TestParseOptionalProviderAndRequireProjectPath(t *testing.T) {
	t.Parallel()

	provider, err := parseOptionalProvider(nil, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("parseOptionalProvider(nil) error = %v", err)
	}
	if provider != domain.ProviderCodex {
		t.Fatalf("provider = %q, want codex", provider)
	}
	if _, err := parseOptionalProvider([]string{"unsupported"}, domain.ProviderCodex); err == nil {
		t.Fatal("parseOptionalProvider() error = nil, want unsupported provider")
	}
	if got := requireProjectPath("  /tmp/project  "); got != "/tmp/project" {
		t.Fatalf("requireProjectPath() = %q, want /tmp/project", got)
	}
}

func cleanupPaths(paths config.Paths) []string {
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

func installFakeDocker(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "docker.log")
	scriptPath := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$VALV_DOCKER_LOG\"\nif [ \"$1\" = \"ps\" ]; then\n  printf '%s' \"$VALV_DOCKER_PS_OUTPUT\"\nfi\nif [ \"$1\" = \"image\" ] && [ \"$2\" = \"ls\" ]; then\n  printf '%s' \"$VALV_DOCKER_IMAGE_LS_OUTPUT\"\nfi\nif [ \"$1\" = \"image\" ] && [ \"$2\" = \"inspect\" ] && [ \"$3\" = \"--format\" ]; then\n  printf '%s' \"$VALV_DOCKER_IMAGE_INSPECT_OUTPUT\"\nfi\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", scriptPath, err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)
	t.Setenv("VALV_DOCKER_LOG", logPath)
	return logPath
}

func fakeCodexRecipeHash() string {
	sum := sha256.Sum256([]byte(imagesservice.DefaultCodexDockerfile()))
	return hex.EncodeToString(sum[:])
}

func installFakePgrep(t *testing.T, exitCode int) {
	t.Helper()

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "pgrep")
	script := fmt.Sprintf("#!/bin/sh\nexit %d\n", exitCode)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", scriptPath, err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}
