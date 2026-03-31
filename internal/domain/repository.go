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
	BindingByProjectID(context.Context, string) (ProjectBinding, error)
	ListBindings(context.Context) ([]ProjectBinding, error)
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
