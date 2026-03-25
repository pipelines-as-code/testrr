# Test Report Hub Build Plan

## Phase 1: Foundation

- Initialize the Go module and frontend asset pipeline.
- Add repository instructions in `AGENTS.md`.
- Define configuration, CLI entrypoints, storage abstraction, and embedded asset layout.
- Keep the app oriented around a single-binary deployment with SQLite by default and Postgres optionally.

## Phase 2: Core Backend

- Implement configuration loading and CLI commands for `serve`, `migrate`, and project management.
- Build the storage layer for projects, credentials, runs, artifacts, and test results.
- Implement project-scoped authentication, password hashing, and browser session handling.
- Add JUnit XML ingestion with support for many files per run and immutable historical runs.

## Phase 3: UI and Analytics

- Build server-rendered pages with `templ`.
- Add a TypeScript bundle with `htmx` and Apache ECharts.
- Implement login, dashboard, upload flow, recent runs, run detail, and per-test history views.
- Expose chart-friendly endpoints and render trend/regression data.

## Phase 4: Verification

- Add parser, storage, auth, and HTTP integration tests.
- Verify both the Go build and frontend asset build.
- Run targeted tests and fix issues until the app builds and the main flows pass.

## Current Implementation Target

- Phase 1 through Phase 4 are in scope for the current delivery.
- JUnit XML is the only implemented importer in this iteration.
- TRX and NUnit stay planned next through the parser abstraction.
- Project dashboards and history are public by default; uploads are API-only and protected by project credentials.
