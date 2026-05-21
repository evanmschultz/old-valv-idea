package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// chdirToProjectRoot creates a temp project dir with a `.git` marker so
// project.Detect() returns the temp dir, then Chdir's the test process into
// it. The previous working directory is restored via t.Cleanup. Returns the
// absolute path of the new project root.
func chdirToProjectRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git): %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir(%s): %v", root, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	return root
}

// writeToolsFile writes contents to <root>/.valv/tools.toml, creating the
// .valv dir if needed. The test fails on any I/O error.
func writeToolsFile(t *testing.T, root, contents string) {
	t.Helper()

	dir := filepath.Join(root, ".valv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tools.toml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// runToolsValidateCmd executes `valv tools validate` against a freshly
// constructed test root command tree. Returns stdout, stderr, and the
// Execute error.
func runToolsValidateCmd(t *testing.T) (string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"tools", "validate"})
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestToolsValidate_ValidManifest(t *testing.T) {
	root := chdirToProjectRoot(t)
	writeToolsFile(t, root, `[tools]
mage = "latest"
gh = "latest"
go = "1.22"
`)

	stdout, _, err := runToolsValidateCmd(t)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "tools.toml is valid") {
		t.Errorf("stdout = %q, want it to contain %q", stdout, "tools.toml is valid")
	}
}

func TestToolsValidate_NoValvDir(t *testing.T) {
	// Temp project root with no .valv/ directory at all.
	_ = chdirToProjectRoot(t)

	stdout, _, err := runToolsValidateCmd(t)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "no tools declared") {
		t.Errorf("stdout = %q, want it to contain %q", stdout, "no tools declared")
	}
}

func TestToolsValidate_EmptyToolsTable(t *testing.T) {
	// Present file with an empty [tools] table — len(m.Tools) == 0.
	root := chdirToProjectRoot(t)
	writeToolsFile(t, root, "[tools]\n")

	stdout, _, err := runToolsValidateCmd(t)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "no tools declared") {
		t.Errorf("stdout = %q, want it to contain %q", stdout, "no tools declared")
	}
}

func TestToolsValidate_InvalidToml(t *testing.T) {
	// Malformed TOML — Load returns a parse error.
	root := chdirToProjectRoot(t)
	writeToolsFile(t, root, "this is = not [valid toml")

	_, _, err := runToolsValidateCmd(t)
	if err == nil {
		t.Fatal("Execute: expected error for malformed TOML, got nil")
	}
	if !strings.Contains(err.Error(), "tools validate") {
		t.Errorf("error = %q, want wrapped with %q", err.Error(), "tools validate")
	}
}

func TestToolsValidate_InvalidToml_UnknownKeys(t *testing.T) {
	// Unknown top-level section — Load returns the strict-undecoded error.
	root := chdirToProjectRoot(t)
	writeToolsFile(t, root, `[tools]
mage = "latest"

[network]
foo = "bar"
`)

	_, _, err := runToolsValidateCmd(t)
	if err == nil {
		t.Fatal("Execute: expected error for unknown top-level key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown keys") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "unknown keys")
	}
}

func TestToolsValidate_ValidationFailure(t *testing.T) {
	// File parses cleanly but Validate rejects the tool name (leading underscore).
	root := chdirToProjectRoot(t)
	writeToolsFile(t, root, `[tools]
_underscore = "latest"
`)

	_, _, err := runToolsValidateCmd(t)
	if err == nil {
		t.Fatal("Execute: expected error for invalid tool name, got nil")
	}
	if !strings.Contains(err.Error(), "invalid tool name") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "invalid tool name")
	}
}

func TestToolsValidate_ZeroByteFile(t *testing.T) {
	// Per PLAN: zero-byte file decodes to an empty manifest → exit 0,
	// "no tools declared". The committed testdata/zero_byte.toml is the
	// source-of-truth zero-byte fixture; copy its contents into the
	// project's .valv/ before invocation. Capture the absolute path BEFORE
	// chdirToProjectRoot — chdir invalidates the relative `testdata/...`
	// reference.
	srcAbs, err := filepath.Abs(filepath.Join("testdata", "zero_byte.toml"))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	contents, err := os.ReadFile(srcAbs)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", srcAbs, err)
	}
	if len(contents) != 0 {
		t.Fatalf("committed fixture %s expected zero bytes, got %d", srcAbs, len(contents))
	}

	root := chdirToProjectRoot(t)
	if err := os.MkdirAll(filepath.Join(root, ".valv"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.valv): %v", err)
	}
	dst := filepath.Join(root, ".valv", "tools.toml")
	if err := os.WriteFile(dst, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", dst, err)
	}

	stdout, _, err := runToolsValidateCmd(t)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "no tools declared") {
		t.Errorf("stdout = %q, want it to contain %q", stdout, "no tools declared")
	}
}

func TestToolsValidate_PermissionDenied(t *testing.T) {
	// chmod 000 cannot survive `git add` cleanly — must materialize in-test
	// against t.TempDir(). Skipped on Windows (no POSIX perm semantics) and
	// when running as root (which bypasses mode checks).
	if runtime.GOOS == "windows" {
		t.Skip("permission-denied semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file mode checks; cannot exercise permission-denied path")
	}

	root := chdirToProjectRoot(t)
	if err := os.MkdirAll(filepath.Join(root, ".valv"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.valv): %v", err)
	}
	path := filepath.Join(root, ".valv", "tools.toml")
	if err := os.WriteFile(path, []byte(`[tools]
mage = "latest"
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	// Restore permissions before TempDir cleanup so the temp tree can be
	// removed cleanly on test exit.
	t.Cleanup(func() {
		_ = os.Chmod(path, 0o644)
	})

	_, _, err := runToolsValidateCmd(t)
	if err == nil {
		t.Fatal("Execute: expected error for unreadable file, got nil")
	}
	if !errors.Is(err, syscall.EACCES) && !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %v, want it to surface a permission-denied indicator", err)
	}
}

func TestToolsValidate_DirectoryAtPath(t *testing.T) {
	// `.valv/tools.toml` is a directory rather than a regular file. mkdir
	// state cannot survive `git add` cleanly — must materialize in-test.
	// Per PLAN Round 4: os.Open on a directory succeeds but the subsequent
	// read surfaces EISDIR; toml.DecodeFile follows the same path. Accept
	// either errors.Is(err, syscall.EISDIR) or the literal substring "is a
	// directory" in the wrapped error string.
	root := chdirToProjectRoot(t)
	if err := os.MkdirAll(filepath.Join(root, ".valv", "tools.toml"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	_, _, err := runToolsValidateCmd(t)
	if err == nil {
		t.Fatal("Execute: expected error for directory-at-path, got nil")
	}
	if !errors.Is(err, syscall.EISDIR) && !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("error = %v, want EISDIR or %q substring", err, "is a directory")
	}
}

// TestToolsValidate_RegisteredOnRoot proves the `tools` branch is wired into
// the root command tree with GroupID="runtime" — guards against regressions
// to the AddCommand variadic.
func TestToolsValidate_RegisteredOnRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetContext(context.Background())

	var found bool
	for _, child := range cmd.Commands() {
		if child.Name() == "tools" {
			found = true
			if child.GroupID != "runtime" {
				t.Errorf("tools.GroupID = %q, want %q", child.GroupID, "runtime")
			}
			// Verify the `validate` subcommand is present.
			var hasValidate bool
			for _, sub := range child.Commands() {
				if sub.Name() == "validate" {
					hasValidate = true
					break
				}
			}
			if !hasValidate {
				t.Errorf("tools branch missing `validate` subcommand")
			}
			break
		}
	}
	if !found {
		t.Fatal("root command tree missing `tools` branch")
	}
}
