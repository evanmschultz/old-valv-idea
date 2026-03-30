package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/evanmschultz/laslig"

	"github.com/evanmschultz/valv/internal/domain"
)

type Policy struct {
	Format domain.OutputFormat
	Style  domain.OutputStyle
}

type Mode struct {
	Format domain.OutputFormat
	Styled bool
	Width  int
}

type (
	Field    = laslig.Field
	ListItem = laslig.ListItem
)

func ResolveMode(out io.Writer, policy Policy) Mode {
	resolved := laslig.ResolveMode(out, toLasligPolicy(policy))
	return Mode{
		Format: fromLasligFormat(resolved.Format),
		Styled: resolved.Styled,
		Width:  resolved.Width,
	}
}

func WriteRecord(out io.Writer, mode Mode, heading string, fields []Field) error {
	if mode.Format == domain.OutputFormatJSON {
		payload := make(map[string]any, len(fields))
		for _, field := range fields {
			payload[strings.ReplaceAll(strings.ToLower(field.Label), " ", "_")] = field.Value
		}
		encoder := json.NewEncoder(out)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			return fmt.Errorf("write json record: %w", err)
		}
		return nil
	}

	if mode.Format == domain.OutputFormatPlain {
		if _, err := fmt.Fprintf(out, "%s\n", heading); err != nil {
			return fmt.Errorf("write plain heading: %w", err)
		}
		if len(fields) == 0 {
			return writePlainEmptyState(out)
		}
		for _, field := range fields {
			if _, err := fmt.Fprintf(out, "%s=%s\n", strings.ReplaceAll(strings.ToLower(field.Label), " ", "_"), field.Value); err != nil {
				return fmt.Errorf("write plain field: %w", err)
			}
		}
		return nil
	}

	if err := newHumanPrinter(out, mode).Record(laslig.Record{
		Title:  heading,
		Fields: fields,
	}); err != nil {
		return fmt.Errorf("write human record: %w", err)
	}
	return nil
}

func WriteList(out io.Writer, mode Mode, heading string, items []ListItem) error {
	return WriteListWithKey(out, mode, heading, "", items)
}

func WriteListWithKey(out io.Writer, mode Mode, heading string, jsonKey string, items []ListItem) error {
	if mode.Format == domain.OutputFormatJSON {
		payload := map[string]any{
			listJSONKey(jsonKey): items,
		}
		encoder := json.NewEncoder(out)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			return fmt.Errorf("write json list: %w", err)
		}
		return nil
	}

	if mode.Format == domain.OutputFormatPlain {
		if _, err := fmt.Fprintf(out, "%s\n", heading); err != nil {
			return fmt.Errorf("write plain list heading: %w", err)
		}
		if len(items) == 0 {
			if _, err := fmt.Fprintln(out, "- (none)"); err != nil {
				return fmt.Errorf("write plain list empty state: %w", err)
			}
			return nil
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(out, "- %s\n", item.Title); err != nil {
				return fmt.Errorf("write plain list item title: %w", err)
			}
			for _, field := range item.Fields {
				key := strings.ReplaceAll(strings.ToLower(field.Label), " ", "_")
				if _, err := fmt.Fprintf(out, "  %s=%s\n", key, field.Value); err != nil {
					return fmt.Errorf("write plain list field: %w", err)
				}
			}
		}
		return nil
	}

	if err := newHumanPrinter(out, mode).List(laslig.List{
		Title: heading,
		Items: items,
	}); err != nil {
		return fmt.Errorf("write human list: %w", err)
	}
	return nil
}

func writePlainEmptyState(out io.Writer) error {
	if _, err := fmt.Fprintln(out, "  (none)"); err != nil {
		return fmt.Errorf("write plain empty state: %w", err)
	}
	return nil
}

func newHumanPrinter(out io.Writer, mode Mode) *laslig.Printer {
	return laslig.NewWithMode(out, laslig.Mode{
		Format: laslig.FormatHuman,
		Styled: mode.Styled,
		Width:  mode.Width,
	})
}

func toLasligPolicy(policy Policy) laslig.Policy {
	return laslig.Policy{
		Format: toLasligFormat(policy.Format),
		Style:  toLasligStyle(policy.Style),
	}
}

func toLasligFormat(format domain.OutputFormat) laslig.Format {
	switch format {
	case domain.OutputFormatHuman:
		return laslig.FormatHuman
	case domain.OutputFormatPlain:
		return laslig.FormatPlain
	case domain.OutputFormatJSON:
		return laslig.FormatJSON
	default:
		return laslig.FormatAuto
	}
}

func fromLasligFormat(format laslig.Format) domain.OutputFormat {
	switch format {
	case laslig.FormatHuman:
		return domain.OutputFormatHuman
	case laslig.FormatPlain:
		return domain.OutputFormatPlain
	case laslig.FormatJSON:
		return domain.OutputFormatJSON
	default:
		return domain.OutputFormatAuto
	}
}

func toLasligStyle(style domain.OutputStyle) laslig.StylePolicy {
	switch style {
	case domain.OutputStyleAlways:
		return laslig.StyleAlways
	case domain.OutputStyleNever:
		return laslig.StyleNever
	default:
		return laslig.StyleAuto
	}
}

func listJSONKey(value string) string {
	if strings.TrimSpace(value) == "" {
		return "items"
	}
	return strings.TrimSpace(value)
}
