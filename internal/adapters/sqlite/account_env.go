package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/evanmschultz/valv/internal/domain"
)

// accountEnvCreateTable is the v2 DDL for the per-account env-var map.
// Ownership is by profile_id (not by account name) so duplicate env_key
// values across accounts are legal and account renames remain stable.
// env_value is plaintext at v0.1 of DROP_14; keychain encryption is
// deferred.
const accountEnvCreateTable = `CREATE TABLE IF NOT EXISTS account_env (
	profile_id TEXT NOT NULL,
	env_key TEXT NOT NULL,
	env_value TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (profile_id, env_key),
	FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);`

// SetAccountEnv inserts or replaces the env value for (profileID, envKey).
// On insert, created_at and updated_at are both set to now-UTC. On replace,
// created_at is preserved and only updated_at advances.
func (s *Store) SetAccountEnv(ctx context.Context, profileID, envKey, envValue string) (domain.AccountEnvEntry, error) {
	now := time.Now().UTC()
	nowStr := formatTime(now)

	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO account_env (profile_id, env_key, env_value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(profile_id, env_key) DO UPDATE SET
		   env_value = excluded.env_value,
		   updated_at = excluded.updated_at`,
		profileID,
		envKey,
		envValue,
		nowStr,
		nowStr,
	); err != nil {
		return domain.AccountEnvEntry{}, fmt.Errorf("set account env %q/%q: %w", profileID, envKey, err)
	}

	// Re-read so created_at reflects the persisted row (which differs from
	// nowStr when the entry was upserted-not-inserted).
	return s.GetAccountEnv(ctx, profileID, envKey)
}

// GetAccountEnv returns the entry for (profileID, envKey), wrapping
// domain.ErrNotFound when no row matches.
func (s *Store) GetAccountEnv(ctx context.Context, profileID, envKey string) (domain.AccountEnvEntry, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT profile_id, env_key, env_value, created_at, updated_at
		 FROM account_env WHERE profile_id = ? AND env_key = ?`,
		profileID,
		envKey,
	)
	entry, err := scanAccountEnv(row, fmt.Sprintf("%s/%s", profileID, envKey))
	if err != nil {
		return domain.AccountEnvEntry{}, err
	}
	return entry, nil
}

// ListAccountEnv returns every env entry for profileID, ordered by env_key
// ascending so callers do not need to re-sort for deterministic output.
func (s *Store) ListAccountEnv(ctx context.Context, profileID string) ([]domain.AccountEnvEntry, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT profile_id, env_key, env_value, created_at, updated_at
		 FROM account_env WHERE profile_id = ? ORDER BY env_key ASC`,
		profileID,
	)
	if err != nil {
		return nil, fmt.Errorf("list account env %q: %w", profileID, err)
	}
	defer rows.Close()

	var entries []domain.AccountEnvEntry
	for rows.Next() {
		entry, err := scanAccountEnv(rows, profileID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list account env %q: %w", profileID, err)
	}
	return entries, nil
}

// UnsetAccountEnv removes the entry for (profileID, envKey), wrapping
// domain.ErrNotFound when no row matches.
func (s *Store) UnsetAccountEnv(ctx context.Context, profileID, envKey string) error {
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM account_env WHERE profile_id = ? AND env_key = ?`,
		profileID,
		envKey,
	)
	if err != nil {
		return fmt.Errorf("unset account env %q/%q: %w", profileID, envKey, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("unset account env %q/%q: rows affected: %w", profileID, envKey, err)
	}
	if affected == 0 {
		return fmt.Errorf("unset account env %q/%q: %w", profileID, envKey, domain.ErrNotFound)
	}
	return nil
}

type accountEnvScanner interface {
	Scan(dest ...any) error
}

func scanAccountEnv(scanner accountEnvScanner, descriptor string) (domain.AccountEnvEntry, error) {
	var entry domain.AccountEnvEntry
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&entry.ProfileID,
		&entry.EnvKey,
		&entry.EnvValue,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AccountEnvEntry{}, fmt.Errorf("account env scan %q: %w", descriptor, domain.ErrNotFound)
		}
		return domain.AccountEnvEntry{}, fmt.Errorf("account env scan %q: %w", descriptor, err)
	}
	parsedCreatedAt, err := parseTime("account env scan", "created_at", createdAt)
	if err != nil {
		return domain.AccountEnvEntry{}, err
	}
	parsedUpdatedAt, err := parseTime("account env scan", "updated_at", updatedAt)
	if err != nil {
		return domain.AccountEnvEntry{}, err
	}
	entry.CreatedAt = parsedCreatedAt
	entry.UpdatedAt = parsedUpdatedAt
	return entry, nil
}
