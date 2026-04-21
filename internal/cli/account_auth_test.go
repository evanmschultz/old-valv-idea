package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/evanmschultz/valv/internal/domain"
)

type stubCodexAccountAuthRunner struct {
	loggedIn   bool
	statusErr  error
	loginErr   error
	statusHits int
	loginHits  int
	homePaths  []string
}

func (s *stubCodexAccountAuthRunner) LoginStatus(_ context.Context, homePath string) (bool, error) {
	s.statusHits++
	s.homePaths = append(s.homePaths, homePath)
	return s.loggedIn, s.statusErr
}

func (s *stubCodexAccountAuthRunner) Login(_ context.Context, homePath string, _ io.Reader, _, _ io.Writer) error {
	s.loginHits++
	s.homePaths = append(s.homePaths, homePath)
	if s.loginErr == nil {
		s.loggedIn = true
	}
	return s.loginErr
}

func (s *stubCodexAccountAuthRunner) Logout(_ context.Context, homePath string, _ io.Reader, _, _ io.Writer) error {
	s.homePaths = append(s.homePaths, homePath)
	s.loggedIn = false
	return nil
}

func installStubCodexAccountAuth(t *testing.T, cmd *cobra.Command, loggedIn bool) *stubCodexAccountAuthRunner {
	t.Helper()

	stub := &stubCodexAccountAuthRunner{loggedIn: loggedIn}
	cmd.SetContext(context.WithValue(cmd.Context(), codexAccountAuthRunnerKey{}, codexAccountAuthRunner(stub)))
	return stub
}

func TestEnsureCodexAccountReadyRejectsNonTTYLogin(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	installStubCodexAccountAuth(t, cmd, false)

	err := ensureCodexAccountReady(cmd, domain.Profile{Name: "work", HomePath: "/tmp/work"}, accountAuthOptions{})
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("ensureCodexAccountReady() error = %v, want non-tty login guidance", err)
	}
}

func TestEnsureCodexAccountReadySkipsAuthenticatedAccount(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	stub := installStubCodexAccountAuth(t, cmd, true)

	if err := ensureCodexAccountReady(cmd, domain.Profile{Name: "work", HomePath: "/tmp/work"}, accountAuthOptions{}); err != nil {
		t.Fatalf("ensureCodexAccountReady() error = %v", err)
	}
	if stub.loginHits != 0 {
		t.Fatalf("Login() hits = %d, want 0", stub.loginHits)
	}
}

func TestEnsureCodexAccountReadySkipsWhenTestBypassIsSet(t *testing.T) {
	t.Setenv(valvTestSkipHostCodexLoginEnv, "1")

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	stub := installStubCodexAccountAuth(t, cmd, false)

	if err := ensureCodexAccountReady(cmd, domain.Profile{Name: "work", HomePath: "/tmp/work"}, accountAuthOptions{}); err != nil {
		t.Fatalf("ensureCodexAccountReady() error = %v", err)
	}
	if stub.statusHits != 0 {
		t.Fatalf("LoginStatus() hits = %d, want 0", stub.statusHits)
	}
	if stub.loginHits != 0 {
		t.Fatalf("Login() hits = %d, want 0", stub.loginHits)
	}
}

func TestShouldSkipHostCodexLoginCheck(t *testing.T) {
	t.Setenv(valvTestSkipHostCodexLoginEnv, "1")
	if !shouldSkipHostCodexLoginCheck() {
		t.Fatal("shouldSkipHostCodexLoginCheck() = false, want true")
	}
}

func TestSystemCodexAccountAuthRunnerLoginStatusRecognizesLoggedIn(t *testing.T) {
	installFakeHostCodex(t, "ok")
	ok, err := systemCodexAccountAuthRunner{}.LoginStatus(context.Background(), "/tmp/work")
	if err != nil {
		t.Fatalf("LoginStatus() error = %v", err)
	}
	if !ok {
		t.Fatal("LoginStatus() = false, want true")
	}
}

func TestSystemCodexAccountAuthRunnerLoginStatusRecognizesLoggedOut(t *testing.T) {
	installFakeHostCodex(t, "loggedout")
	ok, err := systemCodexAccountAuthRunner{}.LoginStatus(context.Background(), "/tmp/work")
	if err != nil {
		t.Fatalf("LoginStatus() error = %v", err)
	}
	if ok {
		t.Fatal("LoginStatus() = true, want false")
	}
}

func TestSystemCodexAccountAuthRunnerLoginStatusReturnsUnexpectedFailureOutput(t *testing.T) {
	installFakeHostCodex(t, "fail")
	_, err := systemCodexAccountAuthRunner{}.LoginStatus(context.Background(), "/tmp/work")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("LoginStatus() error = %v, want unexpected output detail", err)
	}
}

func TestSystemCodexAccountAuthRunnerLoginUsesCODEXHOME(t *testing.T) {
	logPath := installFakeHostCodex(t, "ok")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := systemCodexAccountAuthRunner{}.Login(context.Background(), "/tmp/work", bytes.NewBuffer(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", logPath, err)
	}
	if !strings.Contains(string(content), "CODEX_HOME=/tmp/work") {
		t.Fatalf("fake codex log = %q, want CODEX_HOME entry", string(content))
	}
}

func TestSystemCodexAccountAuthRunnerLogoutUsesCODEXHOME(t *testing.T) {
	logPath := installFakeHostCodex(t, "ok")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := systemCodexAccountAuthRunner{}.Logout(context.Background(), "/tmp/work", bytes.NewBuffer(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", logPath, err)
	}
	if !strings.Contains(string(content), "CODEX_HOME=/tmp/work") || !strings.Contains(string(content), "args:logout") {
		t.Fatalf("fake codex log = %q, want logout CODEX_HOME entry", string(content))
	}
}

func TestProviderClaudeAccountAuthStubs(t *testing.T) {
	t.Parallel()

	dispatchers := []struct {
		name string
		call func(cmd *cobra.Command, account domain.Profile) error
	}{
		{
			name: "ensureManagedAccountReady",
			call: func(cmd *cobra.Command, account domain.Profile) error {
				return ensureManagedAccountReady(cmd, domain.ProviderClaude, account, accountAuthOptions{})
			},
		},
		{
			name: "logoutManagedAccount",
			call: func(cmd *cobra.Command, account domain.Profile) error {
				return logoutManagedAccount(cmd, domain.ProviderClaude, account)
			},
		},
		{
			name: "loginManagedAccount",
			call: func(cmd *cobra.Command, account domain.Profile) error {
				return loginManagedAccount(cmd, domain.ProviderClaude, account)
			},
		},
	}

	for _, tc := range dispatchers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.SetIn(bytes.NewBuffer(nil))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			account := domain.Profile{Provider: domain.ProviderClaude}
			if err := tc.call(cmd, account); err != nil {
				t.Fatalf("%s(ProviderClaude) error = %v, want nil", tc.name, err)
			}
		})
	}
}

func installFakeHostCodex(t *testing.T, mode string) string {
	t.Helper()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "codex.log")
	scriptPath := filepath.Join(dir, "codex")
	script := `#!/bin/sh
set -eu
printf 'args:%s\n' "$*" >> "$FAKE_CODEX_LOG"
printf 'CODEX_HOME=%s\n' "${CODEX_HOME:-}" >> "$FAKE_CODEX_LOG"
mode="${FAKE_CODEX_MODE:-ok}"
if [ "${1:-}" = "login" ] && [ "${2:-}" = "status" ]; then
  case "$mode" in
    ok)
      printf 'Logged in\n'
      exit 0
      ;;
    loggedout)
      printf 'Not logged in\n'
      exit 1
      ;;
    fail)
      printf 'boom\n'
      exit 2
      ;;
  esac
fi
if [ "${1:-}" = "login" ]; then
  printf 'login ok\n'
  exit 0
fi
if [ "${1:-}" = "logout" ]; then
  printf 'logout ok\n'
  exit 0
fi
printf 'unexpected args\n'
exit 64
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", scriptPath, err)
	}
	t.Setenv("FAKE_CODEX_LOG", logPath)
	t.Setenv("FAKE_CODEX_MODE", mode)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}
