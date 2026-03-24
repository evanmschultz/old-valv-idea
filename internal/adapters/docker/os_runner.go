package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type commandFactory func(context.Context, string, ...string) *exec.Cmd

type SystemRunner struct {
	Binary     string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Env        []string
	newCommand commandFactory
}

func NewSystemRunner(binary string, stdin io.Reader, stdout, stderr io.Writer) SystemRunner {
	return SystemRunner{
		Binary: binary,
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
}

func (r SystemRunner) Run(ctx context.Context, args []string) error {
	binary := strings.TrimSpace(r.Binary)
	if binary == "" {
		binary = "docker"
	}

	factory := r.newCommand
	if factory == nil {
		factory = exec.CommandContext
	}

	cmd := factory(ctx, binary, args...)
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if len(r.Env) > 0 {
		cmd.Env = append(os.Environ(), r.Env...)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s %s: %w", binary, strings.Join(args, " "), err)
	}
	return nil
}
