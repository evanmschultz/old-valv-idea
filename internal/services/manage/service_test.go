package manage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	sqliteadapter "github.com/evanmschultz/valv/internal/adapters/sqlite"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/pathutil"
	projectdetect "github.com/evanmschultz/valv/internal/project"
)

func TestCreateProfileUsesDefaultHomeAndPersists(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{Store: store, ProviderRoot: providerRoot})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	profile, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "dev", "")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	wantHome, err := pathutil.Normalize(filepath.Join(providerRoot, "codex", "profiles", "dev"))
	if err != nil {
		t.Fatalf("Normalize(wantHome) error = %v", err)
	}
	if profile.HomePath != wantHome {
		t.Fatalf("CreateProfile().HomePath = %q, want %q", profile.HomePath, wantHome)
	}

	stored, err := store.ProfileByName(context.Background(), domain.ProviderCodex, "dev")
	if err != nil {
		t.Fatalf("ProfileByName() error = %v", err)
	}
	if stored.ID != profile.ID {
		t.Fatalf("stored profile id = %q, want %q", stored.ID, profile.ID)
	}
}

func TestNewRequiresStoreAndProviderRoot(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatal("New() error = nil, want missing dependency failure")
	}

	store, providerRoot := testStore(t)
	if _, err := New(Options{Store: store, ProviderRoot: providerRoot}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
}

func TestBindProjectCreatesMissingProjectAndBinding(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{
		Store:        store,
		ProviderRoot: providerRoot,
		Detect: func(string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: "/tmp/example/project", HasGitMarker: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	profile, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "dev", "/tmp/example/profile")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	result, err := service.BindProject(context.Background(), domain.ProviderCodex, profile.Name, "/tmp/example/project")
	if err != nil {
		t.Fatalf("BindProject() error = %v", err)
	}
	if result.Project.Root != "/tmp/example/project" {
		t.Fatalf("BindProject().Project.Root = %q", result.Project.Root)
	}
	if result.Profile.ID != profile.ID {
		t.Fatalf("BindProject().Profile.ID = %q, want %q", result.Profile.ID, profile.ID)
	}

	storedProject, err := store.ProjectByRoot(context.Background(), "/tmp/example/project")
	if err != nil {
		t.Fatalf("ProjectByRoot() error = %v", err)
	}
	binding, err := store.BindingByProjectID(context.Background(), storedProject.ID)
	if err != nil {
		t.Fatalf("BindingByProjectID() error = %v", err)
	}
	if binding.ProfileID != profile.ID {
		t.Fatalf("binding profile id = %q, want %q", binding.ProfileID, profile.ID)
	}
}

func TestStatusReturnsBoundProjectDetails(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{
		Store:        store,
		ProviderRoot: providerRoot,
		Detect: func(string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: "/tmp/example/project", HasGitMarker: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	profile, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "dev", "/tmp/example/profile")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if _, err := service.BindProject(context.Background(), domain.ProviderCodex, profile.Name, "/tmp/example/project"); err != nil {
		t.Fatalf("BindProject() error = %v", err)
	}

	status, err := service.Status(context.Background(), "/tmp/example/project")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Project.Root != "/tmp/example/project" {
		t.Fatalf("Status().Project.Root = %q", status.Project.Root)
	}
	if status.Profile.Name != profile.Name {
		t.Fatalf("Status().Profile.Name = %q, want %q", status.Profile.Name, profile.Name)
	}
}

func TestBindProjectReturnsProfileLookupErrorWhenMissing(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{
		Store:        store,
		ProviderRoot: providerRoot,
		Detect: func(string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: "/tmp/example/project", HasGitMarker: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = service.BindProject(context.Background(), domain.ProviderCodex, "missing", "/tmp/example/project")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("BindProject() error = %v, want domain.ErrNotFound", err)
	}
}

func TestStatusReturnsUnboundProjectWhenBindingMissing(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{
		Store:        store,
		ProviderRoot: providerRoot,
		Detect: func(string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: "/tmp/example/project", HasGitMarker: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	project, err := domain.NewProject("/tmp/example/project")
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	if _, err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	_, err = service.Status(context.Background(), "/tmp/example/project")
	if !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("Status() error = %v, want domain.ErrUnboundProject", err)
	}
}

func TestStatusReturnsUnboundProjectWhenMissing(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{
		Store:        store,
		ProviderRoot: providerRoot,
		Detect: func(string) (projectdetect.Result, error) {
			return projectdetect.Result{Root: "/tmp/example/project"}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = service.Status(context.Background(), "/tmp/example/project")
	if !errors.Is(err, domain.ErrUnboundProject) {
		t.Fatalf("Status() error = %v, want domain.ErrUnboundProject", err)
	}
}

func testStore(t *testing.T) (*sqliteadapter.Store, string) {
	t.Helper()

	root := t.TempDir()
	store, err := sqliteadapter.NewStore(filepath.Join(root, "valv.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	return store, filepath.Join(root, "providers")
}
