package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/evanmschultz/valv/internal/domain"
)

type Config struct {
	Output  OutputConfig  `toml:"output"`
	Logging LoggingConfig `toml:"logging"`
}

type OutputConfig struct {
	Format string `toml:"format"`
	Style  string `toml:"style"`
}

type LoggingConfig struct {
	Level string `toml:"level"`
}

type EffectiveConfig struct {
	Output  EffectiveOutputConfig
	Logging EffectiveLoggingConfig
}

type EffectiveOutputConfig struct {
	Format domain.OutputFormat
	Style  domain.OutputStyle
}

type EffectiveLoggingConfig struct {
	Level string
}

func Default() Config {
	return Config{
		Output: OutputConfig{
			Format: string(domain.OutputFormatAuto),
			Style:  string(domain.OutputStyleAuto),
		},
		Logging: LoggingConfig{Level: "info"},
	}
}

func Load(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, fmt.Errorf("load config: %w", domain.ErrConfigNotFound)
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("load config %q: %w", path, domain.ErrConfigNotFound)
		}
		return Config{}, fmt.Errorf("stat config %q: %w", path, err)
	}

	cfg := Default()
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("decode config %q: undecoded keys present", path)
	}
	return cfg, nil
}

func (c Config) Effective() (EffectiveConfig, error) {
	format, err := domain.ParseOutputFormat(c.Output.Format)
	if err != nil {
		return EffectiveConfig{}, fmt.Errorf("resolve output format: %w", err)
	}
	style, err := domain.ParseOutputStyle(c.Output.Style)
	if err != nil {
		return EffectiveConfig{}, fmt.Errorf("resolve output style: %w", err)
	}
	level := strings.ToLower(strings.TrimSpace(c.Logging.Level))
	if level == "" {
		level = "info"
	}
	return EffectiveConfig{
		Output:  EffectiveOutputConfig{Format: format, Style: style},
		Logging: EffectiveLoggingConfig{Level: level},
	}, nil
}

func DefaultConfigPath(paths Paths) string {
	return filepath.Join(paths.ConfigDir, "config.toml")
}
