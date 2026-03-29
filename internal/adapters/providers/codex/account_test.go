package codex

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadAccountIdentityReturnsClaimsFromAuthFile(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{
  "auth_mode": "chatgpt",
  "tokens": {
    "id_token": "`+testIDToken(t, idTokenClaims{Email: "person@example.com", Name: "Person Example"})+`"
  }
}`), 0o600); err != nil {
		t.Fatalf("WriteFile(auth.json) error = %v", err)
	}

	identity, err := ReadAccountIdentity(home)
	if err != nil {
		t.Fatalf("ReadAccountIdentity() error = %v", err)
	}
	if !identity.LoggedIn {
		t.Fatal("LoggedIn = false, want true")
	}
	if got, want := identity.AuthMode, "chatgpt"; got != want {
		t.Fatalf("AuthMode = %q, want %q", got, want)
	}
	if got, want := identity.Email, "person@example.com"; got != want {
		t.Fatalf("Email = %q, want %q", got, want)
	}
	if got, want := identity.Name, "Person Example"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
}

func TestReadAccountIdentityHandlesAPIKeyModeWithoutEmail(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{
  "auth_mode": "api_key",
  "OPENAI_API_KEY": "sk-test"
}`), 0o600); err != nil {
		t.Fatalf("WriteFile(auth.json) error = %v", err)
	}

	identity, err := ReadAccountIdentity(home)
	if err != nil {
		t.Fatalf("ReadAccountIdentity() error = %v", err)
	}
	if !identity.LoggedIn {
		t.Fatal("LoggedIn = false, want true")
	}
	if got, want := identity.AuthMode, "api_key"; got != want {
		t.Fatalf("AuthMode = %q, want %q", got, want)
	}
	if identity.Email != "" {
		t.Fatalf("Email = %q, want empty", identity.Email)
	}
}

func TestReadAccountIdentityMissingAuthFileReturnsZeroIdentity(t *testing.T) {
	t.Parallel()

	identity, err := ReadAccountIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("ReadAccountIdentity() error = %v", err)
	}
	if identity.LoggedIn {
		t.Fatal("LoggedIn = true, want false")
	}
}

func testIDToken(t *testing.T, claims idTokenClaims) string {
	t.Helper()

	headerJSON, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("Marshal(header) error = %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal(claims) error = %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON) + ".signature"
}
