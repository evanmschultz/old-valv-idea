package domain

import (
	"fmt"
	"strings"
)

type Provider string

const (
	ProviderCodex Provider = "codex"
)

func ParseProvider(value string) (Provider, error) {
	switch normalized := Provider(strings.ToLower(strings.TrimSpace(value))); normalized {
	case ProviderCodex:
		return normalized, nil
	default:
		return "", fmt.Errorf("parse provider %q: unsupported value", value)
	}
}

type Mode string

const (
	ModeFresh     Mode = "fresh"
	ModeResume    Mode = "resume"
	ModeEphemeral Mode = "ephemeral"
)

type OutputFormat string

const (
	OutputFormatAuto  OutputFormat = "auto"
	OutputFormatHuman OutputFormat = "human"
	OutputFormatPlain OutputFormat = "plain"
	OutputFormatJSON  OutputFormat = "json"
)

type OutputStyle string

const (
	OutputStyleAuto   OutputStyle = "auto"
	OutputStyleAlways OutputStyle = "always"
	OutputStyleNever  OutputStyle = "never"
)

func ParseOutputFormat(value string) (OutputFormat, error) {
	switch normalized := OutputFormat(strings.ToLower(strings.TrimSpace(value))); normalized {
	case OutputFormatAuto, OutputFormatHuman, OutputFormatPlain, OutputFormatJSON:
		return normalized, nil
	default:
		return "", fmt.Errorf("parse output format %q: unsupported value", value)
	}
}

func ParseOutputStyle(value string) (OutputStyle, error) {
	switch normalized := OutputStyle(strings.ToLower(strings.TrimSpace(value))); normalized {
	case OutputStyleAuto, OutputStyleAlways, OutputStyleNever:
		return normalized, nil
	default:
		return "", fmt.Errorf("parse output style %q: unsupported value", value)
	}
}
