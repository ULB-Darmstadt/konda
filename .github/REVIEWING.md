# Reviewing Pull Requests

Quick notes for maintainers. Contributors should read [CONTRIBUTING.md](../CONTRIBUTING.md) instead.

## Before you review

- CI is green (`Backend (Go)` and `Frontend (npm / Parcel)`)
- The PR description and checklist are filled in

If CI fails, ask the contributor to fix it first rather than reviewing the code itself.

## What to check

- Fits KONDA's scope (see [CONTRIBUTING.md](../CONTRIBUTING.md#what-we-accept))
- No secrets, API keys, or `.env` files in the diff
- No licence-incompatible dependencies (must stay GPL-3.0 compatible)
- If the PR adds a new config/environment variable, `.env.example` documents it
- `CHANGELOG.md` / `README.md` updated if behaviour or setup changed
- The pipeline still works end to end

## Merging

- Approve once you're happy with the change and CI passes.
- If you can't review right away, leave a short comment so the contributor knows they haven't been forgotten.
- If a PR is out of scope or won't be accepted, say so and close it, don't leave it hanging.
