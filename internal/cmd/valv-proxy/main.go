package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
)

// newProxyHandler returns an http.Handler that enforces the VALV_PROXY_ALLOWLIST.
//
// FAIL-CLOSED design: every request path that does not pass a hostAllowed check
// is denied with 403. Errors on hijack or dial silently close the connection
// rather than forwarding to an unchecked host.
//
// CONNECT (HTTPS tunnelling): checks hostAllowed(r.Host, allowed) first. On
// deny → 403 + return (no dial). On allow → writes 200, flushes, hijacks the
// conn, dials the target, then bidirectionally copies both directions in
// goroutines guarded by a WaitGroup, with CloseWrite signalling EOF to each
// side when the other finishes. Any dial or hijack error closes without forwarding.
//
// Plain HTTP: checks hostAllowed, denied → 403. Allowed → forwarded via
// httputil.NewSingleHostReverseProxy so the target sees a clean request.
func newProxyHandler(allowed []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			// --- CONNECT path (HTTPS tunnelling) ---
			if !hostAllowed(r.Host, allowed) {
				http.Error(w, "forbidden by allowlist", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			hj, ok := w.(http.Hijacker)
			if !ok {
				return // not hijackable — silently close
			}
			clientConn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			defer clientConn.Close()

			targetConn, err := net.Dial("tcp", r.Host)
			if err != nil {
				return // dial failed — connection already hijacked, just close
			}
			defer targetConn.Close()

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				_, _ = io.Copy(targetConn, clientConn)
				if tc, ok := targetConn.(*net.TCPConn); ok {
					_ = tc.CloseWrite()
				}
			}()
			go func() {
				defer wg.Done()
				_, _ = io.Copy(clientConn, targetConn)
				if tc, ok := clientConn.(*net.TCPConn); ok {
					_ = tc.CloseWrite()
				}
			}()
			wg.Wait()
			return
		}

		// --- Plain HTTP path ---
		if !hostAllowed(r.Host, allowed) {
			http.Error(w, "forbidden by allowlist", http.StatusForbidden)
			return
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		target := &url.URL{Scheme: scheme, Host: r.Host}
		r.RequestURI = ""
		httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
	})
}

func main() {
	addr := os.Getenv("VALV_PROXY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	allowed := parseAllowlist(os.Getenv("VALV_PROXY_ALLOWLIST"))

	fmt.Fprintf(os.Stderr, "valv-proxy: listening on %s, %d host(s) allowed\n", addr, len(allowed))
	if err := http.ListenAndServe(addr, newProxyHandler(allowed)); err != nil {
		fmt.Fprintf(os.Stderr, "valv-proxy: %v\n", err)
		os.Exit(1)
	}
}
