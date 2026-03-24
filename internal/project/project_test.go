package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evanmschultz/valv/internal/pathutil"
)

func TestDetectFromNormalRepoRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	wantRoot := mustAbs(t, root)
	mustMkdirAll(t, filepath.Join(root, ".git"))
	mustMkdirAll(t, filepath.Join(root, "nested", "child"))

	got, err := DetectFrom(filepath.Join(root, "nested", "child"))
	if err != nil {
		t.Fatalf("DetectFrom() error = %v", err)
	}
	if got.Root != wantRoot {
		t.Fatalf("DetectFrom().Root = %q, want %q", got.Root, wantRoot)
	}
	if !got.HasGitMarker {
		t.Fatal("expected git marker")
	}
	if got.GitMarker != filepath.Join(wantRoot, ".git") {
		t.Fatalf("DetectFrom().GitMarker = %q", got.GitMarker)
	}
}

func TestDetectFromLinkedWorktreeRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	wantRoot := mustAbs(t, root)
	mustWriteFile(t, filepath.Join(root, ".git"), "gitdir: /tmp/any-admin-dir\n")
	mustMkdirAll(t, filepath.Join(root, "pkg", "sub"))

	got, err := DetectFrom(filepath.Join(root, "pkg", "sub"))
	if err != nil {
		t.Fatalf("DetectFrom() error = %v", err)
	}
	if got.Root != wantRoot {
		t.Fatalf("DetectFrom().Root = %q, want %q", got.Root, wantRoot)
	}
	if !got.HasGitMarker {
		t.Fatal("expected git marker")
	}
	if got.GitMarker != filepath.Join(wantRoot, ".git") {
		t.Fatalf("DetectFrom().GitMarker = %q", got.GitMarker)
	}
}

func TestDetectFromFallsBackToCurrentDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "project", "nested"))
	wantRoot := mustAbs(t, filepath.Join(root, "project", "nested"))

	got, err := DetectFrom(filepath.Join(root, "project", "nested"))
	if err != nil {
		t.Fatalf("DetectFrom() error = %v", err)
	}
	if got.Root != wantRoot {
		t.Fatalf("DetectFrom().Root = %q, want %q", got.Root, wantRoot)
	}
	if got.HasGitMarker {
		t.Fatal("did not expect git marker")
	}
}

func TestDetectFromFilePathUsesContainingDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	wantRoot := mustAbs(t, root)
	mustMkdirAll(t, filepath.Join(root, ".git"))
	filePath := filepath.Join(root, "nested", "file.txt")
	mustMkdirAll(t, filepath.Dir(filePath))
	mustWriteFile(t, filePath, "hello\n")

	got, err := DetectFrom(filePath)
	if err != nil {
		t.Fatalf("DetectFrom() error = %v", err)
	}
	if got.Root != wantRoot {
		t.Fatalf("DetectFrom().Root = %q, want %q", got.Root, wantRoot)
	}
}

func TestDetectUsesCurrentWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() after chdir error = %v", err)
	}
	wantRoot := mustAbs(t, cwd)

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if got.Root != wantRoot {
		t.Fatalf("Detect().Root = %q, want %q", got.Root, wantRoot)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	got, err := pathutil.Normalize(path)
	if err != nil {
		t.Fatalf("Normalize(%q) error = %v", path, err)
	}
	return got
}

func TestDetectFromNormalizesSymlinkedRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, ".git"))
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "repo-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	mustMkdirAll(t, filepath.Join(link, "nested", "child"))

	got, err := DetectFrom(filepath.Join(link, "nested", "child"))
	if err != nil {
		t.Fatalf("DetectFrom() error = %v", err)
	}
	wantRoot := mustAbs(t, root)
	if got.Root != wantRoot {
		t.Fatalf("DetectFrom().Root = %q, want %q", got.Root, wantRoot)
	}
}
