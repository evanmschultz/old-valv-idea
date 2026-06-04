package networkpolicy

import (
	"os"
	"strings"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
)

// proxyImageRef resolves the valv-proxy sidecar image reference. When the
// VALV_PROXY_IMAGE environment variable is unset or empty, it falls back to
// ("valv-proxy", "dev"). Behavior mirrors claudeImageRef and codexImageRef.
func proxyImageRef() dockeradapter.ImageRef {
	value := strings.TrimSpace(os.Getenv("VALV_PROXY_IMAGE"))
	if value == "" {
		return dockeradapter.NewImageRef("valv-proxy", "dev")
	}

	lastSlash := strings.LastIndex(value, "/")
	lastColon := strings.LastIndex(value, ":")
	if lastColon > lastSlash {
		return dockeradapter.NewImageRef(value[:lastColon], value[lastColon+1:])
	}
	return dockeradapter.NewImageRef(value, "")
}
