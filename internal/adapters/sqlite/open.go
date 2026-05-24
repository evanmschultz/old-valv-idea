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

// OpenOptions selects either a filesystem Path or a raw URI DSN. URI takes
// precedence when both are supplied.
type OpenOptions struct {
	Path string
	URI  string
}

// requiredPragmas is the ordered list of pragmas Valv requires on every
// connection. busy_timeout comes first so it is active during any subsequent
// pragma execution; foreign_keys follows. Ordering matches the documented
// modernc.org/sqlite behavior that DSN pragmas execute in declaration order.
var requiredPragmas = []pragmaSpec{
	{name: "busy_timeout", value: "busy_timeout(5000)"},
	{name: "foreign_keys", value: "foreign_keys(1)"},
}

type pragmaSpec struct {
	// name is the lowercased pragma name (e.g. "busy_timeout").
	name string
	// value is the full `name(arg)` form that gets appended to the DSN.
	value string
}

// Open returns a *sql.DB configured with the Valv DSN policy (busy_timeout
// and foreign_keys pragmas).
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
		return applyRequiredPragmas(options.URI)
	case options.Path != "":
		cleanPath := filepath.Clean(options.Path)
		values := url.Values{}
		values.Set("mode", "rwc")
		return applyRequiredPragmas((&url.URL{
			Scheme:   "file",
			Path:     cleanPath,
			RawQuery: values.Encode(),
		}).String())
	default:
		return "", fmt.Errorf("open sqlite database: %w", domain.ErrInvalidDBPath)
	}
}

// applyRequiredPragmas appends every entry in requiredPragmas to the DSN's
// query string, skipping pragmas whose name (parsed exactly) is already
// present. Exact pragma-name dedup avoids the false-prefix collision that a
// strings.HasPrefix-style check would cause for caller-supplied values like
// "busy_timeout_pragma=foo": that legal value's parsed pragma name is
// "busy_timeout_pragma", which is NOT "busy_timeout", so the required
// busy_timeout(5000) is still appended.
func applyRequiredPragmas(dsn string) (string, error) {
	if strings.TrimSpace(dsn) == "" {
		return "", fmt.Errorf("open sqlite database: %w", domain.ErrInvalidDBPath)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("open sqlite database DSN %q: %w", dsn, err)
	}

	values := parsed.Query()
	existingNames := existingPragmaNames(values["_pragma"])
	for _, pragma := range requiredPragmas {
		if _, present := existingNames[pragma.name]; present {
			continue
		}
		values.Add("_pragma", pragma.value)
		existingNames[pragma.name] = struct{}{}
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

// existingPragmaNames extracts the parsed pragma names from raw _pragma
// values: trim whitespace, isolate the substring up to the first '(' or
// '=', lowercase the result. Empty entries are skipped.
func existingPragmaNames(pragmas []string) map[string]struct{} {
	names := make(map[string]struct{}, len(pragmas))
	for _, raw := range pragmas {
		name := parsePragmaName(raw)
		if name == "" {
			continue
		}
		names[name] = struct{}{}
	}
	return names
}

// parsePragmaName returns the canonical (lowercased) pragma name parsed from
// a raw _pragma value. The pragma name is the substring before the first
// '(' or '=' rune, with surrounding whitespace trimmed.
func parsePragmaName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	end := len(trimmed)
	for i, r := range trimmed {
		if r == '(' || r == '=' {
			end = i
			break
		}
	}
	name := strings.TrimSpace(trimmed[:end])
	return strings.ToLower(name)
}
