package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	SharedHome  string
	ProjectRoot string
	TempRoot    string
	// OtherProviderProfileHome is the host-side profile home of the OTHER
	// provider (e.g. the claude profile home when launching a codex container).
	// When non-empty, PrepareRuntime mounts it at /home/valv/.claude read-write
	// and sets CLAUDE_CONFIG_DIR=/home/valv/.claude in the container environment
	// so the other CLI can authenticate using its native auth-file layout.
	// When empty, the cross-mount and env var are skipped.
	OtherProviderProfileHome string
	Logger                   *log.Logger
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
	sharedHome := profileHome
	if strings.TrimSpace(request.SharedHome) != "" {
		sharedHome, err = pathutil.Normalize(request.SharedHome)
		if err != nil {
			return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: normalize shared home: %w", err)
		}
	}
	if err := os.MkdirAll(sharedHome, 0o755); err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: ensure shared home %q: %w", sharedHome, err)
	}
	debugLog(request.Logger,
		"preparing codex runtime",
		"profile_home", profileHome,
		"shared_home", sharedHome,
		"project_root", projectRoot,
		"temp_root", tempRoot,
		"host_term", strings.TrimSpace(os.Getenv("TERM")),
		"container_term", normalizedContainerTERM(),
	)

	runtimeDir, err := os.MkdirTemp(tempRoot, "codex-runtime-")
	if err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: create runtime dir: %w", err)
	}

	runtimeCodexHome := sharedHome
	if profileHome != sharedHome {
		runtimeCodexHome = filepath.Join(runtimeDir, "codex-home")
		if err := os.MkdirAll(runtimeCodexHome, 0o755); err != nil {
			return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: ensure runtime codex home %q: %w", runtimeCodexHome, err)
		}
		if err := copyDirContents(sharedHome, runtimeCodexHome, nil); err != nil {
			return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: stage shared home %q: %w", sharedHome, err)
		}
		debugLog(request.Logger,
			"staged shared codex home for account runtime",
			"runtime_codex_home", runtimeCodexHome,
			"shared_home", sharedHome,
		)
	}
	mounts := []dockeradapter.MountSpec{
		dockeradapter.NewMountSpec(runtimeCodexHome, ContainerCodexDir, false),
	}
	env := map[string]string{
		"CODEX_HOME": ContainerCodexDir,
		"HOME":       ContainerHomeDir,
		"LOGNAME":    "valv",
		"TERM":       normalizedContainerTERM(),
		"USER":       "valv",
	}

	if strings.TrimSpace(request.OtherProviderProfileHome) != "" {
		otherHome, err := pathutil.Normalize(request.OtherProviderProfileHome)
		if err != nil {
			return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: normalize other provider home: %w", err)
		}
		mounts = append(mounts, dockeradapter.NewMountSpec(otherHome, "/home/valv/.claude", false))
		env["CLAUDE_CONFIG_DIR"] = "/home/valv/.claude"
	}

	bridgeManager, err := newBridgeManager(ctx, request.Logger)
	if err != nil {
		_ = os.RemoveAll(runtimeDir)
		return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: initialize bridge manager: %w", err)
	}
	cleanup := func() error {
		var errs []error
		if runtimeCodexHome != sharedHome {
			debugLog(request.Logger,
				"syncing shared codex state back to host home",
				"runtime_codex_home", runtimeCodexHome,
				"shared_home", sharedHome,
			)
			if err := syncDirContents(runtimeCodexHome, sharedHome, map[string]struct{}{
				"auth.json":   {},
				"config.toml": {},
			}); err != nil {
				errs = append(errs, err)
			}
		}
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
	if profileHome != sharedHome {
		authPath := filepath.Join(profileHome, "auth.json")
		if info, err := os.Stat(authPath); err == nil && !info.IsDir() {
			if err := copyFile(authPath, filepath.Join(runtimeCodexHome, "auth.json")); err != nil {
				_ = cleanup()
				return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: stage account auth: %w", err)
			}
		} else if err != nil && !os.IsNotExist(err) {
			_ = cleanup()
			return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: stat auth file %q: %w", authPath, err)
		}
		if profileResult.HasOverlay {
			if err := copyFile(profileOverlayPath, filepath.Join(runtimeCodexHome, "config.toml")); err != nil {
				_ = cleanup()
				return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: stage profile config overlay: %w", err)
			}
		}
	} else if profileResult.HasOverlay {
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
	debugLog(request.Logger,
		"prepared codex runtime",
		"runtime_codex_home", runtimeCodexHome,
		"env", env,
		"env_passthrough", envPassthrough,
		"warnings", warnings,
		"mount_count", len(mounts),
	)

	return PreparedRuntime{
		ContainerHome:  ContainerHomeDir,
		Env:            env,
		EnvPassthrough: envPassthrough,
		Mounts:         mounts,
		Warnings:       warnings,
		cleanup:        cleanup,
	}, nil
}

func debugLog(logger *log.Logger, msg string, keyvals ...any) {
	if logger == nil {
		return
	}
	logger.Debug(msg, keyvals...)
}

func copyDirContents(src, dst string, exclude map[string]struct{}) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", src)
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == src {
			return nil
		}
		relative, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		parts := strings.Split(relative, string(filepath.Separator))
		if len(parts) > 0 {
			if _, skip := exclude[parts[0]]; skip {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		target := filepath.Join(dst, relative)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target)
	})
}

func syncDirContents(src, dst string, exclude map[string]struct{}) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return copyDirContents(src, dst, exclude)
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%q is a directory", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = out.Close()
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	closed = true
	return nil
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

func normalizedContainerTERM() string {
	value := strings.TrimSpace(os.Getenv("TERM"))
	switch value {
	case "":
		return "xterm-256color"
	default:
		return value
	}
}
