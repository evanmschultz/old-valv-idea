//go:build integration

package images

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evanmschultz/valv/internal/adapters/docker"
	"github.com/evanmschultz/valv/internal/domain"
	"github.com/evanmschultz/valv/internal/services/networkpolicy"
	"github.com/evanmschultz/valv/internal/tools"
)

// testClaudeCLIVersion is defined in service_test.go (same package, shared
// across both build tags). Do not redeclare here.

func TestServiceBuildRealDockerImage(t *testing.T) {
	t.Parallel()

	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}

	root := t.TempDir()
	dockerfile := filepath.Join(root, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.20\nARG CODEX_VERSION\nLABEL valv.codex.version=$CODEX_VERSION\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := docker.NewSystemRunner("docker", nil, &stdout, &stderr)
	svc, err := New(Options{
		Runner:     runner,
		Repository: "valv-test/codex",
		ContextDir: root,
		Dockerfile: "Dockerfile",
		DefaultTag: "it",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{Version: "0.116.0"})
	if err != nil {
		t.Fatalf("Build() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	t.Cleanup(func() {
		_ = runner.Run(context.Background(), []string{"image", "rm", "--force", result.Image.String()})
	})

	if err := runner.Run(context.Background(), []string{"image", "inspect", result.Image.String()}); err != nil {
		t.Fatalf("inspect built image: %v", err)
	}
}

func TestWriteDefaultCodexContextBuildsWithExistingUIDAndGID(t *testing.T) {
	t.Parallel()

	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}

	root := t.TempDir()
	if _, err := WriteDefaultCodexContext(root); err != nil {
		t.Fatalf("WriteDefaultCodexContext() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := docker.NewSystemRunner("docker", nil, &stdout, &stderr)
	svc, err := New(Options{
		Runner:     runner,
		Repository: "valv-test/codex-default",
		ContextDir: root,
		Dockerfile: "Dockerfile",
		DefaultTag: "it",
		UserID:     1000,
		GroupID:    1000,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{Version: "0.116.0"})
	if err != nil {
		t.Fatalf("Build() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	t.Cleanup(func() {
		_ = runner.Run(context.Background(), []string{"image", "rm", "--force", result.Image.String()})
	})

	if err := runner.Run(context.Background(), []string{"image", "inspect", result.Image.String()}); err != nil {
		t.Fatalf("inspect built image: %v", err)
	}
}

func TestWriteDefaultClaudeContextBuildsWithExistingUIDAndGID(t *testing.T) {
	t.Parallel()

	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}

	root := t.TempDir()
	if _, err := WriteDefaultClaudeContext(root); err != nil {
		t.Fatalf("WriteDefaultClaudeContext() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := docker.NewSystemRunner("docker", nil, &stdout, &stderr)
	svc, err := New(Options{
		Runner:     runner,
		Provider:   domain.ProviderClaude,
		Repository: "valv-test/claude-default",
		ContextDir: root,
		Dockerfile: "Dockerfile",
		DefaultTag: "it",
		UserID:     1000,
		GroupID:    1000,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := svc.Build(context.Background(), BuildRequest{Version: testClaudeCLIVersion})
	if err != nil {
		t.Fatalf("Build() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	t.Cleanup(func() {
		_ = runner.Run(context.Background(), []string{"image", "rm", "--force", result.Image.String()})
	})

	if err := runner.Run(context.Background(), []string{"image", "inspect", result.Image.String()}); err != nil {
		t.Fatalf("inspect built image: %v", err)
	}
}

// TestEnsureProjectImage_NetworkPolicyOverlayBuildReachesProxy_DockerDesktopMacOS
// is the Unit 15.2.5.G integration gate: it proves that when NetworkPolicy is
// configured on images.Service, the overlay `docker buildx build` performs its
// install steps through the real valv-proxy sidecar container on the internal
// Docker network, and that the sidecar enforces the allowlist correctly.
//
// Two sub-assertions:
//
//  1. ALLOWED path: `go install rsc.io/quote@v1.5.2` contacts proxy.golang.org
//     and sum.golang.org (both in the default allowlist). The sidecar permits
//     these connections. EnsureProjectImage MUST return nil error.
//
//  2. BLOCKED path: `npm install -g cowsay` contacts registry.npmjs.org which
//     is NOT in the default allowlist. The sidecar rejects the CONNECT. The
//     build fails with a network error. EnsureProjectImage MUST return a non-nil
//     error.
//
// Topology used (per DROP_15 Schema Decision 5):
//   - The workload build container attaches only to the `--internal` Docker
//     network (no external route). HTTP_PROXY and HTTPS_PROXY point at the
//     sidecar alias `valv-proxy:8080`.
//   - The valv-proxy sidecar spans both the internal network (reachable by the
//     build container) and the default bridge (for its own external egress).
//   - The proxy image `valv-proxy:dev` must be present locally (mage buildProxy).
//
// Cleanup: EnsureProjectImage defers the networkpolicy.Cleanup inside the call,
// so the sidecar container + internal network are removed when each build
// finishes. t.Cleanup removes all built test images. No valv-netpol-* networks
// or managed sidecar containers are left behind after the test completes.
//
// The test is gated to Docker Desktop macOS where the `--internal` network +
// sidecar topology is validated. The previous in-process http.Server approach
// (Round 1) was proven broken on macOS because host.docker.internal is
// unreachable from an --internal Docker network (DROP_15.2.5 empirical finding).
func TestEnsureProjectImage_NetworkPolicyOverlayBuildReachesProxy_DockerDesktopMacOS(t *testing.T) {
	// BLOCKED (15.2.5.G): the sidecar-topology rewrite below is correct and
	// caught a real production bug — `docker buildx build --network
	// <valv-netpol-...>` is rejected by buildkit (custom networks unsupported;
	// only none/host/default). Image-build egress through a custom --internal
	// network is therefore architecturally blocked. Awaiting the build-egress
	// design decision (deferred to a valv dep-proxy drop). Re-enable this test
	// when that lands. See drops/DROP_15_NETWORK_POLICY/PLAN.md § 15.2.5.G.
	t.Skip("15.2.5.G blocked on build-egress / buildkit custom-network limitation — see PLAN § 15.2.5.G")
	if runtime.GOOS != "darwin" {
		t.Skipf("Unit 15.2.5.G macOS gate: skipping on %s (Docker Desktop macOS only — sidecar topology validated on macOS)", runtime.GOOS)
	}
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}

	// Build a single test base image that has both the Go toolchain (for go
	// install) and npm (for npm install -g). The base build uses open
	// networking so apk can pull packages from the Alpine CDN; only the
	// overlay builds are subject to the sidecar policy.
	//
	// The valv user is required because BuildOverlayDockerfile appends
	// "USER valv" as the final instruction in the overlay Dockerfile.
	const baseDockerfile = `FROM golang:1.23-alpine
RUN apk add --no-cache nodejs npm \
    && addgroup -S valv \
    && adduser -S -G valv valv
LABEL io.valv.recipe_hash=integration-netpol-base
`
	baseContextDir := t.TempDir()
	baseDockerfilePath := filepath.Join(baseContextDir, "Dockerfile")
	if err := os.WriteFile(baseDockerfilePath, []byte(baseDockerfile), 0o644); err != nil {
		t.Fatalf("write base dockerfile: %v", err)
	}

	var baseStdout, baseStderr bytes.Buffer
	baseRunner := docker.NewSystemRunner("docker", nil, &baseStdout, &baseStderr)
	baseRef := docker.NewImageRef("valv-test/netpol-overlay", "it-base")
	buildBaseArgs, err := docker.BuildImageArgs(docker.ImageBuildRequest{
		ContextDir: baseContextDir,
		Dockerfile: baseDockerfilePath,
		Tags:       []docker.ImageRef{baseRef},
		Builder:    "auto",
		NoCache:    false,
	})
	if err != nil {
		t.Fatalf("BuildImageArgs for base: %v", err)
	}
	if err := baseRunner.Run(context.Background(), buildBaseArgs); err != nil {
		t.Fatalf("build base image: %v\nstdout:\n%s\nstderr:\n%s", err, baseStdout.String(), baseStderr.String())
	}
	t.Cleanup(func() {
		_ = baseRunner.Run(context.Background(), []string{"image", "rm", "--force", baseRef.String()})
	})

	// Wire the real networkpolicy.Service through the adapter so
	// EnsureProjectImage provisions the valv-proxy sidecar + internal network.
	dockerRunner := docker.NewSystemRunner("docker", nil, nil, nil)
	dockerExec := docker.NewExecutor(dockerRunner)
	policySvc, err := networkpolicy.New(networkpolicy.Options{Executor: dockerExec})
	if err != nil {
		t.Fatalf("networkpolicy.New() error = %v", err)
	}

	var buildStdout, buildStderr bytes.Buffer
	overlayRunner := docker.NewSystemRunner("docker", nil, &buildStdout, &buildStderr)
	svc, err := New(Options{
		Runner:        overlayRunner,
		Provider:      domain.ProviderCodex,
		Repository:    "valv-test/netpol-overlay",
		ContextDir:    baseContextDir,
		Dockerfile:    "Dockerfile",
		DefaultTag:    "it-overlay",
		UserID:        1000,
		GroupID:       1000,
		NetworkPolicy: networkPolicyAdapter{svc: policySvc},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// ── Allowed path ─────────────────────────────────────────────────────────
	// go install rsc.io/quote@v1.5.2 contacts proxy.golang.org and
	// sum.golang.org (both in the default allowlist). The sidecar must permit
	// these connections and the overlay build must succeed.
	allowedManifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"quote": {Source: "rsc.io/quote@v1.5.2", Install: "go install"},
		},
	}
	allowedResult, allowedErr := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  allowedManifest,
		BaseImage: baseRef,
		NoCache:   true,
	})
	if allowedErr != nil {
		t.Errorf("ALLOWED path: EnsureProjectImage returned unexpected error: %v\nstdout:\n%s\nstderr:\n%s",
			allowedErr, buildStdout.String(), buildStderr.String())
	}
	t.Cleanup(func() {
		if allowedResult.Image.String() != "" {
			_ = overlayRunner.Run(context.Background(), []string{"image", "rm", "--force", allowedResult.Image.String()})
		}
	})

	// ── Blocked path ──────────────────────────────────────────────────────────
	// npm install -g cowsay contacts registry.npmjs.org which is NOT in the
	// default allowlist (proxy.golang.org / sum.golang.org / github.com /
	// objects.githubusercontent.com). The sidecar must reject the CONNECT
	// tunnel to registry.npmjs.org:443 and the overlay build must fail.
	buildStdout.Reset()
	buildStderr.Reset()
	blockedManifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"cowsay": {Source: "cowsay", Install: "npm install -g"},
		},
	}
	_, blockedErr := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  blockedManifest,
		BaseImage: baseRef,
		NoCache:   true,
	})
	if blockedErr == nil {
		t.Errorf("BLOCKED path: EnsureProjectImage succeeded but expected failure (registry.npmjs.org must be blocked by sidecar)\nstdout:\n%s\nstderr:\n%s",
			buildStdout.String(), buildStderr.String())
	} else {
		t.Logf("BLOCKED path: EnsureProjectImage correctly returned error: %v", blockedErr)
	}
}

// networkPolicyAdapter bridges the consumer-side images.NetworkPolicy
// interface to the production networkpolicy.Service. The two packages
// deliberately have different struct types (consumer-side decoupling); the
// adapter does the value-copy in one place so the integration test can wire
// both without surfacing the shape distinction.
type networkPolicyAdapter struct {
	svc networkpolicy.Service
}

func (a networkPolicyAdapter) Provision(ctx context.Context, req NetworkPolicyRequest) (NetworkPolicyMaterial, NetworkPolicyCleanup, error) {
	material, cleanup, err := a.svc.Provision(ctx, networkpolicy.ProvisionRequest{
		Allowlist: req.Allowlist,
	})
	if err != nil {
		return NetworkPolicyMaterial{}, nil, err
	}
	return NetworkPolicyMaterial{
			HTTPProxyURL:  material.HTTPProxyURL,
			HTTPSProxyURL: material.HTTPSProxyURL,
			NoProxy:       material.NoProxy,
			NetworkName:   material.NetworkName,
		},
		NetworkPolicyCleanup(cleanup),
		nil
}
