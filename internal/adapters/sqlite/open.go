package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/evanmschultz/valv/internal/domain"
	_ "modernc.org/sqlite"
)

type OpenOptions struct {
	Path string
	URI  string
}

func Open(options OpenOptions) (*sql.DB, error) {
	dsn, err := buildDSN(options)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %q: %w", dsn, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database %q: %w", dsn, err)
	}
	return db, nil
}

func buildDSN(options OpenOptions) (string, error) {
	switch {
	case options.URI != "":
		return withPragma(options.URI, "foreign_keys(1)")
	case options.Path != "":
		cleanPath := filepath.Clean(options.Path)
		values := url.Values{}
		values.Set("mode", "rwc")
		return withPragma((&url.URL{
			Scheme:   "file",
			Path:     cleanPath,
			RawQuery: values.Encode(),
		}).String(), "foreign_keys(1)")
	default:
		return "", fmt.Errorf("open sqlite database: %w", domain.ErrInvalidDBPath)
	}
}

func withPragma(dsn, pragma string) (string, error) {
	if strings.TrimSpace(dsn) == "" {
		return "", fmt.Errorf("open sqlite database: %w", domain.ErrInvalidDBPath)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("open sqlite database DSN %q: %w", dsn, err)
	}

	values := parsed.Query()
	for _, existing := range values["_pragma"] {
		if existing == pragma {
			return parsed.String(), nil
		}
	}
	values.Add("_pragma", pragma)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}
