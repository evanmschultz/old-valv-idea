package tools

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEffectiveAllowlist_ZeroValue(t *testing.T) {
	t.Parallel()

	got, err := EffectiveAllowlist(AllowlistConfig{})
	if err != nil {
		t.Fatalf("EffectiveAllowlist(zero): unexpected error: %v", err)
	}
	want := []string{
		"github.com",
		"objects.githubusercontent.com",
		"proxy.golang.org",
		"sum.golang.org",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EffectiveAllowlist(zero) = %v, want %v", got, want)
	}
}

func TestEffectiveAllowlist_UnionWithUserHosts(t *testing.T) {
	t.Parallel()

	cfg := AllowlistConfig{
		Hosts: []string{"npm.example.com", "registry.example.org"},
	}
	got, err := EffectiveAllowlist(cfg)
	if err != nil {
		t.Fatalf("EffectiveAllowlist: unexpected error: %v", err)
	}
	want := []string{
		"github.com",
		"npm.example.com",
		"objects.githubusercontent.com",
		"proxy.golang.org",
		"registry.example.org",
		"sum.golang.org",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EffectiveAllowlist union = %v, want %v", got, want)
	}
}

func TestEffectiveAllowlist_LowercaseAndDedup(t *testing.T) {
	t.Parallel()

	cfg := AllowlistConfig{
		Hosts: []string{
			"NPM.Example.Com",
			" npm.example.com ",
			"Github.com", // dupes a built-in default under case-folding
			"registry.example.org",
			"registry.example.org",
		},
	}
	got, err := EffectiveAllowlist(cfg)
	if err != nil {
		t.Fatalf("EffectiveAllowlist: unexpected error: %v", err)
	}
	want := []string{
		"github.com",
		"npm.example.com",
		"objects.githubusercontent.com",
		"proxy.golang.org",
		"registry.example.org",
		"sum.golang.org",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EffectiveAllowlist lowercase/dedup = %v, want %v", got, want)
	}
}

func TestEffectiveAllowlist_InvalidHost(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		host string
	}{
		{"empty", ""},
		{"only_whitespace", "   "},
		{"url_with_scheme", "https://github.com"},
		{"contains_path", "github.com/foo"},
		{"contains_query", "github.com?q=1"},
		{"port_suffix", "github.com:443"},
		{"leading_dot", ".github.com"},
		{"trailing_dot", "github.com."},
		{"leading_hyphen", "-github.com"},
		{"trailing_hyphen", "github-.com"},
		{"contains_underscore", "git_hub.com"},
		{"contains_space", "git hub.com"},
		{"non_ascii", "münchen.example.com"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := EffectiveAllowlist(AllowlistConfig{Hosts: []string{tc.host}})
			if err == nil {
				t.Fatalf("EffectiveAllowlist(%q): expected error, got nil", tc.host)
			}
			if !errors.Is(err, ErrInvalidAllowlistHost) {
				t.Errorf("EffectiveAllowlist(%q): want errors.Is(err, ErrInvalidAllowlistHost), got %v", tc.host, err)
			}
		})
	}
}

func TestEffectiveAllowlist_DockerAliasAccepted(t *testing.T) {
	t.Parallel()

	// Simple single-label Docker container alias.
	got, err := EffectiveAllowlist(AllowlistConfig{Hosts: []string{"my-service"}})
	if err != nil {
		t.Fatalf("EffectiveAllowlist(my-service): unexpected error: %v", err)
	}
	found := false
	for _, h := range got {
		if h == "my-service" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("EffectiveAllowlist did not include Docker alias 'my-service': got %v", got)
	}
}

func TestLoad_AllowlistTypedDecode(t *testing.T) {
	t.Parallel()

	// A manifest with [allowlist].hosts must decode straight into
	// AllowlistConfig.Hosts.
	dir := t.TempDir()
	valvDir := filepath.Join(dir, ".valv")
	if err := os.MkdirAll(valvDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(valvDir, "tools.toml")
	contents := `[tools]
mage = "latest"

[allowlist]
hosts = ["npm.example.com", "registry.example.org"]
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	want := []string{"npm.example.com", "registry.example.org"}
	if !reflect.DeepEqual(m.Allowlist.Hosts, want) {
		t.Errorf("m.Allowlist.Hosts = %v, want %v", m.Allowlist.Hosts, want)
	}
}

func TestLoad_AllowlistUnknownKeyRejected(t *testing.T) {
	t.Parallel()

	// An unknown sub-key inside [allowlist] should be caught by the strict
	// undecoded-key check.
	dir := t.TempDir()
	valvDir := filepath.Join(dir, ".valv")
	if err := os.MkdirAll(valvDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(valvDir, "tools.toml")
	contents := `[tools]
mage = "latest"

[allowlist]
hosts = ["a.example.com"]
cidrs = ["10.0.0.0/8"]
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load: expected unknown-key error for allowlist.cidrs, got nil")
	}
	if !strings.Contains(err.Error(), "unknown keys") {
		t.Errorf("Load error = %q, want it to mention unknown keys", err.Error())
	}
	if !strings.Contains(err.Error(), "allowlist.cidrs") {
		t.Errorf("Load error = %q, want it to mention allowlist.cidrs", err.Error())
	}
}

func TestWriteAllowlistSection_FreshProjectNoDir(t *testing.T) {
	t.Parallel()

	// Project root exists; .valv/ does NOT exist; WriteAllowlistSection
	// must create the parent dir and write a fresh manifest.
	dir := t.TempDir()
	path := filepath.Join(dir, ".valv", "tools.toml")

	cfg := AllowlistConfig{Hosts: []string{"a.example.com", "b.example.org"}}
	if err := WriteAllowlistSection(path, cfg); err != nil {
		t.Fatalf("WriteAllowlistSection: unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := `[allowlist]
hosts = [
  "a.example.com",
  "b.example.org",
]
`
	if string(got) != want {
		t.Errorf("WriteAllowlistSection fresh-no-dir output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	// Round-trip: Load should decode back into AllowlistConfig.Hosts.
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load after WriteAllowlistSection: %v", err)
	}
	if !reflect.DeepEqual(m.Allowlist.Hosts, cfg.Hosts) {
		t.Errorf("round-trip Hosts mismatch: got %v, want %v", m.Allowlist.Hosts, cfg.Hosts)
	}
}

func TestWriteAllowlistSection_EmptyHostsList(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".valv", "tools.toml")

	if err := WriteAllowlistSection(path, AllowlistConfig{}); err != nil {
		t.Fatalf("WriteAllowlistSection(empty): unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := `[allowlist]
hosts = []
`
	if string(got) != want {
		t.Errorf("WriteAllowlistSection empty output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteAllowlistSection_PreservesPrefixAndSuffix(t *testing.T) {
	t.Parallel()

	// Golden fixture: file preamble + [allowlist] with inline comment +
	// divider comment + [tools] + divider + [env] with interleaved
	// comments. Round-trip rewrite must reproduce every byte outside
	// the [allowlist] span verbatim.
	dir := t.TempDir()
	path := filepath.Join(dir, ".valv", "tools.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const original = `# valv per-project toolchain manifest
# managed by ` + "`valv network`" + ` for the [allowlist] section

[allowlist] # network-policy hosts
hosts = ["stale.example.com"]

# --- tools ---
[tools]
mage = "latest"
# trailing tools comment

# --- environment variables ---
[env]
# preserve me too
GOPRIVATE = "github.com/evanmschultz/*"
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := AllowlistConfig{Hosts: []string{"a.example.com", "b.example.org"}}
	if err := WriteAllowlistSection(path, cfg); err != nil {
		t.Fatalf("WriteAllowlistSection: unexpected error: %v", err)
	}

	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(gotBytes)

	// The bytes BEFORE [allowlist] must match verbatim.
	const wantPrefix = `# valv per-project toolchain manifest
# managed by ` + "`valv network`" + ` for the [allowlist] section

`
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("prefix not preserved:\n--- got prefix ---\n%q\n--- want prefix ---\n%q", got[:min(len(got), len(wantPrefix))], wantPrefix)
	}

	// The bytes from `# --- tools ---` onward (the divider comment and
	// everything below) must match verbatim. That is, the suffix begins
	// at the divider comment? No — divider comment is BETWEEN allowlist
	// and tools, OUTSIDE the [allowlist] span span as defined: span ends
	// "immediately before the first byte of the next top-level section
	// header". So the suffix should begin at the `[tools]` header line,
	// not at the divider comment. Re-read schema decision 3:
	//
	//   "the bytes from the first byte of the next top-level section
	//   header through EOF"
	//
	// That means the divider comment between [allowlist] and [tools] is
	// INSIDE the [allowlist] span and IS legitimately discarded.
	//
	// Per the schema: "Blank lines and comment lines that fall between
	// the [allowlist] header and the next top-level section header are
	// INSIDE the span and may be rewritten as part of the [allowlist]
	// rewrite."
	const wantSuffix = `[tools]
mage = "latest"
# trailing tools comment

# --- environment variables ---
[env]
# preserve me too
GOPRIVATE = "github.com/evanmschultz/*"
`
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("suffix not preserved:\n--- got suffix tail ---\n%q\n--- want suffix ---\n%q", got[max(0, len(got)-len(wantSuffix)-200):], wantSuffix)
	}

	// The rewritten [allowlist] section must be the canonical form.
	if !strings.Contains(got, "[allowlist]\nhosts = [\n  \"a.example.com\",\n  \"b.example.org\",\n]\n") {
		t.Errorf("rewritten allowlist section missing canonical form:\n%s", got)
	}

	// Stale host must no longer appear.
	if strings.Contains(got, "stale.example.com") {
		t.Errorf("stale host still present after rewrite:\n%s", got)
	}

	// And the inline comment on the original [allowlist] header (#
	// network-policy hosts) IS inside the rewritten span and is
	// legitimately discarded — verify it.
	if strings.Contains(got, "# network-policy hosts") {
		t.Errorf("inline [allowlist] comment leaked into rewrite (it is INSIDE the span):\n%s", got)
	}

	// Round-trip parse and confirm Load decodes correctly.
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load after rewrite: %v", err)
	}
	if !reflect.DeepEqual(m.Allowlist.Hosts, cfg.Hosts) {
		t.Errorf("round-trip Hosts: got %v, want %v", m.Allowlist.Hosts, cfg.Hosts)
	}
	if got, want := m.Tools["mage"].Version, "latest"; got != want {
		t.Errorf("round-trip Tools[mage].Version = %q, want %q", got, want)
	}
}

func TestWriteAllowlistSection_AllowlistAsLastSection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".valv", "tools.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const original = `[tools]
mage = "latest"

[allowlist]
hosts = ["stale.example.com"]
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := AllowlistConfig{Hosts: []string{"new.example.com"}}
	if err := WriteAllowlistSection(path, cfg); err != nil {
		t.Fatalf("WriteAllowlistSection: unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := `[tools]
mage = "latest"

[allowlist]
hosts = [
  "new.example.com",
]
`
	if string(got) != want {
		t.Errorf("output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteAllowlistSection_AllowlistAbsentAppendsAfterTrailingBytes(t *testing.T) {
	t.Parallel()

	// When the file does not contain [allowlist], it counts entirely as
	// prefix and the new section is appended after. The existing bytes
	// must be preserved verbatim.
	dir := t.TempDir()
	path := filepath.Join(dir, ".valv", "tools.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const original = `[tools]
mage = "latest"
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := AllowlistConfig{Hosts: []string{"a.example.com"}}
	if err := WriteAllowlistSection(path, cfg); err != nil {
		t.Fatalf("WriteAllowlistSection: unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := `[tools]
mage = "latest"
[allowlist]
hosts = [
  "a.example.com",
]
`
	if string(got) != want {
		t.Errorf("output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteAllowlistSection_RejectsBOM(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".valv", "tools.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := append([]byte{0xEF, 0xBB, 0xBF}, []byte("[tools]\nmage = \"latest\"\n")...)
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := WriteAllowlistSection(path, AllowlistConfig{Hosts: []string{"a.example.com"}})
	if err == nil {
		t.Fatalf("WriteAllowlistSection: expected error for BOM, got nil")
	}
	if !errors.Is(err, ErrUnsupportedManifestShape) {
		t.Errorf("error not wrapping ErrUnsupportedManifestShape: %v", err)
	}
}

func TestWriteAllowlistSection_RejectsCRLF(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".valv", "tools.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const contents = "[tools]\r\nmage = \"latest\"\r\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := WriteAllowlistSection(path, AllowlistConfig{Hosts: []string{"a.example.com"}})
	if err == nil {
		t.Fatalf("WriteAllowlistSection: expected error for CRLF, got nil")
	}
	if !errors.Is(err, ErrUnsupportedManifestShape) {
		t.Errorf("error not wrapping ErrUnsupportedManifestShape: %v", err)
	}
}

func TestWriteAllowlistSection_RejectsMultilineStringOutsideAllowlist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".valv", "tools.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const contents = `[tools]
mage = """latest"""

[allowlist]
hosts = ["a.example.com"]
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := WriteAllowlistSection(path, AllowlistConfig{Hosts: []string{"b.example.com"}})
	if err == nil {
		t.Fatalf("WriteAllowlistSection: expected error for multi-line string outside [allowlist], got nil")
	}
	if !errors.Is(err, ErrUnsupportedManifestShape) {
		t.Errorf("error not wrapping ErrUnsupportedManifestShape: %v", err)
	}
}

func TestWriteAllowlistSection_RejectsArrayOfTablesHeader(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".valv", "tools.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const contents = `[[mirrors]]
url = "https://example.com"

[allowlist]
hosts = ["a.example.com"]
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := WriteAllowlistSection(path, AllowlistConfig{Hosts: []string{"b.example.com"}})
	if err == nil {
		t.Fatalf("WriteAllowlistSection: expected error for array-of-tables header, got nil")
	}
	if !errors.Is(err, ErrUnsupportedManifestShape) {
		t.Errorf("error not wrapping ErrUnsupportedManifestShape: %v", err)
	}
}

func TestWriteAllowlistSection_EmptyPath(t *testing.T) {
	t.Parallel()

	if err := WriteAllowlistSection("", AllowlistConfig{}); err == nil {
		t.Fatalf("WriteAllowlistSection(empty path): expected error, got nil")
	}
}
