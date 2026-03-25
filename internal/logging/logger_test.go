package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger, err := New(Options{Writer: &buf, Level: "debug", Prefix: "valv"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Debug("hello", "component", "test")
	if buf.Len() == 0 {
		t.Fatal("expected log output")
	}
}

func TestNewRejectsInvalidLevel(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if _, err := New(Options{Writer: &buf, Level: "loud"}); err == nil {
		t.Fatal("expected invalid level error")
	}
}

func TestNewDefaultsToInfoLevel(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger, err := New(Options{Writer: &buf})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Info("hello")
	if buf.Len() == 0 {
		t.Fatal("expected info log output")
	}
}

func TestOpenFileCreatesLogPath(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "logs")
	file, err := OpenFile(dir, "valv.log")
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	defer file.Close()

	if _, err := file.WriteString("hello\n"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "valv.log"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(content, []byte("hello")) {
		t.Fatalf("unexpected log file content: %q", content)
	}
}
