package main

import (
	"testing"
)

func TestParseAllowlist(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "empty string",
			raw:  "",
			want: []string{},
		},
		{
			name: "blank whitespace only",
			raw:  "   ",
			want: []string{},
		},
		{
			name: "single host",
			raw:  "github.com",
			want: []string{"github.com"},
		},
		{
			name: "multiple hosts",
			raw:  "github.com,proxy.golang.org",
			want: []string{"github.com", "proxy.golang.org"},
		},
		{
			name: "lowercases entries",
			raw:  "GitHub.COM,PROXY.GOLANG.ORG",
			want: []string{"github.com", "proxy.golang.org"},
		},
		{
			name: "trims whitespace around entries",
			raw:  "  github.com , proxy.golang.org  ",
			want: []string{"github.com", "proxy.golang.org"},
		},
		{
			name: "deduplicates exact entries",
			raw:  "github.com,github.com,proxy.golang.org",
			want: []string{"github.com", "proxy.golang.org"},
		},
		{
			name: "deduplicates case-folded entries",
			raw:  "GitHub.com,github.COM",
			want: []string{"github.com"},
		},
		{
			name: "empty entries from consecutive commas discarded",
			raw:  "github.com,,proxy.golang.org,",
			want: []string{"github.com", "proxy.golang.org"},
		},
		{
			name: "leading comma discarded",
			raw:  ",github.com",
			want: []string{"github.com"},
		},
		{
			name: "preserves first-appearance order",
			raw:  "sum.golang.org,github.com,proxy.golang.org",
			want: []string{"sum.golang.org", "github.com", "proxy.golang.org"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseAllowlist(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("parseAllowlist(%q) len = %d, want %d; got %v", tc.raw, len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseAllowlist(%q)[%d] = %q, want %q", tc.raw, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestHostAllowed(t *testing.T) {
	t.Parallel()
	allowed := []string{"github.com", "proxy.golang.org", "sum.golang.org", "objects.githubusercontent.com"}
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{
			name:   "exact match",
			target: "github.com",
			want:   true,
		},
		{
			name:   "exact match with port stripped",
			target: "github.com:443",
			want:   true,
		},
		{
			name:   "exact match port 80",
			target: "proxy.golang.org:80",
			want:   true,
		},
		{
			name:   "case insensitive target upper",
			target: "GitHub.COM",
			want:   true,
		},
		{
			name:   "case insensitive target mixed with port",
			target: "GitHub.COM:443",
			want:   true,
		},
		{
			name:   "subdomain does not match parent",
			target: "api.github.com",
			want:   false,
		},
		{
			name:   "subdomain with port does not match parent",
			target: "api.github.com:443",
			want:   false,
		},
		{
			name:   "wildcard pattern is not matched",
			target: "*.github.com",
			want:   false,
		},
		{
			name:   "unknown host",
			target: "evil.example.com",
			want:   false,
		},
		{
			name:   "unknown host with port",
			target: "evil.example.com:443",
			want:   false,
		},
		{
			name:   "empty target",
			target: "",
			want:   false,
		},
		{
			name:   "empty allowed list",
			target: "github.com",
			want:   false, // tested below with empty allowed
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// The last case needs an empty allowed list — handle separately.
			list := allowed
			if tc.name == "empty allowed list" {
				list = []string{}
			}
			got := hostAllowed(tc.target, list)
			if got != tc.want {
				t.Errorf("hostAllowed(%q, allowed) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

// TestHostAllowed_EmptyAllowedList verifies that an empty allowlist blocks all hosts.
func TestHostAllowed_EmptyAllowedList(t *testing.T) {
	t.Parallel()
	if hostAllowed("github.com", []string{}) {
		t.Error("hostAllowed with empty allowed list should return false for any host")
	}
	if hostAllowed("github.com:443", []string{}) {
		t.Error("hostAllowed with empty allowed list should return false for any host with port")
	}
}

// TestHostAllowed_NilAllowedList verifies that a nil allowlist blocks all hosts.
func TestHostAllowed_NilAllowedList(t *testing.T) {
	t.Parallel()
	if hostAllowed("github.com", nil) {
		t.Error("hostAllowed with nil allowed list should return false for any host")
	}
}
