# Internal Backend Guidelines

## Scope

These instructions apply to Go backend code under `internal/`.

## Architecture Boundaries

- `router/` registers routes and middleware. Keep route definitions thin.
- `controller/` handles HTTP transport concerns: request decoding, validation
  at the boundary, response mapping, and status codes.
- `httpcontract/` defines request and response contracts used by controllers.
- `service/` owns use-case orchestration and domain decisions.
- `dao/` and `model/` own persistence access and database row mapping.
- `infra/` owns concrete integrations such as database, credentials, provider
  clients, login runner, logging, and Codex runtime code.
- `provider/` defines provider-facing interfaces and provider result types.
- `entity/` defines domain entities, app errors, and shared domain values.

Do not put domain decisions in HTTP handlers when a service layer is the
existing pattern.

## API And Error Handling

- Keep API paths and response contracts stable unless the task explicitly
  changes the public contract.
- Use existing `entity.AppError` codes and error mapping patterns before adding
  new codes.
- Preserve cancellation-aware `context.Context` flow through services,
  providers, DAOs, and infra integrations.
- Do not log secrets, full credentials, refresh tokens, or raw `auth.json`
  payloads.

## Persistence

- Put schema changes in `internal/infra/database/migrations/`; do not rely on
  model changes alone.
- Keep DAO query behavior explicit. Avoid database operations inside loops when
  batching is practical.
- Treat `usage_snapshots.snapshot_json` and credential-related persisted fields
  as compatibility-sensitive persisted formats.
- When changing migrations, DAOs, or persisted models, verify both migration
  behavior and read/write paths where practical.

## Go Style And Verification

- Run `gofmt -w` on edited Go files.
- Prefer focused package tests first, for example:
  `go test ./internal/service -run <TestName> -count=1`.
- For backend behavior changes, run `go test ./...` when practical.
- Run `go vet ./...` for non-trivial backend changes before handoff.
- If the Go build cache is not writable, use:
  `GOCACHE=/tmp/ai-coding-account-manager-go-build go test ./...`.
