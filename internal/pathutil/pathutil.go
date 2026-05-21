package pathutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Normalize returns an absolute, cleaned path and resolves symlinks when the
// target exists. Non-existent paths stay absolute and cleaned so callers can
// use the result for stable storage keys before all directories exist.
func Normalize(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("normalize path: path is required")
	}

	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("normalize path %q: absolute path: %w", path, err)
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return abs, nil
	}

	return "", fmt.Errorf("normalize path %q: resolve symlinks: %w", abs, err)
}

// ResolveWorktreeGitDir inspects <projectRoot>/.git and, when it is a git
// worktree linkfile, returns the absolute path of the common git directory
// (bare repo root). This is the directory that must be bind-mounted into a
// container so that git operations work inside a worktree-based checkout.
//
// Returns ("", nil) when:
//   - <projectRoot>/.git does not exist (not a git repo)
//   - <projectRoot>/.git is a directory (standard, non-worktree repo)
//   - <projectRoot>/.git is a file but does not contain a "gitdir: " line
//     (malformed linkfile — silently skipped)
//
// Returns a non-empty path and nil when a valid linkfile is found. If the
// gitdir contains a "commondir" file, its resolved path is returned; otherwise
// the gitdir itself is returned.
func ResolveWorktreeGitDir(projectRoot string) (string, error) {
	gitPath := filepath.Join(projectRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("resolve worktree git dir: stat %q: %w", gitPath, err)
	}
	if !info.Mode().IsRegular() {
		// Directory or other non-regular file — standard repo, no extra mount needed.
		return "", nil
	}

	raw, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("resolve worktree git dir: read linkfile %q: %w", gitPath, err)
	}

	const prefix = "gitdir: "
	line := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(line, prefix) {
		// Malformed linkfile — silently skip.
		return "", nil
	}

	gitdirRaw := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if gitdirRaw == "" {
		return "", nil
	}

	// Resolve gitdir path — may be relative to projectRoot.
	gitdir := gitdirRaw
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(projectRoot, gitdirRaw)
	}
	gitdir = filepath.Clean(gitdir)

	// Check for commondir file inside the worktree gitdir.
	commondirPath := filepath.Join(gitdir, "commondir")
	commondirRaw, err := os.ReadFile(commondirPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// No commondir — use gitdir directly.
			return gitdir, nil
		}
		return "", fmt.Errorf("resolve worktree git dir: read commondir %q: %w", commondirPath, err)
	}

	commondirLine := strings.TrimSpace(string(commondirRaw))
	if commondirLine == "" {
		return gitdir, nil
	}

	// commondir may be relative to gitdir.
	commondir := commondirLine
	if !filepath.IsAbs(commondir) {
		commondir = filepath.Join(gitdir, commondirLine)
	}
	return filepath.Clean(commondir), nil
}
