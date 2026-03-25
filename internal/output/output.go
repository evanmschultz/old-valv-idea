package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
	"github.com/evanmschultz/valv/internal/domain"
)

type Policy struct {
	Format domain.OutputFormat
	Style  domain.OutputStyle
}

type Mode struct {
	Format domain.OutputFormat
	Styled bool
}

type Field struct {
	Label      string `json:"label"`
	Value      string `json:"value"`
	Identifier bool   `json:"identifier,omitempty"`
	Muted      bool   `json:"muted,omitempty"`
	Badge      bool   `json:"badge,omitempty"`
}

type ListItem struct {
	Title  string  `json:"title"`
	Badge  string  `json:"badge,omitempty"`
	Fields []Field `json:"fields,omitempty"`
}

func ResolveMode(out io.Writer, policy Policy) Mode {
	isTTY := false
	if file, ok := out.(term.File); ok {
		isTTY = term.IsTerminal(file.Fd())
	}

	format := policy.Format
	if format == "" {
		format = domain.OutputFormatAuto
	}
	if format == domain.OutputFormatAuto {
		if isTTY {
			format = domain.OutputFormatHuman
		} else {
			format = domain.OutputFormatPlain
		}
	}

	styled := false
	if format == domain.OutputFormatHuman {
		switch policy.Style {
		case domain.OutputStyleAlways:
			styled = true
		case domain.OutputStyleNever:
			styled = false
		default:
			styled = isTTY
		}
	}

	return Mode{Format: format, Styled: styled}
}

func WriteRecord(out io.Writer, mode Mode, heading string, fields []Field) error {
	if mode.Format == domain.OutputFormatJSON {
		payload := map[string]any{"heading": heading}
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
		for _, field := range fields {
			if _, err := fmt.Fprintf(out, "%s=%s\n", strings.ReplaceAll(strings.ToLower(field.Label), " ", "_"), field.Value); err != nil {
				return fmt.Errorf("write plain field: %w", err)
			}
		}
		return nil
	}

	theme := newTheme(mode)
	if _, err := fmt.Fprintln(out, theme.heading.Render(heading)); err != nil {
		return fmt.Errorf("write human heading: %w", err)
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(out, "  %s %s\n", theme.label.Render(field.Label+":"), renderFieldValue(theme, field)); err != nil {
			return fmt.Errorf("write human field: %w", err)
		}
	}
	return nil
}

func WriteList(out io.Writer, mode Mode, heading string, items []ListItem) error {
	if mode.Format == domain.OutputFormatJSON {
		payload := map[string]any{"heading": heading, "items": items}
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

	theme := newTheme(mode)
	if _, err := fmt.Fprintln(out, theme.heading.Render(heading)); err != nil {
		return fmt.Errorf("write human list heading: %w", err)
	}
	for _, item := range items {
		title := item.Title
		if strings.TrimSpace(item.Badge) != "" {
			title += " " + renderBadge(theme, item.Badge)
		}
		if _, err := fmt.Fprintf(out, "- %s\n", theme.value.Render(title)); err != nil {
			return fmt.Errorf("write human list title: %w", err)
		}
		for _, field := range item.Fields {
			if _, err := fmt.Fprintf(out, "  %s %s\n", theme.label.Render(field.Label+":"), renderFieldValue(theme, field)); err != nil {
				return fmt.Errorf("write human list field: %w", err)
			}
		}
	}
	return nil
}

type theme struct {
	styled  bool
	heading lipgloss.Style
	label   lipgloss.Style
	value   lipgloss.Style
	id      lipgloss.Style
	muted   lipgloss.Style
	badge   lipgloss.Style
}

func newTheme(mode Mode) theme {
	if !mode.Styled {
		base := lipgloss.NewStyle()
		return theme{styled: false, heading: base, label: base, value: base, id: base, muted: base, badge: base}
	}
	return theme{
		styled:  true,
		heading: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9DDCFF")),
		label:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A9C6D8")),
		value:   lipgloss.NewStyle().Foreground(lipgloss.Color("#EAF6FF")),
		id:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A2F2D9")),
		muted:   lipgloss.NewStyle().Foreground(lipgloss.Color("#7A8D99")),
		badge:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0A1E2A")).Background(lipgloss.Color("#9DDCFF")).Padding(0, 1),
	}
}

func renderFieldValue(theme theme, field Field) string {
	switch {
	case field.Badge:
		return renderBadge(theme, field.Value)
	case field.Identifier:
		return theme.id.Render(field.Value)
	case field.Muted:
		return theme.muted.Render(field.Value)
	default:
		return theme.value.Render(field.Value)
	}
}

func renderBadge(theme theme, value string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(value))
	if !theme.styled {
		return "[" + trimmed + "]"
	}
	return theme.badge.Render(trimmed)
}
