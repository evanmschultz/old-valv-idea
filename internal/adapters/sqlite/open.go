package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/evanmschultz/valv/internal/domain"
	_ "modernc.org/sqlite"
)

type OpenOptions struct {
	Path string
	URI  string
}

func Open(options OpenOptions) (*sql.DB, error) {
	var dsn string
	switch {
	case options.URI != "":
		dsn = options.URI
	case options.Path != "":
		cleanPath := filepath.Clean(options.Path)
		values := url.Values{}
		values.Set("mode", "rwc")
		dsn = (&url.URL{
			Scheme:   "file",
			Path:     cleanPath,
			RawQuery: values.Encode(),
		}).String()
	default:
		return nil, fmt.Errorf("open sqlite database: %w", domain.ErrInvalidDBPath)
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
