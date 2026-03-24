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

func (s *Store) Bootstrap(ctx context.Context) error {
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
			project_id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			created_at TEXT NOT NULL,
			modified_at TEXT NOT NULL,
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
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("bootstrap sqlite store: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("bootstrap sqlite store: exec statement: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("bootstrap sqlite store: commit tx: %w", err)
	}
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

func (s *Store) UpsertProjectBinding(ctx context.Context, binding domain.ProjectBinding) (domain.ProjectBinding, error) {
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO project_bindings (project_id, profile_id, provider, created_at, modified_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(project_id) DO UPDATE SET
		   profile_id = excluded.profile_id,
		   provider = excluded.provider,
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

func (s *Store) BindingByProjectID(ctx context.Context, projectID string) (domain.ProjectBinding, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT project_id, profile_id, provider, created_at, modified_at FROM project_bindings WHERE project_id = ?`,
		projectID,
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
