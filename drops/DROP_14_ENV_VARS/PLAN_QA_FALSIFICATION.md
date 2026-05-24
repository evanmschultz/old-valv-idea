verdict: fail

# Plan QA Falsification — Round 5

## Counterexamples

### F5.1 Prefix-based pragma dedup can suppress the required `busy_timeout`

- Plan claim under attack: Unit 14.1 says DSN dedup is a case-insensitive prefix match like `busy_timeout` / `foreign_keys` rather than exact-string equality ([drops/DROP_14_ENV_VARS/PLAN.md](/Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_14_ENV_VARS/PLAN.md:29), [drops/DROP_14_ENV_VARS/PLAN.md](/Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_14_ENV_VARS/PLAN.md:46)).
- Repo evidence: the current DSN helper works on raw `_pragma` query-string values, not parsed pragma names, so any new dedup rule here will be string inspection over arbitrary caller-supplied values ([internal/adapters/sqlite/open.go](/Users/evanschultz/Documents/Code/hylla/valv/main/internal/adapters/sqlite/open.go:53)).
- Counterexample: `Open(OpenOptions{URI: "file:/tmp/valv.db?_pragma=busy_timeout_pragma%3Dfoo"})` yields `_pragma=busy_timeout_pragma=foo`. A naive case-insensitive `HasPrefix("busy_timeout")` rule treats that as an existing busy-timeout pragma and skips appending `busy_timeout(5000)`, even though no actual `busy_timeout(...)` pragma is present. The same false positive shape exists for `foreign_keys`.
- Why this still breaks the claim: the plan currently encodes the buggy matching rule directly. A builder can implement the plan exactly and still lose the required busy policy for supported URI callers.
- Narrow fix: require exact pragma-name parsing, not prefix matching. Normalize one `_pragma` value by trimming spaces, splitting at the first `(` or `=`, lowercasing the resulting pragma name, and comparing for equality with `busy_timeout` / `foreign_keys`. Add explicit tests that `_pragma=busy_timeout_pragma=foo` and `_pragma=foreign_keys_extra=1` do not suppress the required appended pragmas.

### F5.2 Unsupported-schema handling is still underspecified and misses forward-incompatible DBs

- Plan claim under attack: Unit 14.1 drops `user_version = 0` support and says v0 must fail fast with a "clear unsupported-schema error" ([drops/DROP_14_ENV_VARS/PLAN.md](/Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_14_ENV_VARS/PLAN.md:30), [drops/DROP_14_ENV_VARS/PLAN.md](/Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_14_ENV_VARS/PLAN.md:47), [drops/DROP_14_ENV_VARS/PLAN.md](/Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_14_ENV_VARS/PLAN.md:102)).
- Repo evidence:
  - the domain layer has no existing `ErrUnsupportedSchema`-style sentinel today ([internal/domain/errors.go](/Users/evanschultz/Documents/Code/hylla/valv/main/internal/domain/errors.go:5));
  - the current bootstrap logic accepts any `user_version >= 1` and returns success without an upper-bound check ([internal/adapters/sqlite/store.go](/Users/evanschultz/Documents/Code/hylla/valv/main/internal/adapters/sqlite/store.go:126));
  - CLI store opening just wraps bootstrap errors generically, so a vague string-only contract stays vague all the way out ([internal/cli/store.go](/Users/evanschultz/Documents/Code/hylla/valv/main/internal/cli/store.go:11)).
- Counterexample A: a builder can satisfy "clear unsupported-schema error" with a plain `fmt.Errorf("unsupported schema")`. That leaves tests and callers with brittle string matching instead of the repo's normal sentinel-plus-wrap pattern for semantic error categories.
- Counterexample B: seed a DB with `PRAGMA user_version = 3` and the pre-env tables only. If DROP_14 keeps the current "supported when `user_version >= current`" shape and only adds a v0 rejection branch, `Bootstrap` can return nil even though the v2 binary cannot actually trust the schema. The later env CRUD path then fails with table-shape/runtime errors instead of a fail-fast schema gate.
- Why this still breaks the claim: the plan narrows only the lower bound and never states the supported version window or the error identity. That leaves a concrete forward-compat hole and an ambiguous error contract.
- Narrow fix: define an explicit unsupported-schema sentinel, most naturally `domain.ErrUnsupportedSchema`, and require all schema-version rejections to wrap it with version details. Acceptance should cover both directions: seeded `user_version = 0` and seeded `user_version > 2` must fail from bootstrap with `errors.Is(err, domain.ErrUnsupportedSchema)`, while the error string includes the found and supported versions.

## YAGNI Pressure Check

- No new YAGNI failure surfaced in Round 5. The two fixes above are minimal safety constraints on an already-approved design, not extra abstraction.
- The shared `internal/services/run` dependency remains justified by DROP_13's accepted architecture and still has more than one concrete caller (`valv run`, `valv codex`, `valv claude`).

## Hidden Dependency Check

- DROP_13 remains an explicit dependency, and the workflow already prevents drop close while Unit 14.4 is still blocked ([drops/WORKFLOW.md](/Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md:184), [drops/WORKFLOW.md](/Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md:191)).
- Two hidden dependencies are still not explicit enough in the plan:
  - the DSN dedup rule currently depends on raw `_pragma` string grammar, but the plan specifies only a lossy prefix heuristic rather than the exact parsing rule;
  - the schema contract still lacks an explicit supported-version window and stable error identity for both "too old" and "too new" databases.
- I did not confirm a new counterexample for the existing v0 test file references, the `env_key` column naming, the custom plain formatter, or the DROP_13 scheduling question. Those attacks are either already covered by the current unit acceptance or by the drop workflow itself.
