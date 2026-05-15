package cli

import (
	"testing"

	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
)

func TestClaudeAuthDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity claudeprovider.AccountIdentity
		want     string
	}{
		{
			name:     "logged in",
			identity: claudeprovider.AccountIdentity{LoggedIn: true},
			want:     "logged in",
		},
		{
			name:     "not logged in",
			identity: claudeprovider.AccountIdentity{LoggedIn: false},
			want:     "not logged in",
		},
		{
			name:     "logged in with email",
			identity: claudeprovider.AccountIdentity{LoggedIn: true, Email: "user@example.com"},
			want:     "logged in",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := claudeAuthDisplay(tc.identity)
			if got != tc.want {
				t.Errorf("claudeAuthDisplay() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClaudeEmailDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity claudeprovider.AccountIdentity
		want     string
	}{
		{
			name:     "email present",
			identity: claudeprovider.AccountIdentity{LoggedIn: true, Email: "user@example.com"},
			want:     "user@example.com",
		},
		{
			name:     "logged in but no email",
			identity: claudeprovider.AccountIdentity{LoggedIn: true},
			want:     "(identity unavailable)",
		},
		{
			name:     "not logged in",
			identity: claudeprovider.AccountIdentity{LoggedIn: false},
			want:     "(not logged in)",
		},
		{
			name:     "not logged in but email present shows email",
			identity: claudeprovider.AccountIdentity{LoggedIn: false, Email: "shown@example.com"},
			want:     "shown@example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := claudeEmailDisplay(tc.identity)
			if got != tc.want {
				t.Errorf("claudeEmailDisplay() = %q, want %q", got, tc.want)
			}
		})
	}
}
