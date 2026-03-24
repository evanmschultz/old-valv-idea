package domain

import "errors"

var (
	ErrConfigNotFound  = errors.New("config file not found")
	ErrUnsupportedOS   = errors.New("unsupported operating system")
	ErrUnboundProject  = errors.New("project is not bound")
	ErrInvalidOutput   = errors.New("invalid output configuration")
	ErrInvalidLogLevel = errors.New("invalid log level")
	ErrInvalidDBPath   = errors.New("invalid database path")
)
