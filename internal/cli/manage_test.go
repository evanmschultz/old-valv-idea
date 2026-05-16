package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	sqliteadapter "github.com/evanmschultz/valv/internal/adapters/sqlite"
	"github.com/evanmschultz/valv/internal/config"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/pathutil"
)

func TestManageAccountAddCreatesIsolatedNamedAccountAndBindsProject(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	workDir := filepath.Join(projectRoot, "nested")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workDir) error = %v", err)
	}

	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "profile-name")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"account", "add", "codex", "profile-name", "--project", workDir})
	installStubCodexAccountAuth(t, cmd, true)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "account=profile-name") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
	if _, err := os.Stat(profileHome); err != nil {
		t.Fatalf("Stat(%q) error = %v", profileHome, err)
	}

	store, err := sqliteadapter.NewStore(paths.DatabasePath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	profile, err := store.ProfileByName(context.Background(), domain.ProviderCodex, "profile-name")
	if err != nil {
		t.Fatalf("ProfileByName() error = %v", err)
	}
	wantHome, err := pathutil.Normalize(profileHome)
	if err != nil {
		t.Fatalf("Normalize(profileHome) error = %v", err)
	}
	if profile.HomePath != wantHome {
		t.Fatalf("profile home = %q, want %q", profile.HomePath, wantHome)
	}

	normalizedProjectRoot, err := pathutil.Normalize(projectRoot)
	if err != nil {
		t.Fatalf("Normalize(projectRoot) error = %v", err)
	}
	project, err := store.ProjectByRoot(context.Background(), normalizedProjectRoot)
	if err != nil {
		t.Fatalf("ProjectByRoot() error = %v", err)
	}
	binding, err := store.BindingByProjectID(context.Background(), project.ID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("BindingByProjectID() error = %v", err)
	}
	if binding.ProfileID != profile.ID {
		t.Fatalf("binding profile id = %q, want %q", binding.ProfileID, profile.ID)
	}
}

func TestManageAccountAddWithoutNameUsesDefaultHostAccountAndBindsProject(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	output := runManage(t, paths, []string{"account", "add", "codex", "--project", projectRoot})
	wantHome, err := pathutil.Normalize(filepath.Join(paths.HomeDir, ".codex"))
	if err != nil {
		t.Fatalf("Normalize(default home) error = %v", err)
	}
	for _, want := range []string{"Account ready and project bound", "account=default", "home=" + wantHome} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected output %q missing %q", output, want)
		}
	}
}

func TestManageAccountAddWithoutNameReusesExistingSameHomeAccount(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}

	existingHome := filepath.Join(paths.HomeDir, ".codex")
	runManage(t, paths, []string{"account", "add", "codex", "host-codex", "--home", existingHome, "--skip-login", "--no-bind"})

	output := runManage(t, paths, []string{"account", "add", "codex", "--project", projectRoot})
	wantHome, err := pathutil.Normalize(existingHome)
	if err != nil {
		t.Fatalf("Normalize(existingHome) error = %v", err)
	}
	for _, want := range []string{"Account ready and project bound", "account=host-codex", "home=" + wantHome} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected output %q missing %q", output, want)
		}
	}
}

func TestManageAccountAddNoBindSkipsProjectBinding(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	output := runManage(t, paths, []string{"account", "add", "codex", "--no-bind"})
	wantHome, err := pathutil.Normalize(filepath.Join(paths.HomeDir, ".codex"))
	if err != nil {
		t.Fatalf("Normalize(default home) error = %v", err)
	}
	for _, want := range []string{"Account ready", "account=default", "home=" + wantHome} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected no-bind output %q missing %q", output, want)
		}
	}
}

func TestManageBindAndStatusUseRealStoreAndProjectDetection(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	projectRoot := filepath.Join(t.TempDir(), "project")
	workDir := filepath.Join(projectRoot, "nested")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workDir) error = %v", err)
	}

	profileHome := filepath.Join(paths.ProviderRoot, "codex", "profiles", "dev")
	runManage(t, paths, []string{"account", "add", "codex", "dev", "--home", profileHome, "--skip-login", "--no-bind"})
	writeTestCodexAuth(t, profileHome, "developer@example.com", "Developer Example")
	runManage(t, paths, []string{"bind", "codex", "dev", "--project", workDir})

	statusOut := runManage(t, paths, []string{"status", "--project", workDir})
	if !strings.Contains(statusOut, "account=dev") {
		t.Fatalf("unexpected status output: %q", statusOut)
	}
	if !strings.Contains(statusOut, "provider=codex") {
		t.Fatalf("unexpected status output: %q", statusOut)
	}
	if !strings.Contains(statusOut, "auth=ChatGPT") {
		t.Fatalf("unexpected status output: %q", statusOut)
	}
	if !strings.Contains(statusOut, "email=developer@example.com") {
		t.Fatalf("unexpected status output: %q", statusOut)
	}
}

func TestManageAccountHelpSubcommandWorks(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"account", "help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Manage Valv provider accounts", "add", "list"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unexpected account help output %q missing %q", stdout.String(), want)
		}
	}
}

func TestManageProfileAliasNoLongerWorks(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"profile", "help"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("Execute() error = %v, want unknown command", err)
	}
}

func runManage(t *testing.T, paths config.Paths, args []string) string {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	installStubCodexAccountAuth(t, cmd, true)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(%v) error = %v\nstderr=%s", args, err, stderr.String())
	}
	return stdout.String()
}

func writeTestCodexAuth(t *testing.T, homePath, email, name string) {
	t.Helper()
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", homePath, err)
	}
	token := testJWT(t, map[string]string{"email": email, "name": name})
	payload := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]string{
			"id_token": token,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(auth payload) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(homePath, "auth.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile(auth.json) error = %v", err)
	}
}

func TestRunManageUpdateClaudeBuildsImage(t *testing.T) {
	const stubbedVersion = "2.2.0"
	paths := testCodexPaths(t)
	logPath := installFakeDocker(t)
	stubClaudeVersionResolver(t, stubbedVersion)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"update", "claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Provider image built") {
		t.Fatalf("unexpected update output: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "provider=claude") {
		t.Fatalf("unexpected update output %q missing provider=claude", stdout.String())
	}
	if !strings.Contains(stdout.String(), "image=valv-claude:dev") {
		t.Fatalf("unexpected update output %q missing image=valv-claude:dev", stdout.String())
	}
	if !strings.Contains(stdout.String(), "version="+stubbedVersion) {
		t.Fatalf("unexpected update output %q missing version=%s", stdout.String(), stubbedVersion)
	}
	if !strings.Contains(stdout.String(), "checked_at=") {
		t.Fatalf("unexpected update output %q missing checked_at field", stdout.String())
	}
	for _, want := range []string{"Checking provider image", "Provider image check complete"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want substring %q", stderr.String(), want)
		}
	}

	dockerfilePath := filepath.Join(paths.BuildCacheDir, string(domain.ProviderClaude), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		t.Fatalf("Stat(%q) error = %v", dockerfilePath, err)
	}

	logContent := mustReadFile(t, logPath)
	wantBuildArg := fmt.Sprintf("--build-arg CLAUDE_VERSION=%s", stubbedVersion)
	for _, want := range []string{"buildx build", wantBuildArg, "-t valv-claude:dev", "--label io.valv.provider=claude"} {
		if !strings.Contains(logContent, want) {
			t.Fatalf("unexpected docker log %q missing %q", logContent, want)
		}
	}
}

func TestRunManageUpdateCodexRegression(t *testing.T) {
	for _, args := range [][]string{{"update"}, {"update", "codex"}} {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			paths := testCodexPaths(t)
			logPath := installFakeDocker(t)
			stubCodexVersionResolver(t, "0.117.0")

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd := newManageCommand(paths, &rootOptions{})
			cmd.SetContext(context.Background())
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute(%v) error = %v\nstderr=%s", args, err, stderr.String())
			}
			if !strings.Contains(stdout.String(), "Provider image updated") {
				t.Fatalf("args=%v unexpected update output: %q", args, stdout.String())
			}
			if !strings.Contains(stdout.String(), "provider=codex") {
				t.Fatalf("args=%v output %q missing provider=codex", args, stdout.String())
			}
			for _, want := range []string{"Checking provider image", "Provider image check complete"} {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("args=%v stderr = %q, want substring %q", args, stderr.String(), want)
				}
			}

			dockerfilePath := filepath.Join(paths.BuildCacheDir, string(domain.ProviderCodex), "Dockerfile")
			if _, err := os.Stat(dockerfilePath); err != nil {
				t.Fatalf("args=%v Stat(%q) error = %v", args, dockerfilePath, err)
			}

			logContent := mustReadFile(t, logPath)
			for _, want := range []string{"build", "--build-arg CODEX_VERSION=0.117.0", "-t valv-codex:dev"} {
				if !strings.Contains(logContent, want) {
					t.Fatalf("args=%v unexpected docker log %q missing %q", args, logContent, want)
				}
			}
		})
	}
}

func TestRunManageUpdateUnsupportedProvider(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := runManageUpdate(cmd, paths, &rootOptions{}, domain.Provider("foo"))
	if err == nil {
		t.Fatalf("runManageUpdate(provider=foo) error = nil, want unsupported-provider error")
	}
	if !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("runManageUpdate(provider=foo) error = %q, want substring %q", err.Error(), "not supported yet")
	}
	if !strings.Contains(err.Error(), `"foo"`) {
		t.Fatalf("runManageUpdate(provider=foo) error = %q, want provider name in error", err.Error())
	}
}

// TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure verifies that when
// --skip-login is passed, the Claude image-ensure guard is skipped and the
// account is created without needing docker or a running container.
// This pins the `!skipLogin` condition on the FIX 1 guard in runManageAccountAdd.
func TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	// No fake docker installed — if ensureClaudeImageCurrent were called without
	// the skipLogin guard, it would invoke openImagesService → Build → docker → fail.

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"account", "add", "claude", "work", "--skip-login", "--no-bind"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(account add claude work --skip-login) error = %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "account=work") {
		t.Fatalf("unexpected output %q missing account=work", stdout.String())
	}
	if !strings.Contains(stdout.String(), "provider=claude") {
		t.Fatalf("unexpected output %q missing provider=claude", stdout.String())
	}
}

// TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds verifies
// the full code path when VALV_CLAUDE_IMAGE is set (bypasses images service to
// use docker inspect) and the profile already has credentials (already-authed
// short-circuit in ensureClaudeAccountReady). A fake docker binary is installed
// so that docker inspect returns 0. This confirms that runManageAccountAdd flows
// through ensureClaudeImageCurrent → ensureManagedAccountReady in sequence.
// This pins FIX 1 (BLOCK 1): image-ensure is reached for Claude before auth.
func TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds(t *testing.T) {
	// VALV_CLAUDE_IMAGE causes ensureClaudeImageCurrent to call ensureClaudeImageAvailable
	// (docker inspect) instead of the images-service build path. installFakeDocker
	// provides a docker binary that exits 0 for all calls.
	t.Setenv("VALV_CLAUDE_IMAGE", "test/claude:fixed")
	installFakeDocker(t)

	paths := testCodexPaths(t)

	// Create the profile home and pre-write credentials so ensureClaudeAccountReady
	// returns nil at the already-authed check without launching a real container.
	profileHome := filepath.Join(paths.ProviderRoot, "claude", "profiles", "preauthed")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", profileHome, err)
	}
	credPath := filepath.Join(profileHome, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiOauth":{"accessToken":"tok"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", credPath, err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newManageCommand(paths, &rootOptions{})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	// Use --home to point at the pre-authed profile home we created above.
	cmd.SetArgs([]string{"account", "add", "claude", "preauthed", "--home", profileHome, "--no-bind"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(account add claude preauthed --home) error = %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "account=preauthed") {
		t.Fatalf("unexpected output %q missing account=preauthed", stdout.String())
	}
}

func testJWT(t *testing.T, claims map[string]string) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("Marshal(header) error = %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal(claims) error = %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
