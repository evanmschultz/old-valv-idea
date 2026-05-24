package domain

import "errors"

var (
	ErrConfigNotFound  = errors.New("config file not found")
	ErrToolsNotFound   = errors.New("tools file not found")
	ErrUnsupportedOS   = errors.New("unsupported operating system")
	ErrUnboundProject  = errors.New("project is not bound")
	ErrInvalidOutput   = errors.New("invalid output configuration")
	ErrInvalidLogLevel = errors.New("invalid log level")
	ErrInvalidDBPath   = errors.New("invalid database path")
	ErrNotFound        = errors.New("not found")
	// ErrUnsupportedSchema is returned when the SQLite store's PRAGMA user_version
	// falls outside the supported window (currently [1, 2]). Wrap with fmt.Errorf
	// and a message describing the observed and supported versions so operators
	// can identify whether the database is too old (rebuild required) or too new
	// (binary too old).
	ErrUnsupportedSchema = errors.New("unsupported schema")
)
