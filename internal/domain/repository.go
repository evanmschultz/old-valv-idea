package domain

import "context"

type ProjectRepository interface {
	CreateProject(context.Context, Project) (Project, error)
	ProjectByRoot(context.Context, string) (Project, error)
	ListProjects(context.Context) ([]Project, error)
}

type ProfileRepository interface {
	CreateProfile(context.Context, Profile) (Profile, error)
	ProfileByID(context.Context, string) (Profile, error)
	ProfileByName(context.Context, Provider, string) (Profile, error)
	ListProfilesByProvider(context.Context, Provider) ([]Profile, error)
	UpdateProfileName(context.Context, Provider, string, string) (Profile, error)
	DeleteProfile(context.Context, Provider, string) error
}

type BindingRepository interface {
	UpsertProjectBinding(context.Context, ProjectBinding) (ProjectBinding, error)
	BindingByProjectID(context.Context, string, Provider) (ProjectBinding, error)
	ListBindings(context.Context) ([]ProjectBinding, error)
	DeleteBinding(ctx context.Context, projectID string, provider Provider) error
}

type RuntimeRepository interface {
	UpsertRuntime(context.Context, RuntimeRecord) (RuntimeRecord, error)
	RuntimeByID(context.Context, string) (RuntimeRecord, error)
	ListRuntimesByProjectID(context.Context, string) ([]RuntimeRecord, error)
}

type ProviderImageRepository interface {
	ProviderImageState(context.Context, Provider) (ProviderImageState, error)
	UpsertProviderImageState(context.Context, ProviderImageState) (ProviderImageState, error)
}

// AccountEnvRepository persists per-account env-var entries keyed by
// (profile_id, env_key). Duplicate env_key values across different profiles
// are intentional — the same env-var name may legitimately resolve to
// different values for different accounts. Account renames are stable
// because ownership is keyed by Profile.ID, which does not change on rename.
type AccountEnvRepository interface {
	// SetAccountEnv inserts or replaces the env value for (profileID, envKey).
	// Returns the persisted entry on success.
	SetAccountEnv(ctx context.Context, profileID, envKey, envValue string) (AccountEnvEntry, error)
	// GetAccountEnv returns the entry for (profileID, envKey), or wraps
	// ErrNotFound when no row exists.
	GetAccountEnv(ctx context.Context, profileID, envKey string) (AccountEnvEntry, error)
	// ListAccountEnv returns every env entry for profileID, sorted by env_key
	// ascending so callers get deterministic output without re-sorting.
	ListAccountEnv(ctx context.Context, profileID string) ([]AccountEnvEntry, error)
	// UnsetAccountEnv removes the entry for (profileID, envKey). Wraps
	// ErrNotFound when no matching row exists.
	UnsetAccountEnv(ctx context.Context, profileID, envKey string) error
}
