package valvcompat

import (
	"encoding/json"
	"fmt"
	"sync"

	_ "embed"
)

//go:embed codex-openai-compatibility.json
var codexOpenAICompatibilityJSON []byte

type CodexOpenAICompatibility struct {
	Provider           string          `json:"provider"`
	ProviderCommand    string          `json:"provider_command"`
	RuntimeModel       string          `json:"runtime_model"`
	API                APISurface      `json:"api_surface"`
	ManifestVersion    string          `json:"manifest_version"`
	GeneratedAtUTC     string          `json:"generated_at_utc"`
	Updated            string          `json:"updated"`
	LastReviewedAtUTC  string          `json:"last_reviewed_utc"`
	ReviewIntervalDays int             `json:"review_interval_days"`
	CodexFlags         []CodexExecFlag `json:"codex_exec_flags"`
	OpenAIToCodex      []OpenAIToCodex `json:"openai_to_codex"`
}

type APISurface struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

type CodexExecFlag struct {
	Name          string `json:"name"`
	Short         string `json:"short,omitempty"`
	Description   string `json:"description"`
	APIEquivalent string `json:"api_equivalent"`
}

type OpenAIToCodex struct {
	OpenAIField string    `json:"openai_field"`
	OpenAIType  string    `json:"openai_type"`
	Status      string    `json:"status"`
	Codex       CodexCall `json:"codex_invocation"`
	Notes       string    `json:"notes"`
}

type CodexCall struct {
	Command []string `json:"command"`
}

var (
	cachedCompatibility    CodexOpenAICompatibility
	cachedCompatibilityErr error
	loadCompatibilityOnce  sync.Once
)

func CodexOpenAICompatibilityManifest() (CodexOpenAICompatibility, error) {
	loadCompatibilityOnce.Do(func() {
		cachedCompatibilityErr = json.Unmarshal(codexOpenAICompatibilityJSON, &cachedCompatibility)
		if cachedCompatibilityErr != nil {
			cachedCompatibilityErr = fmt.Errorf("decode compatibility manifest: %w", cachedCompatibilityErr)
			return
		}
	})
	if cachedCompatibilityErr != nil {
		return CodexOpenAICompatibility{}, cachedCompatibilityErr
	}
	return cachedCompatibility, nil
}
