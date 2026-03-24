package domain

import "context"

type ProjectRepository interface {
	CreateProject(context.Context, Project) (Project, error)
	ProjectByRoot(context.Context, string) (Project, error)
}

type ProfileRepository interface {
	CreateProfile(context.Context, Profile) (Profile, error)
	ProfileByName(context.Context, Provider, string) (Profile, error)
}

type BindingRepository interface {
	UpsertProjectBinding(context.Context, ProjectBinding) (ProjectBinding, error)
	BindingByProjectID(context.Context, string) (ProjectBinding, error)
}

type RuntimeRepository interface {
	UpsertRuntime(context.Context, RuntimeRecord) (RuntimeRecord, error)
	RuntimeByID(context.Context, string) (RuntimeRecord, error)
	ListRuntimesByProjectID(context.Context, string) ([]RuntimeRecord, error)
}
