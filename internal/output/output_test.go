package output

import (
	"bytes"
	"testing"

	"github.com/evanmschultz/valv/internal/domain"
)

func TestResolveModePlainByDefaultForBuffer(t *testing.T) {
	t.Parallel()
	mode := ResolveMode(&bytes.Buffer{}, Policy{Format: domain.OutputFormatAuto, Style: domain.OutputStyleAuto})
	if mode.Format != domain.OutputFormatPlain {
		t.Fatalf("ResolveMode().Format = %q", mode.Format)
	}
}

func TestWriteRecordPlain(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := WriteRecord(&buf, Mode{Format: domain.OutputFormatPlain}, "Valv paths", []Field{{Label: "database", Value: "/tmp/db.sqlite"}})
	if err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if got, want := buf.String(), "Valv paths\ndatabase=/tmp/db.sqlite\n"; got != want {
		t.Fatalf("WriteRecord() output = %q, want %q", got, want)
	}
}

func TestWriteRecordJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := WriteRecord(&buf, Mode{Format: domain.OutputFormatJSON}, "Valv paths", []Field{{Label: "database", Value: "/tmp/db.sqlite"}})
	if err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if got, want := buf.String(), "{\n  \"database\": \"/tmp/db.sqlite\"\n}\n"; got != want {
		t.Fatalf("WriteRecord() output = %q, want %q", got, want)
	}
}

func TestResolveModeForcedHumanStyle(t *testing.T) {
	t.Parallel()

	mode := ResolveMode(&bytes.Buffer{}, Policy{
		Format: domain.OutputFormatHuman,
		Style:  domain.OutputStyleAlways,
	})
	if !mode.Styled {
		t.Fatal("expected styled human mode")
	}
}

func TestWriteRecordHuman(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteRecord(&buf, Mode{Format: domain.OutputFormatHuman, Styled: false}, "Valv version", []Field{{Label: "version", Value: "dev"}})
	if err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if got, want := buf.String(), "Valv version\n  version: dev\n"; got != want {
		t.Fatalf("WriteRecord() output = %q, want %q", got, want)
	}
}

func TestWriteListPlain(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteList(&buf, Mode{Format: domain.OutputFormatPlain}, "Profiles", []ListItem{{
		Title: "dev",
		Fields: []Field{
			{Label: "home", Value: "/tmp/dev"},
		},
	}})
	if err != nil {
		t.Fatalf("WriteList() error = %v", err)
	}
	if got, want := buf.String(), "Profiles\n- dev\n  home=/tmp/dev\n"; got != want {
		t.Fatalf("WriteList() output = %q, want %q", got, want)
	}
}

func TestWriteListJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteList(&buf, Mode{Format: domain.OutputFormatJSON}, "Profiles", []ListItem{{
		Title: "dev",
		Badge: "active",
	}})
	if err != nil {
		t.Fatalf("WriteList() error = %v", err)
	}
	if got, want := buf.String(), "{\n  \"items\": [\n    {\n      \"title\": \"dev\",\n      \"badge\": \"active\"\n    }\n  ]\n}\n"; got != want {
		t.Fatalf("WriteList() output = %q, want %q", got, want)
	}
}

func TestWriteListWithKeyJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteListWithKey(&buf, Mode{Format: domain.OutputFormatJSON}, "Profiles", "profiles", []ListItem{{
		Title: "dev",
		Badge: "active",
	}})
	if err != nil {
		t.Fatalf("WriteListWithKey() error = %v", err)
	}
	if got, want := buf.String(), "{\n  \"profiles\": [\n    {\n      \"title\": \"dev\",\n      \"badge\": \"active\"\n    }\n  ]\n}\n"; got != want {
		t.Fatalf("WriteListWithKey() output = %q, want %q", got, want)
	}
}

func TestWriteListHumanUsesFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteList(&buf, Mode{Format: domain.OutputFormatHuman, Styled: false}, "Profiles", []ListItem{{
		Title: "dev",
		Badge: "active",
		Fields: []Field{
			{Label: "provider", Value: "codex", Muted: true},
			{Label: "home", Value: "/tmp/dev", Identifier: true},
		},
	}})
	if err != nil {
		t.Fatalf("WriteList() error = %v", err)
	}
	if got, want := buf.String(), "Profiles\n- dev [ACTIVE]\n  provider: codex\n  home: /tmp/dev\n"; got != want {
		t.Fatalf("WriteList() output = %q, want %q", got, want)
	}
}

func TestWriteRecordHumanEmptyState(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := WriteRecord(&buf, Mode{Format: domain.OutputFormatHuman, Styled: false}, "Valv status", nil); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if got, want := buf.String(), "Valv status\n  (none)\n"; got != want {
		t.Fatalf("WriteRecord() output = %q, want %q", got, want)
	}
}

func TestWriteListHumanEmptyState(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := WriteList(&buf, Mode{Format: domain.OutputFormatHuman, Styled: false}, "Profiles", nil); err != nil {
		t.Fatalf("WriteList() error = %v", err)
	}
	if got, want := buf.String(), "Profiles\n  (none)\n"; got != want {
		t.Fatalf("WriteList() output = %q, want %q", got, want)
	}
}

func TestWriteListPlainEmptyState(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := WriteList(&buf, Mode{Format: domain.OutputFormatPlain}, "Profiles", nil); err != nil {
		t.Fatalf("WriteList() error = %v", err)
	}
	if got, want := buf.String(), "Profiles\n  (none)\n"; got != want {
		t.Fatalf("WriteList() output = %q, want %q", got, want)
	}
}

func TestListJSONKey(t *testing.T) {
	t.Parallel()

	if got, want := listJSONKey("profiles"), "profiles"; got != want {
		t.Fatalf("listJSONKey() = %q, want %q", got, want)
	}
	if got, want := listJSONKey(""), "items"; got != want {
		t.Fatalf("listJSONKey(\"\") = %q, want %q", got, want)
	}
}
