package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/evanmschultz/valv/internal/pathutil"
)

// Result describes the resolved project root and whether it was identified by a git marker.
type Result struct {
	Root         string
	GitMarker    string
	HasGitMarker bool
}

// Detect resolves the current project from the process working directory.
func Detect() (Result, error) {
	wd, err := os.Getwd()
	if err != nil {
		return Result{}, fmt.Errorf("detect project from cwd: %w", err)
	}
	return DetectFrom(wd)
}

// DetectFrom resolves the current project from an explicit starting path.
func DetectFrom(start string) (Result, error) {
	current, err := normalizeStart(start)
	if err != nil {
		return Result{}, fmt.Errorf("normalize project start: %w", err)
	}
	fallback := current

	for {
		marker := filepath.Join(current, ".git")
		if info, err := os.Stat(marker); err == nil {
			if info.IsDir() || info.Mode().IsRegular() {
				return Result{
					Root:         current,
					GitMarker:    marker,
					HasGitMarker: true,
				}, nil
			}
		} else if !os.IsNotExist(err) {
			return Result{}, fmt.Errorf("inspect git marker %q: %w", marker, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return Result{
				Root:         fallback,
				HasGitMarker: false,
			}, nil
		}
		current = parent
	}
}

func normalizeStart(start string) (string, error) {
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = wd
	}

	abs, err := pathutil.Normalize(start)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return abs, nil
	}
	return filepath.Dir(abs), nil
}
