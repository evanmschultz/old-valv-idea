package globalswitch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"github.com/evanmschultz/valv/internal/domain"
)

type Store interface {
	ProfileByID(context.Context, string) (domain.Profile, error)
	ProfileByName(context.Context, domain.Provider, string) (domain.Profile, error)
}

type Options struct {
	Store        Store
	ProviderRoot string
	StateDir     string
	Logger       *log.Logger
}

type Service struct {
	store        Store
	providerRoot string
	stateDir     string
	logger       *log.Logger
}

type SwitchResult struct {
	Profile    domain.Profile
	LinkPath   string
	StatePath  string
	TargetPath string
}

type CurrentResult struct {
	Profile    domain.Profile
	LinkPath   string
	StatePath  string
	TargetPath string
}

type stateRecord struct {
	Provider    string    `json:"provider"`
	ProfileID   string    `json:"profile_id"`
	ProfileName string    `json:"profile_name"`
	ProfileHome string    `json:"profile_home"`
	LinkPath    string    `json:"link_path"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func New(options Options) (Service, error) {
	if options.Store == nil {
		return Service{}, fmt.Errorf("new global switch service: store is required")
	}
	if strings.TrimSpace(options.ProviderRoot) == "" {
		return Service{}, fmt.Errorf("new global switch service: provider root is required")
	}
	if strings.TrimSpace(options.StateDir) == "" {
		return Service{}, fmt.Errorf("new global switch service: state dir is required")
	}
	return Service{
		store:        options.Store,
		providerRoot: strings.TrimSpace(options.ProviderRoot),
		stateDir:     strings.TrimSpace(options.StateDir),
		logger:       options.Logger,
	}, nil
}

func (s Service) Switch(ctx context.Context, provider domain.Provider, profileName string) (SwitchResult, error) {
	profile, err := s.store.ProfileByName(ctx, provider, strings.TrimSpace(profileName))
	if err != nil {
		return SwitchResult{}, fmt.Errorf("switch global profile %q/%q: lookup profile: %w", provider, profileName, err)
	}
	linkPath := s.currentLinkPath(provider)
	statePath := s.statePath(provider)
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return SwitchResult{}, fmt.Errorf("switch global profile %q/%q: ensure provider dir: %w", provider, profileName, err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return SwitchResult{}, fmt.Errorf("switch global profile %q/%q: ensure state dir: %w", provider, profileName, err)
	}

	tmpLink := fmt.Sprintf("%s.tmp-%d", linkPath, time.Now().UnixNano())
	if err := os.Remove(tmpLink); err != nil && !os.IsNotExist(err) {
		return SwitchResult{}, fmt.Errorf("switch global profile %q/%q: clear temp link: %w", provider, profileName, err)
	}
	if err := os.Symlink(profile.HomePath, tmpLink); err != nil {
		return SwitchResult{}, fmt.Errorf("switch global profile %q/%q: create temp link: %w", provider, profileName, err)
	}
	if err := os.Rename(tmpLink, linkPath); err != nil {
		_ = os.Remove(tmpLink)
		return SwitchResult{}, fmt.Errorf("switch global profile %q/%q: activate current link: %w", provider, profileName, err)
	}

	state := stateRecord{
		Provider:    string(provider),
		ProfileID:   profile.ID,
		ProfileName: profile.Name,
		ProfileHome: profile.HomePath,
		LinkPath:    linkPath,
		UpdatedAt:   time.Now().UTC(),
	}
	if err := writeState(statePath, state); err != nil {
		return SwitchResult{}, fmt.Errorf("switch global profile %q/%q: persist state: %w", provider, profileName, err)
	}

	s.debug("switched global profile", "provider", provider, "profile", profile.Name, "link_path", linkPath, "state_path", statePath)
	return SwitchResult{Profile: profile, LinkPath: linkPath, StatePath: statePath, TargetPath: profile.HomePath}, nil
}

func (s Service) Current(ctx context.Context, provider domain.Provider) (CurrentResult, error) {
	_ = ctx
	statePath := s.statePath(provider)
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return CurrentResult{}, fmt.Errorf("current global profile %q: %w", provider, domain.ErrNotFound)
		}
		return CurrentResult{}, fmt.Errorf("current global profile %q: read state: %w", provider, err)
	}
	var state stateRecord
	if err := json.Unmarshal(data, &state); err != nil {
		return CurrentResult{}, fmt.Errorf("current global profile %q: decode state: %w", provider, err)
	}
	profile, err := s.store.ProfileByID(ctx, state.ProfileID)
	if err != nil {
		return CurrentResult{}, fmt.Errorf("current global profile %q: lookup profile %q: %w", provider, state.ProfileID, err)
	}
	return CurrentResult{Profile: profile, LinkPath: state.LinkPath, StatePath: statePath, TargetPath: state.ProfileHome}, nil
}

func (s Service) currentLinkPath(provider domain.Provider) string {
	return filepath.Join(s.providerRoot, string(provider), "current")
}

func (s Service) statePath(provider domain.Provider) string {
	return filepath.Join(s.stateDir, "globalswitch", string(provider)+".json")
}

func writeState(path string, state stateRecord) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal global switch state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write global switch state %q: %w", path, err)
	}
	return nil
}

func (s Service) debug(msg string, keyvals ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Debug(msg, keyvals...)
}
