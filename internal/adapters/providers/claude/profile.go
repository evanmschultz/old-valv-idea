package claude

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evanmschultz/valv/internal/pathutil"
)

// HostDefaultProfileName is the name of the default Claude host profile.
const HostDefaultProfileName = "default"

// DefaultHostProfile returns the name and home path for the default Claude
// host profile. The path uses an isolated Valv-managed directory rather than
// the host ~/.claude so that macOS keychain credentials are not mixed with
// container-managed state.
func DefaultHostProfile(homeDir string) (name string, homePath string, err error) {
	trimmedHome := strings.TrimSpace(homeDir)
	if trimmedHome == "" {
		return "", "", fmt.Errorf("resolve claude host profile: home directory is required")
	}
	resolvedHome, err := pathutil.Normalize(filepath.Join(trimmedHome, ".valv", "providers", "claude", "profiles", "default"))
	if err != nil {
		return "", "", fmt.Errorf("resolve claude host profile: normalize host home: %w", err)
	}
	return HostDefaultProfileName, resolvedHome, nil
}

// IsDefaultHostHome reports whether profileHome is the default Claude host
// profile path for the given homeDir.
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
