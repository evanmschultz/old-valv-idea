package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

// --- ResolveWorktreeGitDir tests ---

func TestResolveWorktreeGitDirReturnsCommondirForLinkfile(t *testing.T) {
	t.Parallel()

	// Layout:
	//   <tmp>/bare/          ← common git dir (bare repo root)
	//   <tmp>/worktrees/main/ ← worktree gitdir
	//   <tmp>/project/.git   ← linkfile pointing to <tmp>/worktrees/main
	root := t.TempDir()
	bareDir := filepath.Join(root, "bare")
	worktreeGitDir := filepath.Join(root, "worktrees", "main")
	projectDir := filepath.Join(root, "project")

	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(bareDir) error = %v", err)
	}
	if err := os.MkdirAll(worktreeGitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(worktreeGitDir) error = %v", err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectDir) error = %v", err)
	}

	// Write the linkfile: gitdir: <abs-path-to-worktreeGitDir>
	linkContent := "gitdir: " + worktreeGitDir + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".git"), []byte(linkContent), 0o644); err != nil {
		t.Fatalf("WriteFile(.git linkfile) error = %v", err)
	}

	// commondir inside worktreeGitDir: relative path back to bareDir.
	// worktreeGitDir is <root>/worktrees/main, bareDir is <root>/bare.
	// Relative from worktreeGitDir to bareDir = ../../bare
	rel, err := filepath.Rel(worktreeGitDir, bareDir)
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDir, "commondir"), []byte(rel+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(commondir) error = %v", err)
	}

	got, err := ResolveWorktreeGitDir(projectDir)
	if err != nil {
		t.Fatalf("ResolveWorktreeGitDir() error = %v", err)
	}
	if got != bareDir {
		t.Fatalf("ResolveWorktreeGitDir() = %q, want %q", got, bareDir)
	}
}

func TestResolveWorktreeGitDirReturnsEmptyForRegularRepo(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	gitDir := filepath.Join(projectDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.git dir) error = %v", err)
	}

	got, err := ResolveWorktreeGitDir(projectDir)
	if err != nil {
		t.Fatalf("ResolveWorktreeGitDir() error = %v", err)
	}
	if got != "" {
		t.Fatalf("ResolveWorktreeGitDir() = %q, want empty for regular repo", got)
	}
}

func TestResolveWorktreeGitDirReturnsEmptyWhenGitMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectDir) error = %v", err)
	}

	got, err := ResolveWorktreeGitDir(projectDir)
	if err != nil {
		t.Fatalf("ResolveWorktreeGitDir() error = %v", err)
	}
	if got != "" {
		t.Fatalf("ResolveWorktreeGitDir() = %q, want empty when .git missing", got)
	}
}

func TestResolveWorktreeGitDirHandlesMalformedLinkfile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectDir) error = %v", err)
	}
	// File exists but does not start with "gitdir: "
	if err := os.WriteFile(filepath.Join(projectDir, ".git"), []byte("not a gitdir line\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.git malformed) error = %v", err)
	}

	got, err := ResolveWorktreeGitDir(projectDir)
	if err != nil {
		t.Fatalf("ResolveWorktreeGitDir() error = %v (want silent skip for malformed)", err)
	}
	if got != "" {
		t.Fatalf("ResolveWorktreeGitDir() = %q, want empty for malformed linkfile", got)
	}
}

func TestResolveWorktreeGitDirHandlesRelativeGitdirPath(t *testing.T) {
	t.Parallel()

	// Layout:
	//   <tmp>/bare/
	//   <tmp>/worktrees/main/
	//   <tmp>/project/.git  ← "gitdir: ../worktrees/main" (relative)
	root := t.TempDir()
	bareDir := filepath.Join(root, "bare")
	worktreeGitDir := filepath.Join(root, "worktrees", "main")
	projectDir := filepath.Join(root, "project")

	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(bareDir) error = %v", err)
	}
	if err := os.MkdirAll(worktreeGitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(worktreeGitDir) error = %v", err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectDir) error = %v", err)
	}

	// Relative gitdir path from projectDir to worktreeGitDir
	relGitdir, err := filepath.Rel(projectDir, worktreeGitDir)
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}
	linkContent := "gitdir: " + relGitdir + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".git"), []byte(linkContent), 0o644); err != nil {
		t.Fatalf("WriteFile(.git linkfile) error = %v", err)
	}

	// commondir: relative path from worktreeGitDir to bareDir
	relCommondir, err := filepath.Rel(worktreeGitDir, bareDir)
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDir, "commondir"), []byte(relCommondir+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(commondir) error = %v", err)
	}

	got, err := ResolveWorktreeGitDir(projectDir)
	if err != nil {
		t.Fatalf("ResolveWorktreeGitDir() error = %v", err)
	}
	if got != bareDir {
		t.Fatalf("ResolveWorktreeGitDir() = %q, want %q", got, bareDir)
	}
}

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
