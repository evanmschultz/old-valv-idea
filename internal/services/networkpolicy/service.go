// Package networkpolicy is the shared seam that produces closed-default
// network policy material for Valv runtime and image-build callers.
//
// DROP_15 Unit 15.2.5 introduces the service. It owns three pieces:
//
//  1. Network lifecycle. Provision creates a single Docker network with the
//     --internal flag and a deterministic Valv label so the network can be
//     reclaimed across restarts. The network is the only network the closed-
//     default workload container attaches to; per Schema Decision 5 the proxy
//     sidecar spans both the internal network and the default bridge.
//  2. Sidecar launch. Provision starts a valv-proxy sidecar container on the
//     default bridge network (so it can egress) and then connects it to the
//     internal network with a stable DNS alias (ProxyAlias). The workload
//     reaches the proxy by DNS name on the shared internal subnet.
//  3. Policy material. The PolicyMaterial returned by Provision carries the
//     proxy URLs, NO_PROXY value, and resolved network name that callers
//     thread into ImageBuildRequest / ContainerRunRequest. The material is
//     stable for the life of the provisioned sidecar + network.
//
// Orphan cleanup is handled on every Provision call: networks labeled
// valv=network-policy from prior runs are reclaimed (same name) or removed
// (different name / stale). Unit 15.2.5.E adds container-level cleanup.
package networkpolicy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/evanmschultz/valv/internal/adapters/docker"
)

// Label keys/values applied to managed Docker networks so the orphan-cleanup
// path can reclaim them after a SIGKILLed run.
const (
	// ManagedLabelKey is the docker network label key Valv stamps onto every
	// network it provisions for closed-default network policy.
	ManagedLabelKey = "valv"
	// ManagedLabelValue is the label value paired with ManagedLabelKey. The
	// (key,value) pair together identifies a Valv-managed network-policy
	// resource and gates the orphan reclaim flow.
	ManagedLabelValue = "network-policy"
	// networkNamePrefix is prepended to the deterministic suffix when
	// generating the docker network name. Keeping the prefix human-readable
	// makes `docker network ls` output easier to scan for operators.
	networkNamePrefix = "valv-netpol-"
	// ProxyAlias is the DNS alias assigned to the proxy sidecar container on
	// the internal network. The workload resolves this name to reach the
	// sidecar on the shared internal subnet. Using a named constant ensures
	// buildNoProxy (NO_PROXY exclusion) and the ConnectNetwork alias are
	// always identical — they can never diverge.
	ProxyAlias = "valv-proxy"
	// proxyPort is the port the valv-proxy sidecar listens on.
	proxyPort = "8080"
)

// NetworkExecutor is the subset of docker.Executor surface area the
// networkpolicy service depends on. Defined consumer-side so tests can
// inject a fake without standing up a real docker.Executor.
type NetworkExecutor interface {
	CreateNetwork(ctx context.Context, request docker.NetworkCreateRequest) error
	RemoveNetwork(ctx context.Context, request docker.NetworkRemoveRequest) error
	// ListNetworks returns the names of networks matching the supplied label
	// filter (key=value). The returned slice may be empty when no matches
	// exist; errors from the underlying docker invocation are wrapped.
	ListNetworks(ctx context.Context, label string) ([]string, error)
	// RunContainerDetached launches a container in detached mode and returns
	// the container ID from Docker's stdout. Used to start the proxy sidecar
	// container (Schema Decision 5). The request must have Detached set true.
	RunContainerDetached(ctx context.Context, request docker.ContainerRunRequest) (string, error)
	// ConnectNetwork attaches a running container to an additional network
	// with optional aliases. Used to connect the proxy sidecar to the bridge
	// network so it can reach the internet (Schema Decision 5).
	ConnectNetwork(ctx context.Context, request docker.NetworkConnectRequest) error
}

// Options configures a Service.
type Options struct {
	// Executor is the docker network executor. Required.
	Executor NetworkExecutor
}

// Service provisions and cleans up the docker network plus policy material
// required by closed-default network policy callers.
type Service struct {
	executor NetworkExecutor
}

// New constructs a Service from the supplied Options. Returns a non-nil
// error when required fields are missing.
func New(opts Options) (Service, error) {
	if opts.Executor == nil {
		return Service{}, fmt.Errorf("new networkpolicy service: executor is required")
	}
	return Service{executor: opts.Executor}, nil
}

// ProvisionRequest carries the inputs needed to provision policy material.
type ProvisionRequest struct {
	// Allowlist is the effective allowlist hosts (built-in defaults unioned
	// with user-declared hosts). Callers compute this via
	// tools.EffectiveAllowlist before invoking Provision. The service passes
	// this to the network-name derivation (deterministic network naming) and
	// to the VALV_PROXY_ALLOWLIST env var on the sidecar container. It does
	// NOT place allowlist entries in NO_PROXY — the sidecar proxy enforces
	// the allowlist internally (see buildNoProxy for rationale).
	//
	// At least one allowlist entry is required so that the proxy sidecar has
	// a defined set of hosts to permit; a zero-length allowlist would make
	// the proxy block all egress. Callers that want unrestricted egress should
	// run in open mode instead of invoking Provision.
	Allowlist []string
}

// Valid reports any input violations for a ProvisionRequest.
func (r ProvisionRequest) Valid() error {
	if len(r.Allowlist) == 0 {
		return fmt.Errorf("validate provision request: allowlist is required (use open mode for unrestricted egress)")
	}
	for _, host := range r.Allowlist {
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("validate provision request: allowlist entry is empty")
		}
	}
	return nil
}

// PolicyMaterial carries the runtime/build inputs derived from a successful
// Provision call. The struct is read-only; callers pass HTTPProxyURL /
// HTTPSProxyURL / NoProxy as build args or env vars, and NetworkName into
// docker.ImageBuildRequest.Network / docker.ContainerRunRequest.Network.
type PolicyMaterial struct {
	// HTTPProxyURL is the value callers pass as the HTTP_PROXY build arg or
	// env var. Format: "http://<ProxyAlias>:<proxyPort>" (e.g. "http://valv-proxy:8080").
	HTTPProxyURL string
	// HTTPSProxyURL is the value callers pass as the HTTPS_PROXY build arg
	// or env var. Format: "http://<ProxyAlias>:<proxyPort>" — the proxy daemon
	// receives CONNECT for HTTPS targets over plain HTTP.
	HTTPSProxyURL string
	// NoProxy is the comma-separated value suitable for use as the NO_PROXY
	// build arg or env var. It contains ONLY loopback addresses
	// (localhost, 127.0.0.1) and the sidecar proxy alias (valv-proxy).
	// Allowlist hosts are intentionally absent: the workload routes all
	// HTTP/HTTPS egress through the sidecar, which enforces the allowlist
	// internally. Including allowlist hosts in NO_PROXY would cause clients
	// to attempt direct connections that fail because the internal-only
	// network has no route to the outside.
	NoProxy string
	// NetworkName is the docker network name the container/build attaches
	// to. Always non-empty on a successful Provision.
	NetworkName string
}

// Cleanup undoes the resources Provision allocated. Removing the managed
// network is idempotent — if the network is already gone, Cleanup returns
// nil. Callers should typically defer Cleanup at the point Provision
// succeeds.
type Cleanup func(ctx context.Context) error

// Provision sets up policy material for the supplied request:
//
//  1. Validate the request.
//  2. List existing networks labeled valv=network-policy. If exactly one
//     matches the deterministic name for this allowlist, reuse it
//     (idempotent reclaim). Otherwise remove every label-matched network
//     before creating a fresh one (orphan cleanup).
//  3. Create the network with --internal and the managed label.
//  4. Launch the proxy sidecar container on the default bridge network
//     (so it has external egress) with the allowlist in VALV_PROXY_ALLOWLIST.
//  5. Connect the proxy sidecar to the internal network with ProxyAlias as
//     the DNS alias, so the workload can resolve the sidecar by name.
//  6. Return the PolicyMaterial plus a Cleanup that removes the network.
//
// The workload container is NOT attached here; callers (Unit 15.3.C) attach
// the workload to the internal network only after Provision returns.
//
// The returned PolicyMaterial is always non-zero on success. The returned
// Cleanup is never nil on success; callers may safely invoke it on the
// happy and error paths alike (the only error path that returns a non-nil
// Cleanup is when Provision succeeds — callers see (material, cleanup, nil)
// or (zero, nil, err)).
func (s Service) Provision(ctx context.Context, request ProvisionRequest) (PolicyMaterial, Cleanup, error) {
	if err := request.Valid(); err != nil {
		return PolicyMaterial{}, nil, err
	}

	desiredName := networkName(request.Allowlist)
	managedFilter := ManagedLabelKey + "=" + ManagedLabelValue

	existing, err := s.executor.ListNetworks(ctx, managedFilter)
	if err != nil {
		return PolicyMaterial{}, nil, fmt.Errorf("networkpolicy provision: list managed networks: %w", err)
	}

	reused := false
	for _, name := range existing {
		if name == desiredName {
			reused = true
			continue
		}
		// Stale orphan from a prior run with a different allowlist (or a
		// pre-rename network). Remove unconditionally; the managed label is
		// owned by this service and any value carrying it is fair game.
		if rmErr := s.executor.RemoveNetwork(ctx, docker.NetworkRemoveRequest{Name: name}); rmErr != nil {
			return PolicyMaterial{}, nil, fmt.Errorf("networkpolicy provision: remove stale network %q: %w", name, rmErr)
		}
	}

	if !reused {
		createReq := docker.NetworkCreateRequest{
			Name:     desiredName,
			Internal: true,
			Labels: map[string]string{
				ManagedLabelKey: ManagedLabelValue,
			},
		}
		if err := s.executor.CreateNetwork(ctx, createReq); err != nil {
			return PolicyMaterial{}, nil, fmt.Errorf("networkpolicy provision: create network %q: %w", desiredName, err)
		}
	}

	// Launch the proxy sidecar on the default bridge network. The bridge
	// attachment gives the sidecar external egress while the internal network
	// attachment (below) lets the workload reach it by ProxyAlias. Schema
	// Decision 5: ONLY the proxy spans bridge; the workload is internal-only.
	sidecarID, err := s.executor.RunContainerDetached(ctx, docker.ContainerRunRequest{
		Image:    proxyImageRef(),
		Detached: true,
		Env: map[string]string{
			"VALV_PROXY_ALLOWLIST": strings.Join(request.Allowlist, ","),
			"VALV_PROXY_ADDR":      ":" + proxyPort,
		},
		Labels: map[string]string{
			ManagedLabelKey: ManagedLabelValue,
		},
		// No Network field: defaults to Docker's bridge network, giving the
		// sidecar external egress per Schema Decision 5.
	})
	if err != nil {
		return PolicyMaterial{}, nil, fmt.Errorf("networkpolicy provision: start proxy sidecar: %w", err)
	}

	// Connect the sidecar to the internal network with a stable DNS alias.
	// The workload (attached to internal-only by 15.3.C) resolves ProxyAlias
	// to the sidecar IP on the shared internal subnet.
	if err := s.executor.ConnectNetwork(ctx, docker.NetworkConnectRequest{
		Network:   desiredName,
		Container: sidecarID,
		Aliases:   []string{ProxyAlias},
	}); err != nil {
		return PolicyMaterial{}, nil, fmt.Errorf("networkpolicy provision: connect proxy sidecar to internal network: %w", err)
	}

	proxyURL := "http://" + ProxyAlias + ":" + proxyPort
	material := PolicyMaterial{
		HTTPProxyURL:  proxyURL,
		HTTPSProxyURL: proxyURL,
		NoProxy:       buildNoProxy(),
		NetworkName:   desiredName,
	}
	cleanup := func(ctx context.Context) error {
		if err := s.executor.RemoveNetwork(ctx, docker.NetworkRemoveRequest{Name: desiredName}); err != nil {
			return fmt.Errorf("networkpolicy cleanup: remove network %q: %w", desiredName, err)
		}
		return nil
	}
	return material, cleanup, nil
}

// CleanupStale removes every Valv-managed network without provisioning a
// replacement. Callers use this during startup recovery when the runtime
// has not yet decided whether closed mode is active.
func (s Service) CleanupStale(ctx context.Context) error {
	managedFilter := ManagedLabelKey + "=" + ManagedLabelValue
	existing, err := s.executor.ListNetworks(ctx, managedFilter)
	if err != nil {
		return fmt.Errorf("networkpolicy cleanup stale: list managed networks: %w", err)
	}
	for _, name := range existing {
		if rmErr := s.executor.RemoveNetwork(ctx, docker.NetworkRemoveRequest{Name: name}); rmErr != nil {
			return fmt.Errorf("networkpolicy cleanup stale: remove network %q: %w", name, rmErr)
		}
	}
	return nil
}

// networkName returns a deterministic docker network name for the supplied
// allowlist. The name combines a human-readable prefix with the first 12
// hex chars of a sha256 over the sorted allowlist. Deterministic naming
// drives the idempotent-reclaim branch of Provision.
func networkName(allowlist []string) string {
	cp := make([]string, len(allowlist))
	copy(cp, allowlist)
	sort.Strings(cp)
	h := sha256.Sum256([]byte(strings.Join(cp, ",")))
	return networkNamePrefix + hex.EncodeToString(h[:6])
}

// buildNoProxy returns the NO_PROXY value for a closed-default workload
// container. It contains only loopback exclusions and the sidecar proxy
// alias (ProxyAlias) so that:
//
//   - The sidecar itself is reached directly (no proxy-through-proxy loop).
//   - Loopback traffic bypasses the proxy (standard no-proxy semantics).
//   - ALL other egress — including allowlist hosts — flows through the
//     sidecar, which enforces the allowlist internally.
//
// Allowlist hosts must NEVER appear in NO_PROXY: the internal-only network
// has no direct route to the outside, so a client that bypasses the sidecar
// for an allowlist host would get a connection failure instead of egress.
//
// ProxyAlias is used here rather than the literal "valv-proxy" string so
// that the NO_PROXY exclusion and the ConnectNetwork alias can never diverge.
func buildNoProxy() string {
	entries := []string{"127.0.0.1", "localhost", ProxyAlias}
	sort.Strings(entries)
	return strings.Join(entries, ",")
}
