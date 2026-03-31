package progress

import (
	"fmt"
	"io"

	"github.com/evanmschultz/laslig"
)

// Spinner wraps one laslig spinner so callers can share start/stop orchestration
// without reimplementing the same reader handoff logic in each command path.
type Spinner struct {
	spinner *laslig.Spinner
}

func Start(out io.Writer, policy laslig.Policy, text string) (*Spinner, error) {
	return StartWithPrinter(laslig.New(out, policy), text)
}

func StartWithPrinter(printer *laslig.Printer, text string) (*Spinner, error) {
	spin := printer.NewSpinner()
	if err := spin.Start(text); err != nil {
		return nil, fmt.Errorf("start spinner: %w", err)
	}
	return &Spinner{spinner: spin}, nil
}

func (s *Spinner) Update(text string) error {
	if s == nil || s.spinner == nil {
		return nil
	}
	return s.spinner.Update(text)
}

func (s *Spinner) Stop(message string, level laslig.NoticeLevel) error {
	if s == nil || s.spinner == nil {
		return nil
	}
	return s.spinner.Stop(message, level)
}
