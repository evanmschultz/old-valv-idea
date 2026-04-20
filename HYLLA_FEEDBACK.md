# HYLLA_FEEDBACK

Per-drop record of Hylla misses — searches that forced fallback to Read/Grep/Glob.

## DROP_0_DOCS_BOOTSTRAP — 2026-04-19

None. DROP_0 was docs-only; every unit (U0.1–U0.6) recorded `**Hylla Feedback:** N/A — task touched non-Go files only.` The Hylla MCP was not invoked. First real Hylla exercise arrives in DROP_1 (deleting the OpenAI API wrapper).

## DROP_1_DELETE_API_WRAPPER — 2026-04-19

None. All five units (1.1–1.5) recorded their Hylla Feedback subsection as N/A. Units 1.1–1.4 were Go-file deletions against a file set changing relative to the latest ingest; per `main/CLAUDE.md` § "Code Understanding Rules" item 2 the correct evidence source for files changing since last ingest is `git diff` / direct `Read`/`Grep`, not Hylla. Unit 1.5 touched non-Go files only; per item 3 Hylla is not the correct source for markdown. No Hylla query was forced into a fallback — Hylla was correctly not invoked.
