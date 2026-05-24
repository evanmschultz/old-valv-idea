package docker

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestBuildNetworkCreateArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request NetworkCreateRequest
		want    []string
		wantErr string
	}{
		{
			name: "internal network with default valv label",
			request: NetworkCreateRequest{
				Name:     "valv-netpolicy-abc123",
				Internal: true,
				Labels:   map[string]string{"valv": "network-policy"},
			},
			want: []string{
				"network", "create",
				"--internal",
				"--label", "valv=network-policy",
				"valv-netpolicy-abc123",
			},
		},
		{
			name: "multiple labels sorted deterministically",
			request: NetworkCreateRequest{
				Name:     "valv-netpolicy-xyz",
				Internal: true,
				Labels: map[string]string{
					"zebra":   "last",
					"alpha":   "first",
					"valv":    "network-policy",
					"project": "demo",
				},
			},
			want: []string{
				"network", "create",
				"--internal",
				"--label", "alpha=first",
				"--label", "project=demo",
				"--label", "valv=network-policy",
				"--label", "zebra=last",
				"valv-netpolicy-xyz",
			},
		},
		{
			name: "internal false omits flag",
			request: NetworkCreateRequest{
				Name:     "valv-open-net",
				Internal: false,
				Labels:   map[string]string{"valv": "network-policy"},
			},
			want: []string{
				"network", "create",
				"--label", "valv=network-policy",
				"valv-open-net",
			},
		},
		{
			name: "no labels emits only required args",
			request: NetworkCreateRequest{
				Name:     "valv-bare",
				Internal: true,
			},
			want: []string{
				"network", "create",
				"--internal",
				"valv-bare",
			},
		},
		{
			name: "name with hyphens dots and underscores is accepted",
			request: NetworkCreateRequest{
				Name:     "valv.netpolicy_test-01",
				Internal: true,
			},
			want: []string{
				"network", "create",
				"--internal",
				"valv.netpolicy_test-01",
			},
		},
		{
			name:    "empty name rejected",
			request: NetworkCreateRequest{Name: "", Internal: true},
			wantErr: "name is required",
		},
		{
			name:    "whitespace-only name rejected",
			request: NetworkCreateRequest{Name: "   ", Internal: true},
			wantErr: "name is required",
		},
		{
			name:    "name with space rejected",
			request: NetworkCreateRequest{Name: "valv net", Internal: true},
			wantErr: "invalid network name",
		},
		{
			name:    "name with slash rejected",
			request: NetworkCreateRequest{Name: "valv/net", Internal: true},
			wantErr: "invalid network name",
		},
		{
			name:    "name starting with hyphen rejected",
			request: NetworkCreateRequest{Name: "-leading-hyphen", Internal: true},
			wantErr: "invalid network name",
		},
		{
			name:    "name starting with dot rejected",
			request: NetworkCreateRequest{Name: ".leading-dot", Internal: true},
			wantErr: "invalid network name",
		},
		{
			name: "label with empty key rejected",
			request: NetworkCreateRequest{
				Name:     "valv-net",
				Internal: true,
				Labels:   map[string]string{"": "value"},
			},
			wantErr: "label key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := BuildNetworkCreateArgs(tt.request)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("BuildNetworkCreateArgs() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("BuildNetworkCreateArgs() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildNetworkCreateArgs() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("BuildNetworkCreateArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestBuildNetworkRemoveArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request NetworkRemoveRequest
		want    []string
		wantErr string
	}{
		{
			name:    "single network",
			request: NetworkRemoveRequest{Name: "valv-netpolicy-abc"},
			want:    []string{"network", "rm", "valv-netpolicy-abc"},
		},
		{
			name:    "name is trimmed",
			request: NetworkRemoveRequest{Name: "  valv-net  "},
			want:    []string{"network", "rm", "valv-net"},
		},
		{
			name:    "empty name rejected",
			request: NetworkRemoveRequest{Name: ""},
			wantErr: "name is required",
		},
		{
			name:    "whitespace name rejected",
			request: NetworkRemoveRequest{Name: "   "},
			wantErr: "name is required",
		},
		{
			name:    "name with space rejected",
			request: NetworkRemoveRequest{Name: "valv net"},
			wantErr: "invalid network name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := BuildNetworkRemoveArgs(tt.request)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("BuildNetworkRemoveArgs() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("BuildNetworkRemoveArgs() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildNetworkRemoveArgs() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("BuildNetworkRemoveArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExecutorCreateNetworkForwardsArgs(t *testing.T) {
	t.Parallel()

	var got []string
	exec := NewExecutor(CommandRunnerFunc(func(_ context.Context, args []string) error {
		got = append([]string(nil), args...)
		return nil
	}))

	if err := exec.CreateNetwork(context.Background(), NetworkCreateRequest{
		Name:     "valv-netpolicy-abc",
		Internal: true,
		Labels:   map[string]string{"valv": "network-policy"},
	}); err != nil {
		t.Fatalf("CreateNetwork() error = %v", err)
	}

	want := []string{
		"network", "create",
		"--internal",
		"--label", "valv=network-policy",
		"valv-netpolicy-abc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CreateNetwork() args = %#v, want %#v", got, want)
	}
}

func TestExecutorCreateNetworkReturnsBuildError(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(CommandRunnerFunc(func(_ context.Context, _ []string) error {
		t.Fatal("runner should not be invoked when build fails")
		return nil
	}))

	err := exec.CreateNetwork(context.Background(), NetworkCreateRequest{Name: ""})
	if err == nil {
		t.Fatalf("CreateNetwork() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("CreateNetwork() error = %q, want substring %q", err.Error(), "name is required")
	}
}

func TestExecutorRemoveNetworkForwardsArgs(t *testing.T) {
	t.Parallel()

	var got []string
	exec := NewExecutor(CommandRunnerFunc(func(_ context.Context, args []string) error {
		got = append([]string(nil), args...)
		return nil
	}))

	if err := exec.RemoveNetwork(context.Background(), NetworkRemoveRequest{Name: "valv-netpolicy-abc"}); err != nil {
		t.Fatalf("RemoveNetwork() error = %v", err)
	}

	want := []string{"network", "rm", "valv-netpolicy-abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RemoveNetwork() args = %#v, want %#v", got, want)
	}
}

func TestExecutorRemoveNetworkReturnsBuildError(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(CommandRunnerFunc(func(_ context.Context, _ []string) error {
		t.Fatal("runner should not be invoked when build fails")
		return nil
	}))

	err := exec.RemoveNetwork(context.Background(), NetworkRemoveRequest{Name: ""})
	if err == nil {
		t.Fatalf("RemoveNetwork() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("RemoveNetwork() error = %q, want substring %q", err.Error(), "name is required")
	}
}

func TestNetworkCreateRequestRejectsUntrimmedLabelKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		labelKey  string
		wantError string
	}{
		{
			name:      "leading whitespace rejected",
			labelKey:  "  valv",
			wantError: "leading or trailing whitespace",
		},
		{
			name:      "trailing whitespace rejected",
			labelKey:  "valv  ",
			wantError: "leading or trailing whitespace",
		},
		{
			name:      "both sides rejected",
			labelKey:  "  valv  ",
			wantError: "leading or trailing whitespace",
		},
		{
			name:      "tab whitespace rejected",
			labelKey:  "\tvalv\t",
			wantError: "leading or trailing whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := NetworkCreateRequest{
				Name:     "valv-test",
				Internal: true,
				Labels:   map[string]string{tt.labelKey: "value"},
			}
			_, err := BuildNetworkCreateArgs(req)
			if err == nil {
				t.Fatalf("BuildNetworkCreateArgs() error = nil, want error containing %q", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("BuildNetworkCreateArgs() error = %q, want substring %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestNetworkCreateRequestRejectsLabelKeyWithEquals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		labelKey string
	}{
		{
			name:     "equals in middle",
			labelKey: "k=injected",
		},
		{
			name:     "equals at start",
			labelKey: "=k",
		},
		{
			name:     "multiple equals",
			labelKey: "k=v=w",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := NetworkCreateRequest{
				Name:     "valv-test",
				Internal: true,
				Labels:   map[string]string{tt.labelKey: "value"},
			}
			_, err := BuildNetworkCreateArgs(req)
			if err == nil {
				t.Fatalf("BuildNetworkCreateArgs() error = nil, want error containing '='")
			}
			if !strings.Contains(err.Error(), "=") {
				t.Fatalf("BuildNetworkCreateArgs() error = %q, want substring containing '='", err.Error())
			}
		})
	}
}

func TestNetworkCreateRequestRejectsOverlongName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		networkName   string
		shouldAccept  bool
		wantErrSubstr string
	}{
		{
			name:         "64 bytes accepted (boundary)",
			networkName:  "a" + strings.Repeat("b", 62) + "c", // exactly 64 bytes
			shouldAccept: true,
		},
		{
			name:          "65 bytes rejected",
			networkName:   "a" + strings.Repeat("b", 63) + "c", // exactly 65 bytes
			shouldAccept:  false,
			wantErrSubstr: "at most 64 bytes",
		},
		{
			name:          "256 bytes rejected",
			networkName:   strings.Repeat("a", 256),
			shouldAccept:  false,
			wantErrSubstr: "at most 64 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := NetworkCreateRequest{
				Name:     tt.networkName,
				Internal: true,
			}
			_, err := BuildNetworkCreateArgs(req)
			if tt.shouldAccept {
				if err != nil {
					t.Fatalf("BuildNetworkCreateArgs() error = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Fatalf("BuildNetworkCreateArgs() error = nil, want error containing %q", tt.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("BuildNetworkCreateArgs() error = %q, want substring %q", err.Error(), tt.wantErrSubstr)
				}
			}
		})
	}
}

func TestBuildNetworkConnectArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request NetworkConnectRequest
		want    []string
		wantErr string
	}{
		{
			name: "single alias with sidecar attachment",
			request: NetworkConnectRequest{
				Network:   "valv-netpolicy-abc",
				Container: "container-id-xyz",
				Aliases:   []string{"valv-proxy"},
			},
			want: []string{
				"network", "connect",
				"--alias", "valv-proxy",
				"valv-netpolicy-abc", "container-id-xyz",
			},
		},
		{
			name: "multiple aliases sorted deterministically",
			request: NetworkConnectRequest{
				Network:   "valv-netpolicy-abc",
				Container: "container-id-xyz",
				Aliases:   []string{"zulu", "alpha", "bravo"},
			},
			want: []string{
				"network", "connect",
				"--alias", "alpha",
				"--alias", "bravo",
				"--alias", "zulu",
				"valv-netpolicy-abc", "container-id-xyz",
			},
		},
		{
			name: "no aliases emits only required args",
			request: NetworkConnectRequest{
				Network:   "valv-netpolicy-abc",
				Container: "container-id-xyz",
				Aliases:   nil,
			},
			want: []string{
				"network", "connect",
				"valv-netpolicy-abc", "container-id-xyz",
			},
		},
		{
			name: "empty aliases slice emits only required args",
			request: NetworkConnectRequest{
				Network:   "valv-netpolicy-abc",
				Container: "container-id-xyz",
				Aliases:   []string{},
			},
			want: []string{
				"network", "connect",
				"valv-netpolicy-abc", "container-id-xyz",
			},
		},
		{
			name: "whitespace-padded fields are trimmed",
			request: NetworkConnectRequest{
				Network:   "  valv-net  ",
				Container: "  container-id  ",
				Aliases:   []string{"  alias1  "},
			},
			want: []string{
				"network", "connect",
				"--alias", "alias1",
				"valv-net", "container-id",
			},
		},
		{
			name:    "empty network rejected",
			request: NetworkConnectRequest{Network: "", Container: "ctr", Aliases: nil},
			wantErr: "network is required",
		},
		{
			name:    "whitespace-only network rejected",
			request: NetworkConnectRequest{Network: "   ", Container: "ctr", Aliases: nil},
			wantErr: "network is required",
		},
		{
			name:    "invalid network name rejected",
			request: NetworkConnectRequest{Network: "valv/net", Container: "ctr", Aliases: nil},
			wantErr: "invalid network name",
		},
		{
			name:    "empty container rejected",
			request: NetworkConnectRequest{Network: "valv-net", Container: "", Aliases: nil},
			wantErr: "container is required",
		},
		{
			name:    "whitespace-only container rejected",
			request: NetworkConnectRequest{Network: "valv-net", Container: "   ", Aliases: nil},
			wantErr: "container is required",
		},
		{
			name:    "empty alias in slice rejected",
			request: NetworkConnectRequest{Network: "valv-net", Container: "ctr", Aliases: []string{"valid", ""}},
			wantErr: "empty",
		},
		{
			name:    "whitespace-only alias rejected",
			request: NetworkConnectRequest{Network: "valv-net", Container: "ctr", Aliases: []string{"   "}},
			wantErr: "empty",
		},
		{
			name:    "invalid alias pattern rejected",
			request: NetworkConnectRequest{Network: "valv-net", Container: "ctr", Aliases: []string{"invalid/alias"}},
			wantErr: "invalid alias",
		},
		{
			name:    "network name with leading hyphen rejected",
			request: NetworkConnectRequest{Network: "-invalid", Container: "ctr", Aliases: nil},
			wantErr: "invalid network name",
		},
		{
			name:    "network name exceeds 64 bytes rejected",
			request: NetworkConnectRequest{Network: "a" + strings.Repeat("b", 63) + "c", Container: "ctr", Aliases: nil},
			wantErr: "at most 64 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := BuildNetworkConnectArgs(tt.request)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("BuildNetworkConnectArgs() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("BuildNetworkConnectArgs() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildNetworkConnectArgs() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("BuildNetworkConnectArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExecutorConnectNetworkForwardsArgs(t *testing.T) {
	t.Parallel()

	var got []string
	exec := NewExecutor(CommandRunnerFunc(func(_ context.Context, args []string) error {
		got = append([]string(nil), args...)
		return nil
	}))

	if err := exec.ConnectNetwork(context.Background(), NetworkConnectRequest{
		Network:   "valv-netpolicy-abc",
		Container: "container-id-xyz",
		Aliases:   []string{"valv-proxy"},
	}); err != nil {
		t.Fatalf("ConnectNetwork() error = %v", err)
	}

	want := []string{
		"network", "connect",
		"--alias", "valv-proxy",
		"valv-netpolicy-abc", "container-id-xyz",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ConnectNetwork() args = %#v, want %#v", got, want)
	}
}

func TestExecutorConnectNetworkReturnsBuildError(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(CommandRunnerFunc(func(_ context.Context, _ []string) error {
		t.Fatal("runner should not be invoked when build fails")
		return nil
	}))

	err := exec.ConnectNetwork(context.Background(), NetworkConnectRequest{Network: ""})
	if err == nil {
		t.Fatalf("ConnectNetwork() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "network is required") {
		t.Fatalf("ConnectNetwork() error = %q, want substring %q", err.Error(), "network is required")
	}
}
