# DROP_N — Closeout

Written once at drop close. See `main/drops/WORKFLOW.md` § "Phase 7 — Closeout" for the full step list.

- **Closed:** YYYY-MM-DD
- **Final commit:** <sha>
- **CI run:** <gh run url>

## Hylla Feedback Aggregation

<Summarize every `## Hylla Feedback` subsection from BUILDER_WORKLOG.md. Append the same entry to main/HYLLA_FEEDBACK.md.>

## Refinements

<Ergonomic wins, ergonomic pain, bugs, lessons. Append to main/REFINEMENTS.md (or main/HYLLA_REFINEMENTS.md if Hylla-specific).>

## Ledger Entry

<One-paragraph summary appended to main/LEDGER.md.>

## Wiki Changelog

<One-liner appended to main/WIKI_CHANGELOG.md.>

## Hylla Ingest

- **Triggered:** YYYY-MM-DD HH:MM (after CI green)
- **Mode:** full_enrichment
- **Source:** github.com/evanmschultz/valv@main
- **Result:** <ingest run id + outcome>

## WIKI.md Updates

<List the WIKI.md sections updated in place (or "none — no best-practice change"). git log -- main/WIKI.md captures the diff.>
