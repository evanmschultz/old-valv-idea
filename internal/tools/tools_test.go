package tools

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evanmschultz/valv/internal/domain"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	type assertion func(t *testing.T, m ToolManifest, err error)

	cases := []struct {
		name   string
		path   string
		assert assertion
	}{
		{
			name: "valid_simple",
			path: filepath.Join("testdata", "valid_simple.toml"),
			assert: func(t *testing.T, m ToolManifest, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("Load: unexpected error: %v", err)
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
				if m.Tools["mage"].Source != "" || m.Tools["mage"].Install != "" {
					t.Errorf("string-form tool has non-empty Source/Install: %+v", m.Tools["mage"])
				}
			},
		},
		{
			name: "valid_objects_with_forward_compat_sections",
			path: filepath.Join("testdata", "valid_objects.toml"),
			assert: func(t *testing.T, m ToolManifest, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("Load: unexpected error: %v", err)
				}
				if got, want := len(m.Tools), 3; got != want {
					t.Fatalf("len(Tools) = %d, want %d", got, want)
				}
				if got, want := m.Tools["mage"].Version, "latest"; got != want {
					t.Errorf("Tools[mage].Version = %q, want %q", got, want)
				}
				ta, ok := m.Tools["ta"]
				if !ok {
					t.Fatalf("Tools[ta] missing")
				}
				if got, want := ta.Source, "github.com/evanmschultz/ta@main"; got != want {
					t.Errorf("Tools[ta].Source = %q, want %q", got, want)
				}
				if got, want := ta.Install, "go install"; got != want {
					t.Errorf("Tools[ta].Install = %q, want %q", got, want)
				}
				if ta.Version != "" {
					t.Errorf("object-form tool has non-empty Version: %q", ta.Version)
				}
			},
		},
		{
			name: "valid_quoted_names",
			path: filepath.Join("testdata", "valid_quoted_names.toml"),
			assert: func(t *testing.T, m ToolManifest, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("Load: unexpected error: %v", err)
				}
				if got, want := len(m.Tools), 1; got != want {
					t.Fatalf("len(Tools) = %d, want %d (quoted key should produce a single flat entry, not nested tables)", got, want)
				}
				spec, ok := m.Tools["github.com/foo/bar"]
				if !ok {
					t.Fatalf("Tools[%q] missing; got keys %v", "github.com/foo/bar", keysOf(m.Tools))
				}
				if got, want := spec.Source, "github.com/foo/bar@main"; got != want {
					t.Errorf("Tools[github.com/foo/bar].Source = %q, want %q", got, want)
				}
				if got, want := spec.Install, "go install"; got != want {
					t.Errorf("Tools[github.com/foo/bar].Install = %q, want %q", got, want)
				}
			},
		},
		{
			name: "invalid_unknown_key",
			path: filepath.Join("testdata", "invalid_unknown_key.toml"),
			assert: func(t *testing.T, m ToolManifest, err error) {
				t.Helper()
				if err == nil {
					t.Fatalf("Load: expected error for unknown top-level key, got nil")
				}
				if !strings.Contains(err.Error(), "unknown keys") {
					t.Errorf("error message = %q, want it to mention unknown keys", err.Error())
				}
				if !strings.Contains(err.Error(), "network") {
					t.Errorf("error message = %q, want it to mention the offending key %q", err.Error(), "network")
				}
			},
		},
		{
			name: "absent_file_returns_sentinel",
			path: filepath.Join("testdata", "does_not_exist.toml"),
			assert: func(t *testing.T, m ToolManifest, err error) {
				t.Helper()
				if err == nil {
					t.Fatalf("Load: expected error for absent file, got nil")
				}
				if !errors.Is(err, domain.ErrToolsNotFound) {
					t.Errorf("error = %v, want errors.Is(err, domain.ErrToolsNotFound) == true", err)
				}
			},
		},
		{
			name: "empty_path_returns_sentinel",
			path: "",
			assert: func(t *testing.T, m ToolManifest, err error) {
				t.Helper()
				if err == nil {
					t.Fatalf("Load: expected error for empty path, got nil")
				}
				if !errors.Is(err, domain.ErrToolsNotFound) {
					t.Errorf("error = %v, want errors.Is(err, domain.ErrToolsNotFound) == true", err)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := Load(tc.path)
			tc.assert(t, m, err)
		})
	}
}

func TestToolSpec_UnmarshalTOML_RejectsBadValueShape(t *testing.T) {
	t.Parallel()

	// Reject a non-string source value.
	var spec ToolSpec
	err := spec.UnmarshalTOML(map[string]interface{}{"source": 42, "install": "go install"})
	if err == nil {
		t.Fatalf("UnmarshalTOML(non-string source): expected error, got nil")
	}

	// Reject a non-string install value.
	spec = ToolSpec{}
	err = spec.UnmarshalTOML(map[string]interface{}{"source": "github.com/x/y", "install": 7})
	if err == nil {
		t.Fatalf("UnmarshalTOML(non-string install): expected error, got nil")
	}

	// Reject a wholly wrong type (e.g. an int).
	spec = ToolSpec{}
	err = spec.UnmarshalTOML(int64(99))
	if err == nil {
		t.Fatalf("UnmarshalTOML(int64): expected error, got nil")
	}

	// String value succeeds.
	spec = ToolSpec{}
	if err := spec.UnmarshalTOML("1.22"); err != nil {
		t.Fatalf("UnmarshalTOML(string): unexpected error: %v", err)
	}
	if spec.Version != "1.22" {
		t.Errorf("Version = %q, want %q", spec.Version, "1.22")
	}
}

func keysOf(m map[string]ToolSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
