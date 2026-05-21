package images

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/evanmschultz/valv/internal/tools"
)

// canonicalTool is the per-tool record consumed by both overlay hashing and
// (in Unit 12.2) Dockerfile emission. Source and Install are trimmed exactly
// once via canonicalManifest so downstream consumers never re-trim.
type canonicalTool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Install string `json:"install"`
}

// canonicalManifest returns a deterministic sorted-by-name slice of
// canonicalTool records derived from manifest.Tools. strings.TrimSpace is
// applied EXACTLY ONCE here to Source and Install; both the overlay hash
// (OverlayHash) and the Dockerfile emitter (Unit 12.2) consume this slice so
// trimming never happens twice.
func canonicalManifest(manifest tools.ToolManifest) []canonicalTool {
	if len(manifest.Tools) == 0 {
		return []canonicalTool{}
	}
	out := make([]canonicalTool, 0, len(manifest.Tools))
	for name, spec := range manifest.Tools {
		out = append(out, canonicalTool{
			Name:    name,
			Version: spec.Version,
			Source:  strings.TrimSpace(spec.Source),
			Install: strings.TrimSpace(spec.Install),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// OverlayHash returns the sha256 hex digest of the canonical-manifest JSON
// for use as the per-project image freshness fingerprint. The hash is stable
// across declaration-order permutations and across leading/trailing whitespace
// variations in Source/Install (canonicalManifest applies strings.TrimSpace
// once). The full lowercase hex string is returned; callers that need the
// 12-char short form pass the result to shortOverlayHash to avoid re-hashing.
func OverlayHash(manifest tools.ToolManifest) string {
	canonical := canonicalManifest(manifest)
	payload, err := json.MarshalIndent(canonical, "", "")
	if err != nil {
		// canonicalTool contains only strings — json.MarshalIndent cannot
		// fail on this input. Returning empty string keeps the function
		// total; callers comparing hashes will see a mismatch and rebuild.
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// shortOverlayHash returns the first 12 hex chars of an already-computed
// OverlayHash result. It takes the hash string (not the manifest) so the
// tag-construction site does not re-hash. Short form is suitable for the
// `proj-<short>` Docker tag suffix used by Unit 12.3.
func shortOverlayHash(hash string) string {
	if len(hash) < 12 {
		return hash
	}
	return hash[:12]
}
