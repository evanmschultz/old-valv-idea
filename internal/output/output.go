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
	Label string
	Value string
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

	headingStyle := lipgloss.NewStyle().Bold(true)
	labelStyle := lipgloss.NewStyle().Bold(true)
	if !mode.Styled {
		headingStyle = lipgloss.NewStyle()
		labelStyle = lipgloss.NewStyle()
	}
	if _, err := fmt.Fprintln(out, headingStyle.Render(heading)); err != nil {
		return fmt.Errorf("write human heading: %w", err)
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(out, "  %s %s\n", labelStyle.Render(field.Label+":"), field.Value); err != nil {
			return fmt.Errorf("write human field: %w", err)
		}
	}
	return nil
}
