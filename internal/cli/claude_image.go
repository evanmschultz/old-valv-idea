package cli

import (
	"os"
	"strings"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
)

// claudeImageRef resolves the Claude provider image reference. When the
// VALV_CLAUDE_IMAGE environment variable is unset or empty, it falls back to
// ("valv-claude", "dev"). Behavior parallels codexImageRef.
func claudeImageRef() dockeradapter.ImageRef {
	value := strings.TrimSpace(os.Getenv("VALV_CLAUDE_IMAGE"))
	if value == "" {
		return dockeradapter.NewImageRef("valv-claude", "dev")
	}

	lastSlash := strings.LastIndex(value, "/")
	lastColon := strings.LastIndex(value, ":")
	if lastColon > lastSlash {
		return dockeradapter.NewImageRef(value[:lastColon], value[lastColon+1:])
	}
	return dockeradapter.NewImageRef(value, "")
}

// claudeImageRepository returns the repository segment of the resolved Claude
// image reference.
func claudeImageRepository() string {
	ref := claudeImageRef()
	return ref.Repository
}

// claudeImageTag returns the tag segment of the resolved Claude image
// reference, defaulting to "dev" when the tag is empty.
func claudeImageTag() string {
	ref := claudeImageRef()
	if strings.TrimSpace(ref.Tag) == "" {
		return "dev"
	}
	return ref.Tag
}
