package docker

import (
	"reflect"
	"testing"
)

func TestImageRefString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  ImageRef
		want string
	}{
		{name: "empty", ref: ImageRef{}, want: ""},
		{name: "repository only", ref: ImageRef{Repository: "ghcr.io/valv/codex"}, want: "ghcr.io/valv/codex"},
		{name: "repository tag", ref: ImageRef{Repository: "ghcr.io/valv/codex", Tag: "0.1.0"}, want: "ghcr.io/valv/codex:0.1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.ref.String(); got != tt.want {
				t.Fatalf("ImageRef.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildRunArgs(t *testing.T) {
	t.Parallel()

	req := ContainerRunRequest{
		Name:        "valv-codex-test",
		Image:       NewImageRef("ghcr.io/valv/codex", "0.1.0"),
		WorkingDir:  "/Users/alice/project",
		Env:         map[string]string{"B": "2", "A": "1"},
		Labels:      map[string]string{"io.valv.scope": "interactive", "io.valv.managed": "true"},
		Mounts:      []MountSpec{NewMountSpec("/host/project", "/workspace", true)},
		Args:        []string{"version"},
		Detached:    true,
		Interactive: true,
		TTY:         true,
		Remove:      true,
		User:        "501:20",
		Network:     "none",
		Extra:       []string{"--pull=never"},
	}

	got, err := BuildRunArgs(req)
	if err != nil {
		t.Fatalf("BuildRunArgs() error = %v", err)
	}

	want := []string{
		"run",
		"-d",
		"--rm",
		"-i",
		"-t",
		"--name", "valv-codex-test",
		"--workdir", "/Users/alice/project",
		"--user", "501:20",
		"--network", "none",
		"-e", "A=1",
		"-e", "B=2",
		"--label", "io.valv.managed=true",
		"--label", "io.valv.scope=interactive",
		"--mount", "type=bind,source=/host/project,target=/workspace,readonly",
		"--pull=never",
		"ghcr.io/valv/codex:0.1.0",
		"version",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildRunArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildRunArgsRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	_, err := BuildRunArgs(ContainerRunRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildInspectArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildInspectArgs(" valv-codex-test ")
	if err != nil {
		t.Fatalf("BuildInspectArgs() error = %v", err)
	}
	want := []string{"inspect", "valv-codex-test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildInspectArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildExecArgs(t *testing.T) {
	t.Parallel()

	got, err := BuildExecArgs(ContainerExecRequest{
		ContainerID: "valv-api-runtime",
		WorkingDir:  "/workspace/project",
		Env:         map[string]string{"B": "2", "A": "1"},
		Args:        []string{"codex", "exec", "--json"},
		Interactive: true,
		TTY:         true,
		User:        "501:20",
	})
	if err != nil {
		t.Fatalf("BuildExecArgs() error = %v", err)
	}

	want := []string{
		"exec",
		"-i",
		"-t",
		"--workdir", "/workspace/project",
		"--user", "501:20",
		"-e", "A=1",
		"-e", "B=2",
		"valv-api-runtime",
		"codex", "exec", "--json",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildExecArgs() = %#v, want %#v", got, want)
	}
}
