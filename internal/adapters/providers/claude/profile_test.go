package claude

import (
	"path/filepath"
	"testing"
)

func TestDefaultHostProfileReturnsIsolatedPath(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	name, homePath, err := DefaultHostProfile(homeDir)
	if err != nil {
		t.Fatalf("DefaultHostProfile() error = %v", err)
	}
	if name != HostDefaultProfileName {
		t.Fatalf("name = %q, want %q", name, HostDefaultProfileName)
	}
	wantSuffix := filepath.Join(".valv", "providers", "claude", "profiles", "default")
	if !filepath.IsAbs(homePath) {
		t.Fatalf("homePath = %q, want absolute path", homePath)
	}
	if got := filepath.Join(homeDir, wantSuffix); homePath != got {
		t.Fatalf("homePath = %q, want %q", homePath, got)
	}
}

func TestDefaultHostProfileRejectsEmptyHome(t *testing.T) {
	t.Parallel()

	_, _, err := DefaultHostProfile("")
	if err == nil {
		t.Fatal("DefaultHostProfile(\"\") error = nil, want error")
	}
}

func TestIsDefaultHostHomeReturnsTrue(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	_, isolatedPath, err := DefaultHostProfile(homeDir)
	if err != nil {
		t.Fatalf("DefaultHostProfile() error = %v", err)
	}
	if !IsDefaultHostHome(isolatedPath, homeDir) {
		t.Fatalf("IsDefaultHostHome(%q, %q) = false, want true", isolatedPath, homeDir)
	}
}

func TestIsDefaultHostHomeReturnsFalse(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	otherPath := filepath.Join(homeDir, ".other-dir")
	if IsDefaultHostHome(otherPath, homeDir) {
		t.Fatalf("IsDefaultHostHome(%q, %q) = true, want false", otherPath, homeDir)
	}
}
