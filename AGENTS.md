# Repository Guidelines

## Scope

These instructions apply to the whole repository unless a nested `AGENTS.md`
adds more specific guidance. Prefer the nested file for work under its path.

## Project Structure

This repository is a local AI coding account manager written in Go with a
static frontend.

- `cmd/ai-coding-account-manager/`: process entry point.
- `internal/`: Go backend, service logic, persistence, provider integrations,
  middleware, contracts, and routing.
- `frontend/`: static HTML/CSS/JavaScript frontend served without a build step.
- `config/`: example runtime configuration.
- `scripts/`: local development lifecycle helpers.
- `internal/infra/database/migrations/`: SQL migrations.

## Common Commands

- `mise install`: install the pinned Go toolchain from `mise.toml`.
- `go test ./...`: run the full Go test suite.
- `go vet ./...`: run standard Go static checks.
- `go run ./cmd/ai-coding-account-manager`: run the server directly.
- `go run ./cmd/ai-coding-account-manager --config config/app.fake.json`: run
  with the fake provider.
- `./scripts/local.sh start|fake|stop|logs --follow`: build and manage the
  local app lifecycle.
- `docker compose up --build`: optional hosted run path.

## Cross-Cutting Rules

- Keep changes scoped to the requested behavior. Do not reformat unrelated
  files or rewrite user work.
- Do not commit real `auth.json`, tokens, `.data/`, `.credentials/`, `.run/`,
  or local credential material.
- The service is designed for local use. Do not introduce behavior that exposes
  it to LAN or public networks without explicit approval.
- API write requests must remain same-origin JSON requests unless the API
  contract is intentionally changed.
- Responses, errors, and logs must not include full credentials, refresh tokens,
  or secret file contents.
- When behavior changes, update the smallest relevant tests or verification
  path and report exactly what was run.

## Documentation And Instructions

- Put repository-wide guidance here.
- Put Go backend-specific guidance in `internal/AGENTS.md`.
- Put frontend-specific guidance in `frontend/AGENTS.md`.
- Do not duplicate parent instructions in child files unless the child narrows
  or clarifies them for that subtree.
