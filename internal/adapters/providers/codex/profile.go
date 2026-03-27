package codex

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evanmschultz/valv/internal/pathutil"
)

const HostDefaultProfileName = "default"

func DefaultHostProfile(homeDir string) (name string, homePath string, err error) {
	trimmedHome := strings.TrimSpace(homeDir)
	if trimmedHome == "" {
		return "", "", fmt.Errorf("resolve codex host profile: home directory is required")
	}
	resolvedHome, err := pathutil.Normalize(filepath.Join(trimmedHome, ".codex"))
	if err != nil {
		return "", "", fmt.Errorf("resolve codex host profile: normalize host home: %w", err)
	}
	return HostDefaultProfileName, resolvedHome, nil
}

func IsDefaultHostHome(profileHome string, homeDir string) bool {
	resolvedProfile, err := pathutil.Normalize(profileHome)
	if err != nil {
		return false
	}
	_, resolvedHome, err := DefaultHostProfile(homeDir)
	if err != nil {
		return false
	}
	return resolvedProfile == resolvedHome
}
