package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAccountIdentityCredentialsFilePresent(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".credentials.json"), []byte(`{"claudeAiAccessToken":"tok"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(.credentials.json) error = %v", err)
	}

	identity, err := ReadAccountIdentity(home)
	if err != nil {
		t.Fatalf("ReadAccountIdentity() error = %v", err)
	}
	if !identity.LoggedIn {
		t.Fatal("LoggedIn = false, want true")
	}
	if identity.Email != "" {
		t.Fatalf("Email = %q, want empty (presence-only check)", identity.Email)
	}
	if identity.Name != "" {
		t.Fatalf("Name = %q, want empty (presence-only check)", identity.Name)
	}
}

func TestReadAccountIdentityMissingCredentialsReturnsNotLoggedIn(t *testing.T) {
	t.Parallel()

	identity, err := ReadAccountIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("ReadAccountIdentity() error = %v", err)
	}
	if identity.LoggedIn {
		t.Fatal("LoggedIn = true, want false")
	}
}

func TestReadAccountIdentityIrrelevantContentsStillLoggedIn(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".credentials.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile(.credentials.json) error = %v", err)
	}

	identity, err := ReadAccountIdentity(home)
	if err != nil {
		t.Fatalf("ReadAccountIdentity() error = %v", err)
	}
	if !identity.LoggedIn {
		t.Fatal("LoggedIn = false, want true (format is irrelevant — presence only)")
	}
}
