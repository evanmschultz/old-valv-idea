package sqlite

import (
	"net/url"
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

func TestBuildDSNAddsForeignKeyPragmaForURI(t *testing.T) {
	t.Parallel()

	dsn, err := buildDSN(OpenOptions{URI: "file:valv-test?mode=memory&cache=shared"})
	if err != nil {
		t.Fatalf("buildDSN() error = %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	if got := parsed.Query()["_pragma"]; len(got) != 1 || got[0] != "foreign_keys(1)" {
		t.Fatalf("buildDSN() _pragma = %#v, want []string{\"foreign_keys(1)\"}", got)
	}
}

func TestBuildDSNDoesNotDuplicateForeignKeyPragma(t *testing.T) {
	t.Parallel()

	dsn, err := buildDSN(OpenOptions{URI: "file:valv-test?mode=memory&_pragma=foreign_keys(1)"})
	if err != nil {
		t.Fatalf("buildDSN() error = %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	if got := parsed.Query()["_pragma"]; len(got) != 1 {
		t.Fatalf("buildDSN() _pragma count = %d, want 1", len(got))
	}
}
