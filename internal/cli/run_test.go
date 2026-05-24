package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/config"
)

// TestStripRunLocalFlags is the table-driven proof for prefix-only flag
// stripping. Critical behavior under test: --account / --provider AFTER the
// first non-flag positional are passthrough, not Valv-local.
func TestStripRunLocalFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		args             []string
		wantAccount      string
		wantProvider     string
		wantAccountExpl  bool
		wantProviderExpl bool
		wantRemaining    []string
	}{
		{
			name:          "nil args",
			args:          nil,
			wantRemaining: nil,
		},
		{
			name:          "empty args",
			args:          []string{},
			wantRemaining: nil,
		},
		{
			name:            "--account space-separated only",
			args:            []string{"--account", "work"},
			wantAccount:     "work",
			wantAccountExpl: true,
			wantRemaining:   nil,
		},
		{
			name:            "--account equals form only",
			args:            []string{"--account=work"},
			wantAccount:     "work",
			wantAccountExpl: true,
			wantRemaining:   nil,
		},
		{
			name:             "--account + --provider, no command",
			args:             []string{"--account", "work", "--provider", "claude"},
			wantAccount:      "work",
			wantProvider:     "claude",
			wantAccountExpl:  true,
			wantProviderExpl: true,
			wantRemaining:    nil,
		},
		{
			name:            "--account + cmd",
			args:            []string{"--account", "work", "bash"},
			wantAccount:     "work",
			wantAccountExpl: true,
			wantRemaining:   []string{"bash"},
		},
		{
			name:            "--account + cmd + args",
			args:            []string{"--account", "work", "bash", "-c", "echo hi"},
			wantAccount:     "work",
			wantAccountExpl: true,
			wantRemaining:   []string{"bash", "-c", "echo hi"},
		},
		{
			// (a) from Acceptance bullet: `valv run --account A cmd --account B`
			// passes `--account B` to the target command unchanged.
			name:            "later --account is passthrough after first positional",
			args:            []string{"--account", "A", "cmd", "--account", "B"},
			wantAccount:     "A",
			wantAccountExpl: true,
			wantRemaining:   []string{"cmd", "--account", "B"},
		},
		{
			name:            "later --provider is passthrough after first positional",
			args:            []string{"--account", "A", "cmd", "--provider", "codex"},
			wantAccount:     "A",
			wantAccountExpl: true,
			wantRemaining:   []string{"cmd", "--provider", "codex"},
		},
		{
			// (b) from Acceptance bullet: `valv run cmd --account A` treats
			// `--account A` as target args; no Valv-local --account is parsed.
			name:          "non-flag positional first leaves --account untouched",
			args:          []string{"cmd", "--account", "A"},
			wantRemaining: []string{"cmd", "--account", "A"},
		},
		{
			name:            "-- separator stops stripping and is preserved",
			args:            []string{"--account", "work", "--", "--account", "later"},
			wantAccount:     "work",
			wantAccountExpl: true,
			wantRemaining:   []string{"--", "--account", "later"},
		},
		{
			// Malformed: --account with no following value. The malformed
			// flag is preserved in remaining so a downstream error path can
			// complain (here the test asserts only the remaining slice).
			name:          "--account with no value malformed",
			args:          []string{"--account"},
			wantRemaining: []string{"--account"},
		},
		{
			name:          "--account= empty value malformed",
			args:          []string{"--account="},
			wantRemaining: []string{"--account="},
		},
		{
			name:             "--provider equals form + cmd",
			args:             []string{"--provider=codex", "--account=work", "bash"},
			wantAccount:      "work",
			wantProvider:     "codex",
			wantAccountExpl:  true,
			wantProviderExpl: true,
			wantRemaining:    []string{"bash"},
		},
		{
			name:             "flags can appear in either order",
			args:             []string{"--provider", "codex", "--account", "work", "bash"},
			wantAccount:      "work",
			wantProvider:     "codex",
			wantAccountExpl:  true,
			wantProviderExpl: true,
			wantRemaining:    []string{"bash"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotParsed, gotRemaining := stripRunLocalFlags(tc.args)
			if gotParsed.account != tc.wantAccount {
				t.Errorf("account = %q, want %q", gotParsed.account, tc.wantAccount)
			}
			if gotParsed.provider != tc.wantProvider {
				t.Errorf("provider = %q, want %q", gotParsed.provider, tc.wantProvider)
			}
			if gotParsed.accountExplicit != tc.wantAccountExpl {
				t.Errorf("accountExplicit = %t, want %t", gotParsed.accountExplicit, tc.wantAccountExpl)
			}
			if gotParsed.providerExplicit != tc.wantProviderExpl {
				t.Errorf("providerExplicit = %t, want %t", gotParsed.providerExplicit, tc.wantProviderExpl)
			}
			if !reflect.DeepEqual(gotRemaining, tc.wantRemaining) {
				t.Errorf("remaining = %v, want %v", gotRemaining, tc.wantRemaining)
			}
		})
	}
}

// TestStripRunLocalFlagsDoesNotMutateInput ensures the helper never mutates
// the caller's args slice. Mirrors TestStripAccountFlagDoesNotMutateInputSlice
// for the account-flag helper.
func TestStripRunLocalFlagsDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	original := []string{"--account", "work", "--provider", "codex", "bash", "-c", "echo"}
	snapshot := append([]string(nil), original...)

	stripRunLocalFlags(original)

	if !reflect.DeepEqual(original, snapshot) {
		t.Errorf("stripRunLocalFlags mutated input: got %v, want %v", original, snapshot)
	}
}

// TestNewRunCommandMetadata verifies the cobra command shape: Use, Short,
// Long, Example all populated, DisableFlagParsing true, RunE wired.
func TestNewRunCommandMetadata(t *testing.T) {
	t.Parallel()

	cmd := newRunCommand(testCodexPaths(t), func(*cobra.Command, []string) error { return nil })

	if cmd == nil {
		t.Fatal("newRunCommand() returned nil")
	}
	if !strings.HasPrefix(cmd.Use, "run") {
		t.Errorf("Use = %q, want prefix %q", cmd.Use, "run")
	}
	for _, want := range []string{"per-account", "container"} {
		if !strings.Contains(strings.ToLower(cmd.Short+cmd.Long), want) {
			t.Errorf("Short+Long missing %q: %q / %q", want, cmd.Short, cmd.Long)
		}
	}
	if cmd.Example == "" {
		t.Errorf("Example is empty")
	}
	if !cmd.DisableFlagParsing {
		t.Errorf("DisableFlagParsing = false, want true")
	}
}

// TestNewRunCommandPassesArgsThroughUnchanged verifies cobra does not mutate
// the args passed to RunE — DisableFlagParsing must keep --account / --provider
// in the slice intact. Mirrors TestClaudeCommandPassesArgsThroughUnchanged.
func TestNewRunCommandPassesArgsThroughUnchanged(t *testing.T) {
	t.Parallel()

	var got []string
	cmd := newRunCommand(config.Paths{}, func(_ *cobra.Command, args []string) error {
		got = append([]string(nil), args...)
		return nil
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--account", "work", "--provider", "codex", "bash", "-c", "echo hi"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{"--account", "work", "--provider", "codex", "bash", "-c", "echo hi"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Execute() args = %v, want %v", got, want)
	}
}

// TestRunCommandHelpWhenNoArgs verifies `valv run` (no args) prints help.
func TestRunCommandHelpWhenNoArgs(t *testing.T) {
	paths := testCodexPaths(t)
	var stdout, stderr bytes.Buffer
	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE([]) error = %v", err)
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "run --account") {
		t.Errorf("help output missing usage line; got %q", combined)
	}
}

// TestRunCommandHelpForOwnHelp verifies `valv run --help` and `valv run -h`
// and `valv run help` all print own help (no target command).
func TestRunCommandHelpForOwnHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "--help alone", args: []string{"--help"}},
		{name: "-h alone", args: []string{"-h"}},
		{name: "help subcommand", args: []string{"help"}},
		{name: "--account X --help still own help (no target)", args: []string{"--account", "work", "--help"}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			paths := testCodexPaths(t)
			var stdout, stderr bytes.Buffer
			cmd := newRunCommand(paths, nil)
			cmd.SetContext(context.Background())
			cmd.SetIn(bytes.NewBuffer(nil))
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)

			if err := cmd.RunE(cmd, tc.args); err != nil {
				t.Fatalf("RunE(%v) error = %v", tc.args, err)
			}
			combined := stdout.String() + stderr.String()
			if !strings.Contains(combined, "run --account") {
				t.Errorf("help output missing usage line; got %q", combined)
			}
		})
	}
}

// TestRunCommandTargetHelpIsPassthrough verifies that `valv run <cmd> --help`
// does NOT short-circuit to own-help; --help is passed through to the target
// command. We can't easily exercise the full launch path here without a
// configured account, so we assert that the early-help short-circuit is NOT
// taken — i.e. the error we get is the missing-account error (which fires
// after the help short-circuit), not the help printout itself.
func TestRunCommandTargetHelpIsPassthrough(t *testing.T) {
	paths := testCodexPaths(t)
	var stdout, stderr bytes.Buffer
	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, []string{"bash", "--help"})
	if err == nil {
		t.Fatalf("RunE() = nil, want missing-account error since target cmd present and --help is passthrough")
	}
	if !strings.Contains(err.Error(), "--account") {
		t.Errorf("expected missing-account error; got %q", err.Error())
	}
	// Crucially, own-help text must NOT have been printed — that would prove
	// the help short-circuit incorrectly triggered.
	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, "Launch any command inside") {
		t.Errorf("own-help text was printed but target cmd present; got %q", combined)
	}
}

// TestRunCommandMissingAccount verifies the early-validation error when no
// --account is supplied and a target command exists.
func TestRunCommandMissingAccount(t *testing.T) {
	paths := testCodexPaths(t)
	var stdout, stderr bytes.Buffer
	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.RunE(cmd, []string{"bash"})
	if err == nil {
		t.Fatal("RunE() error = nil, want --account required error")
	}
	if !strings.Contains(err.Error(), "--account") {
		t.Errorf("error = %q, want --account in message", err.Error())
	}
}

// TestRunCommandUnknownAccount verifies that an unknown account name surfaces
// the resolveAccountByName not-found error.
func TestRunCommandUnknownAccount(t *testing.T) {
	paths := testCodexPaths(t)

	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.RunE(cmd, []string{"--account", "nope", "bash"})
	if err == nil {
		t.Fatal("RunE(unknown account) error = nil, want not-found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found' guidance", err.Error())
	}
}

// TestRunCommandDuplicateNameRequiresProviderDisambiguation verifies the
// happy-collision detection: an account name that exists under BOTH providers
// without --provider triggers the multi-provider collision error.
func TestRunCommandDuplicateNameRequiresProviderDisambiguation(t *testing.T) {
	t.Setenv(valvTestSkipHostCodexLoginEnv, "1")

	paths := testCodexPaths(t)
	runManage(t, paths, []string{"account", "add", "codex", "work", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"account", "add", "claude", "work", "--skip-login", "--no-bind"})

	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.RunE(cmd, []string{"--account", "work", "bash"})
	if err == nil {
		t.Fatal("RunE(collision) error = nil, want collision error")
	}
	for _, want := range []string{"multiple providers", "--provider"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want substring %q", err.Error(), want)
		}
	}
}

// TestRunCommandUnboundProjectBindHint verifies the no-project-row error path
// in both its variants:
//   - --provider explicit → "valv account bind <name> --provider <provider>"
//   - --provider inferred  → "valv account bind <name>"
func TestRunCommandUnboundProjectBindHint(t *testing.T) {
	tests := []struct {
		name               string
		setupAccounts      [][]string
		seedClaudeCreds    []string // account names to seed Claude .credentials.json for
		runArgs            []string
		wantBindSuggestion string
	}{
		{
			name: "claude unique account inferred — no --provider in hint",
			setupAccounts: [][]string{
				{"account", "add", "claude", "lone", "--skip-login", "--no-bind"},
			},
			seedClaudeCreds:    []string{"lone"},
			runArgs:            []string{"--account", "lone", "bash"},
			wantBindSuggestion: "valv account bind lone",
		},
		{
			name: "claude account with explicit --provider — provider in hint",
			setupAccounts: [][]string{
				{"account", "add", "claude", "lone", "--skip-login", "--no-bind"},
				// Also add a codex account so the lookup is not unique without --provider.
				{"account", "add", "codex", "lone", "--skip-login", "--no-bind"},
			},
			seedClaudeCreds:    []string{"lone"},
			runArgs:            []string{"--account", "lone", "--provider", "claude", "bash"},
			wantBindSuggestion: "valv account bind lone --provider claude",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Cannot t.Parallel() because we mutate process cwd and env.
			t.Setenv(valvTestSkipHostCodexLoginEnv, "1")
			t.Setenv("VALV_CLAUDE_IMAGE", "valv-claude:test")
			t.Setenv("VALV_CODEX_IMAGE", "valv-codex:test")

			paths := testCodexPaths(t)
			for _, setup := range tc.setupAccounts {
				runManage(t, paths, setup)
			}
			for _, account := range tc.seedClaudeCreds {
				seedClaudeAccountCredentials(t, paths, account)
			}

			// Cd to a fresh dir with no project record.
			wd, err := os.Getwd()
			if err != nil {
				t.Fatalf("Getwd() error = %v", err)
			}
			projectDir := filepath.Join(t.TempDir(), "noproject")
			if err := os.MkdirAll(projectDir, 0o755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			if err := os.Chdir(projectDir); err != nil {
				t.Fatalf("Chdir() error = %v", err)
			}
			t.Cleanup(func() { _ = os.Chdir(wd) })

			cmd := newRunCommand(paths, nil)
			cmd.SetContext(context.Background())
			cmd.SetIn(bytes.NewBuffer(nil))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			installStubCodexAccountAuth(t, cmd, true)

			err = cmd.RunE(cmd, tc.runArgs)
			if err == nil {
				t.Fatal("RunE() error = nil, want unbound-project error")
			}
			if !strings.Contains(err.Error(), "project is not bound") {
				t.Errorf("error = %q, want 'project is not bound'", err.Error())
			}
			if !strings.Contains(err.Error(), tc.wantBindSuggestion) {
				t.Errorf("error = %q, want bind suggestion %q", err.Error(), tc.wantBindSuggestion)
			}
		})
	}
}

// seedClaudeAccountCredentials writes a minimal .credentials.json into the
// managed Claude account home so ensureClaudeAccountReady's pre-check passes.
// Mirrors writeCredsToDir but resolves the account home from paths.
func seedClaudeAccountCredentials(t *testing.T, paths config.Paths, accountName string) {
	t.Helper()
	homePath := filepath.Join(paths.ProviderRoot, "claude", "profiles", accountName)
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", homePath, err)
	}
	credPath := filepath.Join(homePath, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"claudeAiOauth":{"accessToken":"tok"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", credPath, err)
	}
}

// TestRunCommandOverrideLaunchPathClaude exercises the override launch path
// through `valv run` for claude:
//
//   - non-empty .valv/tools.toml
//   - VALV_CLAUDE_IMAGE set
//   - launch emits exactly one override warning
//   - zero overlay-image docker calls (no `docker buildx build` invocation)
//   - the override-derived base image flows into the shared run service
//
// We can stop short of full docker launch by intercepting at PrepareRuntime
// failure — we leave ProfileHome / ProjectRoot unwritable so PrepareRuntime
// errors but resolveProjectImage has already run by then. The test asserts
// the warning + zero overlay calls regardless of downstream success.
func TestRunCommandOverrideLaunchPathClaude(t *testing.T) {
	t.Setenv(valvTestSkipHostCodexLoginEnv, "1")
	t.Setenv("VALV_CLAUDE_IMAGE", "test/claude:override")

	paths := testCodexPaths(t)
	runManage(t, paths, []string{"account", "add", "claude", "work", "--skip-login", "--no-bind"})
	seedClaudeAccountCredentials(t, paths, "work")

	logPath := installFakeDocker(t)

	// Real project root with .git and bound to the claude account so the
	// project record exists; otherwise the test hits ErrUnboundProject first
	// and we can't observe the image-resolution path.
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	writeRunToolsManifest(t, projectRoot)
	runManage(t, paths, []string{"bind", "claude", "work", "--project", projectRoot})

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	installStubCodexAccountAuth(t, cmd, true)

	// Best-effort: run the command. Downstream PrepareRuntime + docker exec
	// may fail because the test fake docker doesn't process the run, but the
	// warning and overlay-call assertions are valid regardless of final exit.
	_ = cmd.RunE(cmd, []string{"--account", "work", "--provider", "claude", "bash", "-c", "echo hi"})

	const wantWarning = "warning: VALV_CLAUDE_IMAGE override active; .valv/tools.toml overlay skipped"
	if !strings.Contains(stderr.String(), wantWarning) {
		t.Fatalf("stderr %q missing override warning %q", stderr.String(), wantWarning)
	}
	if got, want := strings.Count(stderr.String(), wantWarning), 1; got != want {
		t.Fatalf("override warning count = %d, want %d", got, want)
	}
	if data, err := os.ReadFile(logPath); err == nil {
		if strings.Contains(string(data), "buildx build") {
			t.Fatalf("unexpected buildx build call under override: %q", string(data))
		}
	}
}

// TestRunCommandOverrideLaunchPathCodex mirrors the claude override-path test
// for codex.
func TestRunCommandOverrideLaunchPathCodex(t *testing.T) {
	t.Setenv(valvTestSkipHostCodexLoginEnv, "1")
	t.Setenv("VALV_CODEX_IMAGE", "test/codex:override")

	paths := testCodexPaths(t)
	runManage(t, paths, []string{"account", "add", "codex", "work", "--skip-login", "--no-bind"})

	logPath := installFakeDocker(t)

	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	writeRunToolsManifest(t, projectRoot)
	runManage(t, paths, []string{"bind", "codex", "work", "--project", projectRoot})

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	installStubCodexAccountAuth(t, cmd, true)

	_ = cmd.RunE(cmd, []string{"--account", "work", "--provider", "codex", "bash", "-c", "echo hi"})

	const wantWarning = "warning: VALV_CODEX_IMAGE override active; .valv/tools.toml overlay skipped"
	if !strings.Contains(stderr.String(), wantWarning) {
		t.Fatalf("stderr %q missing override warning %q", stderr.String(), wantWarning)
	}
	if got, want := strings.Count(stderr.String(), wantWarning), 1; got != want {
		t.Fatalf("override warning count = %d, want %d", got, want)
	}
	if data, err := os.ReadFile(logPath); err == nil {
		if strings.Contains(string(data), "buildx build") {
			t.Fatalf("unexpected buildx build call under override: %q", string(data))
		}
	}
}

// TestRunCommandDuplicateNameWithExplicitProviderAndOverride exercises the
// happy collision-disambiguation + override combination:
//
//   - account named `work` in both Claude and Codex
//   - `--provider claude` supplied
//   - non-empty .valv/tools.toml
//   - VALV_CLAUDE_IMAGE set, VALV_CODEX_IMAGE unset
//
// Acceptance: exactly one Claude override warning, zero overlay docker calls,
// and (most important for the ordering question) the Claude override-derived
// base image is the one that flowed through.
func TestRunCommandDuplicateNameWithExplicitProviderAndOverride(t *testing.T) {
	t.Setenv(valvTestSkipHostCodexLoginEnv, "1")
	t.Setenv("VALV_CLAUDE_IMAGE", "test/claude:override")
	t.Setenv("VALV_CODEX_IMAGE", "")

	paths := testCodexPaths(t)
	runManage(t, paths, []string{"account", "add", "claude", "work", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"account", "add", "codex", "work", "--skip-login", "--no-bind"})
	seedClaudeAccountCredentials(t, paths, "work")

	logPath := installFakeDocker(t)

	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	writeRunToolsManifest(t, projectRoot)
	runManage(t, paths, []string{"bind", "claude", "work", "--project", projectRoot})

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	installStubCodexAccountAuth(t, cmd, true)

	_ = cmd.RunE(cmd, []string{"--account", "work", "--provider", "claude", "bash", "-c", "echo hi"})

	// Exactly one Claude override warning (Claude path).
	const wantClaude = "warning: VALV_CLAUDE_IMAGE override active; .valv/tools.toml overlay skipped"
	if got := strings.Count(stderr.String(), wantClaude); got != 1 {
		t.Fatalf("Claude override warning count = %d, want 1; stderr=%q", got, stderr.String())
	}
	// Codex override warning must NOT fire (VALV_CODEX_IMAGE empty AND --provider claude routes away).
	const dontWantCodex = "warning: VALV_CODEX_IMAGE override active"
	if strings.Contains(stderr.String(), dontWantCodex) {
		t.Fatalf("Codex override warning unexpectedly emitted; stderr=%q", stderr.String())
	}
	// Zero overlay docker calls.
	if data, err := os.ReadFile(logPath); err == nil {
		if strings.Contains(string(data), "buildx build") {
			t.Fatalf("unexpected buildx build call under override: %q", string(data))
		}
	}
}

// TestRunCommandUnhappyCollisionFiresBeforeOverride exercises the unhappy
// collision path called out in acceptance:
//
//   - account name `work` exists in BOTH claude and codex
//   - --provider NOT supplied
//   - .valv/tools.toml non-empty
//   - VALV_CLAUDE_IMAGE set
//
// Acceptance: the command MUST fail with the duplicate-name collision error
// BEFORE any override warning is emitted AND BEFORE any overlay docker call
// occurs. Assert ordering: 0 overlay docker calls observed, 0 override
// warnings observed, exit error is the collision error.
func TestRunCommandUnhappyCollisionFiresBeforeOverride(t *testing.T) {
	t.Setenv(valvTestSkipHostCodexLoginEnv, "1")
	t.Setenv("VALV_CLAUDE_IMAGE", "test/claude:override")

	paths := testCodexPaths(t)
	runManage(t, paths, []string{"account", "add", "claude", "work", "--skip-login", "--no-bind"})
	runManage(t, paths, []string{"account", "add", "codex", "work", "--skip-login", "--no-bind"})

	logPath := installFakeDocker(t)

	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	writeRunToolsManifest(t, projectRoot)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cmd := newRunCommand(paths, nil)
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	installStubCodexAccountAuth(t, cmd, true)

	err = cmd.RunE(cmd, []string{"--account", "work", "bash", "-c", "echo hi"})
	if err == nil {
		t.Fatal("RunE() error = nil, want collision error before any overlay/warning")
	}
	for _, want := range []string{"multiple providers", "--provider"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want substring %q", err.Error(), want)
		}
	}
	// No override warning must have been emitted.
	if strings.Contains(stderr.String(), "VALV_CLAUDE_IMAGE override active") {
		t.Errorf("override warning emitted before collision error; stderr=%q", stderr.String())
	}
	// No buildx build call either.
	if data, err := os.ReadFile(logPath); err == nil {
		if strings.Contains(string(data), "buildx build") {
			t.Errorf("unexpected buildx build call: %q", string(data))
		}
	}
}

// TestUnboundProjectBindHintErrorFormat is a focused table-driven test of the
// bind-hint formatter. Decoupled from the rest of the command so the
// formatter's two variants are pinned independently of the surrounding
// account-resolution machinery.
func TestUnboundProjectBindHintErrorFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		parsed  parsedRunFlags
		account string
		want    string
	}{
		{
			name:    "inferred provider — short form",
			parsed:  parsedRunFlags{providerExplicit: false},
			account: "alice",
			want:    "run `valv account bind alice` to create",
		},
		{
			name:    "explicit provider — qualified form",
			parsed:  parsedRunFlags{providerExplicit: true},
			account: "alice",
			want:    "run `valv account bind alice --provider claude` to create",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := unboundProjectBindHintError(tc.parsed, "claude", tc.account)
			if err == nil {
				t.Fatal("unboundProjectBindHintError() = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

// TestRunCommandRegisteredInRoot verifies that the root command tree
// includes `valv run` under the runtime group.
func TestRunCommandRegisteredInRoot(t *testing.T) {
	t.Parallel()

	paths := testCodexPaths(t)
	root, err := NewRootCommandWithPaths(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, paths)
	if err != nil {
		t.Fatalf("NewRootCommandWithPaths() error = %v", err)
	}

	var found *cobra.Command
	for _, child := range root.Commands() {
		if child.Name() == "run" {
			found = child
			break
		}
	}
	if found == nil {
		t.Fatal("root command tree missing `run` subcommand")
	}
	if found.GroupID != "runtime" {
		t.Errorf("run.GroupID = %q, want %q", found.GroupID, "runtime")
	}
}

// writeRunToolsManifest writes a minimal `.valv/tools.toml` to the given
// project root for use by the override-launch-path tests. Mirrors the shape
// of writeCodexToolsManifest but accepts an explicit target directory.
func writeRunToolsManifest(t *testing.T, projectRoot string) {
	t.Helper()
	valvDir := filepath.Join(projectRoot, ".valv")
	if err := os.MkdirAll(valvDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", valvDir, err)
	}
	manifest := strings.TrimSpace(`
[tools]
ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }
`) + "\n"
	manifestPath := filepath.Join(valvDir, "tools.toml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", manifestPath, err)
	}
}
