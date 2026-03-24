package sqlite

import (
	"path/filepath"
	"testing"
)

func TestOpenMemoryDatabase(t *testing.T) {
	t.Parallel()
	db, err := Open(OpenOptions{URI: "file:valv-test?mode=memory&cache=shared"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
}

func TestOpenRequiresPathOrURI(t *testing.T) {
	t.Parallel()
	if _, err := Open(OpenOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenFilePathDatabase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "valv.sqlite3")
	db, err := Open(OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS test_path (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
}

func TestOpenFilePathDatabaseEscapesSpecialCharacters(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "valv?#.sqlite3")
	db, err := Open(OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS test_special (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
}
