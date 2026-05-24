//go:build integration

package images

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
// is the Unit 15.2.5 integration gate: it proves that when NetworkPolicy is
// configured, the overlay `docker buildx build` performs its `go install`
// step through the host-side proxy via `host.docker.internal`. The host-side
// fixture is a fully in-process HTTP CONNECT proxy that records which Host
// values each tunneled request targeted.
//
// PLAN.md Unit 15.2.5 acceptance gates this test to Docker Desktop macOS and
// explicitly allows the test to be marked "manual-validation-required"
// pending macOS-runner automation:
//
//	"the integration test must be validated on Docker Desktop macOS; Linux CI
//	runs are NOT evidence for A1 because host.docker.internal semantics differ
//	across Docker Engine deployments. ... Either automate macOS-runner
//	coverage OR mark the test as manual-validation-required on Docker Desktop
//	macOS in builder notes and confirm validation before unit close."
//
// Round 1 ships the test as manual-validation-required (skipped by default
// on every platform). The Round 1 builder ran it locally on Docker Desktop
// macOS via VALV_NETPOL_INTEGRATION_RUN=1 and captured the outcome in
// BUILDER_WORKLOG.md: build args + --network attachment are correct, but
// `host.docker.internal` is unreachable from the --internal docker network
// on Docker Desktop macOS — exactly the A1 risk PLAN.md flagged. The
// orchestrator/dev makes the final block/accept call.
//
// To force-run, set the env var VALV_NETPOL_INTEGRATION_RUN=1.
//
// The test uses NoCache=true to force a rebuild every run; a freshness-label
// cache hit would yield a false green per PLAN guidance.
func TestEnsureProjectImage_NetworkPolicyOverlayBuildReachesProxy_DockerDesktopMacOS(t *testing.T) {
	if os.Getenv("VALV_NETPOL_INTEGRATION_RUN") == "" {
		t.Skipf("Unit 15.2.5 macOS gate: manual-validation-required. Set VALV_NETPOL_INTEGRATION_RUN=1 on Docker Desktop macOS to execute. Round 1 validated locally; see BUILDER_WORKLOG.md.")
	}
	if runtime.GOOS != "darwin" {
		t.Skipf("Unit 15.2.5 macOS gate: skipping on %s (Docker Desktop macOS only — host.docker.internal semantics differ across Engine deployments)", runtime.GOOS)
	}
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Skipf("docker unavailable for integration test: %v", err)
	}

	// Stand up an in-process HTTP CONNECT proxy on a free port. The proxy
	// accepts CONNECT requests, records the target Host, and refuses any
	// host not in the allowlist with a 403. Successful tunnels short-
	// circuit before any real upstream connection — the test only needs to
	// prove the build reached the proxy, not that the proxy is a fully
	// functional egress gateway.
	allowed := map[string]bool{
		"proxy.golang.org:443":              true,
		"sum.golang.org:443":                true,
		"github.com:443":                    true,
		"objects.githubusercontent.com:443": true,
	}
	var (
		mu          sync.Mutex
		seenHosts   []string
		blockedHits int64
	)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			mu.Lock()
			seenHosts = append(seenHosts, r.Host)
			mu.Unlock()
			if !allowed[r.Host] {
				atomic.AddInt64(&blockedHits, 1)
				http.Error(w, "forbidden by valv allowlist", http.StatusForbidden)
				return
			}
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "hijack unsupported", http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
			_ = conn.Close()
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = srv.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	endpoint := "host.docker.internal:" + intToStr(port)

	dockerRunner := docker.NewSystemRunner("docker", nil, nil, nil)
	dockerExec := docker.NewExecutor(dockerRunner)
	policySvc, err := networkpolicy.New(networkpolicy.Options{Executor: dockerExec})
	if err != nil {
		t.Fatalf("networkpolicy.New() error = %v", err)
	}
	policy := networkPolicyAdapter{svc: policySvc}

	contextDir := t.TempDir()
	dockerfile := filepath.Join(contextDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM golang:1.23-alpine\nLABEL io.valv.recipe_hash=integration-base\n"), 0o644); err != nil {
		t.Fatalf("write base dockerfile: %v", err)
	}

	var buildStdout, buildStderr bytes.Buffer
	imageRunner := docker.NewSystemRunner("docker", nil, &buildStdout, &buildStderr)
	svc, err := New(Options{
		Runner:        imageRunner,
		Provider:      domain.ProviderCodex,
		Repository:    "valv-test/netpol-overlay",
		ContextDir:    contextDir,
		Dockerfile:    "Dockerfile",
		DefaultTag:    "it-base",
		UserID:        1000,
		GroupID:       1000,
		NetworkPolicy: policy,
		ProxyEndpoint: endpoint,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := svc.Build(context.Background(), BuildRequest{Version: "0.117.0"}); err != nil {
		t.Fatalf("build base image: %v\nstdout:\n%s\nstderr:\n%s", err, buildStdout.String(), buildStderr.String())
	}
	baseRef := docker.NewImageRef("valv-test/netpol-overlay", "it-base")
	t.Cleanup(func() {
		_ = imageRunner.Run(context.Background(), []string{"image", "rm", "--force", baseRef.String()})
	})

	manifest := tools.ToolManifest{
		Tools: map[string]tools.ToolSpec{
			"hello": {Source: "rsc.io/quote@v1.5.2", Install: "go install"},
		},
	}
	result, err := svc.EnsureProjectImage(context.Background(), EnsureProjectRequest{
		Manifest:  manifest,
		BaseImage: baseRef,
		NoCache:   true,
	})
	t.Cleanup(func() {
		if result.Image.String() != "" {
			_ = imageRunner.Run(context.Background(), []string{"image", "rm", "--force", result.Image.String()})
		}
	})
	// The build may fail because `go install` cannot complete through the
	// dummy CONNECT-only proxy (no real upstream bytes). That's fine —
	// the contract this test proves is that the build reached the proxy
	// with the right Host. So we tolerate err != nil and inspect the
	// proxy log afterward.
	if err != nil {
		t.Logf("overlay build returned error (expected with dummy proxy): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seenHosts) == 0 {
		t.Fatalf("proxy saw zero CONNECT requests — build did not reach proxy via host.docker.internal:%d", port)
	}
	sawAllowed := false
	for _, h := range seenHosts {
		if allowed[h] {
			sawAllowed = true
			break
		}
	}
	if !sawAllowed {
		t.Errorf("proxy saw %d CONNECTs but none were allowlisted: %v", len(seenHosts), seenHosts)
	}
	t.Logf("proxy summary: %d CONNECTs, %d blocked, hosts=%v", len(seenHosts), atomic.LoadInt64(&blockedHits), seenHosts)
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
		Allowlist:     req.Allowlist,
		ProxyEndpoint: req.ProxyEndpoint,
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

// intToStr renders a port number as a decimal string. Kept tiny so the
// integration build does not need a strconv import that overlaps with other
// integration-only utilities; the build-tag gate keeps this out of the
// production binary.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
