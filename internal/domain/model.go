package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
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

func NewProject(root string) (Project, error) {
	cleanRoot := filepath.Clean(strings.TrimSpace(root))
	if cleanRoot == "." || cleanRoot == "" {
		return Project{}, fmt.Errorf("new project: root is required")
	}
	id, err := newID()
	if err != nil {
		return Project{}, fmt.Errorf("new project: %w", err)
	}
	return Project{
		ID:        id,
		Root:      cleanRoot,
		Name:      filepath.Base(cleanRoot),
		CreatedAt: time.Now().UTC(),
	}, nil
}

func NewProfile(provider Provider, name string, homePath string) (Profile, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return Profile{}, fmt.Errorf("new profile: name is required")
	}
	trimmedHome := strings.TrimSpace(homePath)
	if trimmedHome == "" {
		return Profile{}, fmt.Errorf("new profile: home path is required")
	}
	id, err := newID()
	if err != nil {
		return Profile{}, fmt.Errorf("new profile: %w", err)
	}
	return Profile{
		ID:        id,
		Provider:  provider,
		Name:      trimmedName,
		HomePath:  filepath.Clean(trimmedHome),
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

func newID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
