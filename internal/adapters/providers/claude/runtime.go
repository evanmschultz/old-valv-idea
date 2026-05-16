package claude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/pathutil"
)

const (
	// ContainerHomeDir is the home directory inside the Claude container.
	ContainerHomeDir = "/home/valv"
	// ContainerClaudeDir is the Claude config directory inside the container.
	ContainerClaudeDir = "/home/valv/.claude"
)

// PrepareRequest holds the inputs for PrepareRuntime.
type PrepareRequest struct {
	ProfileHome string
	SharedHome  string
	ProjectRoot string
	TempRoot    string
	Logger      *log.Logger
}

// PreparedRuntime holds the runtime environment prepared for a Claude
// container launch. Call Close when the container exits to sync state back
// and remove the temporary runtime directory.
type PreparedRuntime struct {
	ContainerHome  string
	Env            map[string]string
	EnvPassthrough []string
	Mounts         []dockeradapter.MountSpec
	Warnings       []string
	cleanup        func() error
}

// Close runs cleanup: syncs state back to the host profile home and removes
// the temporary runtime directory.
func (p PreparedRuntime) Close() error {
	if p.cleanup == nil {
		return nil
	}
	return p.cleanup()
}

// PrepareRuntime prepares the filesystem and environment for a Claude
// container launch. It creates a temporary runtime directory, optionally
// stages a shared-home copy, sets up mounts, and builds the env map.
// No bridge manager or config translation is performed in v1.
func PrepareRuntime(ctx context.Context, request PrepareRequest) (PreparedRuntime, error) {
	profileHome, err := pathutil.Normalize(request.ProfileHome)
	if err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: normalize profile home: %w", err)
	}
	projectRoot, err := pathutil.Normalize(request.ProjectRoot)
	if err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: normalize project root: %w", err)
	}
	tempRoot, err := pathutil.Normalize(request.TempRoot)
	if err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: normalize temp root: %w", err)
	}
	if err := os.MkdirAll(tempRoot, 0o755); err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: ensure temp root %q: %w", tempRoot, err)
	}
	sharedHome := profileHome
	if strings.TrimSpace(request.SharedHome) != "" {
		sharedHome, err = pathutil.Normalize(request.SharedHome)
		if err != nil {
			return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: normalize shared home: %w", err)
		}
	}
	if err := os.MkdirAll(sharedHome, 0o755); err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: ensure shared home %q: %w", sharedHome, err)
	}
	debugLog(request.Logger,
		"preparing claude runtime",
		"profile_home", profileHome,
		"shared_home", sharedHome,
		"project_root", projectRoot,
		"temp_root", tempRoot,
		"host_term", strings.TrimSpace(os.Getenv("TERM")),
		"container_term", normalizedContainerTERM(),
	)

	runtimeDir, err := os.MkdirTemp(tempRoot, "claude-runtime-")
	if err != nil {
		return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: create runtime dir: %w", err)
	}

	runtimeClaudeHome := sharedHome
	if profileHome != sharedHome {
		runtimeClaudeHome = filepath.Join(runtimeDir, "claude-home")
		if err := os.MkdirAll(runtimeClaudeHome, 0o755); err != nil {
			return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: ensure runtime claude home %q: %w", runtimeClaudeHome, err)
		}
		if err := copyDirContents(sharedHome, runtimeClaudeHome, nil); err != nil {
			return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: stage shared home %q: %w", sharedHome, err)
		}
		debugLog(request.Logger,
			"staged shared claude home for account runtime",
			"runtime_claude_home", runtimeClaudeHome,
			"shared_home", sharedHome,
		)
	}
	mounts := []dockeradapter.MountSpec{
		dockeradapter.NewMountSpec(runtimeClaudeHome, ContainerClaudeDir, false),
	}
	env := map[string]string{
		"CLAUDE_CONFIG_DIR": ContainerClaudeDir,
		"HOME":              ContainerHomeDir,
		"LOGNAME":           "valv",
		"TERM":              normalizedContainerTERM(),
		"USER":              "valv",
	}

	envPassthrough := TerminalEnvPassthrough()

	cleanup := func() error {
		var errs []error
		if runtimeClaudeHome != sharedHome {
			debugLog(request.Logger,
				"syncing shared claude state back to host home",
				"runtime_claude_home", runtimeClaudeHome,
				"shared_home", sharedHome,
			)
			if err := syncDirContents(runtimeClaudeHome, sharedHome, map[string]struct{}{
				".credentials.json": {},
			}); err != nil {
				errs = append(errs, err)
			}
		}
		if err := os.RemoveAll(runtimeDir); err != nil {
			errs = append(errs, err)
		}
		return errorsJoin(errs...)
	}

	debugLog(request.Logger,
		"prepared claude runtime",
		"runtime_claude_home", runtimeClaudeHome,
		"env", env,
		"env_passthrough", envPassthrough,
		"mount_count", len(mounts),
	)

	_ = projectRoot // used for future project-config support; not translated in v1

	return PreparedRuntime{
		ContainerHome:  ContainerHomeDir,
		Env:            env,
		EnvPassthrough: envPassthrough,
		Mounts:         mounts,
		Warnings:       []string{},
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

// TerminalEnvPassthrough returns the list of terminal-related environment
// variable names that are set on the host process and should be forwarded to
// the Claude container. This is called by both PrepareRuntime (launch path)
// and the auth container runner to ensure consistent locale and color handling.
func TerminalEnvPassthrough() []string {
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
