package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/evanmschultz/valv/internal/domain"
)

func newBootstrappedStore(t *testing.T) *Store {
	t.Helper()

	name := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-")
	db, err := Open(OpenOptions{URI: fmt.Sprintf("file:%s?mode=memory&cache=shared", name)})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	store := NewStoreFromDB(db)
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestStoreProjectLifecycle(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	project, err := domain.NewProject("/tmp/example/project")
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	created, err := store.CreateProject(context.Background(), project)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	fetched, err := store.ProjectByRoot(context.Background(), created.Root)
	if err != nil {
		t.Fatalf("ProjectByRoot() error = %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("ProjectByRoot().ID = %q, want %q", fetched.ID, created.ID)
	}
}

func TestStoreProfileAndBindingLifecycle(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	project, _ := domain.NewProject("/tmp/example/project")
	profile, _ := domain.NewProfile(domain.ProviderCodex, "dev", "/tmp/valv/providers/codex/dev")

	if _, err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	fetchedProfile, err := store.ProfileByID(context.Background(), profile.ID)
	if err != nil {
		t.Fatalf("ProfileByID() error = %v", err)
	}
	if fetchedProfile.ID != profile.ID {
		t.Fatalf("ProfileByID().ID = %q, want %q", fetchedProfile.ID, profile.ID)
	}

	binding, err := domain.NewProjectBinding(project.ID, profile.ID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("NewProjectBinding() error = %v", err)
	}
	if _, err := store.UpsertProjectBinding(context.Background(), binding); err != nil {
		t.Fatalf("UpsertProjectBinding() error = %v", err)
	}

	fetched, err := store.BindingByProjectID(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("BindingByProjectID() error = %v", err)
	}
	if fetched.ProfileID != profile.ID {
		t.Fatalf("BindingByProjectID().ProfileID = %q, want %q", fetched.ProfileID, profile.ID)
	}
}

func TestStoreRuntimeLifecycle(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	project, _ := domain.NewProject("/tmp/example/project")
	profile, _ := domain.NewProfile(domain.ProviderCodex, "dev", "/tmp/valv/providers/codex/dev")
	if _, err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	runtimeRecord, err := domain.NewRuntimeRecord(domain.ProviderCodex, project.ID, profile.ID, domain.ModeFresh, "container-1", "ghcr.io/example/codex:1", "running")
	if err != nil {
		t.Fatalf("NewRuntimeRecord() error = %v", err)
	}
	if _, err := store.UpsertRuntime(context.Background(), runtimeRecord); err != nil {
		t.Fatalf("UpsertRuntime() error = %v", err)
	}

	runtimeRecord.Status = "stopped"
	if _, err := store.UpsertRuntime(context.Background(), runtimeRecord); err != nil {
		t.Fatalf("UpsertRuntime() update error = %v", err)
	}

	fetched, err := store.RuntimeByID(context.Background(), runtimeRecord.ID)
	if err != nil {
		t.Fatalf("RuntimeByID() error = %v", err)
	}
	if fetched.Status != "stopped" {
		t.Fatalf("RuntimeByID().Status = %q", fetched.Status)
	}

	records, err := store.ListRuntimesByProjectID(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListRuntimesByProjectID() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListRuntimesByProjectID() len = %d, want 1", len(records))
	}
}

func TestStoreNotFoundErrors(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	if _, err := store.ProjectByRoot(context.Background(), "/missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ProjectByRoot() error = %v, want domain.ErrNotFound", err)
	}
	if _, err := store.ProfileByName(context.Background(), domain.ProviderCodex, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ProfileByName() error = %v, want domain.ErrNotFound", err)
	}
	if _, err := store.ProfileByID(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ProfileByID() error = %v, want domain.ErrNotFound", err)
	}
	if _, err := store.BindingByProjectID(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("BindingByProjectID() error = %v, want domain.ErrNotFound", err)
	}
	if _, err := store.RuntimeByID(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("RuntimeByID() error = %v, want domain.ErrNotFound", err)
	}
}

func TestStoreForeignKeysRejectInvalidBindings(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	binding, err := domain.NewProjectBinding("missing-project", "missing-profile", domain.ProviderCodex)
	if err != nil {
		t.Fatalf("NewProjectBinding() error = %v", err)
	}

	if _, err := store.UpsertProjectBinding(context.Background(), binding); err == nil {
		t.Fatal("UpsertProjectBinding() error = nil, want foreign key failure")
	}
}

func TestStoreProjectByRootRejectsInvalidTimestamp(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	if _, err := store.db.ExecContext(
		context.Background(),
		`INSERT INTO projects (id, root, name, created_at) VALUES (?, ?, ?, ?)`,
		"project-id",
		"/tmp/example/project",
		"project",
		"not-a-timestamp",
	); err != nil {
		t.Fatalf("insert invalid project timestamp error = %v", err)
	}

	if _, err := store.ProjectByRoot(context.Background(), "/tmp/example/project"); err == nil {
		t.Fatal("ProjectByRoot() error = nil, want parse failure")
	}
}

func TestStoreListProfilesByProviderReturnsSortedProfiles(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	profiles := []domain.Profile{
		mustProfile(t, domain.ProviderCodex, "b", "/tmp/valv/providers/codex/b"),
		mustProfile(t, domain.ProviderCodex, "a", "/tmp/valv/providers/codex/a"),
	}
	for _, profile := range profiles {
		if _, err := store.CreateProfile(context.Background(), profile); err != nil {
			t.Fatalf("CreateProfile(%q) error = %v", profile.Name, err)
		}
	}

	listed, err := store.ListProfilesByProvider(context.Background(), domain.ProviderCodex)
	if err != nil {
		t.Fatalf("ListProfilesByProvider() error = %v", err)
	}
	if got, want := len(listed), 2; got != want {
		t.Fatalf("profiles len = %d, want %d", got, want)
	}
	if listed[0].Name != "a" || listed[1].Name != "b" {
		t.Fatalf("profiles order = [%s %s], want [a b]", listed[0].Name, listed[1].Name)
	}
}

func mustProfile(t *testing.T, provider domain.Provider, name, home string) domain.Profile {
	t.Helper()
	profile, err := domain.NewProfile(provider, name, home)
	if err != nil {
		t.Fatalf("NewProfile(%q) error = %v", name, err)
	}
	return profile
}
