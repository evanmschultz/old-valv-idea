package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

	fetched, err := store.BindingByProjectID(context.Background(), project.ID, domain.ProviderCodex)
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

func TestStoreProviderImageStateLifecycle(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	state, err := domain.NewProviderImageState(domain.ProviderCodex, "0.117.0", "0.117.0", "valv-codex:dev", "valv-codex:0-117-0")
	if err != nil {
		t.Fatalf("NewProviderImageState() error = %v", err)
	}
	if _, err := store.UpsertProviderImageState(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderImageState() error = %v", err)
	}

	state.InstalledVersion = "0.118.0"
	state.InstalledVersionTag = "valv-codex:0-118-0"
	state.UpdatedAt = state.UpdatedAt.Add(time.Minute)
	if _, err := store.UpsertProviderImageState(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderImageState() update error = %v", err)
	}

	fetched, err := store.ProviderImageState(context.Background(), domain.ProviderCodex)
	if err != nil {
		t.Fatalf("ProviderImageState() error = %v", err)
	}
	if got, want := fetched.InstalledVersion, "0.118.0"; got != want {
		t.Fatalf("ProviderImageState().InstalledVersion = %q, want %q", got, want)
	}
	if got, want := fetched.InstalledVersionTag, "valv-codex:0-118-0"; got != want {
		t.Fatalf("ProviderImageState().InstalledVersionTag = %q, want %q", got, want)
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
	if _, err := store.BindingByProjectID(context.Background(), "missing", domain.ProviderCodex); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("BindingByProjectID() error = %v, want domain.ErrNotFound", err)
	}
	if _, err := store.RuntimeByID(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("RuntimeByID() error = %v, want domain.ErrNotFound", err)
	}
	if _, err := store.ProviderImageState(context.Background(), domain.ProviderCodex); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ProviderImageState() error = %v, want domain.ErrNotFound", err)
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

func TestStoreUpdateAndDeleteProfile(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	profile := mustProfile(t, domain.ProviderCodex, "personal", "/tmp/valv/providers/codex/personal")
	if _, err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	renamed, err := store.UpdateProfileName(context.Background(), domain.ProviderCodex, "personal", "hylla")
	if err != nil {
		t.Fatalf("UpdateProfileName() error = %v", err)
	}
	if got, want := renamed.Name, "hylla"; got != want {
		t.Fatalf("UpdateProfileName().Name = %q, want %q", got, want)
	}
	if _, err := store.ProfileByName(context.Background(), domain.ProviderCodex, "personal"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ProfileByName(old) error = %v, want domain.ErrNotFound", err)
	}

	if err := store.DeleteProfile(context.Background(), domain.ProviderCodex, "hylla"); err != nil {
		t.Fatalf("DeleteProfile() error = %v", err)
	}
	if _, err := store.ProfileByName(context.Background(), domain.ProviderCodex, "hylla"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ProfileByName(deleted) error = %v, want domain.ErrNotFound", err)
	}
}

func TestStoreListsProjectsAndBindings(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	project, _ := domain.NewProject("/tmp/example/project")
	profile := mustProfile(t, domain.ProviderCodex, "dev", "/tmp/valv/providers/codex/dev")
	if _, err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	binding, err := domain.NewProjectBinding(project.ID, profile.ID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("NewProjectBinding() error = %v", err)
	}
	if _, err := store.UpsertProjectBinding(context.Background(), binding); err != nil {
		t.Fatalf("UpsertProjectBinding() error = %v", err)
	}

	projects, err := store.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if got, want := len(projects), 1; got != want {
		t.Fatalf("len(projects) = %d, want %d", got, want)
	}

	bindings, err := store.ListBindings(context.Background())
	if err != nil {
		t.Fatalf("ListBindings() error = %v", err)
	}
	if got, want := len(bindings), 1; got != want {
		t.Fatalf("len(bindings) = %d, want %d", got, want)
	}
	if got, want := bindings[0].ProfileID, profile.ID; got != want {
		t.Fatalf("ListBindings()[0].ProfileID = %q, want %q", got, want)
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

func TestStoreCompositeBindingsCoexistByProvider(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	project, _ := domain.NewProject("/tmp/example/project")
	codexProfile := mustProfile(t, domain.ProviderCodex, "codex-dev", "/tmp/valv/providers/codex/dev")
	claudeProfile := mustProfile(t, domain.ProviderClaude, "claude-dev", "/tmp/valv/providers/claude/dev")

	if _, err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := store.CreateProfile(context.Background(), codexProfile); err != nil {
		t.Fatalf("CreateProfile(codex) error = %v", err)
	}
	if _, err := store.CreateProfile(context.Background(), claudeProfile); err != nil {
		t.Fatalf("CreateProfile(claude) error = %v", err)
	}

	codexBinding, err := domain.NewProjectBinding(project.ID, codexProfile.ID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("NewProjectBinding(codex) error = %v", err)
	}
	if _, err := store.UpsertProjectBinding(context.Background(), codexBinding); err != nil {
		t.Fatalf("UpsertProjectBinding(codex) error = %v", err)
	}

	claudeBinding, err := domain.NewProjectBinding(project.ID, claudeProfile.ID, domain.ProviderClaude)
	if err != nil {
		t.Fatalf("NewProjectBinding(claude) error = %v", err)
	}
	if _, err := store.UpsertProjectBinding(context.Background(), claudeBinding); err != nil {
		t.Fatalf("UpsertProjectBinding(claude) error = %v", err)
	}

	fetchedCodex, err := store.BindingByProjectID(context.Background(), project.ID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("BindingByProjectID(codex) error = %v", err)
	}
	if fetchedCodex.ProfileID != codexProfile.ID {
		t.Fatalf("BindingByProjectID(codex).ProfileID = %q, want %q", fetchedCodex.ProfileID, codexProfile.ID)
	}
	if fetchedCodex.Provider != domain.ProviderCodex {
		t.Fatalf("BindingByProjectID(codex).Provider = %q, want %q", fetchedCodex.Provider, domain.ProviderCodex)
	}

	fetchedClaude, err := store.BindingByProjectID(context.Background(), project.ID, domain.ProviderClaude)
	if err != nil {
		t.Fatalf("BindingByProjectID(claude) error = %v", err)
	}
	if fetchedClaude.ProfileID != claudeProfile.ID {
		t.Fatalf("BindingByProjectID(claude).ProfileID = %q, want %q", fetchedClaude.ProfileID, claudeProfile.ID)
	}
	if fetchedClaude.Provider != domain.ProviderClaude {
		t.Fatalf("BindingByProjectID(claude).Provider = %q, want %q", fetchedClaude.Provider, domain.ProviderClaude)
	}
}

func TestStoreMigrationPreservesLegacyCodexBinding(t *testing.T) {
	t.Parallel()

	name := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-")
	db, err := Open(OpenOptions{URI: fmt.Sprintf("file:%s?mode=memory&cache=shared", name)})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	ctx := context.Background()
	legacyDDL := []string{
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			root TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE profiles (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			name TEXT NOT NULL,
			home_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(provider, name)
		);`,
		`CREATE TABLE project_bindings (
			project_id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			created_at TEXT NOT NULL,
			modified_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
		);`,
	}
	for _, statement := range legacyDDL {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("legacy DDL exec error = %v", err)
		}
	}

	legacyProjectID := "legacy-project-id"
	legacyProfileID := "legacy-profile-id"
	originalCreatedAt := "2025-01-02T03:04:05Z"
	originalModifiedAt := "2025-01-02T03:04:05Z"

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO projects (id, root, name, created_at) VALUES (?, ?, ?, ?)`,
		legacyProjectID,
		"/tmp/example/legacy",
		"legacy",
		"2025-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed projects row error = %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO profiles (id, provider, name, home_path, created_at) VALUES (?, ?, ?, ?, ?)`,
		legacyProfileID,
		string(domain.ProviderCodex),
		"legacy-codex",
		"/tmp/valv/providers/codex/legacy",
		"2025-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed profiles row error = %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO project_bindings (project_id, profile_id, provider, created_at, modified_at) VALUES (?, ?, ?, ?, ?)`,
		legacyProjectID,
		legacyProfileID,
		string(domain.ProviderCodex),
		originalCreatedAt,
		originalModifiedAt,
	); err != nil {
		t.Fatalf("seed legacy binding row error = %v", err)
	}

	var preVersion int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&preVersion); err != nil {
		t.Fatalf("pre-bootstrap user_version error = %v", err)
	}
	if preVersion != 0 {
		t.Fatalf("pre-bootstrap user_version = %d, want 0", preVersion)
	}

	store := NewStoreFromDB(db)
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	var postVersion int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&postVersion); err != nil {
		t.Fatalf("post-bootstrap user_version error = %v", err)
	}
	if postVersion != 1 {
		t.Fatalf("post-bootstrap user_version = %d, want 1", postVersion)
	}

	binding, err := store.BindingByProjectID(ctx, legacyProjectID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("BindingByProjectID() error = %v", err)
	}
	if binding.ProfileID != legacyProfileID {
		t.Fatalf("BindingByProjectID().ProfileID = %q, want %q", binding.ProfileID, legacyProfileID)
	}
	if got, want := binding.CreatedAt.UTC().Format(time.RFC3339Nano), originalCreatedAt; got != want {
		t.Fatalf("BindingByProjectID().CreatedAt = %q, want %q", got, want)
	}
	if got, want := binding.ModifiedAt.UTC().Format(time.RFC3339Nano), originalModifiedAt; got != want {
		t.Fatalf("BindingByProjectID().ModifiedAt = %q, want %q", got, want)
	}
}

func TestStoreBootstrapIsIdempotentAfterMigration(t *testing.T) {
	t.Parallel()

	name := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-")
	db, err := Open(OpenOptions{URI: fmt.Sprintf("file:%s?mode=memory&cache=shared", name)})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	ctx := context.Background()
	legacyDDL := []string{
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			root TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE profiles (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			name TEXT NOT NULL,
			home_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(provider, name)
		);`,
		`CREATE TABLE project_bindings (
			project_id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			created_at TEXT NOT NULL,
			modified_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
		);`,
	}
	for _, statement := range legacyDDL {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("legacy DDL exec error = %v", err)
		}
	}

	legacyProjectID := "idempotent-project-id"
	legacyProfileID := "idempotent-profile-id"
	originalCreatedAt := "2025-02-03T04:05:06Z"
	originalModifiedAt := "2025-02-03T04:05:06Z"

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO projects (id, root, name, created_at) VALUES (?, ?, ?, ?)`,
		legacyProjectID,
		"/tmp/example/idempotent",
		"idempotent",
		"2025-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed projects row error = %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO profiles (id, provider, name, home_path, created_at) VALUES (?, ?, ?, ?, ?)`,
		legacyProfileID,
		string(domain.ProviderCodex),
		"idempotent-codex",
		"/tmp/valv/providers/codex/idempotent",
		"2025-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed profiles row error = %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO project_bindings (project_id, profile_id, provider, created_at, modified_at) VALUES (?, ?, ?, ?, ?)`,
		legacyProjectID,
		legacyProfileID,
		string(domain.ProviderCodex),
		originalCreatedAt,
		originalModifiedAt,
	); err != nil {
		t.Fatalf("seed legacy binding row error = %v", err)
	}

	store := NewStoreFromDB(db)
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("first Bootstrap() error = %v", err)
	}

	firstBinding, err := store.BindingByProjectID(ctx, legacyProjectID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("first BindingByProjectID() error = %v", err)
	}

	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("second Bootstrap() error = %v", err)
	}

	var stagingCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project_bindings_new'`,
	).Scan(&stagingCount); err != nil {
		t.Fatalf("sqlite_master probe error = %v", err)
	}
	if stagingCount != 0 {
		t.Fatalf("project_bindings_new table count = %d, want 0", stagingCount)
	}

	var bindingCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_bindings`).Scan(&bindingCount); err != nil {
		t.Fatalf("project_bindings count error = %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("project_bindings COUNT(*) = %d, want 1", bindingCount)
	}

	secondBinding, err := store.BindingByProjectID(ctx, legacyProjectID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("second BindingByProjectID() error = %v", err)
	}
	if secondBinding.ProfileID != firstBinding.ProfileID {
		t.Fatalf("ProfileID changed across bootstrap: got %q, want %q", secondBinding.ProfileID, firstBinding.ProfileID)
	}
	if !secondBinding.CreatedAt.Equal(firstBinding.CreatedAt) {
		t.Fatalf("CreatedAt changed across bootstrap: got %v, want %v", secondBinding.CreatedAt, firstBinding.CreatedAt)
	}
	if !secondBinding.ModifiedAt.Equal(firstBinding.ModifiedAt) {
		t.Fatalf("ModifiedAt changed across bootstrap: got %v, want %v", secondBinding.ModifiedAt, firstBinding.ModifiedAt)
	}
}
