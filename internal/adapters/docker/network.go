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
// per Schema Decision 5; the bridge fallback was cut and there is no
// `NetworkConnectRequest` symbol in this drop.
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
	for key := range r.Labels {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("validate network create request: label key is required")
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
