package globalswitch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evanmschultz/valv/internal/adapters/sqlite"
	"github.com/evanmschultz/valv/internal/domain"
)

func TestSwitchAndCurrentProfile(t *testing.T) {
	t.Parallel()

	store, err := bootstrappedStore(t)
	if err != nil {
		t.Fatalf("bootstrappedStore() error = %v", err)
	}

	root := t.TempDir()
	profileHome := filepath.Join(root, "providers", "codex", "profiles", "dev")
	profile, err := domain.NewProfile(domain.ProviderCodex, "dev", profileHome)
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	if _, err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	svc, err := New(Options{
		Store:        store,
		ProviderRoot: filepath.Join(root, "providers"),
		StateDir:     filepath.Join(root, "state"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Switch(context.Background(), domain.ProviderCodex, "dev")
	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if result.Profile.ID != profile.ID {
		t.Fatalf("Switch() profile id = %q, want %q", result.Profile.ID, profile.ID)
	}
	if got, err := os.Readlink(result.LinkPath); err != nil {
		t.Fatalf("Readlink() error = %v", err)
	} else if got != profile.HomePath {
		t.Fatalf("Readlink() = %q, want %q", got, profile.HomePath)
	}
	if _, err := os.Stat(result.StatePath); err != nil {
		t.Fatalf("StatePath stat error = %v", err)
	}

	current, err := svc.Current(context.Background(), domain.ProviderCodex)
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if current.Profile.ID != profile.ID {
		t.Fatalf("Current() profile id = %q, want %q", current.Profile.ID, profile.ID)
	}
}

func TestSwitchMissingProfileFails(t *testing.T) {
	t.Parallel()

	store, err := bootstrappedStore(t)
	if err != nil {
		t.Fatalf("bootstrappedStore() error = %v", err)
	}

	root := t.TempDir()
	svc, err := New(Options{
		Store:        store,
		ProviderRoot: filepath.Join(root, "providers"),
		StateDir:     filepath.Join(root, "state"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := svc.Switch(context.Background(), domain.ProviderCodex, "missing"); err == nil {
		t.Fatal("Switch() error = nil, want failure")
	}
}

func bootstrappedStore(t *testing.T) (*sqlite.Store, error) {
	t.Helper()

	name := strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-")
	db, err := sqlite.Open(sqlite.OpenOptions{URI: "file:" + name + "?mode=memory&cache=shared"})
	if err != nil {
		return nil, err
	}
	store := sqlite.NewStoreFromDB(db)
	if err := store.Bootstrap(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, nil
}
