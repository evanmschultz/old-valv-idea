package globalswitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/domain"
)

type Store interface {
	ProfileByName(context.Context, domain.Provider, string) (domain.Profile, error)
}

type Options struct {
	Store       Store
	HomeDir     string
	RealHomeDir string
	StateDir    string
	Logger      *log.Logger
	Now         func() time.Time
	IsRunning   func(context.Context, string) (bool, error)
}

type Service struct {
	store       Store
	homeDir     string
	realHomeDir string
	stateDir    string
	logger      *log.Logger
	now         func() time.Time
	isRunning   func(context.Context, string) (bool, error)
}

type Result struct {
	Provider    domain.Provider
	ProfileName string
	ProfileHome string
	TargetPath  string
	BackupPath  string
}

type stateRecord struct {
	Provider    string `json:"provider"`
	ProfileName string `json:"profile_name"`
	ProfileHome string `json:"profile_home"`
	TargetPath  string `json:"target_path"`
	UpdatedAt   string `json:"updated_at"`
}

func New(options Options) (Service, error) {
	if options.Store == nil {
		return Service{}, fmt.Errorf("new global switch service: store is required")
	}
	if strings.TrimSpace(options.HomeDir) == "" {
		return Service{}, fmt.Errorf("new global switch service: home dir is required")
	}
	if strings.TrimSpace(options.StateDir) == "" {
		return Service{}, fmt.Errorf("new global switch service: state dir is required")
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	isRunning := options.IsRunning
	if isRunning == nil {
		isRunning = processRunning
	}
	realHomeDir := strings.TrimSpace(options.RealHomeDir)
	if realHomeDir == "" {
		realHomeDir = strings.TrimSpace(options.HomeDir)
	}
	return Service{store: options.Store, homeDir: strings.TrimSpace(options.HomeDir), realHomeDir: realHomeDir, stateDir: strings.TrimSpace(options.StateDir), logger: options.Logger, now: now, isRunning: isRunning}, nil
}

func (s Service) Switch(ctx context.Context, provider domain.Provider, profileName string) (Result, error) {
	if provider != domain.ProviderCodex {
		return Result{}, fmt.Errorf("switch global profile %q: unsupported provider", provider)
	}
	if s.requiresHostProcessGuard() {
		running, err := s.isRunning(ctx, "codex")
		if err != nil {
			return Result{}, fmt.Errorf("switch global profile %q/%q: check running processes: %w", provider, profileName, err)
		}
		if running {
			return Result{}, fmt.Errorf("switch global profile %q/%q: host codex is currently running; exit Codex before switching", provider, profileName)
		}
	}
	profile, err := s.store.ProfileByName(ctx, provider, strings.TrimSpace(profileName))
	if err != nil {
		return Result{}, fmt.Errorf("switch global profile %q/%q: lookup profile: %w", provider, profileName, err)
	}
	result := Result{
		Provider:    provider,
		ProfileName: profile.Name,
		ProfileHome: profile.HomePath,
		TargetPath:  filepath.Join(s.homeDir, ".codex"),
	}
	if err := os.MkdirAll(filepath.Dir(result.TargetPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("switch global profile %q/%q: ensure target parent: %w", provider, profileName, err)
	}
	backupPath, err := s.prepareTarget(result.TargetPath)
	if err != nil {
		return Result{}, fmt.Errorf("switch global profile %q/%q: prepare target: %w", provider, profileName, err)
	}
	result.BackupPath = backupPath
	if err := os.Symlink(profile.HomePath, result.TargetPath); err != nil {
		return Result{}, fmt.Errorf("switch global profile %q/%q: create symlink %q -> %q: %w", provider, profileName, result.TargetPath, profile.HomePath, err)
	}
	if err := s.writeState(result); err != nil {
		return Result{}, fmt.Errorf("switch global profile %q/%q: write state: %w", provider, profileName, err)
	}
	s.debug("switched global profile", "provider", provider, "profile", profile.Name, "target", result.TargetPath, "backup", result.BackupPath)
	return result, nil
}

func (s Service) prepareTarget(target string) (string, error) {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", os.Remove(target)
	}
	backupRoot := filepath.Join(s.stateDir, "global-switch", "codex", "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return "", err
	}
	backupPath := filepath.Join(backupRoot, s.now().Format("20060102T150405Z"))
	if err := os.Rename(target, backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

func (s Service) writeState(result Result) error {
	statePath := filepath.Join(s.stateDir, "global-switch", string(result.Provider), "current.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(stateRecord{
		Provider:    string(result.Provider),
		ProfileName: result.ProfileName,
		ProfileHome: result.ProfileHome,
		TargetPath:  result.TargetPath,
		UpdatedAt:   s.now().Format(time.RFC3339Nano),
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, append(encoded, '\n'), 0o644)
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}

func (s Service) requiresHostProcessGuard() bool {
	return filepath.Clean(s.homeDir) == filepath.Clean(s.realHomeDir)
}

func processRunning(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "pgrep", "-x", "-U", strconv.Itoa(os.Getuid()), strings.TrimSpace(name))
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
