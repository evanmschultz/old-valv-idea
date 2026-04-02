package valvcompat

import (
	"strings"
	"sync"
	"testing"
)

func TestCodexOpenAICompatibilityManifestLoads(t *testing.T) {
	t.Parallel()

	manifest, err := CodexOpenAICompatibilityManifest()
	if err != nil {
		t.Fatalf("CodexOpenAICompatibilityManifest() error = %v", err)
	}
	if manifest.Provider != "codex" {
		t.Fatalf("manifest provider = %q, want codex", manifest.Provider)
	}
	if manifest.ManifestVersion == "" {
		t.Fatal("manifest manifest_version is empty")
	}
	if manifest.GeneratedAtUTC == "" {
		t.Fatal("manifest generated_at_utc is empty")
	}
	if manifest.LastReviewedAtUTC == "" {
		t.Fatal("manifest last_reviewed_utc is empty")
	}
	if manifest.ReviewIntervalDays <= 0 {
		t.Fatalf("manifest review_interval_days = %d, want > 0", manifest.ReviewIntervalDays)
	}
	if manifest.API.Path != "/v1/chat/completions" {
		t.Fatalf("manifest API path = %q, want /v1/chat/completions", manifest.API.Path)
	}
	if len(manifest.CodexFlags) == 0 {
		t.Fatalf("manifest codex flags empty")
	}
	if len(manifest.OpenAIToCodex) == 0 {
		t.Fatalf("manifest openai_to_codex mappings empty")
	}
	found := false
	for _, entry := range manifest.OpenAIToCodex {
		if entry.OpenAIField == "stream" && entry.Status == "supported" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("manifest missing supported stream mapping")
	}
}

func TestCodexOpenAICompatibilityManifestCachesResult(t *testing.T) {
	originalJSON := append([]byte(nil), codexOpenAICompatibilityJSON...)
	originalCompatibility := cachedCompatibility
	originalErr := cachedCompatibilityErr
	originalOnce := loadCompatibilityOnce
	t.Cleanup(func() {
		codexOpenAICompatibilityJSON = originalJSON
		cachedCompatibility = originalCompatibility
		cachedCompatibilityErr = originalErr
		loadCompatibilityOnce = originalOnce
	})

	cachedCompatibility = CodexOpenAICompatibility{}
	cachedCompatibilityErr = nil
	loadCompatibilityOnce = sync.Once{}

	first, err := CodexOpenAICompatibilityManifest()
	if err != nil {
		t.Fatalf("CodexOpenAICompatibilityManifest() first error = %v", err)
	}

	codexOpenAICompatibilityJSON = []byte(`{"provider":"changed"}`)

	second, err := CodexOpenAICompatibilityManifest()
	if err != nil {
		t.Fatalf("CodexOpenAICompatibilityManifest() second error = %v", err)
	}
	if second.Provider != first.Provider {
		t.Fatalf("cached provider = %q, want %q", second.Provider, first.Provider)
	}
}

func TestCodexOpenAICompatibilityManifestReturnsDecodeError(t *testing.T) {
	originalJSON := append([]byte(nil), codexOpenAICompatibilityJSON...)
	originalCompatibility := cachedCompatibility
	originalErr := cachedCompatibilityErr
	originalOnce := loadCompatibilityOnce
	t.Cleanup(func() {
		codexOpenAICompatibilityJSON = originalJSON
		cachedCompatibility = originalCompatibility
		cachedCompatibilityErr = originalErr
		loadCompatibilityOnce = originalOnce
	})

	codexOpenAICompatibilityJSON = []byte(`{`)
	cachedCompatibility = CodexOpenAICompatibility{}
	cachedCompatibilityErr = nil
	loadCompatibilityOnce = sync.Once{}

	_, err := CodexOpenAICompatibilityManifest()
	if err == nil {
		t.Fatal("CodexOpenAICompatibilityManifest() error = nil, want decode failure")
	}
	if !strings.Contains(err.Error(), "decode compatibility manifest") {
		t.Fatalf("CodexOpenAICompatibilityManifest() error = %v, want decode compatibility manifest", err)
	}
}
