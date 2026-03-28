package manage

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Styles describes the screen chrome used by the manage TUI.
type Styles struct {
	Frame    lipgloss.Style
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Notice   lipgloss.Style
	Hint     lipgloss.Style
}

// DefaultStyles returns a compact but opinionated layout for the manage home screen.
func DefaultStyles() Styles {
	return Styles{
		Frame: lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4D728A")),
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#EAF6FF")),
		Subtitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#93A8B7")),
		Notice: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9DDCFF")).
			Bold(true),
		Hint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6E8291")),
	}
}

func (s Styles) header() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		s.Title.Render("Valv Manage"),
		s.Subtitle.Render("Operator surface for project, account, runtime, and cleanup actions."),
	)
}

func (s Styles) footer(status string) string {
	hint := "↑/↓ navigate • enter select • / filter • q quit"
	if strings.TrimSpace(status) != "" {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			s.Notice.Render(status),
			s.Hint.Render(hint),
		)
	}
	return s.Hint.Render(hint)
}
