package tools

import (
	"fmt"
	"regexp"
)

// maxToolCount is the upper bound on declared tools per `.valv/tools.toml`.
// Manifests exceeding this count fail validation. The cap is intentionally
// generous — projects routinely declare a dozen or so tools; 50 leaves
// substantial headroom while bounding the worst-case image-build cost a
// future drop will pay.
const maxToolCount = 50

// toolNameRE is the canonical RE2-compatible regex for valid tool names.
//
// Accepts:
//   - single-character alphanumeric names (e.g. "m")
//   - multi-character names that start AND end alphanumeric, with the
//     permitted punctuation set ".", "_", "/", "-" allowed in between
//     (e.g. "mage", "go-1.22", "github.com/foo/bar", "a..b", "a---b")
//
// Rejects:
//   - empty string
//   - leading or trailing punctuation (e.g. "-bad", "bad-", "_x", "name.")
//   - whitespace anywhere
//   - non-ASCII characters
//   - characters outside the permitted set (e.g. "!", "@")
//
// Consecutive separator characters (e.g. "a..b", "a//b", "a---b") are
// accepted by design — see DROP_11 PLAN.md Schema Decisions.
var toolNameRE = regexp.MustCompile(`^[a-zA-Z0-9]+$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$`)

// Validate checks a ToolManifest for semantic correctness. It returns nil
// when every tool name matches toolNameRE, every entry is either pure
// string-form (Version non-empty; Source + Install empty) or pure
// object-form (Source + Install non-empty; Version empty), and the total
// tool count is within maxToolCount.
//
// Validate returns the first violation as a descriptive fmt.Errorf — it is
// not a multi-error. Callers that need every offending tool listed should
// call Validate iteratively after removing each reported violation.
func Validate(m ToolManifest) error {
	if len(m.Tools) > maxToolCount {
		return fmt.Errorf("validate tools: tool count %d exceeds max of %d", len(m.Tools), maxToolCount)
	}

	for name, spec := range m.Tools {
		if !toolNameRE.MatchString(name) {
			return fmt.Errorf("validate tools: invalid tool name %q (must match %s)", name, toolNameRE.String())
		}
		if err := validateSpec(name, spec); err != nil {
			return err
		}
	}

	return nil
}

// validateSpec enforces the string-form vs. object-form mutual exclusion
// for a single tool entry. A spec is string-form when only Version is set;
// it is object-form when only Source + Install are set. Any mixed shape —
// missing one of source/install in object form, or version set alongside
// source/install — is rejected.
func validateSpec(name string, spec ToolSpec) error {
	hasVersion := spec.Version != ""
	hasSource := spec.Source != ""
	hasInstall := spec.Install != ""

	switch {
	case hasSource || hasInstall:
		// Object form: both source and install required; version must be empty.
		if !hasSource {
			return fmt.Errorf("validate tools: tool %q has install but missing source", name)
		}
		if !hasInstall {
			return fmt.Errorf("validate tools: tool %q has source but missing install", name)
		}
		if hasVersion {
			return fmt.Errorf("validate tools: tool %q sets version alongside source/install (use one form)", name)
		}
		return nil
	case hasVersion:
		// String form: version set, source/install empty — already covered by the switch guard.
		return nil
	default:
		// All three empty.
		return fmt.Errorf("validate tools: tool %q has empty spec (set version, or source + install)", name)
	}
}
