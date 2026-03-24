package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeReturnsAbsolutePathForMissingLocation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	missing := filepath.Join(root, "missing", "child")

	got, err := Normalize(missing)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	want, err := filepath.Abs(missing)
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	if got != want {
		t.Fatalf("Normalize() = %q, want %q", got, want)
	}
}

func TestNormalizeResolvesExistingSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "linked-root")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	got, err := Normalize(link)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if got != want {
		t.Fatalf("Normalize() = %q, want %q", got, want)
	}
}

func TestNormalizeRejectsEmptyPath(t *testing.T) {
	t.Parallel()

	if _, err := Normalize("   "); err == nil {
		t.Fatal("Normalize() error = nil, want failure")
	}
}
