package main

import (
	"strings"
)

// parseAllowlist splits a comma-separated list of hostnames (as supplied via
// the VALV_PROXY_ALLOWLIST environment variable), normalises each entry to
// lowercase, trims surrounding whitespace, and deduplicates.  Empty entries
// produced by leading/trailing commas or consecutive commas are discarded.
//
// Separator choice: comma (",").  This matches the canonical env-var list
// convention used throughout the Valv codebase and is easy to set in Docker
// -e flags and container env blocks.
//
// The returned slice is in the order of first appearance after deduplication.
// Callers that need deterministic ordering must sort the result themselves.
func parseAllowlist(raw string) []string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		h := strings.ToLower(strings.TrimSpace(p))
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

// hostAllowed reports whether target is present in the allowed set.
// target may arrive with an optional ":port" suffix (e.g. "github.com:443"
// from an HTTP CONNECT request); the port is stripped before comparison.
// Matching is case-insensitive (both sides are lowercased).
// No wildcards or CIDR ranges are supported — exact host match only.
func hostAllowed(target string, allowed []string) bool {
	host, _, hasPort := strings.Cut(target, ":")
	if !hasPort {
		host = target
	}
	host = strings.ToLower(host)
	for _, a := range allowed {
		if a == host {
			return true
		}
	}
	return false
}
