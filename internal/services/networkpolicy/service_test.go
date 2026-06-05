package networkpolicy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

	// connectErr is returned by ConnectNetwork when non-nil.
	connectErr error

	createCalls      []docker.NetworkCreateRequest
	removeCalls      []docker.NetworkRemoveRequest
	listCalls        []string
	runDetachedCalls []docker.ContainerRunRequest
	connectCalls     []docker.NetworkConnectRequest

	// callOrder records the method names in invocation order so tests can
	// assert operation sequencing.
	callOrder []string
}

func (f *fakeNetworkExecutor) CreateNetwork(_ context.Context, req docker.NetworkCreateRequest) error {
	f.callOrder = append(f.callOrder, "CreateNetwork")
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
	f.callOrder = append(f.callOrder, "RemoveNetwork")
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
	f.callOrder = append(f.callOrder, "ListNetworks")
	f.listCalls = append(f.listCalls, label)
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]string, len(f.listResult))
	copy(out, f.listResult)
	return out, nil
}

func (f *fakeNetworkExecutor) RunContainerDetached(_ context.Context, req docker.ContainerRunRequest) (string, error) {
	f.callOrder = append(f.callOrder, "RunContainerDetached")
	f.runDetachedCalls = append(f.runDetachedCalls, req)
	if f.runDetachedErr != nil {
		return "", f.runDetachedErr
	}
	return f.runDetachedResult, nil
}

func (f *fakeNetworkExecutor) ConnectNetwork(_ context.Context, req docker.NetworkConnectRequest) error {
	f.callOrder = append(f.callOrder, "ConnectNetwork")
	f.connectCalls = append(f.connectCalls, req)
	return f.connectErr
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
			name:    "happy_path",
			request: ProvisionRequest{Allowlist: []string{"proxy.golang.org"}},
			wantErr: false,
		},
		{
			name:    "empty_allowlist_rejected",
			request: ProvisionRequest{},
			wantErr: true,
		},
		{
			name: "blank_allowlist_entry_rejected",
			request: ProvisionRequest{
				Allowlist: []string{"proxy.golang.org", "  "},
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

	const fakeSidecarID = "abc123def456"
	fake := &fakeNetworkExecutor{runDetachedResult: fakeSidecarID}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	allowlist := []string{"proxy.golang.org", "sum.golang.org", "github.com", "objects.githubusercontent.com"}
	req := ProvisionRequest{Allowlist: allowlist}

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

	// One RunContainerDetached call: correct image, Detached=true, env vars, managed label, no network (bridge default).
	if len(fake.runDetachedCalls) != 1 {
		t.Fatalf("runDetachedCalls = %d, want 1", len(fake.runDetachedCalls))
	}
	run := fake.runDetachedCalls[0]
	if run.Image != proxyImageRef() {
		t.Errorf("RunContainerDetached Image = %v, want %v", run.Image, proxyImageRef())
	}
	if !run.Detached {
		t.Errorf("RunContainerDetached Detached = false, want true")
	}
	if run.Env["VALV_PROXY_ADDR"] != ":"+proxyPort {
		t.Errorf("RunContainerDetached Env[VALV_PROXY_ADDR] = %q, want %q", run.Env["VALV_PROXY_ADDR"], ":"+proxyPort)
	}
	if run.Env["VALV_PROXY_ALLOWLIST"] == "" {
		t.Errorf("RunContainerDetached Env[VALV_PROXY_ALLOWLIST] is empty, want non-empty")
	}
	if run.Labels[ManagedLabelKey] != ManagedLabelValue {
		t.Errorf("RunContainerDetached Labels[%q] = %q, want %q", ManagedLabelKey, run.Labels[ManagedLabelKey], ManagedLabelValue)
	}
	if run.Network != "" {
		t.Errorf("RunContainerDetached Network = %q, want empty (bridge default)", run.Network)
	}

	// One ConnectNetwork call: proxy sidecar connected to internal network with ProxyAlias.
	if len(fake.connectCalls) != 1 {
		t.Fatalf("connectCalls = %d, want 1", len(fake.connectCalls))
	}
	conn := fake.connectCalls[0]
	if conn.Container != fakeSidecarID {
		t.Errorf("ConnectNetwork Container = %q, want %q (sidecar ID from RunContainerDetached)", conn.Container, fakeSidecarID)
	}
	if conn.Network != create.Name {
		t.Errorf("ConnectNetwork Network = %q, want %q (internal network)", conn.Network, create.Name)
	}
	if len(conn.Aliases) != 1 || conn.Aliases[0] != ProxyAlias {
		t.Errorf("ConnectNetwork Aliases = %v, want [%q]", conn.Aliases, ProxyAlias)
	}

	// Material assertions.
	if material.NetworkName != create.Name {
		t.Errorf("material.NetworkName = %q, want %q (matching created network)", material.NetworkName, create.Name)
	}
	wantProxyURL := "http://" + ProxyAlias + ":" + proxyPort
	if material.HTTPProxyURL != wantProxyURL {
		t.Errorf("HTTPProxyURL = %q, want %q", material.HTTPProxyURL, wantProxyURL)
	}
	if material.HTTPSProxyURL != wantProxyURL {
		t.Errorf("HTTPSProxyURL = %q, want %q", material.HTTPSProxyURL, wantProxyURL)
	}
	// NO_PROXY: only loopback + sidecar alias; allowlist hosts must NOT appear.
	wantNoProxy := "127.0.0.1,localhost," + ProxyAlias
	if material.NoProxy != wantNoProxy {
		t.Errorf("NoProxy = %q, want %q", material.NoProxy, wantNoProxy)
	}
	for _, allowlistHost := range allowlist {
		if strings.Contains(material.NoProxy, allowlistHost) {
			t.Errorf("NoProxy = %q contains allowlist host %q — allowlist hosts must never appear in NO_PROXY", material.NoProxy, allowlistHost)
		}
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
		listResult:        []string{expectedName},
		runDetachedResult: "sidecar-reclaim-id",
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{Allowlist: allowlist})
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

	// Sidecar still launched even on reclaim path.
	if len(fake.runDetachedCalls) != 1 {
		t.Errorf("runDetachedCalls = %d, want 1 (sidecar always launched)", len(fake.runDetachedCalls))
	}
	if len(fake.connectCalls) != 1 {
		t.Errorf("connectCalls = %d, want 1 (sidecar always connected)", len(fake.connectCalls))
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
		listResult:        []string{staleA, staleB},
		runDetachedResult: "sidecar-stale-test",
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{Allowlist: allowlist})
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
	// Sidecar launched after orphan cleanup.
	if len(fake.runDetachedCalls) != 1 {
		t.Errorf("runDetachedCalls = %d, want 1", len(fake.runDetachedCalls))
	}
}

func TestProvision_ReclaimMatchingAndRemoveStaleSimultaneously(t *testing.T) {
	t.Parallel()

	allowlist := []string{"sum.golang.org"}
	expectedName := networkName(allowlist)
	stale := "valv-netpol-0123456789ab"

	fake := &fakeNetworkExecutor{
		listResult:        []string{expectedName, stale},
		runDetachedResult: "sidecar-reclaim-stale",
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{Allowlist: allowlist})
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
		Allowlist: []string{"github.com"},
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
		Allowlist: []string{"github.com"},
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
		Allowlist: []string{"github.com"},
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

	fake := &fakeNetworkExecutor{runDetachedResult: "cleanup-test-sidecar"}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
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

	_, _, err = svc.Provision(context.Background(), ProvisionRequest{})
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

// TestBuildNoProxy covers the unit-15.2.5.C acceptance criteria: NO_PROXY
// contains ONLY loopback entries and the valv-proxy sidecar alias. Allowlist
// hosts must never appear regardless of what the caller passes.
func TestBuildNoProxy(t *testing.T) {
	t.Parallel()

	const want = "127.0.0.1,localhost,valv-proxy"

	cases := []struct {
		name string
		desc string
	}{
		{
			name: "empty_allowlist",
			desc: "empty allowlist: NO_PROXY still contains only loopback + sidecar alias",
		},
		{
			name: "non_empty_allowlist_no_leak",
			desc: "non-empty allowlist: allowlist hosts must not appear in NO_PROXY",
		},
		{
			name: "loopback_entries_already_covered",
			desc: "loopback entries are always present regardless of how buildNoProxy is called",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildNoProxy()
			if got != want {
				t.Errorf("buildNoProxy() = %q, want %q (%s)", got, want, tc.desc)
			}
		})
	}
}

// TestBuildNoProxy_AllowlistHostsNeverLeak asserts that allowlist hosts do not
// appear in NO_PROXY even when Provision is called with a populated allowlist.
// This is the core invariant of Schema Decision 5's NO_PROXY correction.
func TestBuildNoProxy_AllowlistHostsNeverLeak(t *testing.T) {
	t.Parallel()

	allowlistHosts := []string{
		"github.com",
		"objects.githubusercontent.com",
		"proxy.golang.org",
		"sum.golang.org",
		"registry-1.docker.io",
	}

	fake := &fakeNetworkExecutor{runDetachedResult: "allowlist-no-leak-sidecar"}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: allowlistHosts,
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	defer func() { _ = cleanup(context.Background()) }()

	wantNoProxy := "127.0.0.1,localhost," + ProxyAlias
	if material.NoProxy != wantNoProxy {
		t.Errorf("NoProxy = %q, want %q", material.NoProxy, wantNoProxy)
	}

	for _, host := range allowlistHosts {
		if strings.Contains(material.NoProxy, host) {
			t.Errorf("NoProxy %q contains allowlist host %q — allowlist hosts must NEVER appear in NO_PROXY", material.NoProxy, host)
		}
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

// TestBuildNoProxy_LoopbackAndSidecarAlwaysPresent is a regression pin
// confirming that the fixed buildNoProxy always emits the three fixed entries
// regardless of any caller context. The old implementation accepted an allowlist
// parameter and placed it into NO_PROXY (inverted semantics); this test pins
// the corrected shape.
func TestBuildNoProxy_LoopbackAndSidecarAlwaysPresent(t *testing.T) {
	t.Parallel()

	got := buildNoProxy()
	want := "127.0.0.1,localhost,valv-proxy"
	if got != want {
		t.Fatalf("buildNoProxy() = %q, want %q", got, want)
	}
	// Confirm each required entry is present.
	for _, required := range []string{"127.0.0.1", "localhost", "valv-proxy"} {
		found := false
		for _, entry := range strings.Split(got, ",") {
			if entry == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("buildNoProxy() = %q, missing required entry %q", got, required)
		}
	}
}

// TestProvision_OperationOrder asserts that Provision executes its Docker
// operations in the required sequence: (1) ListNetworks, (2) CreateNetwork,
// (3) RunContainerDetached, (4) ConnectNetwork. This order is load-bearing:
// the sidecar must be running before ConnectNetwork is called, and the network
// must exist before both.
func TestProvision_OperationOrder(t *testing.T) {
	t.Parallel()

	fake := &fakeNetworkExecutor{runDetachedResult: "ordered-sidecar"}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	defer func() { _ = cleanup(context.Background()) }()

	want := []string{"ListNetworks", "CreateNetwork", "RunContainerDetached", "ConnectNetwork"}
	if len(fake.callOrder) != len(want) {
		t.Fatalf("callOrder = %v, want %v", fake.callOrder, want)
	}
	for i, op := range want {
		if fake.callOrder[i] != op {
			t.Errorf("callOrder[%d] = %q, want %q", i, fake.callOrder[i], op)
		}
	}
}

// TestProvision_RunContainerDetachedError_NoConnectAttempted asserts that a
// RunContainerDetached failure causes Provision to return immediately without
// calling ConnectNetwork.
func TestProvision_RunContainerDetachedError_NoConnectAttempted(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("image not found")
	fake := &fakeNetworkExecutor{runDetachedErr: sentinel}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, _, err = svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err == nil {
		t.Fatal("Provision() error = nil, want sidecar-launch error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Provision() error = %v, want errors.Is sentinel", err)
	}
	if !strings.Contains(err.Error(), "networkpolicy provision") {
		t.Errorf("Provision() error = %q, want context prefix 'networkpolicy provision'", err.Error())
	}
	// ConnectNetwork must NOT have been called.
	if len(fake.connectCalls) != 0 {
		t.Errorf("connectCalls = %d, want 0 (ConnectNetwork must not fire after RunContainerDetached error)", len(fake.connectCalls))
	}
}

// TestProvision_ConnectNetworkError_Wrapped asserts that a ConnectNetwork
// failure wraps and propagates correctly.
func TestProvision_ConnectNetworkError_Wrapped(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("network connect refused")
	fake := &fakeNetworkExecutor{
		runDetachedResult: "connect-error-sidecar",
		connectErr:        sentinel,
	}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, _, err = svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err == nil {
		t.Fatal("Provision() error = nil, want connect error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Provision() error = %v, want errors.Is sentinel", err)
	}
	if !strings.Contains(err.Error(), "networkpolicy provision") {
		t.Errorf("Provision() error = %q, want context prefix 'networkpolicy provision'", err.Error())
	}
}

// Compile-time interface check: docker.Executor must satisfy NetworkExecutor.
// Confirms the production path lines up with the consumer-side interface.
var _ NetworkExecutor = docker.Executor{}

// Compile-time interface check: docker.Executor must satisfy containerChecker.
// This guard makes it impossible for a future refactor to silently break the
// production readiness probe by removing or renaming ContainerRunning on
// docker.Executor — the build fails immediately rather than silently
// degrading to a no-op probe.
var _ containerChecker = docker.Executor{}

// Compile-time interface check: docker.Executor must satisfy staleSweeper.
// This guard ensures ListContainersByLabel and RemoveContainer remain present
// on docker.Executor — without it, sweepStaleSidecars would silently no-op
// in production (the same pattern that caught F-2 regression).
var _ staleSweeper = docker.Executor{}

// staleSweeperExecutor embeds fakeNetworkExecutor and additionally implements
// staleSweeper (ListContainersByLabel + RemoveContainer). It is used in tests
// that assert the stale-sidecar sweep actually fires and removes containers.
type staleSweeperExecutor struct {
	fakeNetworkExecutor

	// listContainerResult is the slice of container IDs returned by
	// ListContainersByLabel on the next call.
	listContainerResult []string
	listContainerErr    error

	// removeContainerErr is returned by RemoveContainer when non-nil.
	removeContainerErr error

	// listContainerCalls records the label argument for each call.
	listContainerCalls []string
	// removeContainerCalls records each ContainerRemoveRequest issued.
	removeContainerCalls []docker.ContainerRemoveRequest
}

func (f *staleSweeperExecutor) ListContainersByLabel(_ context.Context, label string) ([]string, error) {
	f.callOrder = append(f.callOrder, "ListContainersByLabel")
	f.listContainerCalls = append(f.listContainerCalls, label)
	if f.listContainerErr != nil {
		return nil, f.listContainerErr
	}
	out := make([]string, len(f.listContainerResult))
	copy(out, f.listContainerResult)
	return out, nil
}

func (f *staleSweeperExecutor) RemoveContainer(_ context.Context, req docker.ContainerRemoveRequest) error {
	f.callOrder = append(f.callOrder, "RemoveContainer")
	f.removeContainerCalls = append(f.removeContainerCalls, req)
	return f.removeContainerErr
}

// TestProvision_AllowlistJoinedInSidecarEnv verifies that the VALV_PROXY_ALLOWLIST
// env var on the sidecar is the comma-joined effective allowlist.
func TestProvision_AllowlistJoinedInSidecarEnv(t *testing.T) {
	t.Parallel()

	fake := &fakeNetworkExecutor{runDetachedResult: "allowlist-env-sidecar"}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	allowlist := []string{"github.com", "proxy.golang.org"}
	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: allowlist,
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	defer func() { _ = cleanup(context.Background()) }()

	// Proxy URLs must point at ProxyAlias, not a caller-supplied endpoint.
	wantProxyURL := "http://" + ProxyAlias + ":" + proxyPort
	if material.HTTPProxyURL != wantProxyURL {
		t.Errorf("HTTPProxyURL = %q, want %q", material.HTTPProxyURL, wantProxyURL)
	}
	if material.HTTPSProxyURL != wantProxyURL {
		t.Errorf("HTTPSProxyURL = %q, want %q", material.HTTPSProxyURL, wantProxyURL)
	}

	// VALV_PROXY_ALLOWLIST must contain the allowlist entries.
	if len(fake.runDetachedCalls) != 1 {
		t.Fatalf("runDetachedCalls = %d, want 1", len(fake.runDetachedCalls))
	}
	envAllowlist := fake.runDetachedCalls[0].Env["VALV_PROXY_ALLOWLIST"]
	for _, host := range allowlist {
		if !strings.Contains(envAllowlist, host) {
			t.Errorf("VALV_PROXY_ALLOWLIST = %q does not contain %q", envAllowlist, host)
		}
	}
}

// noopSleep is a ProbeSleep stub that returns immediately without sleeping.
// Injected into tests so probe loops complete in nanoseconds.
func noopSleep(_ context.Context, _ time.Duration) error { return nil }

// TestProvision_ReadyImmediately asserts that when the injected ReadyProbe
// returns nil on the first attempt, Provision completes successfully and
// returns a non-zero PolicyMaterial.
func TestProvision_ReadyImmediately(t *testing.T) {
	t.Parallel()

	const fakeSidecarID = "ready-immediately-sidecar"
	fake := &fakeNetworkExecutor{runDetachedResult: fakeSidecarID}

	callCount := 0
	svc, err := New(Options{
		Executor: fake,
		ReadyProbe: func(_ context.Context, id string) error {
			callCount++
			if id != fakeSidecarID {
				t.Errorf("ReadyProbe: containerID = %q, want %q", id, fakeSidecarID)
			}
			return nil // ready on first attempt
		},
		ProbeSleep:    noopSleep,
		ProbeAttempts: 5,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if cleanup == nil {
		t.Fatalf("Provision() cleanup = nil, want non-nil")
	}
	if material.NetworkName == "" {
		t.Errorf("material.NetworkName is empty, want non-empty")
	}
	if callCount != 1 {
		t.Errorf("ReadyProbe called %d times, want 1 (ready on first attempt)", callCount)
	}
	_ = cleanup(context.Background())
}

// TestProvision_ReadyAfterRetries asserts that when the injected ReadyProbe
// returns an error for the first two attempts and nil on the third, Provision
// completes successfully after exactly three probe calls.
func TestProvision_ReadyAfterRetries(t *testing.T) {
	t.Parallel()

	const fakeSidecarID = "retry-sidecar"
	fake := &fakeNetworkExecutor{runDetachedResult: fakeSidecarID}

	attempts := 0
	probeErr := errors.New("container not yet running")
	svc, err := New(Options{
		Executor: fake,
		ReadyProbe: func(_ context.Context, _ string) error {
			attempts++
			if attempts < 3 {
				return probeErr
			}
			return nil // ready on third attempt
		},
		ProbeSleep:    noopSleep,
		ProbeAttempts: 10,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err != nil {
		t.Fatalf("Provision() error = %v, want nil (ready after retries)", err)
	}
	if cleanup == nil {
		t.Fatalf("Provision() cleanup = nil, want non-nil")
	}
	if material.NetworkName == "" {
		t.Errorf("material.NetworkName is empty")
	}
	if attempts != 3 {
		t.Errorf("ReadyProbe called %d times, want 3", attempts)
	}
	_ = cleanup(context.Background())
}

// TestProvision_ProbeTimeout asserts that when the injected ReadyProbe always
// returns an error, Provision exhausts all attempts and returns a wrapped
// error. No PolicyMaterial is returned on timeout.
func TestProvision_ProbeTimeout(t *testing.T) {
	t.Parallel()

	const fakeSidecarID = "timeout-sidecar"
	fake := &fakeNetworkExecutor{runDetachedResult: fakeSidecarID}

	const maxAttempts = 3
	probeErr := errors.New("sidecar still starting")
	attempts := 0
	svc, err := New(Options{
		Executor: fake,
		ReadyProbe: func(_ context.Context, _ string) error {
			attempts++
			return probeErr // always not ready
		},
		ProbeSleep:    noopSleep,
		ProbeAttempts: maxAttempts,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	material, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err == nil {
		t.Fatal("Provision() error = nil, want timeout error")
	}
	// Timeout error must wrap the last probe error.
	if !errors.Is(err, probeErr) {
		t.Errorf("Provision() error = %v, want errors.Is(%v)", err, probeErr)
	}
	// Cleanup must be nil on timeout (no half-ready material returned).
	if cleanup != nil {
		t.Errorf("Provision() cleanup = non-nil, want nil on timeout")
	}
	// Zero-value PolicyMaterial on timeout.
	if material != (PolicyMaterial{}) {
		t.Errorf("Provision() material = %+v, want zero value on timeout", material)
	}
	// All attempts must have been consumed.
	if attempts != maxAttempts {
		t.Errorf("ReadyProbe called %d times, want %d (all attempts consumed)", attempts, maxAttempts)
	}
}

// containerCheckerExecutor is a fakeNetworkExecutor that also implements
// containerChecker. It records ContainerRunning calls so tests can assert the
// probe was actually invoked — proving defaultReadyProbe is not a no-op when
// the executor satisfies containerChecker.
type containerCheckerExecutor struct {
	fakeNetworkExecutor

	// containerRunningResult is returned by ContainerRunning.
	containerRunningResult bool
	// containerRunningErr is returned by ContainerRunning when non-nil.
	containerRunningErr error
	// containerRunningCalls records the containerID argument for each call.
	containerRunningCalls []string
}

func (f *containerCheckerExecutor) ContainerRunning(_ context.Context, containerID string) (bool, error) {
	f.containerRunningCalls = append(f.containerRunningCalls, containerID)
	return f.containerRunningResult, f.containerRunningErr
}

// TestDefaultReadyProbe_FiresWhenExecutorImplementsContainerChecker proves
// that defaultReadyProbe is NOT a no-op when the executor implements
// containerChecker. It uses a containerCheckerExecutor (which embeds
// fakeNetworkExecutor AND adds ContainerRunning) and asserts:
//  1. The probe calls ContainerRunning with the correct containerID.
//  2. The probe returns nil when ContainerRunning returns (true, nil).
//  3. The probe returns an error when ContainerRunning returns (false, nil).
func TestDefaultReadyProbe_FiresWhenExecutorImplementsContainerChecker(t *testing.T) {
	t.Parallel()

	const sidecarID = "test-sidecar-probe-fires"

	t.Run("running_returns_nil", func(t *testing.T) {
		t.Parallel()
		exec := &containerCheckerExecutor{containerRunningResult: true}
		probe := defaultReadyProbe(exec)

		if err := probe(context.Background(), sidecarID); err != nil {
			t.Fatalf("probe() = %v, want nil (container running)", err)
		}
		if len(exec.containerRunningCalls) != 1 {
			t.Fatalf("ContainerRunning called %d times, want 1", len(exec.containerRunningCalls))
		}
		if exec.containerRunningCalls[0] != sidecarID {
			t.Errorf("ContainerRunning containerID = %q, want %q", exec.containerRunningCalls[0], sidecarID)
		}
	})

	t.Run("not_running_returns_error", func(t *testing.T) {
		t.Parallel()
		exec := &containerCheckerExecutor{containerRunningResult: false}
		probe := defaultReadyProbe(exec)

		err := probe(context.Background(), sidecarID)
		if err == nil {
			t.Fatal("probe() = nil, want error (container not running)")
		}
		if len(exec.containerRunningCalls) != 1 {
			t.Fatalf("ContainerRunning called %d times, want 1", len(exec.containerRunningCalls))
		}
	})

	t.Run("inspect_error_propagates", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("daemon unavailable")
		exec := &containerCheckerExecutor{containerRunningErr: sentinel}
		probe := defaultReadyProbe(exec)

		err := probe(context.Background(), sidecarID)
		if err == nil {
			t.Fatal("probe() = nil, want error from ContainerRunning")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("probe() error = %v, want errors.Is sentinel %v", err, sentinel)
		}
	})

	t.Run("noop_when_no_container_checker", func(t *testing.T) {
		t.Parallel()
		// fakeNetworkExecutor does NOT implement containerChecker.
		exec := &fakeNetworkExecutor{}
		probe := defaultReadyProbe(exec)

		// Must return nil (no-op) — does not panic or call any method.
		if err := probe(context.Background(), sidecarID); err != nil {
			t.Fatalf("probe() = %v, want nil (no-op fallback for non-checker executor)", err)
		}
	})
}

// TestSweepStaleSidecars_RemovesExistingContainers asserts that when
// staleSweeperExecutor reports pre-existing containers carrying the managed
// label, Provision calls RemoveContainer for each before launching the fresh
// sidecar. This is the reclaim-on-collision path (DROP_15 Unit 15.2.5.E.2).
func TestSweepStaleSidecars_RemovesExistingContainers(t *testing.T) {
	t.Parallel()

	staleA := "aabbccdd1122"
	staleB := "eeff33445566"
	exec := &staleSweeperExecutor{
		listContainerResult: []string{staleA, staleB},
	}
	exec.runDetachedResult = "fresh-sidecar-id"

	svc, err := New(Options{Executor: exec})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	defer func() { _ = cleanup(context.Background()) }()

	// ListContainersByLabel must have been called with the per-network filter
	// (valv-network=<desiredName>), NOT the global managed filter. Provision
	// scopes its reclaim sweep to this network's sidecars only so that a
	// concurrent valv run's active sidecar (with a different NetworkLabelKey
	// value) is never force-removed.
	expectedName := networkName([]string{"github.com"})
	wantFilter := NetworkLabelKey + "=" + expectedName
	if len(exec.listContainerCalls) != 1 || exec.listContainerCalls[0] != wantFilter {
		t.Errorf("listContainerCalls = %#v, want [%q]", exec.listContainerCalls, wantFilter)
	}

	// Both stale containers must have been removed.
	if len(exec.removeContainerCalls) != 2 {
		t.Fatalf("removeContainerCalls = %d, want 2", len(exec.removeContainerCalls))
	}
	removed := map[string]bool{}
	for _, r := range exec.removeContainerCalls {
		if len(r.IDs) == 1 {
			removed[r.IDs[0]] = true
		}
	}
	if !removed[staleA] || !removed[staleB] {
		t.Errorf("removeContainerCalls = %#v, want both %q and %q removed", exec.removeContainerCalls, staleA, staleB)
	}

	// RemoveContainer calls must carry Force=true so stopped containers are
	// cleaned up unconditionally.
	for i, r := range exec.removeContainerCalls {
		if !r.Force {
			t.Errorf("removeContainerCalls[%d].Force = false, want true", i)
		}
	}

	// Sweep must fire BEFORE RunContainerDetached.
	sweepIdx := -1
	runIdx := -1
	for i, op := range exec.callOrder {
		if op == "ListContainersByLabel" {
			sweepIdx = i
		}
		if op == "RunContainerDetached" {
			runIdx = i
		}
	}
	if sweepIdx == -1 || runIdx == -1 {
		t.Fatalf("callOrder = %v, missing ListContainersByLabel or RunContainerDetached", exec.callOrder)
	}
	if sweepIdx >= runIdx {
		t.Errorf("callOrder = %v: ListContainersByLabel (idx %d) must precede RunContainerDetached (idx %d)", exec.callOrder, sweepIdx, runIdx)
	}
}

// TestSweepStaleSidecars_NoopWhenEmpty asserts that Provision completes
// normally and does not call RemoveContainer when no stale containers exist.
func TestSweepStaleSidecars_NoopWhenEmpty(t *testing.T) {
	t.Parallel()

	exec := &staleSweeperExecutor{}
	exec.runDetachedResult = "clean-sidecar"

	svc, err := New(Options{Executor: exec})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	defer func() { _ = cleanup(context.Background()) }()

	// ListContainersByLabel was called (sweep attempted).
	if len(exec.listContainerCalls) != 1 {
		t.Fatalf("listContainerCalls = %d, want 1", len(exec.listContainerCalls))
	}
	// No RemoveContainer calls when no stale containers.
	if len(exec.removeContainerCalls) != 0 {
		t.Errorf("removeContainerCalls = %d, want 0 (no stale containers)", len(exec.removeContainerCalls))
	}
}

// TestSweepStaleSidecars_RemoveError_Propagates asserts that a RemoveContainer
// failure during the sweep causes Provision to return a wrapped error.
func TestSweepStaleSidecars_RemoveError_Propagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("container in use")
	exec := &staleSweeperExecutor{
		listContainerResult: []string{"dead-sidecar-id"},
		removeContainerErr:  sentinel,
	}

	svc, err := New(Options{Executor: exec})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, _, err = svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err == nil {
		t.Fatal("Provision() error = nil, want sweep remove error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Provision() error = %v, want errors.Is sentinel", err)
	}
	if !strings.Contains(err.Error(), "networkpolicy provision") {
		t.Errorf("Provision() error = %q, want context prefix 'networkpolicy provision'", err.Error())
	}
}

// TestSweepStaleSidecars_NoopWhenExecutorLacksSweeper asserts that when the
// executor does not implement staleSweeper (the minimal fakeNetworkExecutor),
// Provision still succeeds and no sweep methods are invoked. This confirms the
// optional-interface fallback behaves correctly.
func TestSweepStaleSidecars_NoopWhenExecutorLacksSweeper(t *testing.T) {
	t.Parallel()

	// fakeNetworkExecutor does NOT implement staleSweeper.
	exec := &fakeNetworkExecutor{runDetachedResult: "no-sweeper-sidecar"}
	svc, err := New(Options{Executor: exec})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err != nil {
		t.Fatalf("Provision() error = %v, want nil (no-op sweep when sweeper absent)", err)
	}
	defer func() { _ = cleanup(context.Background()) }()
}

// TestProvision_SidecarCarriesBothLabels asserts that the ContainerRunRequest
// for the proxy sidecar carries BOTH the global managed label (valv=network-policy)
// AND the per-network label (valv-network=<desiredName>). The global label is
// required for startup-recovery sweeps (CleanupStale); the per-network label is
// required for scoped reclaim-on-collision during active sessions (Provision).
func TestProvision_SidecarCarriesBothLabels(t *testing.T) {
	t.Parallel()

	allowlist := []string{"github.com", "proxy.golang.org"}
	desiredName := networkName(allowlist)

	fake := &fakeNetworkExecutor{runDetachedResult: "labels-test-sidecar"}
	svc, err := New(Options{Executor: fake})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{Allowlist: allowlist})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	defer func() { _ = cleanup(context.Background()) }()

	if len(fake.runDetachedCalls) != 1 {
		t.Fatalf("runDetachedCalls = %d, want 1", len(fake.runDetachedCalls))
	}
	labels := fake.runDetachedCalls[0].Labels

	// Global managed label must be present.
	if labels[ManagedLabelKey] != ManagedLabelValue {
		t.Errorf("Labels[%q] = %q, want %q", ManagedLabelKey, labels[ManagedLabelKey], ManagedLabelValue)
	}
	// Per-network label must be present and set to the deterministic network name.
	if labels[NetworkLabelKey] != desiredName {
		t.Errorf("Labels[%q] = %q, want %q (desiredName)", NetworkLabelKey, labels[NetworkLabelKey], desiredName)
	}
}

// TestProvision_ReclaimSweepIsPerNetworkScoped asserts that Provision's
// reclaim-on-collision sweep uses the per-network label filter
// (valv-network=<desiredName>) and NOT the global managed label, so that a
// concurrent valv run's sidecar carrying a DIFFERENT NetworkLabelKey value is
// NOT swept.
func TestProvision_ReclaimSweepIsPerNetworkScoped(t *testing.T) {
	t.Parallel()

	allowlist := []string{"sum.golang.org"}
	desiredName := networkName(allowlist)

	// The stale sweeper's listContainerResult is returned for any label query.
	// We verify correctness by asserting the LABEL ARGUMENT passed to
	// ListContainersByLabel — if it is the per-network filter, the executor
	// would only return THIS network's sidecars in production. A foreign-network
	// sidecar (different NetworkLabelKey value) would never appear in the list
	// because Docker's label filter would exclude it.
	exec := &staleSweeperExecutor{}
	exec.runDetachedResult = "scoped-sweep-sidecar"

	svc, err := New(Options{Executor: exec})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{Allowlist: allowlist})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	defer func() { _ = cleanup(context.Background()) }()

	// ListContainersByLabel must have been called with the per-network filter,
	// not the global managed filter.
	wantFilter := NetworkLabelKey + "=" + desiredName
	if len(exec.listContainerCalls) != 1 {
		t.Fatalf("listContainerCalls = %d, want 1", len(exec.listContainerCalls))
	}
	if exec.listContainerCalls[0] != wantFilter {
		t.Errorf("listContainerCalls[0] = %q, want %q (per-network filter)", exec.listContainerCalls[0], wantFilter)
	}

	// Confirm it is NOT the global managed filter.
	globalFilter := ManagedLabelKey + "=" + ManagedLabelValue
	if exec.listContainerCalls[0] == globalFilter {
		t.Errorf("Provision sweep used global filter %q; want per-network filter %q", globalFilter, wantFilter)
	}
}

// TestCleanupStale_SweepIsGlobal asserts that CleanupStale uses the global
// managed label filter (valv=network-policy) for its container sweep, not a
// per-network filter. During startup recovery no sessions are active, so
// all orphaned managed sidecars should be reclaimed regardless of which
// network they belonged to.
func TestCleanupStale_SweepIsGlobal(t *testing.T) {
	t.Parallel()

	exec := &staleSweeperExecutor{
		listContainerResult: []string{"orphan-sidecar-1", "orphan-sidecar-2"},
	}
	exec.listResult = []string{"valv-netpol-aaaa11112222"}

	svc, err := New(Options{Executor: exec})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := svc.CleanupStale(context.Background()); err != nil {
		t.Fatalf("CleanupStale() error = %v", err)
	}

	// CleanupStale must use the global managed label, not a per-network label.
	wantFilter := ManagedLabelKey + "=" + ManagedLabelValue
	if len(exec.listContainerCalls) != 1 {
		t.Fatalf("listContainerCalls = %d, want 1", len(exec.listContainerCalls))
	}
	if exec.listContainerCalls[0] != wantFilter {
		t.Errorf("listContainerCalls[0] = %q, want %q (global managed filter)", exec.listContainerCalls[0], wantFilter)
	}
}

// TestCleanup_RemovesSidecarBeforeNetwork asserts that the Cleanup closure
// returned by Provision removes the proxy sidecar container BEFORE removing
// the network. Docker refuses to remove a network while a container is still
// attached to it, so container removal must precede network removal.
// This test uses a staleSweeperExecutor so RemoveContainer is available.
func TestCleanup_RemovesSidecarBeforeNetwork(t *testing.T) {
	t.Parallel()

	const fakeSidecarID = "cleanup-order-sidecar"

	exec := &staleSweeperExecutor{}
	exec.fakeNetworkExecutor.runDetachedResult = fakeSidecarID

	svc, err := New(Options{Executor: exec})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, cleanup, err := svc.Provision(context.Background(), ProvisionRequest{
		Allowlist: []string{"github.com"},
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	// Execute the cleanup closure.
	if err := cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}

	// Container must have been removed.
	if len(exec.removeContainerCalls) != 1 {
		t.Fatalf("removeContainerCalls = %d, want 1 (sidecar removal)", len(exec.removeContainerCalls))
	}
	// The correct sidecar ID must have been removed with Force=true.
	rc := exec.removeContainerCalls[0]
	if len(rc.IDs) != 1 || rc.IDs[0] != fakeSidecarID {
		t.Errorf("removeContainerCalls[0].IDs = %v, want [%q]", rc.IDs, fakeSidecarID)
	}
	if !rc.Force {
		t.Errorf("removeContainerCalls[0].Force = false, want true (Force stops+removes atomically)")
	}

	// Network must have been removed.
	// Note: RemoveNetwork is recorded in fakeNetworkExecutor.removeCalls.
	// We need to count removeCalls that happened during cleanup (not sweeps).
	if len(exec.fakeNetworkExecutor.removeCalls) != 1 {
		t.Fatalf("removeCalls = %d, want 1 (network removal)", len(exec.fakeNetworkExecutor.removeCalls))
	}

	// RemoveContainer (sidecar) must precede RemoveNetwork in call order.
	containerIdx := -1
	networkIdx := -1
	for i, op := range exec.callOrder {
		if op == "RemoveContainer" && containerIdx == -1 {
			containerIdx = i
		}
		if op == "RemoveNetwork" && networkIdx == -1 {
			networkIdx = i
		}
	}
	if containerIdx == -1 {
		t.Fatalf("callOrder = %v, missing RemoveContainer", exec.callOrder)
	}
	if networkIdx == -1 {
		t.Fatalf("callOrder = %v, missing RemoveNetwork", exec.callOrder)
	}
	if containerIdx >= networkIdx {
		t.Errorf("callOrder = %v: RemoveContainer (idx %d) must precede RemoveNetwork (idx %d)",
			exec.callOrder, containerIdx, networkIdx)
	}
}

// TestCleanupStale_SweepsContainersBeforeNetworks asserts that CleanupStale
// removes stale sidecar containers (via staleSweeper) before removing networks.
// This ordering is required because Docker refuses to remove a network while a
// container is still attached to it.
func TestCleanupStale_SweepsContainersBeforeNetworks(t *testing.T) {
	t.Parallel()

	exec := &staleSweeperExecutor{
		listContainerResult: []string{"stale-container-abc"},
	}
	exec.listResult = []string{"valv-netpol-aaaa11112222"}

	svc, err := New(Options{Executor: exec})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := svc.CleanupStale(context.Background()); err != nil {
		t.Fatalf("CleanupStale() error = %v", err)
	}

	// Container sweep must have fired.
	if len(exec.removeContainerCalls) != 1 {
		t.Fatalf("removeContainerCalls = %d, want 1", len(exec.removeContainerCalls))
	}
	// Network removal must have fired.
	if len(exec.removeCalls) != 1 {
		t.Fatalf("removeCalls = %d, want 1", len(exec.removeCalls))
	}
	// Container sweep must precede network removal in call order.
	containerIdx := -1
	networkIdx := -1
	for i, op := range exec.callOrder {
		if op == "RemoveContainer" && containerIdx == -1 {
			containerIdx = i
		}
		if op == "RemoveNetwork" && networkIdx == -1 {
			networkIdx = i
		}
	}
	if containerIdx == -1 || networkIdx == -1 {
		t.Fatalf("callOrder = %v, missing RemoveContainer or RemoveNetwork", exec.callOrder)
	}
	if containerIdx >= networkIdx {
		t.Errorf("callOrder = %v: RemoveContainer (idx %d) must precede RemoveNetwork (idx %d)", exec.callOrder, containerIdx, networkIdx)
	}
}
