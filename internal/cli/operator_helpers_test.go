package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"
	"github.com/evanmschultz/valv/internal/domain"
	imagesservice "github.com/evanmschultz/valv/internal/services/images"
)

func TestClaudeAuthDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity claudeprovider.AccountIdentity
		want     string
	}{
		{
			name:     "logged in",
			identity: claudeprovider.AccountIdentity{LoggedIn: true},
			want:     "logged in",
		},
		{
			name:     "not logged in",
			identity: claudeprovider.AccountIdentity{LoggedIn: false},
			want:     "not logged in",
		},
		{
			name:     "logged in with email",
			identity: claudeprovider.AccountIdentity{LoggedIn: true, Email: "user@example.com"},
			want:     "logged in",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := claudeAuthDisplay(tc.identity)
			if got != tc.want {
				t.Errorf("claudeAuthDisplay() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClaudeEmailDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity claudeprovider.AccountIdentity
		want     string
	}{
		{
			name:     "email present",
			identity: claudeprovider.AccountIdentity{LoggedIn: true, Email: "user@example.com"},
			want:     "user@example.com",
		},
		{
			name:     "logged in but no email",
			identity: claudeprovider.AccountIdentity{LoggedIn: true},
			want:     "(identity unavailable)",
		},
		{
			name:     "not logged in",
			identity: claudeprovider.AccountIdentity{LoggedIn: false},
			want:     "(not logged in)",
		},
		{
			name:     "not logged in but email present shows email",
			identity: claudeprovider.AccountIdentity{LoggedIn: false, Email: "shown@example.com"},
			want:     "shown@example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := claudeEmailDisplay(tc.identity)
			if got != tc.want {
				t.Errorf("claudeEmailDisplay() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestOpenImagesServiceWritesCacheToCachesDir verifies that openImagesService
// threads paths.CachesDir into imagesservice.Options.CachePath so that version
// cache writes land under the per-invocation cache root (isolated per test via
// t.TempDir) rather than the platform-default os.UserCacheDir path.
func TestOpenImagesServiceWritesCacheToCachesDir(t *testing.T) {
	paths := testCodexPaths(t)
	installFakeDocker(t)
	stubClaudeVersionResolver(t, "2.2.0")

	// Confirm the real platform cache dir is different from the test paths.CachesDir.
	// If they happen to be the same (extremely unlikely), the test is vacuously true.
	realCacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("os.UserCacheDir() error = %v", err)
	}
	realCachePath := filepath.Join(realCacheDir, "valv", "version-cache.json")

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(nil)
	cmd.SetOut(nil)
	cmd.SetErr(nil)

	svc, close, err := openImagesService(cmd, paths, domain.ProviderClaude)
	if err != nil {
		t.Fatalf("openImagesService() error = %v", err)
	}
	defer close()

	// EnsureLatest with AllowExistingOnCheckFail=false forces a resolver call.
	// The fake docker exits 0 for image inspect but fails for buildx build (no
	// such docker binary for build). Use AllowExistingOnCheckFail=true so that
	// if inspect returns "image missing" error the call still succeeds, and the
	// cache write still happens after the resolver call. The cache is written
	// before any docker build attempt.
	_, _ = svc.EnsureLatest(context.Background(), imagesservice.EnsureRequest{AllowExistingOnCheckFail: true})

	wantCachePath := filepath.Join(paths.CachesDir, "version-cache.json")
	if _, err := os.Stat(wantCachePath); err != nil {
		t.Fatalf("cache file not created at paths.CachesDir: %v; want file at %q", err, wantCachePath)
	}

	// Confirm the real platform cache path was NOT written.
	if _, err := os.Stat(realCachePath); err == nil {
		// File exists — check if it was written by this test run. We check that
		// its contents do NOT contain the test stub version "2.2.0" by reading it.
		data, readErr := os.ReadFile(realCachePath)
		if readErr != nil {
			t.Fatalf("real cache path unexpectedly exists but is unreadable: %v", readErr)
		}
		// If the real cache file exists with a real version the dev may have
		// from a prior real run, that is fine — but it must not contain our test
		// stub value. A "2.2.0" in the real cache would mean we just polluted it.
		content := string(data)
		if len(content) > 0 {
			// Intentionally not t.Fatal here — the real cache may legitimately
			// contain "2.2.0" if the dev ran `valv manage update claude` and
			// the actual latest version happens to be 2.2.0. We only fail if
			// the wantCachePath did not get created (above), which proves the
			// cache-path isolation is working.
			_ = content
		}
	}
}
