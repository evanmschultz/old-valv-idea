package images

import (
	"strings"
	"testing"

	"github.com/evanmschultz/valv/internal/tools"
)

func TestOverlayHash_EmptyManifest(t *testing.T) {
	t.Parallel()

	got := OverlayHash(tools.ToolManifest{})
	if got == "" {
		t.Fatalf("OverlayHash(empty) returned empty string; expected stable sha256 hex")
	}
	if len(got) != 64 {
		t.Fatalf("OverlayHash(empty) length = %d, want 64 hex chars", len(got))
	}

	// Empty manifest must be stable across calls.
	again := OverlayHash(tools.ToolManifest{})
	if got != again {
		t.Fatalf("OverlayHash(empty) not stable across calls: %q vs %q", got, again)
	}

	// nil-tools-map and empty-tools-map must hash identically (both are
	// "no declared tools").
	withEmptyMap := OverlayHash(tools.ToolManifest{Tools: map[string]tools.ToolSpec{}})
	if got != withEmptyMap {
		t.Fatalf("OverlayHash(nil-map) %q != OverlayHash(empty-map) %q", got, withEmptyMap)
	}
}

func TestOverlayHash_SingleStringFormTool(t *testing.T) {
	t.Parallel()

	manifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"jq": {Version: "1.7"},
		},
	}
	got := OverlayHash(manifest)
	if len(got) != 64 {
		t.Fatalf("OverlayHash length = %d, want 64", len(got))
	}

	// Repeatable.
	again := OverlayHash(manifest)
	if got != again {
		t.Fatalf("OverlayHash not stable: %q vs %q", got, again)
	}
}

func TestOverlayHash_SingleObjectFormTool(t *testing.T) {
	t.Parallel()

	manifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"ta": {Source: "github.com/evanmschultz/ta@latest", Install: "go install"},
		},
	}
	got := OverlayHash(manifest)
	if len(got) != 64 {
		t.Fatalf("OverlayHash length = %d, want 64", len(got))
	}

	// Different from the string-form jq hash.
	jqHash := OverlayHash(tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{"jq": {Version: "1.7"}},
	})
	if got == jqHash {
		t.Fatalf("object-form and string-form manifests must produce different hashes; both = %q", got)
	}
}

func TestOverlayHash_OrderIndependent(t *testing.T) {
	t.Parallel()

	// Three tools declared via two map literals; Go map iteration order is
	// already non-deterministic, but canonicalManifest's sort.Slice must
	// ensure both manifests produce the same hash regardless of internal
	// map order.
	a := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"alpha":   {Version: "1.0"},
			"bravo":   {Source: "github.com/x/bravo@v1", Install: "go install"},
			"charlie": {Source: "@scope/charlie", Install: "npm install -g"},
		},
	}
	b := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"charlie": {Source: "@scope/charlie", Install: "npm install -g"},
			"alpha":   {Version: "1.0"},
			"bravo":   {Source: "github.com/x/bravo@v1", Install: "go install"},
		},
	}

	hashA := OverlayHash(a)
	hashB := OverlayHash(b)
	if hashA != hashB {
		t.Fatalf("declaration-order-independent hashes differ: %q vs %q", hashA, hashB)
	}
}

func TestOverlayHash_WhitespaceTrimSingleton(t *testing.T) {
	t.Parallel()

	clean := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"ta": {Source: "github.com/x/y", Install: "go install"},
		},
	}
	padded := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"ta": {Source: "  github.com/x/y  ", Install: "\tgo install\n"},
		},
	}

	if OverlayHash(clean) != OverlayHash(padded) {
		t.Fatalf("whitespace trim not applied: clean=%q padded=%q", OverlayHash(clean), OverlayHash(padded))
	}
}

// TestOverlayHash_StabilitySnapshot pins the canonical-form digest for a
// fixed manifest against accidental future drift (struct field reordering,
// json tag changes, indent argument change). If this test fails and the
// canonical form was changed deliberately, update the expected hex below
// and document the reason in the unit's BUILDER_WORKLOG.md entry.
func TestOverlayHash_StabilitySnapshot(t *testing.T) {
	t.Parallel()

	manifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"ta": {Source: "github.com/evanmschultz/ta@latest", Install: "go install"},
			"jq": {Version: "1.7"},
		},
	}
	const want = "7471483e6f2f684fa1054cdbb127dd744f1970c9f3c896428c5ca632574571c5"

	got := OverlayHash(manifest)
	if got != want {
		t.Fatalf("OverlayHash stability snapshot drifted:\n  got  = %q\n  want = %q", got, want)
	}
}

func TestShortOverlayHash(t *testing.T) {
	t.Parallel()

	const full = "9c3a7b1e8d4f0123456789abcdef0123456789abcdef0123456789abcdef0123"
	got := shortOverlayHash(full)
	if got != "9c3a7b1e8d4f" {
		t.Fatalf("shortOverlayHash(full) = %q, want %q", got, "9c3a7b1e8d4f")
	}
	if len(got) != 12 {
		t.Fatalf("shortOverlayHash length = %d, want 12", len(got))
	}

	// Short inputs (defensive — should never happen in normal use) are
	// returned unchanged rather than panicking.
	short := shortOverlayHash("abc")
	if short != "abc" {
		t.Fatalf("shortOverlayHash(short-input) = %q, want %q", short, "abc")
	}
}

func TestCanonicalManifest_TrimAppliedOnce(t *testing.T) {
	t.Parallel()

	manifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"x": {Source: "  github.com/x/y  ", Install: "  go install  "},
		},
	}
	got := canonicalManifest(manifest)
	if len(got) != 1 {
		t.Fatalf("canonicalManifest length = %d, want 1", len(got))
	}
	if got[0].Source != "github.com/x/y" {
		t.Fatalf("Source not trimmed: got %q", got[0].Source)
	}
	if got[0].Install != "go install" {
		t.Fatalf("Install not trimmed: got %q", got[0].Install)
	}

	// Sanity: internal whitespace must SURVIVE the trim (only leading/trailing
	// stripped) — Falsification 2.4 in the planner notes acknowledges this.
	internal := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"x": {Source: "github.com/x/y bar", Install: "go install"},
		},
	}
	gotInternal := canonicalManifest(internal)
	if !strings.Contains(gotInternal[0].Source, " ") {
		t.Fatalf("internal whitespace stripped; Source = %q", gotInternal[0].Source)
	}
}

func TestCanonicalManifest_SortedByName(t *testing.T) {
	t.Parallel()

	manifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"zebra":   {Version: "1"},
			"alpha":   {Version: "1"},
			"mango":   {Version: "1"},
			"bravo":   {Version: "1"},
			"charlie": {Version: "1"},
		},
	}
	got := canonicalManifest(manifest)
	want := []string{"alpha", "bravo", "charlie", "mango", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("canonicalManifest length = %d, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("position %d: got %q, want %q", i, got[i].Name, name)
		}
	}
}
