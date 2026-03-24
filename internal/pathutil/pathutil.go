package pathutil

import (
	"errors"
	"fmt"
	"io/fs"
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
