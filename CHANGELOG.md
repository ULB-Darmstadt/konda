# Changelog

This file summarizes changes made while preparing this project for public release.

## 2026-03-04 — Graph visualization & NVL licensing

### Changed
- Commented out Neo4j NVL-specific code and configuration in `view/pages/knowledge-graph.html`, `web/scripts.js`, `package.json`, and `package-lock.json`.
- Updated the README to document generic graph visualization integration and clarify how to optionally enable the proprietary Neo4j NVL-based visualization locally (without redistributing NVL).

### Added
- `THIRD_PARTY_LICENSE_AUDIT.md` documenting third-party dependencies and their licenses.

## 2026-01-25 — Public repo preparation

### Changed
- Updated the Go module path to `git.rwth-aachen.de/dsma/publications/software/konda` and rewired all internal imports.
- Standardized configuration to load from repo-root `.env` (or an explicit `ENV_FILE`).
- Made Neo4j GenAI integration configurable via environment variables (provider, resource, embedding deployment, token).
- Made the Neo4j HTTP base URL configurable via `NEO4J_HTTP_URL` (used by RDF endpoints).
- Updated README to remove internal deployment assumptions and document local setup.

### Added
- `.env.example` with documented required variables.
- `.gitignore` to prevent committing runtime artifacts (binaries, DB files, workspaces, `.env`, generated frontend output).

## Database dump
- The database dump located under `dump/basedb.dump` is tracked using Git LFS.
- Git LFS must be installed to correctly fetch this file after cloning the repository.

### Removed
- Internal development notes (`todo.md`) and local token tracking file (`tokenTracker.txt`) from the repository.
- Admin CLI tooling (`cmd/invitecli/`) removed.
- User authentication (login/registration) removed; the tool is accessible without signing in.

### Fixed
- Token tracking now defaults to a per-user cache directory (override with `TOKEN_TRACKER_FILE`).

### Notes
- This repository uses Git LFS for large files such as database dumps. Git LFS must be installed to correctly fetch these files after cloning.
- The frontend assets are embedded from `web/static/`, including `web/static/generated-frontend/`; run `npm run build` before `go build` for local builds.
