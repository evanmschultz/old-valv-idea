package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanmschultz/valv/internal/pathutil"
)

type Project struct {
	ID        string
	Root      string
	Name      string
	CreatedAt time.Time
}

type Profile struct {
	ID        string
	Provider  Provider
	Name      string
	HomePath  string
	CreatedAt time.Time
}

type ProjectBinding struct {
	ProjectID  string
	ProfileID  string
	Provider   Provider
	CreatedAt  time.Time
	ModifiedAt time.Time
}

type RuntimeRecord struct {
	ID          string
	Provider    Provider
	ProjectID   string
	ProfileID   string
	Mode        Mode
	ContainerID string
	ImageRef    string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ProviderImageState struct {
	Provider            Provider
	LatestVersion       string
	LatestCheckedAt     time.Time
	InstalledVersion    string
	InstalledImageRef   string
	InstalledVersionTag string
	UpdatedAt           time.Time
}

func NewProject(root string) (Project, error) {
	normalizedRoot, err := pathutil.Normalize(root)
	if err != nil {
		return Project{}, fmt.Errorf("new project: %w", err)
	}
	id, err := newID()
	if err != nil {
		return Project{}, fmt.Errorf("new project: %w", err)
	}
	return Project{
		ID:        id,
		Root:      normalizedRoot,
		Name:      filepath.Base(normalizedRoot),
		CreatedAt: time.Now().UTC(),
	}, nil
}

func NewProfile(provider Provider, name string, homePath string) (Profile, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return Profile{}, fmt.Errorf("new profile: name is required")
	}
	normalizedHome, err := pathutil.Normalize(homePath)
	if err != nil {
		return Profile{}, fmt.Errorf("new profile: %w", err)
	}
	id, err := newID()
	if err != nil {
		return Profile{}, fmt.Errorf("new profile: %w", err)
	}
	return Profile{
		ID:        id,
		Provider:  provider,
		Name:      trimmedName,
		HomePath:  normalizedHome,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func NewProjectBinding(projectID string, profileID string, provider Provider) (ProjectBinding, error) {
	if strings.TrimSpace(projectID) == "" {
		return ProjectBinding{}, fmt.Errorf("new project binding: project id is required")
	}
	if strings.TrimSpace(profileID) == "" {
		return ProjectBinding{}, fmt.Errorf("new project binding: profile id is required")
	}
	now := time.Now().UTC()
	return ProjectBinding{
		ProjectID:  projectID,
		ProfileID:  profileID,
		Provider:   provider,
		CreatedAt:  now,
		ModifiedAt: now,
	}, nil
}

func NewRuntimeRecord(provider Provider, projectID string, profileID string, mode Mode, containerID string, imageRef string, status string) (RuntimeRecord, error) {
	if strings.TrimSpace(projectID) == "" {
		return RuntimeRecord{}, fmt.Errorf("new runtime record: project id is required")
	}
	if strings.TrimSpace(profileID) == "" {
		return RuntimeRecord{}, fmt.Errorf("new runtime record: profile id is required")
	}
	if strings.TrimSpace(containerID) == "" {
		return RuntimeRecord{}, fmt.Errorf("new runtime record: container id is required")
	}
	if strings.TrimSpace(imageRef) == "" {
		return RuntimeRecord{}, fmt.Errorf("new runtime record: image ref is required")
	}
	id, err := newID()
	if err != nil {
		return RuntimeRecord{}, fmt.Errorf("new runtime record: %w", err)
	}
	now := time.Now().UTC()
	return RuntimeRecord{
		ID:          id,
		Provider:    provider,
		ProjectID:   projectID,
		ProfileID:   profileID,
		Mode:        mode,
		ContainerID: strings.TrimSpace(containerID),
		ImageRef:    strings.TrimSpace(imageRef),
		Status:      strings.TrimSpace(status),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func NewProviderImageState(provider Provider, latestVersion, installedVersion, installedImageRef, installedVersionTag string) (ProviderImageState, error) {
	if strings.TrimSpace(string(provider)) == "" {
		return ProviderImageState{}, fmt.Errorf("new provider image state: provider is required")
	}
	now := time.Now().UTC()
	return ProviderImageState{
		Provider:            provider,
		LatestVersion:       strings.TrimSpace(latestVersion),
		LatestCheckedAt:     now,
		InstalledVersion:    strings.TrimSpace(installedVersion),
		InstalledImageRef:   strings.TrimSpace(installedImageRef),
		InstalledVersionTag: strings.TrimSpace(installedVersionTag),
		UpdatedAt:           now,
	}, nil
}

func newID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
