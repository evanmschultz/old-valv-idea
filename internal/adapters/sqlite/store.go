package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/evanmschultz/valv/internal/domain"
)

type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := Open(OpenOptions{Path: path})
	if err != nil {
		return nil, fmt.Errorf("new sqlite store: %w", err)
	}
	return &Store{db: db}, nil
}

func NewStoreFromDB(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// minSupportedSchemaVersion / maxSupportedSchemaVersion bound the schema
// window the current binary understands. Bootstrap rejects PRAGMA
// user_version values outside [min, max] with domain.ErrUnsupportedSchema.
// DROP_14 dropped legacy v0 support; the minimum supported starting schema
// is v1. New databases bootstrap straight to v2.
const (
	minSupportedSchemaVersion = 1
	maxSupportedSchemaVersion = 2
)

// coreSchemaTables is the set of table names whose presence (with
// user_version = 0) indicates a legacy v0 database. v0 is no longer
// supported as of DROP_14; pre-existing rows must be migrated by rebuilding
// the database, not by auto-upgrade.
var coreSchemaTables = []string{
	"projects",
	"profiles",
	"project_bindings",
	"runtimes",
	"provider_images",
}

func (s *Store) Bootstrap(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap sqlite store: acquire conn: %w", err)
	}
	defer conn.Close()

	// Detect "legacy v0 database" vs "brand-new database" BEFORE running any
	// DDL. A legacy v0 database has core tables present but user_version = 0
	// (because previous bootstrap iterations never set the marker). DROP_14
	// dropped v0 support, so this state must be rejected with
	// ErrUnsupportedSchema rather than silently auto-migrating.
	var preUserVersion int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&preUserVersion); err != nil {
		return fmt.Errorf("bootstrap sqlite store: read user_version: %w", err)
	}
	if preUserVersion == 0 {
		legacy, err := hasAnyCoreTable(ctx, conn)
		if err != nil {
			return err
		}
		if legacy {
			return fmt.Errorf(
				"bootstrap sqlite store: unsupported schema: got user_version=0 with pre-existing core tables, supported window [%d, %d]: %w",
				minSupportedSchemaVersion, maxSupportedSchemaVersion, domain.ErrUnsupportedSchema,
			)
		}
	}

	statements := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			root TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS profiles (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			name TEXT NOT NULL,
			home_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(provider, name)
		);`,
		`CREATE TABLE IF NOT EXISTS project_bindings (
			project_id TEXT NOT NULL,
			profile_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			created_at TEXT NOT NULL,
			modified_at TEXT NOT NULL,
			PRIMARY KEY (project_id, provider),
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS runtimes (
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
		`CREATE INDEX IF NOT EXISTS idx_runtimes_project_id ON runtimes(project_id);`,
		`CREATE TABLE IF NOT EXISTS provider_images (
			provider TEXT PRIMARY KEY,
			latest_version TEXT NOT NULL,
			latest_checked_at TEXT NOT NULL,
			installed_version TEXT NOT NULL,
			installed_image_ref TEXT NOT NULL,
			installed_version_tag TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
	}

	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("bootstrap sqlite store: exec statement: %w", err)
		}
	}

	return s.migrateSchemaOnConn(ctx, conn)
}

// hasAnyCoreTable reports whether any of the DROP_14-era core tables are
// present in sqlite_master. Used during pre-DDL legacy detection.
func hasAnyCoreTable(ctx context.Context, conn *sql.Conn) (bool, error) {
	for _, name := range coreSchemaTables {
		var count int
		if err := conn.QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			name,
		).Scan(&count); err != nil {
			return false, fmt.Errorf("bootstrap sqlite store: probe legacy table %q: %w", name, err)
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

// migrateSchemaOnConn advances the schema forward through the supported
// version window on the supplied connection.
//
// Migrations run inside an explicit BEGIN IMMEDIATE block so that a second
// process opening the same database during first-time bootstrap blocks on
// the reserved lock instead of seeing SQLITE_BUSY. The DSN-level
// busy_timeout(5000) configured in open.go absorbs the wait while the
// first writer commits; once the loser proceeds it observes the final
// user_version and takes the no-op path.
//
// Pre-DDL legacy v0 detection lives in Bootstrap (hasAnyCoreTable). By the
// time migrateSchemaOnConn runs, user_version = 0 always means "fresh
// database" and is stamped straight to v2. v1 databases upgrade to v2.
// Versions above maxSupportedSchemaVersion are rejected with
// ErrUnsupportedSchema.
func (s *Store) migrateSchemaOnConn(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("bootstrap sqlite store: migrate schema: begin immediate: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		}
	}()

	var userVersion int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
		return fmt.Errorf("bootstrap sqlite store: read user_version: %w", err)
	}

	if userVersion > maxSupportedSchemaVersion {
		return fmt.Errorf(
			"bootstrap sqlite store: unsupported schema: got user_version=%d, supported window [%d, %d]: %w",
			userVersion, minSupportedSchemaVersion, maxSupportedSchemaVersion, domain.ErrUnsupportedSchema,
		)
	}

	// Fresh database (user_version = 0 + no pre-existing core tables — the
	// legacy-v0 case was already rejected in Bootstrap) or v1 database both
	// need the account_env table. CREATE TABLE IF NOT EXISTS makes the
	// statement idempotent across both starting points.
	if userVersion < 2 {
		if _, err := conn.ExecContext(ctx, accountEnvCreateTable); err != nil {
			return fmt.Errorf("bootstrap sqlite store: create account_env: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `PRAGMA user_version = 2`); err != nil {
			return fmt.Errorf("bootstrap sqlite store: set user_version=2: %w", err)
		}
	}

	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return fmt.Errorf("bootstrap sqlite store: migrate schema: commit: %w", err)
	}
	committed = true
	return nil
}

func (s *Store) CreateProject(ctx context.Context, project domain.Project) (domain.Project, error) {
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO projects (id, root, name, created_at) VALUES (?, ?, ?, ?)`,
		project.ID,
		project.Root,
		project.Name,
		formatTime(project.CreatedAt),
	); err != nil {
		return domain.Project{}, fmt.Errorf("create project %q: %w", project.Root, err)
	}
	return project, nil
}

func (s *Store) ProjectByRoot(ctx context.Context, root string) (domain.Project, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, root, name, created_at FROM projects WHERE root = ?`, root)
	var project domain.Project
	var createdAt string
	if err := row.Scan(&project.ID, &project.Root, &project.Name, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, fmt.Errorf("project by root %q: %w", root, domain.ErrNotFound)
		}
		return domain.Project{}, fmt.Errorf("project by root %q: %w", root, err)
	}
	parsedCreatedAt, err := parseTime("project by root", "created_at", createdAt)
	if err != nil {
		return domain.Project{}, err
	}
	project.CreatedAt = parsedCreatedAt
	return project, nil
}

func (s *Store) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, root, name, created_at FROM projects ORDER BY root ASC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		var project domain.Project
		var createdAt string
		if err := rows.Scan(&project.ID, &project.Root, &project.Name, &createdAt); err != nil {
			return nil, fmt.Errorf("list projects: scan row: %w", err)
		}
		parsedCreatedAt, err := parseTime("list projects", "created_at", createdAt)
		if err != nil {
			return nil, err
		}
		project.CreatedAt = parsedCreatedAt
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projects, nil
}

func (s *Store) CreateProfile(ctx context.Context, profile domain.Profile) (domain.Profile, error) {
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO profiles (id, provider, name, home_path, created_at) VALUES (?, ?, ?, ?, ?)`,
		profile.ID,
		string(profile.Provider),
		profile.Name,
		profile.HomePath,
		formatTime(profile.CreatedAt),
	); err != nil {
		return domain.Profile{}, fmt.Errorf("create profile %q: %w", profile.Name, err)
	}
	return profile, nil
}

func (s *Store) ProfileByID(ctx context.Context, id string) (domain.Profile, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, provider, name, home_path, created_at FROM profiles WHERE id = ?`,
		id,
	)
	var profile domain.Profile
	var providerValue string
	var createdAt string
	if err := row.Scan(&profile.ID, &providerValue, &profile.Name, &profile.HomePath, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Profile{}, fmt.Errorf("profile by id %q: %w", id, domain.ErrNotFound)
		}
		return domain.Profile{}, fmt.Errorf("profile by id %q: %w", id, err)
	}
	profile.Provider = domain.Provider(providerValue)
	parsedCreatedAt, err := parseTime("profile by id", "created_at", createdAt)
	if err != nil {
		return domain.Profile{}, err
	}
	profile.CreatedAt = parsedCreatedAt
	return profile, nil
}

func (s *Store) ProfileByName(ctx context.Context, provider domain.Provider, name string) (domain.Profile, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, provider, name, home_path, created_at FROM profiles WHERE provider = ? AND name = ?`,
		string(provider),
		name,
	)
	var profile domain.Profile
	var providerValue string
	var createdAt string
	if err := row.Scan(&profile.ID, &providerValue, &profile.Name, &profile.HomePath, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Profile{}, fmt.Errorf("profile by name %q/%q: %w", provider, name, domain.ErrNotFound)
		}
		return domain.Profile{}, fmt.Errorf("profile by name %q/%q: %w", provider, name, err)
	}
	profile.Provider = domain.Provider(providerValue)
	parsedCreatedAt, err := parseTime("profile by name", "created_at", createdAt)
	if err != nil {
		return domain.Profile{}, err
	}
	profile.CreatedAt = parsedCreatedAt
	return profile, nil
}

func (s *Store) ListProfilesByProvider(ctx context.Context, provider domain.Provider) ([]domain.Profile, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, provider, name, home_path, created_at FROM profiles WHERE provider = ? ORDER BY name ASC`,
		string(provider),
	)
	if err != nil {
		return nil, fmt.Errorf("list profiles by provider %q: %w", provider, err)
	}
	defer rows.Close()

	var profiles []domain.Profile
	for rows.Next() {
		var profile domain.Profile
		var providerValue string
		var createdAt string
		if err := rows.Scan(&profile.ID, &providerValue, &profile.Name, &profile.HomePath, &createdAt); err != nil {
			return nil, fmt.Errorf("list profiles by provider %q: scan row: %w", provider, err)
		}
		profile.Provider = domain.Provider(providerValue)
		parsedCreatedAt, err := parseTime("list profiles by provider", "created_at", createdAt)
		if err != nil {
			return nil, err
		}
		profile.CreatedAt = parsedCreatedAt
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list profiles by provider %q: %w", provider, err)
	}
	return profiles, nil
}

func (s *Store) UpdateProfileName(ctx context.Context, provider domain.Provider, currentName, newName string) (domain.Profile, error) {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE profiles SET name = ? WHERE provider = ? AND name = ?`,
		newName,
		string(provider),
		currentName,
	)
	if err != nil {
		return domain.Profile{}, fmt.Errorf("update profile name %q/%q->%q: %w", provider, currentName, newName, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.Profile{}, fmt.Errorf("update profile name %q/%q->%q: rows affected: %w", provider, currentName, newName, err)
	}
	if affected == 0 {
		return domain.Profile{}, fmt.Errorf("update profile name %q/%q->%q: %w", provider, currentName, newName, domain.ErrNotFound)
	}
	return s.ProfileByName(ctx, provider, newName)
}

func (s *Store) DeleteProfile(ctx context.Context, provider domain.Provider, name string) error {
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM profiles WHERE provider = ? AND name = ?`,
		string(provider),
		name,
	)
	if err != nil {
		return fmt.Errorf("delete profile %q/%q: %w", provider, name, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete profile %q/%q: rows affected: %w", provider, name, err)
	}
	if affected == 0 {
		return fmt.Errorf("delete profile %q/%q: %w", provider, name, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) UpsertProjectBinding(ctx context.Context, binding domain.ProjectBinding) (domain.ProjectBinding, error) {
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO project_bindings (project_id, profile_id, provider, created_at, modified_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, provider) DO UPDATE SET
		   profile_id = excluded.profile_id,
		   modified_at = excluded.modified_at`,
		binding.ProjectID,
		binding.ProfileID,
		string(binding.Provider),
		formatTime(binding.CreatedAt),
		formatTime(binding.ModifiedAt),
	); err != nil {
		return domain.ProjectBinding{}, fmt.Errorf("upsert project binding %q: %w", binding.ProjectID, err)
	}
	return binding, nil
}

func (s *Store) BindingByProjectID(ctx context.Context, projectID string, provider domain.Provider) (domain.ProjectBinding, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT project_id, profile_id, provider, created_at, modified_at FROM project_bindings WHERE project_id = ? AND provider = ?`,
		projectID,
		string(provider),
	)
	var binding domain.ProjectBinding
	var providerValue string
	var createdAt string
	var modifiedAt string
	if err := row.Scan(&binding.ProjectID, &binding.ProfileID, &providerValue, &createdAt, &modifiedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ProjectBinding{}, fmt.Errorf("binding by project id %q: %w", projectID, domain.ErrNotFound)
		}
		return domain.ProjectBinding{}, fmt.Errorf("binding by project id %q: %w", projectID, err)
	}
	binding.Provider = domain.Provider(providerValue)
	parsedCreatedAt, err := parseTime("binding by project id", "created_at", createdAt)
	if err != nil {
		return domain.ProjectBinding{}, err
	}
	parsedModifiedAt, err := parseTime("binding by project id", "modified_at", modifiedAt)
	if err != nil {
		return domain.ProjectBinding{}, err
	}
	binding.CreatedAt = parsedCreatedAt
	binding.ModifiedAt = parsedModifiedAt
	return binding, nil
}

func (s *Store) ListBindings(ctx context.Context) ([]domain.ProjectBinding, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT project_id, profile_id, provider, created_at, modified_at
		 FROM project_bindings
		 ORDER BY modified_at DESC, project_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	defer rows.Close()

	var bindings []domain.ProjectBinding
	for rows.Next() {
		var binding domain.ProjectBinding
		var providerValue string
		var createdAt string
		var modifiedAt string
		if err := rows.Scan(&binding.ProjectID, &binding.ProfileID, &providerValue, &createdAt, &modifiedAt); err != nil {
			return nil, fmt.Errorf("list bindings: scan row: %w", err)
		}
		binding.Provider = domain.Provider(providerValue)
		parsedCreatedAt, err := parseTime("list bindings", "created_at", createdAt)
		if err != nil {
			return nil, err
		}
		parsedModifiedAt, err := parseTime("list bindings", "modified_at", modifiedAt)
		if err != nil {
			return nil, err
		}
		binding.CreatedAt = parsedCreatedAt
		binding.ModifiedAt = parsedModifiedAt
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	return bindings, nil
}

func (s *Store) DeleteBinding(ctx context.Context, projectID string, provider domain.Provider) error {
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM project_bindings WHERE project_id = ? AND provider = ?`,
		projectID,
		string(provider),
	)
	if err != nil {
		return fmt.Errorf("delete binding %q/%q: %w", projectID, provider, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete binding %q/%q: rows affected: %w", projectID, provider, err)
	}
	if affected == 0 {
		return fmt.Errorf("delete binding %q/%q: %w", projectID, provider, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) UpsertRuntime(ctx context.Context, runtime domain.RuntimeRecord) (domain.RuntimeRecord, error) {
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO runtimes (id, provider, project_id, profile_id, mode, container_id, image_ref, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   container_id = excluded.container_id,
		   image_ref = excluded.image_ref,
		   status = excluded.status,
		   updated_at = excluded.updated_at`,
		runtime.ID,
		string(runtime.Provider),
		runtime.ProjectID,
		runtime.ProfileID,
		string(runtime.Mode),
		runtime.ContainerID,
		runtime.ImageRef,
		runtime.Status,
		formatTime(runtime.CreatedAt),
		formatTime(runtime.UpdatedAt),
	); err != nil {
		return domain.RuntimeRecord{}, fmt.Errorf("upsert runtime %q: %w", runtime.ID, err)
	}
	return runtime, nil
}

func (s *Store) RuntimeByID(ctx context.Context, id string) (domain.RuntimeRecord, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, provider, project_id, profile_id, mode, container_id, image_ref, status, created_at, updated_at
		 FROM runtimes WHERE id = ?`,
		id,
	)
	return scanRuntime(row, id)
}

func (s *Store) ListRuntimesByProjectID(ctx context.Context, projectID string) ([]domain.RuntimeRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, provider, project_id, profile_id, mode, container_id, image_ref, status, created_at, updated_at
		 FROM runtimes WHERE project_id = ? ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list runtimes by project id %q: %w", projectID, err)
	}
	defer rows.Close()

	var records []domain.RuntimeRecord
	for rows.Next() {
		record, err := scanRuntime(rows, projectID)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runtimes by project id %q: %w", projectID, err)
	}
	return records, nil
}

func (s *Store) ProviderImageState(ctx context.Context, provider domain.Provider) (domain.ProviderImageState, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT provider, latest_version, latest_checked_at, installed_version, installed_image_ref, installed_version_tag, updated_at
		 FROM provider_images WHERE provider = ?`,
		string(provider),
	)
	var state domain.ProviderImageState
	var providerValue string
	var latestCheckedAt string
	var updatedAt string
	if err := row.Scan(
		&providerValue,
		&state.LatestVersion,
		&latestCheckedAt,
		&state.InstalledVersion,
		&state.InstalledImageRef,
		&state.InstalledVersionTag,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ProviderImageState{}, fmt.Errorf("provider image state %q: %w", provider, domain.ErrNotFound)
		}
		return domain.ProviderImageState{}, fmt.Errorf("provider image state %q: %w", provider, err)
	}
	parsedLatestCheckedAt, err := parseTime("provider image state", "latest_checked_at", latestCheckedAt)
	if err != nil {
		return domain.ProviderImageState{}, err
	}
	parsedUpdatedAt, err := parseTime("provider image state", "updated_at", updatedAt)
	if err != nil {
		return domain.ProviderImageState{}, err
	}
	state.Provider = domain.Provider(providerValue)
	state.LatestCheckedAt = parsedLatestCheckedAt
	state.UpdatedAt = parsedUpdatedAt
	return state, nil
}

func (s *Store) UpsertProviderImageState(ctx context.Context, state domain.ProviderImageState) (domain.ProviderImageState, error) {
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO provider_images (provider, latest_version, latest_checked_at, installed_version, installed_image_ref, installed_version_tag, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(provider) DO UPDATE SET
		   latest_version = excluded.latest_version,
		   latest_checked_at = excluded.latest_checked_at,
		   installed_version = excluded.installed_version,
		   installed_image_ref = excluded.installed_image_ref,
		   installed_version_tag = excluded.installed_version_tag,
		   updated_at = excluded.updated_at`,
		string(state.Provider),
		state.LatestVersion,
		formatTime(state.LatestCheckedAt),
		state.InstalledVersion,
		state.InstalledImageRef,
		state.InstalledVersionTag,
		formatTime(state.UpdatedAt),
	); err != nil {
		return domain.ProviderImageState{}, fmt.Errorf("upsert provider image state %q: %w", state.Provider, err)
	}
	return state, nil
}

type runtimeScanner interface {
	Scan(dest ...any) error
}

func scanRuntime(scanner runtimeScanner, descriptor string) (domain.RuntimeRecord, error) {
	var record domain.RuntimeRecord
	var providerValue string
	var modeValue string
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&record.ID,
		&providerValue,
		&record.ProjectID,
		&record.ProfileID,
		&modeValue,
		&record.ContainerID,
		&record.ImageRef,
		&record.Status,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RuntimeRecord{}, fmt.Errorf("runtime scan %q: %w", descriptor, domain.ErrNotFound)
		}
		return domain.RuntimeRecord{}, fmt.Errorf("runtime scan %q: %w", descriptor, err)
	}
	record.Provider = domain.Provider(providerValue)
	record.Mode = domain.Mode(modeValue)
	var err error
	record.CreatedAt, err = parseTime(fmt.Sprintf("runtime scan %q", descriptor), "created_at", createdAt)
	if err != nil {
		return domain.RuntimeRecord{}, err
	}
	record.UpdatedAt, err = parseTime(fmt.Sprintf("runtime scan %q", descriptor), "updated_at", updatedAt)
	if err != nil {
		return domain.RuntimeRecord{}, err
	}
	return record, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(operation, field, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: parse %s %q: %w", operation, field, value, err)
	}
	return parsed, nil
}
