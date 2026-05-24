package docker

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// dockerNetworkNamePattern mirrors Docker's accepted network name shape:
// must begin with an ASCII letter or digit, followed by letters, digits,
// hyphens, underscores, or dots. Reject leading hyphens/dots and any
// whitespace or path separators. Source: docker network create reference.
var dockerNetworkNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// NetworkCreateRequest carries the inputs for `docker network create`.
//
// DROP_15 uses this exclusively to provision a single `--internal` network
// per Schema Decision 5.
type NetworkCreateRequest struct {
	// Name is the Docker network name. Required.
	Name string
	// Internal sets the `--internal` flag when true, blocking external egress
	// for containers attached only to this network (per Docker docs).
	Internal bool
	// Labels are passed through as `--label key=value` in deterministic key order.
	Labels map[string]string
}

// Valid reports any input violations for a NetworkCreateRequest.
func (r NetworkCreateRequest) Valid() error {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return fmt.Errorf("validate network create request: name is required")
	}
	if !dockerNetworkNamePattern.MatchString(name) {
		return fmt.Errorf("validate network create request: invalid network name %q", name)
	}
	if len(name) > 64 {
		return fmt.Errorf("validate network create request: network name must be at most 64 bytes, got %d", len(name))
	}
	for key := range r.Labels {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("validate network create request: label key is required")
		}
		// Reject keys that differ when trimmed (contain surrounding whitespace).
		if strings.TrimSpace(key) != key {
			return fmt.Errorf("validate network create request: label key must not have leading or trailing whitespace")
		}
		// Reject keys containing '=' which would break Docker's label parsing.
		if strings.ContainsRune(key, '=') {
			return fmt.Errorf("validate network create request: label key must not contain '='")
		}
	}
	return nil
}

// BuildNetworkCreateArgs renders a deterministic `docker network create` arg
// vector for the supplied request. Args are ordered: flags first (`--internal`
// then sorted labels), positional name last.
func BuildNetworkCreateArgs(request NetworkCreateRequest) ([]string, error) {
	if err := request.Valid(); err != nil {
		return nil, err
	}

	args := []string{"network", "create"}
	if request.Internal {
		args = append(args, "--internal")
	}

	if len(request.Labels) > 0 {
		keys := make([]string, 0, len(request.Labels))
		for key := range request.Labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args = append(args, "--label", fmt.Sprintf("%s=%s", key, request.Labels[key]))
		}
	}

	args = append(args, strings.TrimSpace(request.Name))
	return args, nil
}

// NetworkRemoveRequest carries the inputs for `docker network rm`.
type NetworkRemoveRequest struct {
	// Name is the Docker network name to remove. Required.
	Name string
}

// Valid reports any input violations for a NetworkRemoveRequest.
func (r NetworkRemoveRequest) Valid() error {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return fmt.Errorf("validate network remove request: name is required")
	}
	if !dockerNetworkNamePattern.MatchString(name) {
		return fmt.Errorf("validate network remove request: invalid network name %q", name)
	}
	if len(name) > 64 {
		return fmt.Errorf("validate network remove request: network name must be at most 64 bytes, got %d", len(name))
	}
	return nil
}

// BuildNetworkRemoveArgs renders a deterministic `docker network rm` arg
// vector for the supplied request.
func BuildNetworkRemoveArgs(request NetworkRemoveRequest) ([]string, error) {
	if err := request.Valid(); err != nil {
		return nil, err
	}
	return []string{"network", "rm", strings.TrimSpace(request.Name)}, nil
}

// NetworkConnectRequest carries the inputs for `docker network connect`.
//
// The sidecar-proxy topology (Schema Decision 5) uses this to attach a
// container to multiple networks with a stable alias on the internal network.
type NetworkConnectRequest struct {
	// Network is the Docker network name to connect to. Required.
	Network string
	// Container is the container ID or name to connect. Required.
	Container string
	// Aliases are network aliases (DNS names) for the container on this network.
	// Optional. Each alias must be non-empty and pattern-valid.
	Aliases []string
}

// Valid reports any input violations for a NetworkConnectRequest.
func (r NetworkConnectRequest) Valid() error {
	network := strings.TrimSpace(r.Network)
	if network == "" {
		return fmt.Errorf("validate network connect request: network is required")
	}
	if !dockerNetworkNamePattern.MatchString(network) {
		return fmt.Errorf("validate network connect request: invalid network name %q", network)
	}
	if len(network) > 64 {
		return fmt.Errorf("validate network connect request: network name must be at most 64 bytes, got %d", len(network))
	}

	container := strings.TrimSpace(r.Container)
	if container == "" {
		return fmt.Errorf("validate network connect request: container is required")
	}

	for i, alias := range r.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			return fmt.Errorf("validate network connect request: alias %d is empty", i)
		}
		if !dockerNetworkNamePattern.MatchString(alias) {
			return fmt.Errorf("validate network connect request: invalid alias %q at index %d", alias, i)
		}
	}

	return nil
}

// BuildNetworkConnectArgs renders a deterministic `docker network connect` arg
// vector for the supplied request. Aliases are sorted for determinism; the
// positional network and container names are last.
func BuildNetworkConnectArgs(request NetworkConnectRequest) ([]string, error) {
	if err := request.Valid(); err != nil {
		return nil, err
	}

	args := []string{"network", "connect"}

	// Sort aliases for deterministic output.
	if len(request.Aliases) > 0 {
		sortedAliases := make([]string, len(request.Aliases))
		copy(sortedAliases, request.Aliases)
		sort.Strings(sortedAliases)
		for _, alias := range sortedAliases {
			alias = strings.TrimSpace(alias)
			args = append(args, "--alias", alias)
		}
	}

	// Positional network and container names last.
	args = append(args, strings.TrimSpace(request.Network), strings.TrimSpace(request.Container))
	return args, nil
}
