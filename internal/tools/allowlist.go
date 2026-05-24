package tools

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// AllowlistConfig is the typed representation of the `[allowlist]` section
// of a `.valv/tools.toml` manifest. DROP_15 introduces the typed schema;
// DROP_11 had captured the section as toml.Primitive while reserving the
// shape.
//
// Hosts holds the user-declared exact hostnames or Docker aliases that
// should be reachable from a Valv-managed container under closed-default
// network policy. CIDRs, URL prefixes, ports, schemes, paths, and wildcard
// syntax are NOT supported in DROP_15 — see EffectiveAllowlist /
// validateHost for the host-shape contract.
type AllowlistConfig struct {
	Hosts []string `toml:"hosts"`
}

// defaultAllowlistHosts is the unexported source-of-truth for the built-in
// hosts the effective allowlist always contains, even when no user-declared
// hosts are present. The list covers the Go module proxy + Go sum DB + the
// GitHub release-object CDN + github.com itself, which together let plain
// `go install github.com/<org>/<repo>` work without any user allowlist
// entries.
//
// The slice is sorted lexicographically so callers iterating it observe a
// deterministic order; EffectiveAllowlist also dedupes and re-sorts after
// unioning user hosts.
//
// Kept package-private to prevent external callers from mutating the global
// allowlist policy in place. External access goes through
// DefaultAllowlistHosts(), which always returns a fresh copy.
var defaultAllowlistHosts = []string{
	"github.com",
	"objects.githubusercontent.com",
	"proxy.golang.org",
	"sum.golang.org",
}

// DefaultAllowlistHosts returns a fresh copy of the built-in default
// allowlist hosts. The returned slice may be mutated freely by callers
// without affecting subsequent calls or the package's internal state.
//
// This is the only supported way to read the defaults from outside the
// tools package; the underlying slice is intentionally unexported so that
// security-sensitive allowlist policy cannot be mutated as a side effect.
func DefaultAllowlistHosts() []string {
	out := make([]string, len(defaultAllowlistHosts))
	copy(out, defaultAllowlistHosts)
	return out
}

// ErrUnsupportedManifestShape is returned by WriteAllowlistSection when the
// existing on-disk manifest uses a TOML shape the helper deliberately does
// NOT support: UTF-8 BOM, CRLF line endings, or a multi-line string outside
// the bounded `[allowlist]` section span. Callers can detect this case via
// errors.Is.
var ErrUnsupportedManifestShape = errors.New("unsupported tools.toml shape for section-safe edit")

// ErrInvalidAllowlistHost is returned by EffectiveAllowlist when one of the
// user-declared hosts fails shape validation. The wrapped error reports the
// offending token verbatim.
var ErrInvalidAllowlistHost = errors.New("invalid allowlist host")

// hostShapeRE accepts an exact hostname or Docker alias. Each
// dot-separated label must begin and end with an alphanumeric character and
// may contain alphanumerics or hyphens between. This matches RFC 1123-style
// host labels plus Docker container aliases (which follow the same rules).
//
// Explicit rejections (enforced by this regex plus the additional checks
// below):
//   - empty string
//   - URL-shaped values containing `://`, `/`, `?`, or `#`
//   - port suffixes (`:<digits>`)
//   - whitespace anywhere
//   - leading/trailing dots or hyphens
//   - uppercase letters (callers must lowercase first; EffectiveAllowlist
//     does this for them)
var hostShapeRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

// EffectiveAllowlist returns the deterministic effective allowlist for the
// given AllowlistConfig: the union of the built-in default hosts and the
// user hosts, with each user host lowercased, trimmed of surrounding
// whitespace, validated, and deduped. The result is sorted
// lexicographically.
//
// Zero-value config (cfg.Hosts == nil) returns exactly the built-in
// defaults. User hosts that fail validateHost cause EffectiveAllowlist to
// return an error wrapping ErrInvalidAllowlistHost; the partial result is
// discarded.
func EffectiveAllowlist(cfg AllowlistConfig) ([]string, error) {
	seen := make(map[string]struct{}, len(defaultAllowlistHosts)+len(cfg.Hosts))
	out := make([]string, 0, len(defaultAllowlistHosts)+len(cfg.Hosts))

	for _, h := range defaultAllowlistHosts {
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}

	for _, raw := range cfg.Hosts {
		normalized := strings.ToLower(strings.TrimSpace(raw))
		if err := validateHost(normalized); err != nil {
			return nil, fmt.Errorf("effective allowlist: %q: %w", raw, err)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	sort.Strings(out)
	return out, nil
}

// validateHost rejects empty strings, URL-shaped values, ports, paths, and
// any value that does not match hostShapeRE. It also enforces the RFC 1123
// length limits: each dot-separated label must be at most 63 bytes, and the
// total host length must be at most 253 bytes. The caller must lowercase
// and trim the value first; EffectiveAllowlist does this.
func validateHost(h string) error {
	if h == "" {
		return fmt.Errorf("%w: empty host", ErrInvalidAllowlistHost)
	}
	// Fast-path rejections with a clear message.
	if strings.ContainsAny(h, " \t\r\n") {
		return fmt.Errorf("%w: contains whitespace", ErrInvalidAllowlistHost)
	}
	if strings.Contains(h, "://") {
		return fmt.Errorf("%w: URL-shaped (contains scheme)", ErrInvalidAllowlistHost)
	}
	if strings.ContainsAny(h, "/?#") {
		return fmt.Errorf("%w: contains path or query characters", ErrInvalidAllowlistHost)
	}
	if strings.Contains(h, ":") {
		return fmt.Errorf("%w: port suffix not allowed", ErrInvalidAllowlistHost)
	}
	if !hostShapeRE.MatchString(h) {
		return fmt.Errorf("%w: not a valid hostname or Docker alias", ErrInvalidAllowlistHost)
	}
	// RFC 1123 length limits. Enforced AFTER character-shape so the error
	// for obviously broken values stays specific.
	if len(h) > 253 {
		return fmt.Errorf("%w: host exceeds 253 bytes (RFC 1123 limit)", ErrInvalidAllowlistHost)
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) > 63 {
			return fmt.Errorf("%w: label %q exceeds 63 bytes (RFC 1123 limit)", ErrInvalidAllowlistHost, label)
		}
	}
	return nil
}

// utf8BOM is the byte sequence Go's bufio readers do NOT strip; we reject
// any manifest that begins with it because the section-safe rewrite
// contract preserves bytes verbatim outside `[allowlist]`, which would
// silently propagate the BOM. Callers can normalize externally.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// WriteAllowlistSection rewrites only the `[allowlist]` section of the
// `.valv/tools.toml` manifest at path. Behavior:
//
//   - If the parent directory (typically `.valv/`) does not exist, it is
//     created via os.MkdirAll with mode 0o755.
//   - If the file does not exist, a fresh manifest containing only the
//     `[allowlist]` section is written.
//   - If the file exists, its contents are split into three byte regions:
//     (1) bytes before the `[allowlist]` header line, (2) the bounded
//     `[allowlist]` span ending immediately before the next top-level
//     `[section]` header (or EOF), and (3) the bytes from the next section
//     header through EOF. Regions (1) and (3) are preserved verbatim; only
//     region (2) is rewritten.
//   - Unsupported shapes (UTF-8 BOM, CRLF line endings, multi-line string
//     outside the `[allowlist]` span) return an error wrapping
//     ErrUnsupportedManifestShape.
//
// The rewritten `[allowlist]` section uses LF line endings, no inline
// comments, and follows the canonical form:
//
//	[allowlist]
//	hosts = [
//	  "a.example",
//	  "b.example",
//	]
//
// When cfg.Hosts is empty, the rewritten section emits `hosts = []` on a
// single line. cfg.Hosts is written verbatim (no lowercasing or deduping)
// so the writer is a pure mechanical edit; callers that want normalization
// should apply EffectiveAllowlist semantics before constructing the cfg.
func WriteAllowlistSection(path string, cfg AllowlistConfig) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("write allowlist section: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("write allowlist section: ensure parent dir: %w", err)
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("write allowlist section: read existing: %w", err)
		}
		// Fresh-file path: emit just the [allowlist] section.
		body := renderAllowlistSection(cfg)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write allowlist section: write fresh: %w", err)
		}
		return nil
	}

	prefix, suffix, err := splitAllowlistSpan(existing)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.Write(prefix)
	// Ensure exactly one LF between the preserved prefix and the
	// rewritten allowlist section, but only when the prefix has content
	// and does not already end in LF.
	if len(prefix) > 0 && prefix[len(prefix)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString(renderAllowlistSection(cfg))
	// Ensure exactly one LF separator before the suffix's leading bytes.
	// renderAllowlistSection guarantees its output ends with a newline;
	// the suffix begins with the next top-level section header (no
	// leading newline). If the suffix is empty (no following section),
	// no extra newline is needed.
	buf.Write(suffix)

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write allowlist section: write rewrite: %w", err)
	}
	return nil
}

// renderAllowlistSection emits the canonical `[allowlist]` block.
func renderAllowlistSection(cfg AllowlistConfig) string {
	var b strings.Builder
	b.WriteString("[allowlist]\n")
	if len(cfg.Hosts) == 0 {
		b.WriteString("hosts = []\n")
		return b.String()
	}
	b.WriteString("hosts = [\n")
	for _, h := range cfg.Hosts {
		fmt.Fprintf(&b, "  %q,\n", h)
	}
	b.WriteString("]\n")
	return b.String()
}

// topLevelHeaderRE matches a TOML top-level `[section]` header at line
// start, allowing trailing whitespace and an optional `# comment` after the
// closing bracket. Captures the section name for span-boundary checks.
//
// The regex deliberately rejects array-of-tables headers (`[[…]]`) — those
// are not a supported manifest shape for DROP_15.
var topLevelHeaderRE = regexp.MustCompile(`^\[([^\[\]]+)\][ \t]*(?:#.*)?$`)

// splitAllowlistSpan splits existing manifest bytes into a (prefix, suffix)
// pair, where:
//
//   - prefix is everything up to and including the LF terminator of the
//     line PRECEDING the `[allowlist]` header (or empty if `[allowlist]` is
//     the first line). When `[allowlist]` is absent, prefix is the entire
//     file content with any trailing LF preserved and suffix is empty.
//   - suffix begins at the first byte of the next top-level `[section]`
//     header line after `[allowlist]` and runs through EOF. If
//     `[allowlist]` is the last (or only) section, suffix is empty.
//
// splitAllowlistSpan rejects:
//   - UTF-8 BOM at start of file
//   - any CR byte (CRLF or solo CR)
//   - a multi-line basic or literal string (triple-quote `"""` or
//     triple-apostrophe sequence) opened outside the `[allowlist]` span —
//     the section-safe contract cannot reason about
//     spans split across that boundary. Triple-quote sequences inside
//     comments are NOT a multi-line string per TOML semantics and are
//     allowed; the implementation strips the comment portion of each line
//     before scanning for triple-quote openers.
//   - top-level section headers that begin with leading whitespace
//     (`  [name]`). The supported shape requires section headers to start
//     at column 0; leading-whitespace headers are valid TOML but would
//     break the byte-preservation contract because the rewrite cannot keep
//     the leading whitespace attached to the canonical (column-0)
//     replacement section.
//   - array-of-tables headers (`[[name]]`) anywhere in the file
func splitAllowlistSpan(content []byte) ([]byte, []byte, error) {
	if bytes.HasPrefix(content, utf8BOM) {
		return nil, nil, fmt.Errorf("write allowlist section: %w: UTF-8 BOM at start of file", ErrUnsupportedManifestShape)
	}
	if bytes.IndexByte(content, '\r') >= 0 {
		return nil, nil, fmt.Errorf("write allowlist section: %w: CR byte found (CRLF not supported)", ErrUnsupportedManifestShape)
	}

	// Reject array-of-tables headers anywhere in the file. We scan with
	// bufio so we can also detect multi-line strings opened outside the
	// `[allowlist]` span (which would break section-boundary detection).
	sc := bufio.NewScanner(bytes.NewReader(content))
	// Allow long lines; tools.toml manifests are not large but we should
	// not silently truncate.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		// Byte offset of the `[` in `[allowlist]` header, or -1 if absent.
		allowlistStart = -1
		// Byte offset of the `[` in the next top-level section after
		// `[allowlist]`, or len(content) if `[allowlist]` is last.
		nextStart = -1
		// Running byte offset at the START of the current line.
		offset = 0
		// Track multi-line string state for the run OUTSIDE the
		// `[allowlist]` span. We allow `"""` / `'''` inside the
		// `[allowlist]` span because that span gets rewritten anyway; but
		// we cannot let one START outside the span and span the boundary.
		insideAllowlistSpan = false
	)

	for sc.Scan() {
		line := sc.Bytes()
		// Tracking whether the line is structurally a header has to
		// happen before we update the offset.
		lineLen := len(line) + 1 // +1 for the LF we know is there for full lines; last line may be EOF without LF — handled below.

		// Detect array-of-tables headers anywhere — unsupported shape.
		// bufio.Scanner strips the LF terminator; bytes.HasPrefix on the
		// raw line is fine.
		trimmed := bytes.TrimLeft(line, " \t")
		if bytes.HasPrefix(trimmed, []byte("[[")) {
			return nil, nil, fmt.Errorf("write allowlist section: %w: array-of-tables header at offset %d", ErrUnsupportedManifestShape, offset)
		}

		// Detect a top-level `[section]` header. Reject leading-whitespace
		// headers BEFORE applying the section-state transitions: they are
		// valid TOML but unsupported by the section-safe rewrite (see the
		// doc comment for the rationale).
		if m := topLevelHeaderRE.FindSubmatch(trimmed); m != nil {
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				return nil, nil, fmt.Errorf("write allowlist section: %w: indented section header at offset %d (top-level [section] headers must start at column 0)", ErrUnsupportedManifestShape, offset)
			}
			name := strings.TrimSpace(string(m[1]))
			headerOffset := offset + bytes.Index(line, []byte("["))
			if name == "allowlist" {
				if allowlistStart >= 0 {
					return nil, nil, fmt.Errorf("write allowlist section: %w: duplicate [allowlist] header", ErrUnsupportedManifestShape)
				}
				allowlistStart = headerOffset
				insideAllowlistSpan = true
			} else if allowlistStart >= 0 && nextStart < 0 {
				nextStart = headerOffset
				insideAllowlistSpan = false
			} else if allowlistStart < 0 {
				// Section before [allowlist] — fine, just part of prefix.
				insideAllowlistSpan = false
			}
		}

		// Outside the `[allowlist]` span, reject multi-line string
		// openers because we cannot track them across a partial rewrite.
		// First strip the comment portion of the line: per TOML spec, `#`
		// starts a comment that runs to end-of-line UNLESS it sits inside
		// a string. We only need to distinguish triple-quote sequences in
		// comments from real multi-line-string openers, so a minimal
		// comment-stripper that respects single-quoted basic / literal
		// strings is sufficient — and conservative: any leftover triple
		// quote in CODE is still rejected.
		if !insideAllowlistSpan {
			codeOnly := stripLineComment(line)
			if bytes.Contains(codeOnly, []byte(`"""`)) || bytes.Contains(codeOnly, []byte(`'''`)) {
				return nil, nil, fmt.Errorf("write allowlist section: %w: multi-line string outside [allowlist] span", ErrUnsupportedManifestShape)
			}
		}

		offset += lineLen
	}
	if err := sc.Err(); err != nil {
		return nil, nil, fmt.Errorf("write allowlist section: scan: %w", err)
	}

	// Correct the trailing-line offset: if the file does NOT end with a
	// newline, the scanner counted +1 too many bytes for the final
	// iteration. We reconcile by clamping offsets to the real length.
	if offset > len(content) {
		offset = len(content)
	}

	if allowlistStart < 0 {
		// No [allowlist] section in file: prefix = whole file, suffix = "".
		return content, nil, nil
	}
	if nextStart < 0 {
		// [allowlist] is last: suffix is empty.
		return content[:allowlistStart], nil, nil
	}
	return content[:allowlistStart], content[nextStart:], nil
}

// stripLineComment returns the bytes of line up to (but excluding) the
// first `#` that begins a TOML comment. A `#` inside a single- or
// double-quoted string is NOT a comment introducer; this helper tracks
// string state byte-by-byte to make that distinction.
//
// The helper deliberately recognizes only single-line `"..."` basic
// strings and `'...'` literal strings. Multi-line strings (the triple
// double-quote and triple apostrophe forms) are exactly the shape
// splitAllowlistSpan needs to detect downstream, so stripLineComment
// leaves them intact in the returned
// slice — its only job is to mask comment text so a stray `#` in a
// comment does not hide a real triple-quote opener earlier on the line,
// nor cause a triple-quote sequence appearing only inside a comment to
// trip the outside-span multi-line-string rejection.
func stripLineComment(line []byte) []byte {
	inBasic := false
	inLiteral := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inBasic:
			if c == '\\' && i+1 < len(line) {
				// Skip escape sequence inside basic strings so an
				// escaped quote (`\"`) does not flip the state.
				i++
				continue
			}
			if c == '"' {
				inBasic = false
			}
		case inLiteral:
			// Literal strings have no escapes.
			if c == '\'' {
				inLiteral = false
			}
		default:
			if c == '#' {
				return line[:i]
			}
			if c == '"' {
				inBasic = true
			} else if c == '\'' {
				inLiteral = true
			}
		}
	}
	return line
}
