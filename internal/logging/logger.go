package logging

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/evanmschultz/valv/internal/domain"
)

type Options struct {
	Writer io.Writer
	Level  string
	Prefix string
}

func New(options Options) (*log.Logger, error) {
	level, err := parseLevel(options.Level)
	if err != nil {
		return nil, err
	}
	if options.Writer == nil {
		return nil, fmt.Errorf("new logger: writer is required")
	}
	logger := log.NewWithOptions(options.Writer, log.Options{
		Level:  level,
		Prefix: options.Prefix,
	})
	return logger, nil
}

func parseLevel(value string) (log.Level, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "", "info":
		return log.InfoLevel, nil
	case "debug":
		return log.DebugLevel, nil
	case "warn", "warning":
		return log.WarnLevel, nil
	case "error":
		return log.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("parse log level %q: %w", value, domain.ErrInvalidLogLevel)
	}
}
