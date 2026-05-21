package tools

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/evanmschultz/valv/internal/domain"
)

// ToolsFilePath is the project-relative path at which Valv looks for the
// declarative toolchain manifest. A project declares its container's tools
// by writing this file at its repository root.
const ToolsFilePath = ".valv/tools.toml"

// Resolve loads and validates the `.valv/tools.toml` manifest for the
// project rooted at projectDir.
//
// An absent file is not an error: when Load reports domain.ErrToolsNotFound
// Resolve returns an empty ToolManifest and a nil error, representing "this
// project declares no tools." Any other Load error or any Validate error is
// wrapped and returned.
//
// Resolve binds the absent-file check exclusively to
// errors.Is(err, domain.ErrToolsNotFound); it does not fall back to
// os.IsNotExist. Callers that want a different absent-file policy can call
// Load and Validate directly.
func Resolve(projectDir string) (ToolManifest, error) {
	path := filepath.Join(projectDir, ToolsFilePath)

	m, err := Load(path)
	if err != nil {
		if errors.Is(err, domain.ErrToolsNotFound) {
			return ToolManifest{}, nil
		}
		return ToolManifest{}, fmt.Errorf("resolve tools %q: %w", projectDir, err)
	}

	if err := Validate(m); err != nil {
		return ToolManifest{}, fmt.Errorf("resolve tools %q: %w", projectDir, err)
	}

	return m, nil
}
