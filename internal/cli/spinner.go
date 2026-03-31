package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/evanmschultz/laslig"

	progressutil "github.com/evanmschultz/valv/internal/progress"
)

func runWithCLIQuietSpinner(out io.Writer, startText, successText, failureText string, fn func() error) error {
	spin, err := progressutil.Start(out, laslig.Policy{
		Format: laslig.FormatAuto,
		Style:  laslig.StyleAuto,
	}, startText)
	if err != nil {
		return fmt.Errorf("start quiet spinner: %w", err)
	}

	if err := fn(); err != nil {
		if stopErr := spin.Stop(failureText, laslig.NoticeErrorLevel); stopErr != nil {
			return errors.Join(err, fmt.Errorf("stop quiet spinner: %w", stopErr))
		}
		return err
	}

	if err := spin.Stop(successText, laslig.NoticeInfoLevel); err != nil {
		return fmt.Errorf("stop quiet spinner: %w", err)
	}
	return nil
}
