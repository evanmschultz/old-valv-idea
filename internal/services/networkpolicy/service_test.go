package networkpolicy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/evanmschultz/valv/internal/adapters/docker"
)

// fakeNetworkExecutor is a recording stub for NetworkExecutor used in the
// table-driven tests below.
type fakeNetworkExecutor struct {
	// existing networks returned by ListNetworks on the next call. The
	// service drains this on each call so successive ListNetworks calls
	// reflect what create/remove operations did to the world.
	listResult []string
	listErr    error

	createErr error
	removeErr error

	// runDetachedResult is the container ID returned by RunContainerDetached.
	runDetachedResult string
	runDetachedErr    error

	createCalls      []docker.NetworkCreateRequest
	removeCalls      []docker.NetworkRemoveRequest
	listCalls        []string
	runDetachedCalls []docker.ContainerRunRequest
	connectCalls     []docker.NetworkConnectRequest
}

func (f *fakeNetworkExecutor) CreateNetwork(_ context.Context, req docker.NetworkCreateRequest) error {
	f.createCalls = append(f.createCalls, req)
	if f.createErr != nil {
		return f.createErr
	}
	// Reflect creation into the visible list so subsequent ListNetworks
	// calls behave the way docker would.
	f.listResult = append(f.listResult, req.Name)
	return nil
}

func (f *fakeNetworkExecutor) RemoveNetwork(_ context.Context, req docker.NetworkRemoveRequest) error {
	f.removeCalls = append(f.removeCalls, req)
	if f.removeErr != nil {
		return f.removeErr
	}
	out := make([]string, 0, len(f.listResult))
	for _, name := range f.listResult {
		if name != req.Name {
			out = append(out, name)
		}
	}
	f.listResult = out
	return nil
}

func (f *fakeNetworkExecutor) ListNetworks(_ context.Context, label string) ([]string, error) {
	f.listCalls = append(f.listCalls, label)
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]string, len(f.listResult))
	copy(out, f.listResult)
	return out, nil
}

func (f *fakeNetworkExecutor) RunContainerDetached(_ context.Context, req docker.ContainerRunRequest) (string, error) {
	f.runDetachedCalls = append(f.runDetachedCalls, req)
	if f.runDetachedErr != nil {
		return "", f.runDetachedErr
	}
	return f.runDetachedResult, nil
}

func (f *fakeNetworkExecutor) ConnectNetwork(_ context.Context, req docker.NetworkConnectRequest) error {
	f.connectCalls = append(f.connectCalls, req)
	return nil
}

func TestNew_RequiresExecutor(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatalf("New(zero Options) = nil error, want error for missing executor")
	}
}

func TestNew_Success(t *testing.T) {
	t.Parallel()

	svc, err := New(Options{Executor: &fakeNetworkExecutor{}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if svc.executor == nil {
		t.Fatalf("New() returned service with nil executor")
	}
}

func TestProvisionRequest_Valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		request ProvisionRequest
		wantErr bool
	}{
		{
			name: "happy_path",
			request: ProvisionRequest{
				Allowlist:     []string{"proxy.golang.org"},
				ProxyEndpoint: "host.docker.internal:18080",
			},
			wantErr: false,
		},
		{
			name:    "empty_allowlist_rejected",
			request: ProvisionRequest{ProxyEndpoint: "host.docker.internal:18080"},
			wantErr: true,
		},
		{
			name: "blank_allowlist_entry_rejected",
			request: ProvisionRequest{
				Allowlist:     []string{"proxy.golang.org", "  "},
				ProxyEndpoint: "host.docker.internal:18080",
			},
			wantErr: true,
		},
		{
			name: "blank_endpoint_rejected",
			request: ProvisionRequest{
				Allowlist:     []string{"proxy.golang.org"},
				ProxyEndpoint: "   ",
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.request.Valid()
			if tc.wantErr && err == nil {
				t.Fatalf("Valid() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Valid() = %v, want nil", err)
			}
		})
	}
}

func TestProvision_CreatesNetworkOnFreshHost(t *testing.T) {
	t.Parallel()

	fake := &fakeNetworkExecutor{}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	allowlist := []string{"proxy.golang.org", "sum.golang.org", "github.com", "objects.githubusercontent.com"}
	req := ProvisionRequest{
		Allowlist:     allowlist,
		ProxyEndpoint: "host.docker.internal:18080",
	}

	material, cleanup, err := svc.Provision(context.Background(), req)
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if cleanup == nil {
		t.Fatalf("Provision() cleanup = nil, want non-nil")
	}

	// ListNetworks must be called with the managed-label filter.
	if len(fake.listCalls) != 1 || fake.listCalls[0] != ManagedLabelKey+"="+ManagedLabelValue {
		t.Fatalf("listCalls = %#v, want exactly [%q]", fake.listCalls, ManagedLabelKey+"="+ManagedLabelValue)
	}

	// One CreateNetwork call: --internal=true, managed label set.
	if len(fake.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1", len(fake.createCalls))
	}
	create := fake.createCalls[0]
	if !create.Internal {
		t.Errorf("CreateNetwork Internal = false, want true")
	}
	if create.Labels[ManagedLabelKey] != ManagedLabelValue {
		t.Errorf("CreateNetwork Labels[%q] = %q, want %q", ManagedLabelKey, create.Labels[ManagedLabelKey], ManagedLabelValue)
	}
	if !strings.HasPrefix(create.Name, networkNamePrefix) {
		t.Errorf("CreateNetwork Name = %q, want prefix %q", create.Name, networkNamePrefix)
	}

	// Material assertions.
	if material.NetworkName != create.Name {
		t.Errorf("material.NetworkName = %q, want %q (matching created network)", material.NetworkName, create.Name)
	}
	if material.HTTPProxyURL != "http://host.docker.internal:18080" {
		t.Errorf("HTTPProxyURL = %q, want %q", material.HTTPProxyURL, "http://host.docker.internal:18080")
	}
	if material.HTTPSProxyURL != "http://host.docker.internal:18080" {
		t.Errorf("HTTPSProxyURL = %q, want %q", material.HTTPSProxyURL, "http://host.docker.internal:18080")
	}
	// NO_PROXY: comma-separated, sorted.
	wantNoProxy := "github.com,objects.githubusercontent.com,proxy.golang.org,sum.golang.org"
	if material.NoProxy != wantNoProxy {
		t.Errorf("NoProxy = %q, want %q", material.NoProxy, wantNoProxy)
	}

	// Cleanup removes the network.
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if len(fake.removeCalls) != 1 {
		t.Fatalf("removeCalls = %d, want 1", len(fake.removeCalls))
	}
	if fake.removeCalls[0].Name != create.Name {
		t.Errorf("cleanup removed %q, want %q", fake.removeCalls[0].Name, create.Name)
	}
}

func TestProvision_IdempotentReclaimOfMatchingNetwork(t *testing.T) {
	t.Parallel()

	allowlist := []string{"github.com", "proxy.golang.org"}
	expectedName := networkName(allowlist)

	fake := &fakeNetworkExecutor{
		listResult: []string{expectedName},
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist:     allowlist,
		ProxyEndpoint: "host.docker.internal:18080",
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	// Reused: no CreateNetwork call, no RemoveNetwork call.
	if len(fake.createCalls) != 0 {
		t.Errorf("createCalls = %d, want 0 (reuse path)", len(fake.createCalls))
	}
	if len(fake.removeCalls) != 0 {
		t.Errorf("removeCalls = %d, want 0 (reuse path)", len(fake.removeCalls))
	}
	if material.NetworkName != expectedName {
		t.Errorf("material.NetworkName = %q, want %q", material.NetworkName, expectedName)
	}

	// Cleanup still removes the reclaimed network so callers don't have
	// to track the reuse decision.
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if len(fake.removeCalls) != 1 {
		t.Fatalf("post-cleanup removeCalls = %d, want 1", len(fake.removeCalls))
	}
}

func TestProvision_RemovesStaleOrphansBeforeCreate(t *testing.T) {
	t.Parallel()

	allowlist := []string{"github.com"}
	expectedName := networkName(allowlist)

	staleA := "valv-netpol-deadbeefcafe"
	staleB := "valv-netpol-feedfacebeef"
	fake := &fakeNetworkExecutor{
		listResult: []string{staleA, staleB},
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist:     allowlist,
		ProxyEndpoint: "host.docker.internal:18080",
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if cleanup == nil {
		t.Fatal("Provision() cleanup = nil")
	}

	// Both stale orphans removed, fresh network created.
	if len(fake.removeCalls) != 2 {
		t.Fatalf("removeCalls = %d, want 2 (stale removal)", len(fake.removeCalls))
	}
	removed := map[string]bool{}
	for _, r := range fake.removeCalls {
		removed[r.Name] = true
	}
	if !removed[staleA] || !removed[staleB] {
		t.Errorf("removeCalls = %#v, want removal of both %q and %q", fake.removeCalls, staleA, staleB)
	}
	if len(fake.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1 (fresh create after orphan cleanup)", len(fake.createCalls))
	}
	if fake.createCalls[0].Name != expectedName {
		t.Errorf("created network name = %q, want %q", fake.createCalls[0].Name, expectedName)
	}
	if material.NetworkName != expectedName {
		t.Errorf("material.NetworkName = %q, want %q", material.NetworkName, expectedName)
	}
}

func TestProvision_ReclaimMatchingAndRemoveStaleSimultaneously(t *testing.T) {
	t.Parallel()

	allowlist := []string{"sum.golang.org"}
	expectedName := networkName(allowlist)
	stale := "valv-netpol-0123456789ab"

	fake := &fakeNetworkExecutor{
		listResult: []string{expectedName, stale},
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist:     allowlist,
		ProxyEndpoint: "host.docker.internal:18080",
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if cleanup == nil {
		t.Fatal("Provision() cleanup = nil")
	}

	// Stale is removed; matching is reclaimed; no Create call.
	if len(fake.createCalls) != 0 {
		t.Errorf("createCalls = %d, want 0 (reclaim path)", len(fake.createCalls))
	}
	if len(fake.removeCalls) != 1 || fake.removeCalls[0].Name != stale {
		t.Errorf("removeCalls = %#v, want exactly [%q]", fake.removeCalls, stale)
	}
	if material.NetworkName != expectedName {
		t.Errorf("material.NetworkName = %q, want %q", material.NetworkName, expectedName)
	}
}

func TestProvision_ListError_Wrapped(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("docker daemon offline")
	fake := &fakeNetworkExecutor{listErr: sentinel}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, _, err = svc.Provision(context.Background(), ProvisionRequest{
		Allowlist:     []string{"github.com"},
		ProxyEndpoint: "host.docker.internal:18080",
	})
	if err == nil {
		t.Fatal("Provision() error = nil, want list error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Provision() error = %v, want errors.Is sentinel", err)
	}
	if !strings.Contains(err.Error(), "networkpolicy provision") {
		t.Errorf("Provision() error = %q, want context prefix 'networkpolicy provision'", err.Error())
	}
}

func TestProvision_CreateError_Wrapped(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("docker daemon refused")
	fake := &fakeNetworkExecutor{createErr: sentinel}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, _, err = svc.Provision(context.Background(), ProvisionRequest{
		Allowlist:     []string{"github.com"},
		ProxyEndpoint: "host.docker.internal:18080",
	})
	if err == nil {
		t.Fatal("Provision() error = nil, want create error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Provision() error = %v, want errors.Is sentinel", err)
	}
	if !strings.Contains(err.Error(), "networkpolicy provision") {
		t.Errorf("Provision() error = %q, want context prefix 'networkpolicy provision'", err.Error())
	}
}

func TestProvision_StaleRemoveError_Wrapped(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("network in use")
	fake := &fakeNetworkExecutor{
		listResult: []string{"valv-netpol-stalenetw0rk"},
		removeErr:  sentinel,
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, _, err = svc.Provision(context.Background(), ProvisionRequest{
		Allowlist:     []string{"github.com"},
		ProxyEndpoint: "host.docker.internal:18080",
	})
	if err == nil {
		t.Fatal("Provision() error = nil, want stale-removal error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Provision() error = %v, want errors.Is sentinel", err)
	}
}

func TestProvision_CleanupError_Wrapped(t *testing.T) {
	t.Parallel()

	fake := &fakeNetworkExecutor{}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist:     []string{"github.com"},
		ProxyEndpoint: "host.docker.internal:18080",
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	// Now fail the cleanup.
	sentinel := errors.New("network busy")
	fake.removeErr = sentinel
	if cleanupErr := cleanup(context.Background()); cleanupErr == nil {
		t.Fatal("cleanup() error = nil, want wrapped removal error")
	} else if !errors.Is(cleanupErr, sentinel) {
		t.Errorf("cleanup() error = %v, want errors.Is sentinel", cleanupErr)
	}
}

func TestProvision_InvalidRequest_Wrapped(t *testing.T) {
	t.Parallel()

	svc, err := New(Options{Executor: &fakeNetworkExecutor{}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, _, err = svc.Provision(context.Background(), ProvisionRequest{ProxyEndpoint: "host.docker.internal:18080"})
	if err == nil {
		t.Fatal("Provision(invalid) error = nil, want validation error")
	}
}

func TestNetworkName_DeterministicAndOrderInvariant(t *testing.T) {
	t.Parallel()

	first := networkName([]string{"a.example", "b.example", "c.example"})
	second := networkName([]string{"c.example", "a.example", "b.example"})
	if first != second {
		t.Fatalf("networkName order variance: %q vs %q", first, second)
	}
	other := networkName([]string{"a.example", "b.example"})
	if first == other {
		t.Fatalf("networkName collision: same name %q for different inputs", first)
	}
	if !strings.HasPrefix(first, networkNamePrefix) {
		t.Errorf("networkName = %q, want prefix %q", first, networkNamePrefix)
	}
}

func TestBuildNoProxy_SortedDedupedAndTrimmed(t *testing.T) {
	t.Parallel()

	got := buildNoProxy([]string{"  b ", "a", "b", "c", "a"})
	want := "a,b,c"
	if got != want {
		t.Fatalf("buildNoProxy = %q, want %q", got, want)
	}
}

func TestCleanupStale_RemovesAllManagedNetworks(t *testing.T) {
	t.Parallel()

	fake := &fakeNetworkExecutor{
		listResult: []string{"valv-netpol-aaaa11112222", "valv-netpol-bbbb33334444"},
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := svc.CleanupStale(context.Background()); err != nil {
		t.Fatalf("CleanupStale() error = %v", err)
	}
	if len(fake.removeCalls) != 2 {
		t.Fatalf("removeCalls = %d, want 2", len(fake.removeCalls))
	}
}

func TestCleanupStale_ListError_Wrapped(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("daemon unreachable")
	fake := &fakeNetworkExecutor{listErr: sentinel}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = svc.CleanupStale(context.Background())
	if err == nil {
		t.Fatal("CleanupStale() error = nil, want list error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("CleanupStale() error = %v, want errors.Is sentinel", err)
	}
}

func TestCleanupStale_RemoveError_Wrapped(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("network in use by container")
	fake := &fakeNetworkExecutor{
		listResult: []string{"valv-netpol-aaaa11112222"},
		removeErr:  sentinel,
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = svc.CleanupStale(context.Background())
	if err == nil {
		t.Fatal("CleanupStale() error = nil, want remove error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("CleanupStale() error = %v, want errors.Is sentinel", err)
	}
}

func TestPolicyMaterial_NoProxyOmitsBlankAllowlistEntries(t *testing.T) {
	t.Parallel()

	// Direct buildNoProxy test, since ProvisionRequest.Valid() would reject
	// blank entries up front. Behavior pin against future regressions if
	// validation moves.
	got := buildNoProxy([]string{"", " github.com ", "proxy.golang.org"})
	want := "github.com,proxy.golang.org"
	if got != want {
		t.Fatalf("buildNoProxy = %q, want %q", got, want)
	}
}

// Compile-time interface check: docker.Executor must satisfy NetworkExecutor.
// Confirms the production path lines up with the consumer-side interface.
var _ NetworkExecutor = docker.Executor{}

func TestProvision_TrimsProxyEndpointWhitespace(t *testing.T) {
	t.Parallel()

	fake := &fakeNetworkExecutor{}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist:     []string{"github.com"},
		ProxyEndpoint: "  host.docker.internal:18080  ",
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	defer func() { _ = cleanup(context.Background()) }()

	want := "http://host.docker.internal:18080"
	if material.HTTPProxyURL != want {
		t.Errorf("HTTPProxyURL = %q, want %q", material.HTTPProxyURL, want)
	}
	if material.HTTPSProxyURL != want {
		t.Errorf("HTTPSProxyURL = %q, want %q", material.HTTPSProxyURL, want)
	}
}
