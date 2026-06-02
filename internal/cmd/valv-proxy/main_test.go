package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// dialProxy opens a raw TCP connection to the proxy server and sends a CONNECT
// request, then returns the connection along with the proxy's response.
func dialProxy(t *testing.T, proxyAddr, target string) (net.Conn, *http.Response) {
	t.Helper()
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := fmt.Fprint(conn, req); err != nil {
		conn.Close()
		t.Fatalf("write CONNECT: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		conn.Close()
		t.Fatalf("read CONNECT response: %v", err)
	}
	return conn, resp
}

// TestNewProxyHandler_CONNECT_AllowedHostTunnels verifies that a CONNECT request
// to an allowed host:port is tunnelled through to the backend TCP server.
func TestNewProxyHandler_CONNECT_AllowedHostTunnels(t *testing.T) {
	// Stand up a minimal TCP echo backend.
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	backendAddr := backend.Addr().String()
	backendHost, backendPort, _ := net.SplitHostPort(backendAddr)

	// Accept one connection from the proxy and echo back a sentinel.
	const sentinel = "hello-from-backend"
	go func() {
		c, err := backend.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		fmt.Fprint(c, sentinel)
	}()

	// Start the proxy, allowing only the backend host (without port).
	allowed := []string{backendHost}
	proxy := httptest.NewServer(newProxyHandler(allowed))
	defer proxy.Close()

	target := net.JoinHostPort(backendHost, backendPort)
	conn, resp := dialProxy(t, proxy.Listener.Addr().String(), target)
	defer conn.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Read the sentinel through the tunnel.
	buf := make([]byte, len(sentinel))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read through tunnel: %v", err)
	}
	if string(buf) != sentinel {
		t.Errorf("tunnel payload: got %q, want %q", string(buf), sentinel)
	}
}

// TestNewProxyHandler_CONNECT_DeniedHost verifies that a CONNECT request to a
// host not in the allowlist receives a 403 and the backend is never dialled.
func TestNewProxyHandler_CONNECT_DeniedHost(t *testing.T) {
	// Backend should never be dialled; if it is, Accept returns immediately.
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	dialCount := 0
	go func() {
		c, err := backend.Accept()
		if err != nil {
			return
		}
		dialCount++
		c.Close()
	}()

	// Allow a different host so the backend host is denied.
	allowed := []string{"allowed.example.com"}
	proxy := httptest.NewServer(newProxyHandler(allowed))
	defer proxy.Close()

	// Target uses the backend's actual address but the handler checks r.Host
	// which is the CONNECT target string — we send a denied hostname here.
	conn, err := net.Dial("tcp", proxy.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	target := "denied.example.com:443"
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	fmt.Fprint(conn, req)

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	if dialCount != 0 {
		t.Errorf("backend was dialled %d time(s), expected 0", dialCount)
	}
}

// TestNewProxyHandler_PlainHTTP_AllowedHost verifies that plain HTTP requests to
// an allowed host are forwarded to the backend.
func TestNewProxyHandler_PlainHTTP_AllowedHost(t *testing.T) {
	// Backend HTTP server returns a fixed body.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backend-response")
	}))
	defer backend.Close()

	// Extract host from backend URL ("127.0.0.1").
	backendHost := strings.Split(strings.TrimPrefix(backend.URL, "http://"), ":")[0]

	allowed := []string{backendHost}
	proxy := httptest.NewServer(newProxyHandler(allowed))
	defer proxy.Close()

	// Send a plain HTTP request through the proxy to the backend.
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				u, _ := url.Parse(proxy.URL)
				return u, nil
			},
		},
	}
	resp, err := client.Get(backend.URL)
	if err != nil {
		t.Fatalf("GET through proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "backend-response" {
		t.Errorf("body: got %q, want %q", string(body), "backend-response")
	}
}

// TestNewProxyHandler_PlainHTTP_DeniedHost verifies that plain HTTP requests to a
// host not in the allowlist receive a 403.
func TestNewProxyHandler_PlainHTTP_DeniedHost(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "should-not-reach")
	}))
	defer backend.Close()

	// Allow a different host.
	allowed := []string{"allowed.example.com"}
	proxy := httptest.NewServer(newProxyHandler(allowed))
	defer proxy.Close()

	// Issue a plain HTTP GET via the proxy where the Host header targets a denied
	// host (we route the TCP connection to the proxy but address a denied Host).
	req, _ := http.NewRequest(http.MethodGet, "http://denied.example.com/", nil)
	client := &http.Client{
		Transport: &http.Transport{
			// Force the connection to go to our proxy.
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("tcp", proxy.Listener.Addr().String())
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		// Some HTTP clients surface a 403 as an error; inspect if so.
		if strings.Contains(err.Error(), "403") {
			return
		}
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// TestNewProxyHandler_EmptyAllowlistDeniesAll verifies fail-closed: with an empty
// allowlist every request (both CONNECT and plain HTTP) is denied with 403.
func TestNewProxyHandler_EmptyAllowlistDeniesAll(t *testing.T) {
	allowed := []string{}
	proxy := httptest.NewServer(newProxyHandler(allowed))
	defer proxy.Close()

	t.Run("CONNECT", func(t *testing.T) {
		conn, err := net.Dial("tcp", proxy.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		target := "github.com:443"
		fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("CONNECT: expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("PlainHTTP", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "http://github.com/", nil)
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial("tcp", proxy.Listener.Addr().String())
				},
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			if strings.Contains(err.Error(), "403") {
				return
			}
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("PlainHTTP: expected 403, got %d", resp.StatusCode)
		}
	})
}
