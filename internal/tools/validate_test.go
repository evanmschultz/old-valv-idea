package tools

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_ToolName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		toolKey string
		wantErr bool
	}{
		// Valid names — single-char alphanumeric or alnum-bounded with permitted chars between.
		{name: "simple_word", toolKey: "mage", wantErr: false},
		{name: "two_chars", toolKey: "gh", wantErr: false},
		{name: "single_char_letter", toolKey: "m", wantErr: false},
		{name: "single_char_digit", toolKey: "9", wantErr: false},
		{name: "path_like", toolKey: "github.com/foo/bar", wantErr: false},
		{name: "version_suffix", toolKey: "go-1.22", wantErr: false},
		{name: "namespaced", toolKey: "a.b.c", wantErr: false},
		{name: "double_dot_accepted_by_design", toolKey: "a..b", wantErr: false},
		{name: "double_slash_accepted_by_design", toolKey: "a//b", wantErr: false},
		{name: "multi_dash_accepted_by_design", toolKey: "a---b", wantErr: false},
		{name: "underscore_internal", toolKey: "go_tool", wantErr: false},

		// Invalid names — empty, whitespace, leading/trailing punctuation, disallowed chars.
		{name: "empty_string", toolKey: "", wantErr: true},
		{name: "embedded_space", toolKey: "with space", wantErr: true},
		{name: "leading_dash", toolKey: "-bad", wantErr: true},
		{name: "trailing_dash", toolKey: "bad-name-", wantErr: true},
		{name: "leading_underscore", toolKey: "_underscore", wantErr: true},
		{name: "trailing_dot", toolKey: "name.", wantErr: true},
		{name: "trailing_slash", toolKey: "name/", wantErr: true},
		{name: "bang_disallowed", toolKey: "bad!char", wantErr: true},
		{name: "leading_whitespace", toolKey: " name", wantErr: true},
		{name: "trailing_whitespace", toolKey: "name ", wantErr: true},
		{name: "at_disallowed", toolKey: "a@b", wantErr: true},
		{name: "non_ascii", toolKey: "naïve", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Wrap each name in a valid string-form spec so only the name
			// check is exercised. An empty tools map is itself valid, so
			// the test must populate one entry.
			m := ToolManifest{
				Tools: map[string]ToolSpec{
					tc.toolKey: {Version: "latest"},
				},
			}

			err := Validate(m)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate(name=%q): expected error, got nil", tc.toolKey)
				}
				if !strings.Contains(err.Error(), "invalid tool name") {
					t.Errorf("Validate(name=%q): error = %q, want it to mention %q", tc.toolKey, err.Error(), "invalid tool name")
				}
				// Empty-string special case: Go's map key formatting will still emit "" in the message.
				if tc.toolKey != "" && !strings.Contains(err.Error(), tc.toolKey) {
					t.Errorf("Validate(name=%q): error = %q, want it to mention the offending name", tc.toolKey, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate(name=%q): unexpected error: %v", tc.toolKey, err)
			}
		})
	}
}

func TestValidate_SpecShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		spec        ToolSpec
		wantErr     bool
		errContains string
	}{
		{
			name:    "string_form_valid",
			spec:    ToolSpec{Version: "latest"},
			wantErr: false,
		},
		{
			name:    "string_form_pinned_version",
			spec:    ToolSpec{Version: "1.22"},
			wantErr: false,
		},
		{
			name:    "object_form_valid",
			spec:    ToolSpec{Source: "github.com/evanmschultz/ta@main", Install: "go install"},
			wantErr: false,
		},
		{
			name:        "object_form_missing_install",
			spec:        ToolSpec{Source: "github.com/evanmschultz/ta@main"},
			wantErr:     true,
			errContains: "missing install",
		},
		{
			name:        "object_form_missing_source",
			spec:        ToolSpec{Install: "go install"},
			wantErr:     true,
			errContains: "missing source",
		},
		{
			name:        "mixed_version_and_source",
			spec:        ToolSpec{Version: "1.0", Source: "github.com/x/y", Install: "go install"},
			wantErr:     true,
			errContains: "version alongside source/install",
		},
		{
			name:        "mixed_version_and_source_only",
			spec:        ToolSpec{Version: "1.0", Source: "github.com/x/y"},
			wantErr:     true,
			errContains: "missing install",
		},
		{
			name:        "all_empty",
			spec:        ToolSpec{},
			wantErr:     true,
			errContains: "empty spec",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := ToolManifest{
				Tools: map[string]ToolSpec{
					"mage": tc.spec,
				},
			}

			err := Validate(m)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate(%+v): expected error, got nil", tc.spec)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("Validate(%+v): error = %q, want substring %q", tc.spec, err.Error(), tc.errContains)
				}
				if !strings.Contains(err.Error(), "mage") {
					t.Errorf("Validate(%+v): error = %q, want it to mention offending tool %q", tc.spec, err.Error(), "mage")
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate(%+v): unexpected error: %v", tc.spec, err)
			}
		})
	}
}

func TestValidate_EmptyManifest(t *testing.T) {
	t.Parallel()

	if err := Validate(ToolManifest{}); err != nil {
		t.Fatalf("Validate(zero ToolManifest): unexpected error: %v", err)
	}
	if err := Validate(ToolManifest{Tools: map[string]ToolSpec{}}); err != nil {
		t.Fatalf("Validate(empty Tools map): unexpected error: %v", err)
	}
}

func TestValidate_ToolCount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		count   int
		wantErr bool
	}{
		{name: "count_0", count: 0, wantErr: false},
		{name: "count_1", count: 1, wantErr: false},
		{name: "count_50_boundary", count: 50, wantErr: false},
		{name: "count_51_over_boundary", count: 51, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tools := make(map[string]ToolSpec, tc.count)
			for i := 0; i < tc.count; i++ {
				// Generate unique valid names: "tool0", "tool1", ...
				tools[fmt.Sprintf("tool%d", i)] = ToolSpec{Version: "latest"}
			}
			m := ToolManifest{Tools: tools}

			err := Validate(m)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate(count=%d): expected error, got nil", tc.count)
				}
				if !strings.Contains(err.Error(), "exceeds max") {
					t.Errorf("Validate(count=%d): error = %q, want substring %q", tc.count, err.Error(), "exceeds max")
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate(count=%d): unexpected error: %v", tc.count, err)
			}
		})
	}
}

func TestValidate_LoadedFixtureMissingInstall(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "invalid_object_missing_install.toml")
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%s): unexpected error: %v", path, err)
	}

	// Sanity: the parsed tool entry has source but no install.
	spec, ok := m.Tools["ta"]
	if !ok {
		t.Fatalf("Tools[ta] missing in loaded fixture")
	}
	if spec.Source == "" {
		t.Fatalf("Tools[ta].Source unexpectedly empty: %+v", spec)
	}
	if spec.Install != "" {
		t.Fatalf("Tools[ta].Install unexpectedly populated: %+v", spec)
	}

	err = Validate(m)
	if err == nil {
		t.Fatalf("Validate(loaded missing-install fixture): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing install") {
		t.Errorf("Validate: error = %q, want substring %q", err.Error(), "missing install")
	}
	if !strings.Contains(err.Error(), "ta") {
		t.Errorf("Validate: error = %q, want it to mention offending tool %q", err.Error(), "ta")
	}
}
