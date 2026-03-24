package logging

import (
	"bytes"
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
