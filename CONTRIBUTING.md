# Contributing to KONDA

Thanks for your interest in contributing! KONDA is maintained by the [Data Stream Management and Analysis Group (DSMA)](https://dsma.rwth-aachen.de/) at RWTH Aachen University, and we welcome contributions in the form of pull requests.

For clear bug fixes or typos, just submit a pull request. For new features or if there's any doubt about how to fix a bug, please open an issue first to discuss it. This saves everyone rework, and we're happy to help you find the right approach.

Please keep discussion on GitHub (issues and pull requests) so others can follow along.

---

## What we accept

**In scope:** bug fixes, documentation, pipeline improvements, and performance improvements with evidence.

**Out of scope (for now):** large refactors without prior discussion, and licence-incompatible dependencies (everything must stay compatible with GPL-3.0).

New features are welcome, just open an issue first, as noted above.

---

## Getting set up

You'll need Go, Node.js, and a running Neo4j instance. See the [README](README.md) for the full list of prerequisites and setup instructions.

1. **Fork** the repository (your own personal copy) and **clone** your fork to your machine. If you're new to this, GitHub has a [great guide on forking](https://docs.github.com/en/get-started/quickstart/fork-a-repo).
2. Follow the [README](README.md) to install dependencies and get KONDA running locally.
3. Create a branch from `main` for your work (for example `fix/…`, `feat/…`, or `docs/…`).

Please don't commit `.env` files or credentials.

---

## Developing

A few things to keep in mind while writing code:

- Keep naming and style consistent with the existing code, avoid abbreviations in names.
- Add a short comment above new public functions explaining what they do.
- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages, in English (for example `fix(analyzer): handle empty entity list`).
- Keep each pull request focused on one thing. Fix one bug or add one feature per PR, and don't mix unrelated changes.

Before opening your pull request, run the same checks our CI runs, so you catch problems early:

```sh
gofmt -w .        # format Go code
go vet ./...      # catch common mistakes
go build ./...    # make sure it compiles
npm run build     # make sure the frontend builds
```

If your change affects behaviour or setup, update [`CHANGELOG.md`](CHANGELOG.md) and the [`README.md`](README.md) accordingly.

---

## Opening a pull request

Once you're happy with your change and the checks above pass, open a pull request against `main` and fill in the [pull request template](.github/pull_request_template.md). Please include a clear description of what your change does, and link any related issue (`Closes #...`).

When you open the PR, our CI automatically runs the checks above. If any fail, please try to fix them, we're unlikely to be able to review until they pass. If you get stuck on a failing check, leave a note in the PR and we'll try to help.

---

## Code review

After the checks pass, a maintainer will review your code. We review as soon as we can, if you haven't heard back after a while, feel free to leave a polite ping.

Once your pull request is approved, it will be merged into `main`.

---

## Licence

KONDA is released under **GPL-3.0**. By contributing, you agree your contribution may be distributed under this licence.

---

## Questions?

Not sure if your idea fits, or how to approach something? Open a GitHub issue and ask — that's what it's for.
