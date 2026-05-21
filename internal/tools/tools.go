// Package tools parses and resolves the per-project `.valv/tools.toml` file
// that declares which tools a project's Valv container should provide.
//
// DROP_11 owns the schema, parser, validation, and per-project resolution.
// DROP_12 will consume the parsed manifest to build per-project layered
// images. DROP_14 and DROP_15 will type-decode the [allowlist] and [env]
// sections; this package captures them as toml.Primitive and discards their
// contents.
package tools

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/evanmschultz/valv/internal/domain"
)

// ToolSpec describes a single declared tool. A plain string value in
// `[tools]` decodes to ToolSpec{Version: "<value>"}. An inline table
// `{ source = "...", install = "..." }` decodes to
// ToolSpec{Source: "...", Install: "..."}.
type ToolSpec struct {
	// Version is the requested version string for a registry-known tool
	// (e.g. "latest", "1.22", git sha). Mutually exclusive with Source +
	// Install at validation time.
	Version string
	// Source is the custom-install source ref (e.g. a Go module path with
	// version suffix). Used together with Install.
	Source string
	// Install is the install command/strategy for a custom-source tool
	// (e.g. "go install"). Used together with Source.
	Install string
}

// UnmarshalTOML implements the toml.Unmarshaler interface for ToolSpec so
// `[tools]` map values can be either a plain string (version shorthand) or
// an inline table with source/install fields. BurntSushi/toml cannot
// natively decode a map[string]ToolSpec whose values are heterogeneous, so
// the decode is dispatched on the runtime type of v.
func (t *ToolSpec) UnmarshalTOML(v interface{}) error {
	switch val := v.(type) {
	case string:
		t.Version = val
		return nil
	case map[string]interface{}:
		if src, ok := val["source"]; ok {
			s, ok := src.(string)
			if !ok {
				return fmt.Errorf("tool source must be a string, got %T", src)
			}
			t.Source = s
		}
		if inst, ok := val["install"]; ok {
			s, ok := inst.(string)
			if !ok {
				return fmt.Errorf("tool install must be a string, got %T", inst)
			}
			t.Install = s
		}
		return nil
	default:
		return fmt.Errorf("tool value must be a string or inline table, got %T", v)
	}
}

// ToolManifest is the parsed structure of a `.valv/tools.toml` file. The
// Allowlist and Env fields are captured as toml.Primitive for forward
// compatibility — DROP_14 and DROP_15 will type-decode them. DROP_11
// resolves them via meta.PrimitiveDecode into discarded targets so the
// strict undecoded-key check still works for genuine unknown sections.
type ToolManifest struct {
	Tools     map[string]ToolSpec `toml:"tools"`
	Allowlist toml.Primitive      `toml:"allowlist"`
	Env       toml.Primitive      `toml:"env"`
}

// Load parses the `.valv/tools.toml` file at the given path. It returns
// domain.ErrToolsNotFound (wrapped) when the file is absent so callers can
// distinguish "no manifest declared" from other I/O failures via
// errors.Is. The strict meta.Undecoded() check rejects any unknown
// top-level key beyond tools/allowlist/env. Allowlist and Env are
// explicitly decoded into discarded map[string]any targets before the
// undecoded check; declaring toml.Primitive fields alone is not enough to
// satisfy the strict check.
func Load(path string) (ToolManifest, error) {
	if strings.TrimSpace(path) == "" {
		return ToolManifest{}, fmt.Errorf("load tools: %w", domain.ErrToolsNotFound)
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ToolManifest{}, fmt.Errorf("load tools %q: %w", path, domain.ErrToolsNotFound)
		}
		return ToolManifest{}, fmt.Errorf("stat tools %q: %w", path, err)
	}

	var m ToolManifest
	meta, err := toml.DecodeFile(path, &m)
	if err != nil {
		return ToolManifest{}, fmt.Errorf("decode tools %q: %w", path, err)
	}

	// Mark Allowlist and Env as decoded so meta.Undecoded() does not
	// flag their inline keys (e.g. allowlist.hosts, env.GOPRIVATE) as
	// unknown. Empirical testing against BurntSushi/toml v1.6.0 confirmed
	// that declaring toml.Primitive fields alone is NOT enough; the
	// PrimitiveDecode call is mandatory. DROP_14 and DROP_15 will re-call
	// PrimitiveDecode with their typed targets when they implement those
	// sections — the discarded map[string]any here is intentional.
	var discardAllowlist map[string]any
	if err := meta.PrimitiveDecode(m.Allowlist, &discardAllowlist); err != nil {
		return ToolManifest{}, fmt.Errorf("decode tools %q allowlist: %w", path, err)
	}
	var discardEnv map[string]any
	if err := meta.PrimitiveDecode(m.Env, &discardEnv); err != nil {
		return ToolManifest{}, fmt.Errorf("decode tools %q env: %w", path, err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		return ToolManifest{}, fmt.Errorf("decode tools %q: unknown keys: %s", path, strings.Join(keys, ", "))
	}

	return m, nil
}
