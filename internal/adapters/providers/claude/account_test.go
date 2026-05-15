package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeCredentials writes a minimal .credentials.json fixture to dir.
func writeCredentials(t *testing.T, dir string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(.credentials.json): %v", err)
	}
}

// writeConfig writes a .claude.json config fixture to dir.
func writeConfig(t *testing.T, dir string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), b, 0o600); err != nil {
		t.Fatalf("WriteFile(.claude.json): %v", err)
	}
}

func TestReadAccountIdentity(t *testing.T) {
	t.Parallel()

	type oauthAccount struct {
		EmailAddress string `json:"emailAddress,omitempty"`
	}
	type configShape struct {
		OAuthAccount *oauthAccount `json:"oauthAccount,omitempty"`
	}

	tests := []struct {
		name         string
		setup        func(t *testing.T, dir string)
		wantLoggedIn bool
		wantEmail    string
	}{
		{
			name: "credentials present with email in config",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeCredentials(t, dir, `{"claudeAiOauth":{"accessToken":"tok","refreshToken":"ref"}}`)
				writeConfig(t, dir, configShape{OAuthAccount: &oauthAccount{EmailAddress: "user@example.com"}})
			},
			wantLoggedIn: true,
			wantEmail:    "user@example.com",
		},
		{
			name: "credentials present no email in config",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeCredentials(t, dir, `{"claudeAiOauth":{"accessToken":"tok"}}`)
				writeConfig(t, dir, configShape{OAuthAccount: &oauthAccount{}})
			},
			wantLoggedIn: true,
			wantEmail:    "",
		},
		{
			name: "credentials present no config file",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeCredentials(t, dir, `{"claudeAiOauth":{"accessToken":"tok"}}`)
			},
			wantLoggedIn: true,
			wantEmail:    "",
		},
		{
			name: "credentials present config has invalid JSON",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeCredentials(t, dir, `{"claudeAiOauth":{"accessToken":"tok"}}`)
				if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`NOT JSON`), 0o600); err != nil {
					t.Fatalf("WriteFile(.claude.json): %v", err)
				}
			},
			wantLoggedIn: true,
			wantEmail:    "",
		},
		{
			name: "no credentials file",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				// no files written — empty dir
			},
			wantLoggedIn: false,
			wantEmail:    "",
		},
		{
			name: "credentials path is a directory",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(dir, ".credentials.json"), 0o700); err != nil {
					t.Fatalf("Mkdir(.credentials.json): %v", err)
				}
			},
			wantLoggedIn: false,
			wantEmail:    "",
		},
		{
			name: "config uses legacy .config.json key",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeCredentials(t, dir, `{"claudeAiOauth":{"accessToken":"tok"}}`)
				b, err := json.Marshal(configShape{OAuthAccount: &oauthAccount{EmailAddress: "legacy@example.com"}})
				if err != nil {
					t.Fatalf("json.Marshal: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, ".config.json"), b, 0o600); err != nil {
					t.Fatalf("WriteFile(.config.json): %v", err)
				}
			},
			wantLoggedIn: true,
			wantEmail:    "legacy@example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			tc.setup(t, dir)

			identity, err := ReadAccountIdentity(dir)
			if err != nil {
				t.Fatalf("ReadAccountIdentity() error = %v", err)
			}
			if identity.LoggedIn != tc.wantLoggedIn {
				t.Errorf("LoggedIn = %v, want %v", identity.LoggedIn, tc.wantLoggedIn)
			}
			if identity.Email != tc.wantEmail {
				t.Errorf("Email = %q, want %q", identity.Email, tc.wantEmail)
			}
		})
	}
}
