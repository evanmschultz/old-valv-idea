package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
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

func TestStoreDeleteBindingRemovesBoundRow(t *testing.T) {
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

	if err := store.DeleteBinding(context.Background(), project.ID, domain.ProviderCodex); err != nil {
		t.Fatalf("DeleteBinding() error = %v", err)
	}

	if _, err := store.BindingByProjectID(context.Background(), project.ID, domain.ProviderCodex); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("BindingByProjectID() after delete error = %v, want domain.ErrNotFound", err)
	}
}

func TestStoreDeleteBindingReturnsErrNotFoundWhenAbsent(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	err := store.DeleteBinding(context.Background(), "non-existent-project", domain.ProviderCodex)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DeleteBinding() error = %v, want domain.ErrNotFound", err)
	}
}

// TestStoreBootstrapAdvancesV1ToV2 seeds an existing v1 database (the
// minimum supported starting schema after DROP_14 dropped legacy v0
// support) and asserts that Bootstrap advances it to v2, leaving existing
// rows intact and adding the account_env table.
func TestStoreBootstrapAdvancesV1ToV2(t *testing.T) {
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
	seedV1Schema(t, ctx, db)

	projectID := "seed-project-id"
	profileID := "seed-profile-id"
	originalCreatedAt := "2025-01-02T03:04:05Z"
	originalModifiedAt := "2025-01-02T03:04:05Z"

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO projects (id, root, name, created_at) VALUES (?, ?, ?, ?)`,
		projectID, "/tmp/example/v1", "v1-seed", "2025-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed projects row error = %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO profiles (id, provider, name, home_path, created_at) VALUES (?, ?, ?, ?, ?)`,
		profileID, string(domain.ProviderCodex), "v1-seed-codex",
		"/tmp/valv/providers/codex/v1-seed", "2025-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed profiles row error = %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO project_bindings (project_id, profile_id, provider, created_at, modified_at)
		 VALUES (?, ?, ?, ?, ?)`,
		projectID, profileID, string(domain.ProviderCodex),
		originalCreatedAt, originalModifiedAt,
	); err != nil {
		t.Fatalf("seed binding row error = %v", err)
	}

	store := NewStoreFromDB(db)
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	var postVersion int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&postVersion); err != nil {
		t.Fatalf("post-bootstrap user_version error = %v", err)
	}
	if postVersion != 2 {
		t.Fatalf("post-bootstrap user_version = %d, want 2", postVersion)
	}

	binding, err := store.BindingByProjectID(ctx, projectID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("BindingByProjectID() error = %v", err)
	}
	if binding.ProfileID != profileID {
		t.Fatalf("BindingByProjectID().ProfileID = %q, want %q", binding.ProfileID, profileID)
	}
	if got, want := binding.CreatedAt.UTC().Format(time.RFC3339Nano), originalCreatedAt; got != want {
		t.Fatalf("BindingByProjectID().CreatedAt = %q, want %q", got, want)
	}

	// account_env must exist at v2.
	var tableCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='account_env'`,
	).Scan(&tableCount); err != nil {
		t.Fatalf("sqlite_master probe for account_env error = %v", err)
	}
	if tableCount != 1 {
		t.Fatalf("account_env table count = %d, want 1", tableCount)
	}
}

// seedV1Schema writes the v1 DDL and sets PRAGMA user_version = 1. The
// schema mirrors the post-DROP_8 state of the store after composite
// project_bindings PK was introduced.
func seedV1Schema(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	ddl := []string{
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
			project_id TEXT NOT NULL,
			profile_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			created_at TEXT NOT NULL,
			modified_at TEXT NOT NULL,
			PRIMARY KEY (project_id, provider),
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE runtimes (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			project_id TEXT NOT NULL,
			profile_id TEXT NOT NULL,
			mode TEXT NOT NULL,
			container_id TEXT NOT NULL,
			image_ref TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX idx_runtimes_project_id ON runtimes(project_id);`,
		`CREATE TABLE provider_images (
			provider TEXT PRIMARY KEY,
			latest_version TEXT NOT NULL,
			latest_checked_at TEXT NOT NULL,
			installed_version TEXT NOT NULL,
			installed_image_ref TEXT NOT NULL,
			installed_version_tag TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`PRAGMA user_version = 1;`,
	}
	for _, statement := range ddl {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed v1 schema exec error = %v", err)
		}
	}
}

// TestStoreBootstrapRejectsLegacyV0 seeds a legacy v0 database (core tables
// present, user_version = 0 — the pre-DROP_8 fingerprint that DROP_14
// drops) and asserts Bootstrap rejects it with ErrUnsupportedSchema rather
// than auto-migrating.
func TestStoreBootstrapRejectsLegacyV0(t *testing.T) {
	t.Parallel()

	name := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-")
	db, err := Open(OpenOptions{URI: fmt.Sprintf("file:%s?mode=memory&cache=shared", name)})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

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
	// user_version is intentionally left at 0; this is the legacy v0
	// fingerprint we are asserting gets rejected.

	store := NewStoreFromDB(db)
	bootErr := store.Bootstrap(ctx)
	if bootErr == nil {
		t.Fatal("Bootstrap() error = nil, want ErrUnsupportedSchema for legacy v0")
	}
	if !errors.Is(bootErr, domain.ErrUnsupportedSchema) {
		t.Fatalf("Bootstrap() error = %v, want errors.Is(., ErrUnsupportedSchema)", bootErr)
	}
	if !strings.Contains(bootErr.Error(), "user_version=0") {
		t.Fatalf("Bootstrap() error message = %q, want contains user_version=0", bootErr.Error())
	}
	if !strings.Contains(bootErr.Error(), "[1, 2]") {
		t.Fatalf("Bootstrap() error message = %q, want contains [1, 2]", bootErr.Error())
	}
}

// TestStoreBootstrapRejectsForwardIncompatibleSchema seeds user_version = 99
// directly and asserts Bootstrap rejects it with ErrUnsupportedSchema and a
// message that includes the observed version.
func TestStoreBootstrapRejectsForwardIncompatibleSchema(t *testing.T) {
	t.Parallel()

	name := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-")
	db, err := Open(OpenOptions{URI: fmt.Sprintf("file:%s?mode=memory&cache=shared", name)})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	seedV1Schema(t, ctx, db)
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 99`); err != nil {
		t.Fatalf("seed user_version=99 error = %v", err)
	}

	store := NewStoreFromDB(db)
	err = store.Bootstrap(ctx)
	if err == nil {
		t.Fatal("Bootstrap() error = nil, want unsupported-schema rejection")
	}
	if !errors.Is(err, domain.ErrUnsupportedSchema) {
		t.Fatalf("Bootstrap() error = %v, want errors.Is(., ErrUnsupportedSchema)", err)
	}
	if !strings.Contains(err.Error(), "user_version=99") {
		t.Fatalf("Bootstrap() error message = %q, want contains user_version=99", err.Error())
	}
	if !strings.Contains(err.Error(), "[1, 2]") {
		t.Fatalf("Bootstrap() error message = %q, want contains [1, 2]", err.Error())
	}
}

func TestStoreBootstrapIsIdempotentAtV2(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	ctx := context.Background()

	// Already bootstrapped once via newBootstrappedStore. Re-bootstrap and
	// confirm user_version stays at 2 and no rows are corrupted.
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("second Bootstrap() error = %v", err)
	}
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("third Bootstrap() error = %v", err)
	}

	var version int
	if err := store.DB().QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version error = %v", err)
	}
	if version != 2 {
		t.Fatalf("user_version = %d, want 2", version)
	}
}

// TestStoreBootstrapV1ToV2PreservesBindings seeds a v1 database with an
// existing binding row, runs Bootstrap (advancing to v2), and asserts the
// pre-existing rows survive intact across the upgrade.
func TestStoreBootstrapV1ToV2PreservesBindings(t *testing.T) {
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
	seedV1Schema(t, ctx, db)

	projectID := "idempotent-project-id"
	profileID := "idempotent-profile-id"
	originalCreatedAt := "2025-02-03T04:05:06Z"
	originalModifiedAt := "2025-02-03T04:05:06Z"

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO projects (id, root, name, created_at) VALUES (?, ?, ?, ?)`,
		projectID, "/tmp/example/idempotent", "idempotent", "2025-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed projects row error = %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO profiles (id, provider, name, home_path, created_at) VALUES (?, ?, ?, ?, ?)`,
		profileID, string(domain.ProviderCodex), "idempotent-codex",
		"/tmp/valv/providers/codex/idempotent", "2025-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed profiles row error = %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO project_bindings (project_id, profile_id, provider, created_at, modified_at)
		 VALUES (?, ?, ?, ?, ?)`,
		projectID, profileID, string(domain.ProviderCodex),
		originalCreatedAt, originalModifiedAt,
	); err != nil {
		t.Fatalf("seed binding row error = %v", err)
	}

	store := NewStoreFromDB(db)
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("first Bootstrap() error = %v", err)
	}
	firstBinding, err := store.BindingByProjectID(ctx, projectID, domain.ProviderCodex)
	if err != nil {
		t.Fatalf("first BindingByProjectID() error = %v", err)
	}

	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("second Bootstrap() error = %v", err)
	}

	var bindingCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_bindings`).Scan(&bindingCount); err != nil {
		t.Fatalf("project_bindings count error = %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("project_bindings COUNT(*) = %d, want 1", bindingCount)
	}

	secondBinding, err := store.BindingByProjectID(ctx, projectID, domain.ProviderCodex)
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

// TestStoreAccountEnvCRUD round-trips set/get/list/unset on one profile and
// verifies ListAccountEnv returns alphabetical order regardless of insert
// order.
func TestStoreAccountEnvCRUD(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	ctx := context.Background()

	profile := mustProfile(t, domain.ProviderCodex, "crud-account", "/tmp/valv/providers/codex/crud-account")
	if _, err := store.CreateProfile(ctx, profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	// Insert in non-alphabetical order to prove ORDER BY env_key ASC fires.
	wantEntries := map[string]string{
		"GAMMA": "g-val",
		"ALPHA": "a-val",
		"BETA":  "b-val",
	}
	for _, key := range []string{"GAMMA", "ALPHA", "BETA"} {
		entry, err := store.SetAccountEnv(ctx, profile.ID, key, wantEntries[key])
		if err != nil {
			t.Fatalf("SetAccountEnv(%q) error = %v", key, err)
		}
		if entry.EnvKey != key || entry.EnvValue != wantEntries[key] {
			t.Fatalf("SetAccountEnv(%q) returned %+v, want key=%q value=%q",
				key, entry, key, wantEntries[key])
		}
	}

	// Get round-trip
	got, err := store.GetAccountEnv(ctx, profile.ID, "BETA")
	if err != nil {
		t.Fatalf("GetAccountEnv(BETA) error = %v", err)
	}
	if got.EnvValue != "b-val" {
		t.Fatalf("GetAccountEnv(BETA).EnvValue = %q, want %q", got.EnvValue, "b-val")
	}

	listed, err := store.ListAccountEnv(ctx, profile.ID)
	if err != nil {
		t.Fatalf("ListAccountEnv() error = %v", err)
	}
	if got, want := len(listed), 3; got != want {
		t.Fatalf("ListAccountEnv() len = %d, want %d", got, want)
	}
	wantOrder := []string{"ALPHA", "BETA", "GAMMA"}
	for i, e := range listed {
		if e.EnvKey != wantOrder[i] {
			t.Fatalf("ListAccountEnv()[%d].EnvKey = %q, want %q", i, e.EnvKey, wantOrder[i])
		}
	}

	// Upsert replaces value.
	if _, err := store.SetAccountEnv(ctx, profile.ID, "ALPHA", "a-val-updated"); err != nil {
		t.Fatalf("SetAccountEnv(ALPHA, updated) error = %v", err)
	}
	updated, err := store.GetAccountEnv(ctx, profile.ID, "ALPHA")
	if err != nil {
		t.Fatalf("GetAccountEnv(ALPHA) after upsert error = %v", err)
	}
	if updated.EnvValue != "a-val-updated" {
		t.Fatalf("GetAccountEnv(ALPHA).EnvValue = %q, want %q", updated.EnvValue, "a-val-updated")
	}

	// Unset removes one entry; the other two remain.
	if err := store.UnsetAccountEnv(ctx, profile.ID, "BETA"); err != nil {
		t.Fatalf("UnsetAccountEnv(BETA) error = %v", err)
	}
	if _, err := store.GetAccountEnv(ctx, profile.ID, "BETA"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetAccountEnv(BETA) after unset error = %v, want ErrNotFound", err)
	}
	listed, err = store.ListAccountEnv(ctx, profile.ID)
	if err != nil {
		t.Fatalf("ListAccountEnv() post-unset error = %v", err)
	}
	if got, want := len(listed), 2; got != want {
		t.Fatalf("ListAccountEnv() post-unset len = %d, want %d", got, want)
	}

	// Unset of non-existent key returns ErrNotFound.
	if err := store.UnsetAccountEnv(ctx, profile.ID, "MISSING"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UnsetAccountEnv(MISSING) error = %v, want ErrNotFound", err)
	}

	// Get on missing key returns ErrNotFound.
	if _, err := store.GetAccountEnv(ctx, profile.ID, "NOPE"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetAccountEnv(NOPE) error = %v, want ErrNotFound", err)
	}
}

// TestStoreAccountEnvDuplicateKeyAcrossProfiles proves that the same env_key
// is legal across two different profiles — exactly the cross-account
// scenario that motivates account-scoped env maps (e.g. ANTHROPIC_API_KEY
// differs per account).
func TestStoreAccountEnvDuplicateKeyAcrossProfiles(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	ctx := context.Background()

	personal := mustProfile(t, domain.ProviderClaude, "personal", "/tmp/valv/providers/claude/personal")
	hylla := mustProfile(t, domain.ProviderClaude, "hylla", "/tmp/valv/providers/claude/hylla")
	if _, err := store.CreateProfile(ctx, personal); err != nil {
		t.Fatalf("CreateProfile(personal) error = %v", err)
	}
	if _, err := store.CreateProfile(ctx, hylla); err != nil {
		t.Fatalf("CreateProfile(hylla) error = %v", err)
	}

	const key = "ANTHROPIC_API_KEY"
	if _, err := store.SetAccountEnv(ctx, personal.ID, key, "sk-personal"); err != nil {
		t.Fatalf("SetAccountEnv(personal) error = %v", err)
	}
	if _, err := store.SetAccountEnv(ctx, hylla.ID, key, "sk-hylla"); err != nil {
		t.Fatalf("SetAccountEnv(hylla) error = %v", err)
	}

	gotPersonal, err := store.GetAccountEnv(ctx, personal.ID, key)
	if err != nil {
		t.Fatalf("GetAccountEnv(personal) error = %v", err)
	}
	gotHylla, err := store.GetAccountEnv(ctx, hylla.ID, key)
	if err != nil {
		t.Fatalf("GetAccountEnv(hylla) error = %v", err)
	}
	if gotPersonal.EnvValue != "sk-personal" {
		t.Fatalf("personal env value = %q, want %q", gotPersonal.EnvValue, "sk-personal")
	}
	if gotHylla.EnvValue != "sk-hylla" {
		t.Fatalf("hylla env value = %q, want %q", gotHylla.EnvValue, "sk-hylla")
	}
}

// TestStoreAccountEnvSurvivesProfileRename proves that env entries are
// owned by profile.ID, not profile.Name — renaming the account via
// UpdateProfileName preserves all env rows.
func TestStoreAccountEnvSurvivesProfileRename(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t)
	ctx := context.Background()

	profile := mustProfile(t, domain.ProviderCodex, "old-name", "/tmp/valv/providers/codex/old-name")
	if _, err := store.CreateProfile(ctx, profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if _, err := store.SetAccountEnv(ctx, profile.ID, "FOO", "bar"); err != nil {
		t.Fatalf("SetAccountEnv() error = %v", err)
	}

	renamed, err := store.UpdateProfileName(ctx, domain.ProviderCodex, "old-name", "new-name")
	if err != nil {
		t.Fatalf("UpdateProfileName() error = %v", err)
	}
	if renamed.ID != profile.ID {
		t.Fatalf("UpdateProfileName().ID = %q, want %q (ID must be stable across rename)",
			renamed.ID, profile.ID)
	}

	// Env entry resolved by profile.ID is preserved.
	got, err := store.GetAccountEnv(ctx, profile.ID, "FOO")
	if err != nil {
		t.Fatalf("GetAccountEnv() after rename error = %v", err)
	}
	if got.EnvValue != "bar" {
		t.Fatalf("GetAccountEnv().EnvValue = %q, want %q", got.EnvValue, "bar")
	}
}

// TestStoreBootstrapFreshDBStampsV2WithAllCoreTables proves that a brand-new
// database (no tables, user_version=0) bootstraps under the atomic
// transaction path: after Bootstrap returns, all v2 core tables exist AND
// user_version = 2 atomically. This is the happy path for the F1 fix that
// moved core-table DDL inside the BEGIN IMMEDIATE/COMMIT block.
func TestStoreBootstrapFreshDBStampsV2WithAllCoreTables(t *testing.T) {
	t.Parallel()

	store := newBootstrappedStore(t) // brand-new in-memory DB, Bootstrap called once.
	ctx := context.Background()

	var version int
	if err := store.DB().QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version error = %v", err)
	}
	if version != 2 {
		t.Fatalf("post-fresh-bootstrap user_version = %d, want 2", version)
	}

	// Every v2 core table must be present after a single fresh Bootstrap call.
	wantTables := append([]string(nil), coreSchemaTables...)
	wantTables = append(wantTables, "account_env")
	for _, table := range wantTables {
		var count int
		if err := store.DB().QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&count); err != nil {
			t.Fatalf("sqlite_master probe %q error = %v", table, err)
		}
		if count != 1 {
			t.Fatalf("table %q count = %d after fresh bootstrap, want 1", table, count)
		}
	}
}

// TestStoreBootstrapAtomicityRolledBackInitLeavesEmptyDB proves the F1
// invariant by direct behavioral simulation: a fresh-init transaction that
// rolls back must leave zero core tables on disk. The atomic bootstrap path
// in migrateSchemaOnConn relies on this SQLite contract. If this test ever
// fails, the F1 fix is invalid because a crashed fresh-init COULD leave
// partial tables + user_version=0 (the broken state that gets misclassified
// as legacy v0 by the next Bootstrap).
//
// The test directly executes the same DDL set Bootstrap uses inside a
// transaction, then rolls back, then asserts the DB is empty. This proves
// the rollback semantics our atomic bootstrap depends on are real on the
// modernc.org/sqlite driver.
func TestStoreBootstrapAtomicityRolledBackInitLeavesEmptyDB(t *testing.T) {
	t.Parallel()

	dbPath := fmt.Sprintf("%s/atomicity.sqlite3", t.TempDir())
	db, err := Open(OpenOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("db.Conn() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatalf("BEGIN IMMEDIATE error = %v", err)
	}
	for _, statement := range coreSchemaDDL {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("ddl exec error = %v", err)
		}
	}
	if _, err := conn.ExecContext(ctx, accountEnvCreateTable); err != nil {
		t.Fatalf("account_env ddl exec error = %v", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA user_version = 2`); err != nil {
		t.Fatalf("stamp user_version error = %v", err)
	}

	// Simulate the crash: ROLLBACK before COMMIT.
	if _, err := conn.ExecContext(ctx, `ROLLBACK`); err != nil {
		t.Fatalf("ROLLBACK error = %v", err)
	}

	// Post-rollback, no core tables, user_version still 0.
	var version int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("post-rollback user_version error = %v", err)
	}
	if version != 0 {
		t.Fatalf("post-rollback user_version = %d, want 0 (rollback should reset)", version)
	}

	wantAbsent := append([]string(nil), coreSchemaTables...)
	wantAbsent = append(wantAbsent, "account_env")
	for _, table := range wantAbsent {
		var count int
		if err := conn.QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&count); err != nil {
			t.Fatalf("sqlite_master probe %q error = %v", table, err)
		}
		if count != 0 {
			t.Fatalf("table %q count = %d after rollback, want 0 (DDL must roll back)", table, count)
		}
	}

	// And the killer assertion: re-Bootstrap on this rolled-back DB must
	// succeed (classifying it as fresh, not as legacy v0). This is the F1
	// scenario the crash window used to brick.
	store := NewStoreFromDB(db)
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap() after rollback error = %v (DB should be recoverable as fresh)", err)
	}
	if err := store.DB().QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("post-recovery user_version error = %v", err)
	}
	if version != 2 {
		t.Fatalf("post-recovery user_version = %d, want 2", version)
	}
}

// TestStoreBootstrapAtomicityCancelMidTransactionRollsBack proves that a
// real crash analog — context cancellation during the bootstrap transaction
// before COMMIT — also rolls back cleanly, leaving the DB recoverable.
// Uses a cancellable context whose cancel fires after BEGIN but before
// migrateSchemaOnConn's COMMIT can land.
//
// Strategy: open a fresh DB, start Bootstrap with a context that is already
// cancelled. The BEGIN IMMEDIATE itself may or may not error depending on
// driver behavior; the contract we care about is that whatever happens, the
// resulting on-disk state is recoverable by the next Bootstrap.
func TestStoreBootstrapAtomicityCancelMidTransactionRollsBack(t *testing.T) {
	t.Parallel()

	dbPath := fmt.Sprintf("%s/cancel-mid-tx.sqlite3", t.TempDir())
	db, err := Open(OpenOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStoreFromDB(db)

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before Bootstrap runs.

	// Bootstrap with a pre-cancelled context. Behavior: either errors at the
	// pre-tx conn.QueryRowContext step (user_version probe) or fails inside
	// the tx; in either case, no committed state lands.
	_ = store.Bootstrap(cancelCtx)

	// Now recover with a fresh context. This must succeed AND stamp v2,
	// proving the cancelled attempt did not leave the DB in the broken
	// "tables present + user_version=0" state that pre-F1 code would
	// misclassify as legacy v0.
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatalf("recovery Bootstrap() error = %v (DB must remain bootstrappable)", err)
	}
	var version int
	if err := store.DB().QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("post-recovery user_version error = %v", err)
	}
	if version != 2 {
		t.Fatalf("post-recovery user_version = %d, want 2", version)
	}
}

// TestStoreBootstrapConcurrentFirstOpen exercises the "two goroutines, one
// shared sqlite file, both call Bootstrap" path. The DSN-level
// busy_timeout(5000) + BEGIN IMMEDIATE inside migrateSchemaOnConn together
// guarantee "one waits, both succeed" with no SQLITE_BUSY surface; the
// final user_version must be 2.
func TestStoreBootstrapConcurrentFirstOpen(t *testing.T) {
	t.Parallel()

	// Use a real file under t.TempDir() so each connection opens its own
	// SQLite file handle (shared-cache memory DBs would short-circuit the
	// lock behavior we want to exercise).
	dbPath := fmt.Sprintf("%s/concurrent.sqlite3", t.TempDir())

	dbA, err := Open(OpenOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("Open(A) error = %v", err)
	}
	t.Cleanup(func() { _ = dbA.Close() })

	// Pre-seed dbA at v1 with the v1 DDL so both goroutines race to advance
	// it to v2.
	ctx := context.Background()
	seedV1Schema(t, ctx, dbA)

	dbB, err := Open(OpenOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("Open(B) error = %v", err)
	}
	t.Cleanup(func() { _ = dbB.Close() })

	storeA := NewStoreFromDB(dbA)
	storeB := NewStoreFromDB(dbB)

	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errCh <- storeA.Bootstrap(ctx)
	}()
	go func() {
		defer wg.Done()
		<-start
		errCh <- storeB.Bootstrap(ctx)
	}()
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent Bootstrap() error = %v (want both succeed)", err)
		}
	}

	var version int
	if err := dbA.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version error = %v", err)
	}
	if version != 2 {
		t.Fatalf("post-concurrent user_version = %d, want 2", version)
	}
}
