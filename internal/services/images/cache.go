package images

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/evanmschultz/valv/internal/domain"
)

const versionCacheTTL = 24 * time.Hour

// versionCacheEntry holds a single provider's cached version and the timestamp
// at which it was last checked.
type versionCacheEntry struct {
	Version   string    `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
}

// versionCacheFile is the on-disk JSON schema.
type versionCacheFile struct {
	Providers map[string]versionCacheEntry `json:"providers"`
}

// defaultCachePath returns the canonical cache file path:
//
//	$XDG_CACHE_HOME/valv/version-cache.json  (via os.UserCacheDir)
//	$HOME/.cache/valv/version-cache.json      (fallback if UserCacheDir fails)
//	$TMPDIR/valv-version-cache.json           (last-resort fallback)
func defaultCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
		return filepath.Join(dir, "valv-version-cache.json")
	}
	return filepath.Join(dir, "valv", "version-cache.json")
}

// providerKey maps a domain.Provider to the string key used in the cache file.
func providerKey(p domain.Provider) string {
	return string(p)
}

// readVersionCache reads the cache file at path and returns the parsed contents.
// Any read or parse error is silently returned as an empty cache, never
// propagated to callers — cache errors must not block version resolution.
func readVersionCache(path string) versionCacheFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return versionCacheFile{}
	}
	var c versionCacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		return versionCacheFile{}
	}
	return c
}

// cachedVersion returns the cached version for provider p and whether it is
// still within TTL relative to now.  If no entry exists or the entry is stale,
// returns ("", false).
func cachedVersion(c versionCacheFile, p domain.Provider, now time.Time) (string, bool) {
	if c.Providers == nil {
		return "", false
	}
	entry, ok := c.Providers[providerKey(p)]
	if !ok {
		return "", false
	}
	if entry.Version == "" {
		return "", false
	}
	if now.Sub(entry.CheckedAt) >= versionCacheTTL {
		return "", false
	}
	return entry.Version, true
}

// writeVersionCache writes an updated entry for provider p into the cache file
// at path.  Existing entries for other providers are preserved.  Any error is
// returned so the caller can log it, but callers MUST treat write failures as
// non-fatal.
func writeVersionCache(path, version string, p domain.Provider, now time.Time) error {
	c := readVersionCache(path)
	if c.Providers == nil {
		c.Providers = make(map[string]versionCacheEntry)
	}
	c.Providers[providerKey(p)] = versionCacheEntry{
		Version:   version,
		CheckedAt: now.UTC().Truncate(time.Second),
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal version cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create version cache dir: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write version cache: %w", err)
	}
	return nil
}
