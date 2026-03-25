package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func OpenFile(dir string, name string) (*os.File, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("open log file: directory is required")
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("open log file: name is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("open log file: ensure directory %q: %w", dir, err)
	}
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	return file, nil
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
