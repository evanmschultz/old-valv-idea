package domain

import "testing"

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

func TestNewProjectRejectsEmptyRoot(t *testing.T) {
	t.Parallel()

	if _, err := NewProject(""); err == nil {
		t.Fatal("expected error")
	}
}
