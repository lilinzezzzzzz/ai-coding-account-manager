# Repository Guidelines

## Project Structure & Module Organization

This repository is a local AI coding account manager written in Go with a static frontend. The process entry point is `cmd/ai-coding-account-manager/`. Backend code lives under `internal/`: `app/` wires configuration and services, `router/` registers Chi routes and middleware, `controller/` handles HTTP requests, `httpcontract/` defines API contracts, `service/` orchestrates use cases, `dao/` and `model/` cover persistence, and `infra/` holds database, provider, credentials, login runner, and Codex runtime implementations. Static assets are in `frontend/static/`. Config examples are in `config/`, local scripts in `scripts/`, and SQL migrations in `internal/infra/database/migrations/`.

## Build, Test, and Development Commands

- `mise install`: install the pinned Go toolchain from `mise.toml`.
- `go test ./...`: run the full Go test suite.
- `go test ./internal/config ./internal/security`: run focused package tests.
- `gofmt -w <files>`: format edited Go files before committing.
- `go vet ./...`: run standard static checks.
- `go run ./cmd/ai-coding-account-manager`: run the server directly.
- `go run ./cmd/ai-coding-account-manager --config config/app.fake.json`: run with the fake provider.
- `./scripts/local.sh start|fake|stop|logs --follow`: build and manage the local app lifecycle.
- `docker compose up --build`: optional hosted run path.

## Coding Style & Naming Conventions

Use idiomatic Go: tabs via `gofmt`, short package names, exported identifiers only for cross-package APIs, and explicit error returns. Keep domain logic out of HTTP handlers; controllers validate and delegate to services. Frontend code is plain HTML, CSS, and JavaScript with no build chain.

## Testing Guidelines

Tests use Go's standard `testing` package and live beside implementation files as `*_test.go`. Name tests by behavior, for example `TestLoadFileAllowsWildcardBindAddr`. Add focused regression tests for config validation, request security, persistence, and API behavior. If the Go build cache is not writable, run `GOCACHE=/tmp/ai-coding-account-manager-go-build go test ./...`.

## Security & Configuration Tips

Do not commit real `auth.json`, tokens, `.data/`, `.credentials/`, or `.run/`. The service is designed for local use; do not expose it to LAN or public networks. API write requests require same-origin JSON requests, and responses or logs must not include full credentials or refresh tokens.
