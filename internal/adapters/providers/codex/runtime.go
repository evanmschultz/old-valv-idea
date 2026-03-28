package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/log"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/pathutil"
)

const (
	ContainerHomeDir  = "/home/valv"
	ContainerCodexDir = "/home/valv/.codex"
)

type PrepareRequest struct {
	ProfileHome string
	ProjectRoot string
	TempRoot    string
	Logger      *log.Logger
}

type PreparedRuntime struct {
	ContainerHome  string
	Env            map[string]string
	EnvPassthrough []string
	Mounts         []dockeradapter.MountSpec
	Warnings       []string
	cleanup        func() error
}

func (p PreparedRuntime) Close() error {
	if p.cleanup == nil {
		return nil
	}
	return p.cleanup()
}

func PrepareRuntime(ctx context.Context, request PrepareRequest) (PreparedRuntime, error) {
	profileHome, err := pathutil.Normalize(request.ProfileHome)
	if err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: normalize profile home: %w", err)
	}
	projectRoot, err := pathutil.Normalize(request.ProjectRoot)
	if err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: normalize project root: %w", err)
	}
	tempRoot, err := pathutil.Normalize(request.TempRoot)
	if err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: normalize temp root: %w", err)
	}
	if err := os.MkdirAll(tempRoot, 0o755); err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: ensure temp root %q: %w", tempRoot, err)
	}

	runtimeDir, err := os.MkdirTemp(tempRoot, "codex-runtime-")
	if err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: create runtime dir: %w", err)
	}

	mounts := []dockeradapter.MountSpec{
		dockeradapter.NewMountSpec(profileHome, ContainerCodexDir, false),
	}
	env := map[string]string{
		"CODEX_HOME": ContainerCodexDir,
		"HOME":       ContainerHomeDir,
		"LOGNAME":    "valv",
		"USER":       "valv",
	}
	if _, ok := os.LookupEnv("TERM"); !ok {
		env["TERM"] = "xterm-256color"
	}

	bridgeManager, err := newBridgeManager(ctx, request.Logger)
	if err != nil {
		_ = os.RemoveAll(runtimeDir)
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: initialize bridge manager: %w", err)
	}
	cleanup := func() error {
		var errs []error
		if err := bridgeManager.Close(); err != nil {
			errs = append(errs, err)
		}
		if err := os.RemoveAll(runtimeDir); err != nil {
			errs = append(errs, err)
		}
		return errorsJoin(errs...)
	}

	warnings := []string{}
	profileOverlayPath := filepath.Join(runtimeDir, "profile-config.toml")
	profileResult, err := translateConfigFile(ctx, translateRequest{
		ConfigPath:        filepath.Join(profileHome, "config.toml"),
		ProjectRoot:       projectRoot,
		ScopeRoot:         profileHome,
		OverlayOutputPath: profileOverlayPath,
		BridgeManager:     bridgeManager,
		Logger:            request.Logger,
	})
	if err != nil {
		_ = cleanup()
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: translate profile config: %w", err)
	}
	warnings = append(warnings, profileResult.Warnings...)
	envPassthrough := terminalEnvPassthrough()
	envPassthrough = appendUniqueStrings(envPassthrough, profileResult.EnvPassthrough...)
	if profileResult.HasOverlay {
		mounts = append(mounts, dockeradapter.NewMountSpec(profileOverlayPath, filepath.Join(ContainerCodexDir, "config.toml"), true))
	}

	projectConfigPath := filepath.Join(projectRoot, ".codex", "config.toml")
	projectOverlayPath := filepath.Join(runtimeDir, "project-config.toml")
	projectResult, err := translateConfigFile(ctx, translateRequest{
		ConfigPath:        projectConfigPath,
		ProjectRoot:       projectRoot,
		ScopeRoot:         projectRoot,
		OverlayOutputPath: projectOverlayPath,
		BridgeManager:     bridgeManager,
		Logger:            request.Logger,
	})
	if err != nil {
		_ = cleanup()
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: translate project config: %w", err)
	}
	warnings = append(warnings, projectResult.Warnings...)
	envPassthrough = appendUniqueStrings(envPassthrough, projectResult.EnvPassthrough...)
	if projectResult.HasOverlay {
		mounts = append(mounts, dockeradapter.NewMountSpec(projectOverlayPath, projectConfigPath, true))
	}

	return PreparedRuntime{
		ContainerHome:  ContainerHomeDir,
		Env:            env,
		EnvPassthrough: envPassthrough,
		Mounts:         mounts,
		Warnings:       warnings,
		cleanup:        cleanup,
	}, nil
}

type translateRequest struct {
	ConfigPath        string
	ProjectRoot       string
	ScopeRoot         string
	OverlayOutputPath string
	BridgeManager     *bridgeManager
	Logger            *log.Logger
}

type translateResult struct {
	HasOverlay     bool
	Warnings       []string
	EnvPassthrough []string
}

func translateConfigFile(ctx context.Context, request translateRequest) (translateResult, error) {
	content, err := os.ReadFile(request.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return translateResult{}, nil
		}
		return translateResult{}, fmt.Errorf("read config %q: %w", request.ConfigPath, err)
	}

	cfg := map[string]any{}
	if _, err := toml.Decode(string(content), &cfg); err != nil {
		return translateResult{}, fmt.Errorf("decode config %q: %w", request.ConfigPath, err)
	}

	servers, changed, warnings, envPassthrough, err := translateMCPServers(ctx, cfg, request.ProjectRoot, request.ScopeRoot, request.BridgeManager)
	if err != nil {
		return translateResult{}, fmt.Errorf("translate MCP servers for %q: %w", request.ConfigPath, err)
	}
	if !changed {
		return translateResult{EnvPassthrough: envPassthrough}, nil
	}
	cfg["mcp_servers"] = servers
	if err := os.MkdirAll(filepath.Dir(request.OverlayOutputPath), 0o755); err != nil {
		return translateResult{}, fmt.Errorf("ensure overlay dir for %q: %w", request.OverlayOutputPath, err)
	}
	file, err := os.Create(request.OverlayOutputPath)
	if err != nil {
		return translateResult{}, fmt.Errorf("create overlay %q: %w", request.OverlayOutputPath, err)
	}
	defer file.Close()
	if err := toml.NewEncoder(file).Encode(cfg); err != nil {
		return translateResult{}, fmt.Errorf("encode overlay %q: %w", request.OverlayOutputPath, err)
	}
	return translateResult{HasOverlay: true, Warnings: warnings, EnvPassthrough: envPassthrough}, nil
}

func translateMCPServers(ctx context.Context, cfg map[string]any, projectRoot, scopeRoot string, manager *bridgeManager) (map[string]any, bool, []string, []string, error) {
	raw, ok := cfg["mcp_servers"]
	if !ok {
		return nil, false, nil, nil, nil
	}
	serverMap, ok := raw.(map[string]any)
	if !ok {
		return nil, false, nil, nil, fmt.Errorf("mcp_servers is %T, want table", raw)
	}
	changed := false
	warnings := []string{}
	envPassthrough := []string{}
	out := make(map[string]any, len(serverMap))
	for name, value := range serverMap {
		entry, ok := value.(map[string]any)
		if !ok {
			out[name] = value
			continue
		}
		copied := cloneMap(entry)
		envPassthrough = appendUniqueStrings(envPassthrough, passthroughEnvFromEntry(copied)...)
		switch {
		case stringValue(copied["command"]) != "":
			targetURL, warning, err := manager.BridgeCommand(ctx, name, commandSpec{
				Command:   stringValue(copied["command"]),
				Args:      stringSlice(copied["args"]),
				Env:       stringMap(copied["env"]),
				ScopeRoot: scopeRoot,
				Cwd:       projectRoot,
			})
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
				changed = true
				continue
			}
			delete(copied, "command")
			delete(copied, "args")
			delete(copied, "env")
			copied["url"] = targetURL
			if warning != "" {
				warnings = append(warnings, fmt.Sprintf("%s: %s", name, warning))
			}
			out[name] = copied
			changed = true
		case stringValue(copied["url"]) != "":
			rawURL := stringValue(copied["url"])
			rewritten, didRewrite, err := rewriteLoopbackURL(rawURL)
			if err != nil {
				return nil, false, nil, nil, fmt.Errorf("rewrite URL for %q: %w", name, err)
			}
			if didRewrite {
				copied["url"] = rewritten
				changed = true
			}
			out[name] = copied
		default:
			out[name] = copied
		}
	}
	return out, changed, warnings, envPassthrough, nil
}

func cloneMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, entry := range value {
		switch typed := entry.(type) {
		case map[string]any:
			out[key] = cloneMap(typed)
		case []any:
			out[key] = append([]any(nil), typed...)
		default:
			out[key] = typed
		}
	}
	return out
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			if text, ok := entry.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			if strings.TrimSpace(entry) != "" {
				out = append(out, strings.TrimSpace(entry))
			}
		}
		return out
	default:
		return nil
	}
}

func stringMap(value any) map[string]string {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, entry := range typed {
			if text, ok := entry.(string); ok {
				out[key] = text
			}
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, entry := range typed {
			out[key] = entry
		}
		return out
	default:
		return nil
	}
}

func passthroughEnvFromEntry(entry map[string]any) []string {
	headers, ok := entry["env_http_headers"]
	if !ok {
		return nil
	}
	table, ok := headers.(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(table))
	for _, raw := range table {
		name, ok := raw.(string)
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := os.LookupEnv(name); exists {
			out = append(out, name)
		}
	}
	return out
}

func appendUniqueStrings(dst []string, values ...string) []string {
	if len(values) == 0 {
		return dst
	}
	seen := make(map[string]struct{}, len(dst))
	for _, value := range dst {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		dst = append(dst, value)
		seen[value] = struct{}{}
	}
	return dst
}

func errorsJoin(errs ...error) error {
	return errors.Join(errs...)
}

func terminalEnvPassthrough() []string {
	names := []string{
		"TERM",
		"COLORTERM",
		"TERM_PROGRAM",
		"TERM_PROGRAM_VERSION",
		"LANG",
		"LC_CTYPE",
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := os.LookupEnv(name); ok {
			out = append(out, name)
		}
	}
	return out
}
