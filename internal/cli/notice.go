package cli

import (
	"fmt"
	"io"

	"github.com/evanmschultz/laslig"
)

func writeCLINotice(out io.Writer, level laslig.NoticeLevel, title, body string, detail ...string) error {
	if err := laslig.New(out, laslig.Policy{
		Format: laslig.FormatAuto,
		Style:  laslig.StyleAuto,
	}).Notice(laslig.Notice{
		Level:  level,
		Title:  title,
		Body:   body,
		Detail: detail,
	}); err != nil {
		return fmt.Errorf("write cli notice: %w", err)
	}
	return nil
}
