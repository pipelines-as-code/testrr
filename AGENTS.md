# Repository Agent Rules

## Mandatory Workflow

- Always read and follow `PLANS.md` before making changes.
- Execute work in the phase order defined in `PLANS.md` unless the user explicitly overrides it.
- When a task spans multiple phases, finish the current phase cleanly before advancing.
- Keep `PLANS.md` aligned with the implementation whenever the plan materially changes.

## Implementation Rules

- Do not commit or rewrite git history without explicit user approval.
- Prefer standard library facilities unless an external dependency materially reduces complexity.
- Keep the product runnable as a single Go binary with embedded frontend assets.
- Treat SQLite as the default backend and Postgres as an optional backend.
- Add or update tests for changed behavior.

## Handoff

- Report commands run, results, changed files, and any remaining follow-ups.
