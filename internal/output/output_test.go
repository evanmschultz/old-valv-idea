package output

import (
	"bytes"
	"strings"
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
	if !strings.Contains(buf.String(), "database=/tmp/db.sqlite") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}

func TestWriteRecordJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := WriteRecord(&buf, Mode{Format: domain.OutputFormatJSON}, "Valv paths", []Field{{Label: "database", Value: "/tmp/db.sqlite"}})
	if err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if !strings.Contains(buf.String(), `"database": "/tmp/db.sqlite"`) {
		t.Fatalf("unexpected json output: %q", buf.String())
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
	if !strings.Contains(buf.String(), "version: dev") {
		t.Fatalf("unexpected human output: %q", buf.String())
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
	for _, want := range []string{"Profiles", "- dev", "home=/tmp/dev"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("unexpected plain list output %q missing %q", buf.String(), want)
		}
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
	for _, want := range []string{`"heading": "Profiles"`, `"title": "dev"`, `"badge": "active"`} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("unexpected json list output %q missing %q", buf.String(), want)
		}
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
	for _, want := range []string{"Profiles", "- dev [ACTIVE]", "provider: codex", "home: /tmp/dev"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("unexpected human list output %q missing %q", buf.String(), want)
		}
	}
}
