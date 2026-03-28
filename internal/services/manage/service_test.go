package manage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestCreateProfileReturnsExistingProfileForSameNameAndHome(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{Store: store, ProviderRoot: providerRoot})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "host-codex", "/tmp/example/host")
	if err != nil {
		t.Fatalf("CreateProfile(first) error = %v", err)
	}
	second, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "host-codex", "/tmp/example/host")
	if err != nil {
		t.Fatalf("CreateProfile(second) error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("CreateProfile(second).ID = %q, want %q", second.ID, first.ID)
	}
}

func TestCreateProfileReturnsClearErrorForDifferentExistingHome(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{Store: store, ProviderRoot: providerRoot})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "host-codex", "/tmp/example/host-a"); err != nil {
		t.Fatalf("CreateProfile(first) error = %v", err)
	}
	_, err = service.CreateProfile(context.Background(), domain.ProviderCodex, "host-codex", "/tmp/example/host-b")
	if err == nil || !strings.Contains(err.Error(), "profile already exists with home") {
		t.Fatalf("CreateProfile(second) error = %v, want clear existing-home message", err)
	}
}

func TestCreateProfileReturnsClearErrorForDifferentExistingNameOnSameHome(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{Store: store, ProviderRoot: providerRoot})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "host", "/tmp/example/host"); err != nil {
		t.Fatalf("CreateProfile(first) error = %v", err)
	}
	_, err = service.CreateProfile(context.Background(), domain.ProviderCodex, "default", "/tmp/example/host")
	if err == nil || !strings.Contains(err.Error(), "already belongs to account") {
		t.Fatalf("CreateProfile(second) error = %v, want same-home account guidance", err)
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

func TestDefaultHostProfileUsesConfiguredHomeDir(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{Store: store, ProviderRoot: providerRoot, HomeDir: "/tmp/example-home"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	spec, err := service.DefaultHostProfile(domain.ProviderCodex)
	if err != nil {
		t.Fatalf("DefaultHostProfile() error = %v", err)
	}
	if spec.Name != "default" {
		t.Fatalf("DefaultHostProfile().Name = %q, want default", spec.Name)
	}
	wantHome, err := pathutil.Normalize("/tmp/example-home/.codex")
	if err != nil {
		t.Fatalf("Normalize(wantHome) error = %v", err)
	}
	if spec.HomePath != wantHome {
		t.Fatalf("DefaultHostProfile().HomePath = %q, want %q", spec.HomePath, wantHome)
	}
}

func TestCreateDefaultHostProfileUsesProviderDefaultNameAndHome(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{Store: store, ProviderRoot: providerRoot, HomeDir: "/tmp/example-home"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	profile, err := service.CreateDefaultHostProfile(context.Background(), domain.ProviderCodex)
	if err != nil {
		t.Fatalf("CreateDefaultHostProfile() error = %v", err)
	}
	if profile.Name != "default" {
		t.Fatalf("CreateDefaultHostProfile().Name = %q, want default", profile.Name)
	}
	wantHome, err := pathutil.Normalize("/tmp/example-home/.codex")
	if err != nil {
		t.Fatalf("Normalize(wantHome) error = %v", err)
	}
	if profile.HomePath != wantHome {
		t.Fatalf("CreateDefaultHostProfile().HomePath = %q, want %q", profile.HomePath, wantHome)
	}
}

func TestCreateDefaultHostProfileReusesExistingSameHomeAccount(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{Store: store, ProviderRoot: providerRoot, HomeDir: "/tmp/example-home"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	existing, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "host-codex", "/tmp/example-home/.codex")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	profile, err := service.CreateDefaultHostProfile(context.Background(), domain.ProviderCodex)
	if err != nil {
		t.Fatalf("CreateDefaultHostProfile() error = %v", err)
	}
	if profile.ID != existing.ID {
		t.Fatalf("CreateDefaultHostProfile().ID = %q, want %q", profile.ID, existing.ID)
	}
}

func TestCreateProfileSeedsConfigFromDefaultHostHome(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	homeDir := t.TempDir()
	hostCodexDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(hostCodexDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(hostCodexDir) error = %v", err)
	}
	hostConfig := []byte("model = \"gpt-5.4\"\n")
	if err := os.WriteFile(filepath.Join(hostCodexDir, "config.toml"), hostConfig, 0o600); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	service, err := New(Options{Store: store, ProviderRoot: providerRoot, HomeDir: homeDir})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	profile, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "work", "")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(profile.HomePath, "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile(seed config) error = %v", err)
	}
	if string(content) != string(hostConfig) {
		t.Fatalf("seeded config = %q, want %q", string(content), string(hostConfig))
	}
}

func TestCreateProfileDoesNotOverwriteExistingProfileConfig(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	homeDir := t.TempDir()
	hostCodexDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(hostCodexDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(hostCodexDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(hostCodexDir, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(host config) error = %v", err)
	}

	service, err := New(Options{Store: store, ProviderRoot: providerRoot, HomeDir: homeDir})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	profileHome := filepath.Join(providerRoot, "codex", "profiles", "work")
	if err := os.MkdirAll(profileHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileHome) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileHome, "config.toml"), []byte("model = \"custom\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile config) error = %v", err)
	}

	profile, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "work", "")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(profile.HomePath, "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile(profile config) error = %v", err)
	}
	if string(content) != "model = \"custom\"\n" {
		t.Fatalf("profile config = %q, want custom config preserved", string(content))
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

func TestListProfilesReturnsPersistedProfiles(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	service, err := New(Options{Store: store, ProviderRoot: providerRoot})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "b", "/tmp/example/b"); err != nil {
		t.Fatalf("CreateProfile(b) error = %v", err)
	}
	if _, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "a", "/tmp/example/a"); err != nil {
		t.Fatalf("CreateProfile(a) error = %v", err)
	}

	result, err := service.ListProfiles(context.Background(), domain.ProviderCodex)
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if got, want := len(result.Profiles), 2; got != want {
		t.Fatalf("profiles len = %d, want %d", got, want)
	}
	if result.Profiles[0].Name != "a" || result.Profiles[1].Name != "b" {
		t.Fatalf("profiles order = [%s %s], want [a b]", result.Profiles[0].Name, result.Profiles[1].Name)
	}
}

func TestListProfilesCollapsesSameHomeAliasesForPresentation(t *testing.T) {
	t.Parallel()

	store, providerRoot := testStore(t)
	homeRoot := t.TempDir()
	service, err := New(Options{
		Store:        store,
		ProviderRoot: providerRoot,
		HomeDir:      homeRoot,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	hostHome := filepath.Join(homeRoot, ".codex")
	if _, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "host-codex", hostHome); err != nil {
		t.Fatalf("CreateProfile(host-codex) error = %v", err)
	}
	defaultProfile, err := domain.NewProfile(domain.ProviderCodex, "default", hostHome)
	if err != nil {
		t.Fatalf("NewProfile(default) error = %v", err)
	}
	if _, err := store.CreateProfile(context.Background(), defaultProfile); err != nil {
		t.Fatalf("CreateProfile(default) error = %v", err)
	}
	if _, err := service.CreateProfile(context.Background(), domain.ProviderCodex, "work", filepath.Join(providerRoot, "codex", "profiles", "work")); err != nil {
		t.Fatalf("CreateProfile(work) error = %v", err)
	}

	result, err := service.ListProfiles(context.Background(), domain.ProviderCodex)
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if got, want := len(result.Profiles), 2; got != want {
		t.Fatalf("profiles len = %d, want %d", got, want)
	}
	if result.Profiles[0].Name != "default" || result.Profiles[1].Name != "work" {
		t.Fatalf("profiles order = [%s %s], want [default work]", result.Profiles[0].Name, result.Profiles[1].Name)
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
