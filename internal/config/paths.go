package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/evanmschultz/valv/internal/domain"
)

type Paths struct {
	HomeDir        string
	AppSupportRoot string
	DatabaseDir    string
	DatabasePath   string
	ProviderRoot   string
	StateDir       string
	ConfigDir      string
	LogsDir        string
	CachesDir      string
	BuildCacheDir  string
	TempCacheDir   string
	RuntimeTmpDir  string
	PIDsDir        string
	LocksDir       string
	SocketsDir     string
}

func ResolvePaths(homeDir string) (Paths, error) {
	if homeDir == "" {
		if value := strings.TrimSpace(os.Getenv("VALV_TEST_HOME_DIR")); value != "" {
			homeDir = value
		}
	}
	if runtime.GOOS != "darwin" && homeDir == "" {
		return Paths{}, fmt.Errorf("resolve valv paths: %w", domain.ErrUnsupportedOS)
	}
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve user home: %w", err)
		}
	}
	appSupportRoot := filepath.Join(homeDir, "Library", "Application Support", "valv")
	logsRoot := filepath.Join(homeDir, "Library", "Logs", "valv")
	cacheRoot := filepath.Join(homeDir, "Library", "Caches", "valv")
	runtimeTmpRoot := filepath.Join("/tmp", "valv-"+strconv.Itoa(os.Getuid()))

	return Paths{
		HomeDir:        homeDir,
		AppSupportRoot: appSupportRoot,
		DatabaseDir:    filepath.Join(appSupportRoot, "db"),
		DatabasePath:   filepath.Join(appSupportRoot, "db", "valv.sqlite3"),
		ProviderRoot:   filepath.Join(appSupportRoot, "providers"),
		StateDir:       filepath.Join(appSupportRoot, "state"),
		ConfigDir:      filepath.Join(appSupportRoot, "config"),
		LogsDir:        logsRoot,
		CachesDir:      cacheRoot,
		BuildCacheDir:  filepath.Join(cacheRoot, "build"),
		TempCacheDir:   filepath.Join(cacheRoot, "tmp"),
		RuntimeTmpDir:  runtimeTmpRoot,
		PIDsDir:        filepath.Join(runtimeTmpRoot, "pids"),
		LocksDir:       filepath.Join(runtimeTmpRoot, "locks"),
		SocketsDir:     filepath.Join(runtimeTmpRoot, "sockets"),
	}, nil
}

func (p Paths) Ensure() error {
	for _, dir := range []string{
		p.DatabaseDir,
		p.ProviderRoot,
		p.StateDir,
		p.ConfigDir,
		p.LogsDir,
		p.CachesDir,
		p.BuildCacheDir,
		p.TempCacheDir,
		p.RuntimeTmpDir,
		p.PIDsDir,
		p.LocksDir,
		p.SocketsDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ensure directory %q: %w", dir, err)
		}
	}
	return nil
}
