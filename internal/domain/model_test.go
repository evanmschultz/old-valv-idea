package domain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewProject(t *testing.T) {
	t.Parallel()

	project, err := NewProject("/tmp/example/project")
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	if project.Root != "/tmp/example/project" {
		t.Fatalf("NewProject().Root = %q", project.Root)
	}
	if project.Name != "project" {
		t.Fatalf("NewProject().Name = %q", project.Name)
	}
	if project.ID == "" {
		t.Fatal("expected generated project id")
	}
}

func TestNewProfile(t *testing.T) {
	t.Parallel()

	profile, err := NewProfile(ProviderCodex, "dev", "/tmp/valv/providers/codex/dev")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	if profile.Provider != ProviderCodex {
		t.Fatalf("NewProfile().Provider = %q", profile.Provider)
	}
	if profile.Name != "dev" {
		t.Fatalf("NewProfile().Name = %q", profile.Name)
	}
}

func TestNewProjectBinding(t *testing.T) {
	t.Parallel()

	binding, err := NewProjectBinding("project-id", "profile-id", ProviderCodex)
	if err != nil {
		t.Fatalf("NewProjectBinding() error = %v", err)
	}
	if binding.ProjectID != "project-id" || binding.ProfileID != "profile-id" {
		t.Fatalf("unexpected binding: %+v", binding)
	}
}

func TestNewRuntimeRecord(t *testing.T) {
	t.Parallel()

	record, err := NewRuntimeRecord(ProviderCodex, "project-id", "profile-id", ModeFresh, "container-1", "ghcr.io/example/codex:1", "running")
	if err != nil {
		t.Fatalf("NewRuntimeRecord() error = %v", err)
	}
	if record.ID == "" {
		t.Fatal("expected runtime id")
	}
	if record.Mode != ModeFresh {
		t.Fatalf("NewRuntimeRecord().Mode = %q", record.Mode)
	}
}

func TestNewProviderImageState(t *testing.T) {
	t.Parallel()

	state, err := NewProviderImageState(ProviderCodex, "0.117.0", "0.117.0", "valv-codex:dev", "valv-codex:0-117-0")
	if err != nil {
		t.Fatalf("NewProviderImageState() error = %v", err)
	}
	if got, want := state.Provider, ProviderCodex; got != want {
		t.Fatalf("Provider = %q, want %q", got, want)
	}
	if got, want := state.InstalledVersionTag, "valv-codex:0-117-0"; got != want {
		t.Fatalf("InstalledVersionTag = %q, want %q", got, want)
	}
}

func TestNewProjectRejectsEmptyRoot(t *testing.T) {
	t.Parallel()

	if _, err := NewProject(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewProjectBindingRejectsEmptyIDs(t *testing.T) {
	t.Parallel()

	if _, err := NewProjectBinding("", "profile-id", ProviderCodex); err == nil {
		t.Fatal("NewProjectBinding() error = nil, want project-id failure")
	}
	if _, err := NewProjectBinding("project-id", "", ProviderCodex); err == nil {
		t.Fatal("NewProjectBinding() error = nil, want profile-id failure")
	}
}

func TestNewRuntimeRecordRejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()

	if _, err := NewRuntimeRecord(ProviderCodex, "", "profile-id", ModeFresh, "container-1", "img:1", "running"); err == nil {
		t.Fatal("NewRuntimeRecord() error = nil, want project-id failure")
	}
	if _, err := NewRuntimeRecord(ProviderCodex, "project-id", "profile-id", ModeFresh, "", "img:1", "running"); err == nil {
		t.Fatal("NewRuntimeRecord() error = nil, want container-id failure")
	}
	if _, err := NewRuntimeRecord(ProviderCodex, "project-id", "profile-id", ModeFresh, "container-1", "", "running"); err == nil {
		t.Fatal("NewRuntimeRecord() error = nil, want image-ref failure")
	}
}

func TestNewProjectNormalizesExistingSymlinkedPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", root, err)
	}
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "project-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	project, err := NewProject(link)
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	if project.Root != wantRoot {
		t.Fatalf("NewProject().Root = %q, want %q", project.Root, wantRoot)
	}
}
