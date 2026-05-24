package sqlite

import (
	"net/url"
	"path/filepath"
	"slices"
	"testing"
)

func TestOpenMemoryDatabase(t *testing.T) {
	t.Parallel()
	db, err := Open(OpenOptions{URI: "file:valv-test?mode=memory&cache=shared"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
}

func TestOpenRequiresPathOrURI(t *testing.T) {
	t.Parallel()
	if _, err := Open(OpenOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenFilePathDatabase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "valv.sqlite3")
	db, err := Open(OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS test_path (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
}

func TestOpenFilePathDatabaseEscapesSpecialCharacters(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "valv?#.sqlite3")
	db, err := Open(OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS test_special (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
}

// TestBuildDSNAppliesRequiredPragmas drives the four documented dedup
// scenarios for Unit 14.1 acceptance:
//
//  1. no existing pragmas -> both appended once;
//  2. existing busy_timeout(30000) -> preserved, no duplicate appended;
//  3. existing foreign_keys(0)    -> preserved, no duplicate appended;
//  4. existing _pragma=busy_timeout_pragma=foo (false-prefix value) ->
//     preserved AND busy_timeout(5000) IS appended because the parsed
//     pragma name `busy_timeout_pragma` does not equal `busy_timeout`.
func TestBuildDSNAppliesRequiredPragmas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		wantPragmas    []string
		mustContainAll []string // every entry must be present at least once
	}{
		{
			name:        "no existing pragmas appends both",
			input:       "file:valv-test?mode=memory&cache=shared",
			wantPragmas: []string{"busy_timeout(5000)", "foreign_keys(1)"},
		},
		{
			name:           "existing busy_timeout with different arg preserved",
			input:          "file:valv-test?mode=memory&_pragma=busy_timeout(30000)",
			mustContainAll: []string{"busy_timeout(30000)", "foreign_keys(1)"},
		},
		{
			name:           "existing foreign_keys with 0 preserved",
			input:          "file:valv-test?mode=memory&_pragma=foreign_keys(0)",
			mustContainAll: []string{"foreign_keys(0)", "busy_timeout(5000)"},
		},
		{
			name:           "false-prefix value still triggers append",
			input:          "file:valv-test?mode=memory&_pragma=busy_timeout_pragma%3Dfoo",
			mustContainAll: []string{"busy_timeout_pragma=foo", "busy_timeout(5000)", "foreign_keys(1)"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dsn, err := buildDSN(OpenOptions{URI: tc.input})
			if err != nil {
				t.Fatalf("buildDSN() error = %v", err)
			}
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatalf("url.Parse(%q) error = %v", dsn, err)
			}
			got := parsed.Query()["_pragma"]

			if tc.wantPragmas != nil {
				if len(got) != len(tc.wantPragmas) {
					t.Fatalf("_pragma len = %d, want %d; got=%#v", len(got), len(tc.wantPragmas), got)
				}
				for _, want := range tc.wantPragmas {
					if !slices.Contains(got, want) {
						t.Fatalf("_pragma missing %q in %#v", want, got)
					}
				}
			}
			for _, want := range tc.mustContainAll {
				if !slices.Contains(got, want) {
					t.Fatalf("_pragma missing %q in %#v", want, got)
				}
			}

			// busy_timeout(5000) and foreign_keys(1) must never appear more
			// than once: the dedup rule is exact pragma-name match, and we
			// only ever append one entry per required pragma.
			countBusyAdd := 0
			countFKAdd := 0
			for _, p := range got {
				if p == "busy_timeout(5000)" {
					countBusyAdd++
				}
				if p == "foreign_keys(1)" {
					countFKAdd++
				}
			}
			if countBusyAdd > 1 {
				t.Fatalf("busy_timeout(5000) appears %d times in %#v", countBusyAdd, got)
			}
			if countFKAdd > 1 {
				t.Fatalf("foreign_keys(1) appears %d times in %#v", countFKAdd, got)
			}
		})
	}
}

func TestBuildDSNFilePathAppliesBothPragmas(t *testing.T) {
	t.Parallel()

	dsn, err := buildDSN(OpenOptions{Path: filepath.Join(t.TempDir(), "valv.sqlite3")})
	if err != nil {
		t.Fatalf("buildDSN() error = %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", dsn, err)
	}
	got := parsed.Query()["_pragma"]
	if !slices.Contains(got, "busy_timeout(5000)") {
		t.Fatalf("path DSN missing busy_timeout(5000): %#v", got)
	}
	if !slices.Contains(got, "foreign_keys(1)") {
		t.Fatalf("path DSN missing foreign_keys(1): %#v", got)
	}
}

func TestParsePragmaName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"busy_timeout(5000)", "busy_timeout"},
		{"foreign_keys(1)", "foreign_keys"},
		{"  busy_timeout(5000)  ", "busy_timeout"},
		{"BUSY_TIMEOUT(5000)", "busy_timeout"},
		{"busy_timeout_pragma=foo", "busy_timeout_pragma"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range tests {
		if got := parsePragmaName(tc.input); got != tc.want {
			t.Fatalf("parsePragmaName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
