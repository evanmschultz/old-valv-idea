package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_AbsentFile(t *testing.T) {
	t.Parallel()

	// A temp dir with no .valv/ subdir at all — manifest absent.
	dir := t.TempDir()

	m, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve(%s): unexpected error: %v", dir, err)
	}
	if len(m.Tools) != 0 {
		t.Errorf("Tools = %v, want empty map (absent file should yield empty manifest)", m.Tools)
	}
}

func TestResolve_AbsentFile_DotValvDirExistsButNoToml(t *testing.T) {
	t.Parallel()

	// Edge case: .valv/ exists as a directory but tools.toml inside it does not.
	// Still maps to the absent-file path.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".valv"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	m, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve(%s): unexpected error: %v", dir, err)
	}
	if len(m.Tools) != 0 {
		t.Errorf("Tools = %v, want empty map (absent tools.toml inside .valv/ should yield empty manifest)", m.Tools)
	}
}

func TestResolve_ValidManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixture(t, dir, `[tools]
mage = "latest"
gh = "latest"
go = "1.22"
`)

	m, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve(%s): unexpected error: %v", dir, err)
	}
	if got, want := len(m.Tools), 3; got != want {
		t.Fatalf("len(Tools) = %d, want %d", got, want)
	}
	if got, want := m.Tools["mage"].Version, "latest"; got != want {
		t.Errorf("Tools[mage].Version = %q, want %q", got, want)
	}
	if got, want := m.Tools["go"].Version, "1.22"; got != want {
		t.Errorf("Tools[go].Version = %q, want %q", got, want)
	}
}

func TestResolve_ValidManifest_WithForwardCompatSections(t *testing.T) {
	t.Parallel()

	// DROP_15 typed Allowlist: Resolve must surface m.Allowlist.Hosts.
	// [env] remains PrimitiveDecode'd until DROP_14.
	dir := t.TempDir()
	writeFixture(t, dir, `[tools]
mage = "latest"
ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }

[allowlist]
hosts = ["github.com", "proxy.golang.org"]

[env]
GOPRIVATE = "github.com/evanmschultz/*"
`)

	m, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve(%s): unexpected error: %v", dir, err)
	}
	if got, want := len(m.Tools), 2; got != want {
		t.Fatalf("len(Tools) = %d, want %d", got, want)
	}
	ta, ok := m.Tools["ta"]
	if !ok {
		t.Fatalf("Tools[ta] missing")
	}
	if ta.Source == "" || ta.Install == "" {
		t.Errorf("object-form tool not decoded: %+v", ta)
	}
	wantHosts := []string{"github.com", "proxy.golang.org"}
	if got := m.Allowlist.Hosts; len(got) != len(wantHosts) {
		t.Fatalf("len(Allowlist.Hosts) = %d, want %d (got %v)", len(got), len(wantHosts), got)
	}
	for i, want := range wantHosts {
		if m.Allowlist.Hosts[i] != want {
			t.Errorf("Allowlist.Hosts[%d] = %q, want %q", i, m.Allowlist.Hosts[i], want)
		}
	}
}

func TestResolve_LoadParseError(t *testing.T) {
	t.Parallel()

	// Malformed TOML triggers a decode error from Load — Resolve must wrap
	// and return without ever reaching Validate.
	dir := t.TempDir()
	writeFixture(t, dir, "this is = not [valid toml")

	_, err := Resolve(dir)
	if err == nil {
		t.Fatalf("Resolve: expected error for malformed TOML, got nil")
	}
	if !strings.Contains(err.Error(), "resolve tools") {
		t.Errorf("error = %q, want it to be wrapped with %q", err.Error(), "resolve tools")
	}
}

func TestResolve_LoadUnknownKeyError(t *testing.T) {
	t.Parallel()

	// Unknown top-level section triggers the strict undecoded check in Load.
	dir := t.TempDir()
	writeFixture(t, dir, `[tools]
mage = "latest"

[network]
foo = "bar"
`)

	_, err := Resolve(dir)
	if err == nil {
		t.Fatalf("Resolve: expected error for unknown top-level key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown keys") {
		t.Errorf("error = %q, want it to mention unknown keys", err.Error())
	}
}

func TestResolve_ValidateError(t *testing.T) {
	t.Parallel()

	// File parses cleanly via Load but the tool name fails Validate
	// (leading underscore is rejected by design).
	dir := t.TempDir()
	writeFixture(t, dir, `[tools]
_underscore = "latest"
`)

	_, err := Resolve(dir)
	if err == nil {
		t.Fatalf("Resolve: expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid tool name") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "invalid tool name")
	}
	if !strings.Contains(err.Error(), "_underscore") {
		t.Errorf("error = %q, want it to mention the offending tool", err.Error())
	}
}

func TestResolve_ToolsFilePathConstant(t *testing.T) {
	t.Parallel()

	// Sanity check: the constant ties Resolve's join target to a known value.
	// Future drops depend on this path being stable.
	if ToolsFilePath != ".valv/tools.toml" {
		t.Errorf("ToolsFilePath = %q, want %q", ToolsFilePath, ".valv/tools.toml")
	}
}

// writeFixture creates dir/.valv/tools.toml with the given contents,
// failing the test on any I/O error. Always called against t.TempDir().
func writeFixture(t *testing.T, dir, contents string) {
	t.Helper()

	valvDir := filepath.Join(dir, ".valv")
	if err := os.MkdirAll(valvDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", valvDir, err)
	}
	path := filepath.Join(valvDir, "tools.toml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
