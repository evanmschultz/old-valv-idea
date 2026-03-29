package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	binding, err := store.BindingByProjectID(context.Background(), project.ID)
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
