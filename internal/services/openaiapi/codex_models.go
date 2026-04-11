package openaiapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	openaiapi "github.com/evanmschultz/valv/internal/api/openai"
	"github.com/evanmschultz/valv/internal/domain"
)

const defaultAPIReasoningEffort = "medium"

type codexModelsCache struct {
	Models []codexModelMetadata `json:"models"`
}

type codexModelMetadata struct {
	Slug                  string                    `json:"slug"`
	DefaultReasoningLevel string                    `json:"default_reasoning_level"`
	SupportedReasoning    []codexSupportedReasoning `json:"supported_reasoning_levels"`
}

type codexSupportedReasoning struct {
	Effort string `json:"effort"`
}

func resolveAPIReasoningEffort(profile domain.Profile, request openaiapi.Request) string {
	if effort := strings.TrimSpace(request.ReasoningEffort); effort != "" {
		return effort
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		return defaultAPIReasoningEffort
	}
	cachePath := filepath.Join(profile.HomePath, "models_cache.json")
	content, err := os.ReadFile(cachePath)
	if err != nil {
		return defaultAPIReasoningEffort
	}
	var cache codexModelsCache
	if err := json.Unmarshal(content, &cache); err != nil {
		return defaultAPIReasoningEffort
	}
	for _, entry := range cache.Models {
		if strings.TrimSpace(entry.Slug) != model {
			continue
		}
		if effort := strings.TrimSpace(entry.DefaultReasoningLevel); effort != "" {
			return effort
		}
		break
	}
	return defaultAPIReasoningEffort
}

func codexConfigOverride(key, value string) string {
	return fmt.Sprintf(`%s=%q`, strings.TrimSpace(key), strings.TrimSpace(value))
}
