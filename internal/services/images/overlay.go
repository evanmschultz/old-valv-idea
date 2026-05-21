package images

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/tools"
)

const (
	installGoInstall  = "go install"
	installNpmInstall = "npm install -g"
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

// BuildOverlayDockerfile renders the per-project overlay Dockerfile that runs
// the install commands declared in manifest on top of baseImage. The Dockerfile
// uses exec-form RUN (JSON array) for every tool — Source values reach the
// container as a single literal argv element with no /bin/sh -c interpolation,
// closing the shell-injection surface that DROP_12 planner decision 6 calls
// out. Tools are emitted in canonicalManifest order (sorted by name) so layer
// caching is deterministic across declaration-order permutations.
//
// v1 supports two install verbs: "go install" and "npm install -g". String-form
// specs (Version set, Source/Install empty), unsupported install verbs, and
// empty Source/Install (after canonicalManifest trim) return a wrapped error.
func BuildOverlayDockerfile(manifest tools.ToolManifest, baseImage docker.ImageRef) (string, error) {
	canonical := canonicalManifest(manifest)

	var buf strings.Builder
	fmt.Fprintf(&buf, "FROM %s\n", baseImage.String())
	buf.WriteString("\n")
	buf.WriteString("USER root\n")
	buf.WriteString("\n")
	buf.WriteString("ENV NPM_CONFIG_UPDATE_NOTIFIER=false \\\n")
	buf.WriteString("    NPM_CONFIG_FUND=false \\\n")
	buf.WriteString("    NPM_CONFIG_AUDIT=false \\\n")
	buf.WriteString("    GOBIN=/usr/local/bin\n")
	buf.WriteString("\n")

	for _, tool := range canonical {
		// String-form detection: Version-only spec with Source AND Install
		// both empty. canonicalManifest has already trimmed Source/Install, so
		// the empty check here reflects "absent after trim" not "literally
		// blank in source". v1 rejects string-form tools per PLAN.md decision 7.
		if tool.Source == "" && tool.Install == "" {
			return "", fmt.Errorf("build overlay dockerfile: tool %q uses unsupported string-form spec (v1 requires object form with source+install)", tool.Name)
		}
		if tool.Source == "" {
			return "", fmt.Errorf("build overlay dockerfile: tool %q has empty source", tool.Name)
		}
		if tool.Install == "" {
			return "", fmt.Errorf("build overlay dockerfile: tool %q has empty install", tool.Name)
		}

		var argv []string
		switch tool.Install {
		case installGoInstall:
			argv = []string{"go", "install", tool.Source}
		case installNpmInstall:
			argv = []string{"npm", "install", "-g", tool.Source}
		default:
			return "", fmt.Errorf("build overlay dockerfile: tool %q has unsupported install verb %q", tool.Name, tool.Install)
		}

		// json.Marshal on a []string slice cannot fail — every element is a
		// string. The marshaled output is a JSON array Docker treats as exec-
		// form RUN: argv elements stay literal, no shell interpolation.
		encoded, err := json.Marshal(argv)
		if err != nil {
			return "", fmt.Errorf("build overlay dockerfile: marshal argv for tool %q: %w", tool.Name, err)
		}
		fmt.Fprintf(&buf, "RUN %s\n", encoded)
	}

	buf.WriteString("\n")
	buf.WriteString("USER valv\n")
	return buf.String(), nil
}
