package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/creack/pty/v2"
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

func TestQuietRunnerSuppressesSuccessOutput(t *testing.T) {
	t.Parallel()

	runner := NewQuietRunner(os.Args[0], nil)
	runner.Env = []string{"GO_WANT_HELPER_PROCESS=1"}

	if err := runner.Run(context.Background(), []string{"-test.run=TestSystemRunnerHelperProcess", "--", "hello", "world"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestQuietRunnerReturnsHelpfulBuildxFailure(t *testing.T) {
	t.Parallel()

	runner := NewQuietRunner(os.Args[0], nil)
	runner.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_FAIL=1"}

	err := runner.Run(context.Background(), []string{"buildx", "version"})
	if err == nil || !strings.Contains(err.Error(), "docker buildx is required but unavailable") {
		t.Fatalf("Run() error = %v, want buildx guidance", err)
	}
}

func TestSharedTTYFileDetectsSharedPTY(t *testing.T) {
	t.Parallel()

	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open() error = %v", err)
	}
	defer master.Close()
	defer slave.Close()

	stdoutFD, err := syscall.Dup(int(slave.Fd()))
	if err != nil {
		t.Fatalf("syscall.Dup(stdout) error = %v", err)
	}
	stderrFD, err := syscall.Dup(int(slave.Fd()))
	if err != nil {
		t.Fatalf("syscall.Dup(stderr) error = %v", err)
	}
	stdoutFile := os.NewFile(uintptr(stdoutFD), "stdout-tty")
	stderrFile := os.NewFile(uintptr(stderrFD), "stderr-tty")
	defer stdoutFile.Close()
	defer stderrFile.Close()

	file, ok := sharedTTYFile(slave, stdoutFile, stderrFile)
	if !ok {
		t.Fatal("sharedTTYFile() = not ok, want shared PTY detection")
	}
	if file != slave {
		t.Fatalf("sharedTTYFile() file = %v, want slave", file)
	}
}
