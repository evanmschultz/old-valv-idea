package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	argv := os.Args
	defer func() { os.Args = argv }()
	os.Args = []string{"valv", "version", "--format", "plain"}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRealMain(t *testing.T) {
	argv := os.Args
	defer func() { os.Args = argv }()
	os.Args = []string{"valv", "version", "--format", "plain"}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if got := realMain(context.Background(), &stdout, &stderr); got != 0 {
		t.Fatalf("realMain() = %d, want 0", got)
	}
}

func TestRunReturnsErrorOnInvalidArgs(t *testing.T) {
	argv := os.Args
	defer func() { os.Args = argv }()
	os.Args = []string{"valv", "version", "--format", "yaml"}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run(context.Background(), &stdout, &stderr); err == nil {
		t.Fatal("expected run() error")
	}
}

func TestRealMainReturnsFailureCode(t *testing.T) {
	argv := os.Args
	defer func() { os.Args = argv }()
	os.Args = []string{"valv", "version", "--format", "yaml"}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if got := realMain(context.Background(), &stdout, &stderr); got != 1 {
		t.Fatalf("realMain() = %d, want 1", got)
	}
	if count := strings.Count(stderr.String(), "unsupported value"); count != 1 {
		t.Fatalf("stderr occurrence count = %d, want 1; stderr=%q", count, stderr.String())
	}
}

func TestRunUsesTestHomeOverride(t *testing.T) {
	argv := os.Args
	defer func() { os.Args = argv }()
	os.Args = []string{"valv", "paths", "--format", "plain"}
	home := t.TempDir()
	t.Setenv("VALV_TEST_HOME_DIR", home)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "database="+filepath.Join(home, "Library", "Application Support", "valv", "db", "valv.sqlite3")) {
		t.Fatalf("stdout = %q, want override database path", stdout.String())
	}
}
