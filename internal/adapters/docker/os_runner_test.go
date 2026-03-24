package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSystemRunnerRun(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewSystemRunner(os.Args[0], nil, &stdout, &stderr)
	runner.Env = []string{"GO_WANT_HELPER_PROCESS=1"}

	err := runner.Run(context.Background(), []string{"-test.run=TestSystemRunnerHelperProcess", "--", "hello", "world"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "hello world" {
		t.Fatalf("Run() stdout = %q, want %q", got, "hello world")
	}
}

func TestSystemRunnerRunWrapsFailure(t *testing.T) {
	t.Parallel()

	runner := NewSystemRunner(os.Args[0], nil, io.Discard, io.Discard)
	runner.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_FAIL=1"}

	err := runner.Run(context.Background(), []string{"-test.run=TestSystemRunnerHelperProcess", "--", "hello"})
	if err == nil || !strings.Contains(err.Error(), "run "+os.Args[0]) {
		t.Fatalf("Run() error = %v, want wrapped failure", err)
	}
}

func TestSystemRunnerHelperProcess(t *testing.T) {
	t.Helper()
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if os.Getenv("GO_HELPER_FAIL") == "1" {
		fmt.Fprintln(os.Stderr, "forced failure")
		os.Exit(2)
	}

	args := os.Args
	for index, arg := range args {
		if arg == "--" {
			fmt.Fprintln(os.Stdout, strings.Join(args[index+1:], " "))
			os.Exit(0)
		}
	}
	os.Exit(0)
}
