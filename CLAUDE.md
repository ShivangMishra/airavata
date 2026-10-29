# Apache Airavata — Go API server

Single Go module (`github.com/apache/airavata`, `go 1.25.0`) serving a REST API over PostgreSQL that
registers Slurm clusters, credentials, data and applications, and launches/monitors batch jobs over SSH.
The Java/Thrift codebase is gone; comments mentioning Spring/Java explain *why* behaviour was carried over.
Domain terms (cluster vs cluster config, template vs deployment, process vs batch section) → `CONTEXT.md`.

## Commands

Targeted first. Everything below runs in seconds unless flagged.

- Test one package: `go test ./internal/server/ -run TestStorageGroupSharing`
- Test everything: `go test ./...` (~3s, SQLite in-memory, no services needed)
- Vet: `go vet ./...` — also flags stdlib APIs newer than go.mod's `go 1.25` (local toolchain may be newer; CI uses 1.25)
- Format: `gofmt -w <file>` (a hook does this after every edit)
- Build: `go build ./...`; binary: `go build -o bin/airavata-server ./cmd/airavata-server`
- Race suite, as CI runs it: `go test -race ./...` (~25s; run before opening a PR, not after every edit)
- Postgres-only schema tests (skip silently otherwise):
  `docker compose -f dev-tools/compose/compose.yml up -d postgres` then
  `AIRAVATA_TEST_POSTGRES_DSN='postgres://airavata:123456@localhost:15432/airavata?sslmode=disable' go test ./internal/db/`
- Run the server locally → `/run-local-server` skill.

No Makefile, no codegen, no golangci-lint. CI (`.github/workflows/ci.yml`) = `gofmt -l .` empty, `go vet`, `go build`, `go test -race`.

**Definition of done:** `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass — paste the output.
If the change touches entities or `internal/db/migrations/`, the Postgres schema tests above must pass too.

## Layout (one line per folder; it is not a conventional Go layout)

- `api/<vertical>/{model,dto,repository,service,controller}/` — one package per layer per vertical:
  `iam`, `credentials`, `compute`, `application`, `data`, `process`, `sharing`.
- `internal/app/app.go` — the *only* place repositories and services are constructed and wired.
- `internal/server/server.go` — the *only* place routes are registered; `server_test.go` is the HTTP test harness.
- `internal/db/` — `Entities()` list, AutoMigrate, embedded versioned migrations in `migrations/`.
- `internal/orchestration/` — go-workflows engine, Slurm script rendering, email-driven job monitoring.
- `internal/ssh/` — SSH/SCP client used for job submission and data transfer.
- `dev-tools/web-api-client/` — separate Next.js console for manual API calls (has its own CLAUDE.md).
- `.vscode/launch.json` is a stale Java leftover; ignore it.

## Architecture rules

- Controllers only bind/validate (`httpx.Bind`), call one service method, and write via `httpx.WriteJSON`/`httpx.WriteError`.
  No auth, no DB, no business rules in controllers.
- Authorization happens in the **service** method, first thing: `auth.RequireAdmin`, `auth.RequireAuthenticated`,
  or the vertical's ownership/sharing `access` check. The middleware never rejects anonymous callers, so a service
  method without a guard is publicly reachable.
- Return `httpx.NotFound/BadRequest/Conflict/Forbidden/Invalid`, not bare errors, for anything the caller caused.
  Any other error becomes an opaque 500. Map `gorm.ErrRecordNotFound` with the vertical's `notFoundAs`.
- Multi-row writes run in `s.db.WithContext(ctx).Transaction(...)` using `repo.WithTx(tx)`; never write through a
  GORM association (e.g. `cluster.Partitions`) — save each child through its own repository.
- Ownership/sharing logic lives in `api/sharing/service` (`Access`, `ResourceGuard`). Do not re-implement it per vertical.
- Comments: default to none. Never write multi-paragraph docstrings or multi-line comment blocks; one short line at most.
- Use stdlib `net/http` `ServeMux` patterns (`"GET /api/v1/x/{id}"`), not a router library. New deps need a reason.

Reference implementations — copy these, don't invent:
- Admin-writable catalogue resource: `api/compute/` (SlurmCluster: model → repository → service → dto → controller).
- Owned + shareable resource: `api/compute/service/config.go` + `guards.go` (SlurmClusterConfig).
- HTTP behaviour test: `internal/server/server_test.go` (`newHarness`, `h.mustDo`, tokens `tokenAlice/tokenBob/tokenAdmin`).
- Pure unit test: `internal/orchestration/slurm_test.go`.

## Invariants (hooks enforce the starred ones)

- IMPORTANT: a schema change = edit the entity **and** add `internal/db/migrations/NNNN_description.sql` with the
  next number. Never edit an applied migration (*); fix mistakes with a new one. Procedure → `/schema-change` skill.
- Secrets (`PrivateKey`, `Passphrase`, tokens) are tagged `json:"-"` on models and never appear in a response DTO or log line.
- Every value interpolated into a remote shell command goes through `ssh.ShellQuote`; values written into `#SBATCH`
  lines are rejected if they contain `\r`/`\n` (see `buildSlurmScript`).
- Roles come from the `user_roles` table, never from token claims. Principal names are `cilogon:<id>` or `root`.
- Deleting a shareable record must delete its `resource_sharings` rows in the same transaction
  (`sharing.WithTx(tx).DeleteByResources`, as in `SCPDataStorageService.Delete`) — no FK does it.
- New `.go` files start with the ASF license header copied from any existing file (*).
- API behaviour changes update `docs/api.md` **and** `docs/openapi.yaml`; sharing changes also `docs/sharing.md`
  (its worked example is asserted by `internal/server/docs_sharing_test.go`).

## Gotchas

- Tests build services with a `nil` workflow backend (the go-workflows SQLite driver collides with `glebarez/sqlite`).
  Code reached from HTTP must tolerate `ExecutionEngine` having no orchestrator (`ErrNoWorkflowBackend`).
- SQLite tests don't prove Postgres DDL. Constraint names are capped at 63 chars (`db.NamingStrategy`); cycles between
  tables fail on Postgres only (see `Process.LastStatusID` comment).
- The root token is regenerated on every start unless `AIRAVATA_ROOT_ACCOUNT_TOKEN` is set.
- `internal/ssh` uses `InsecureIgnoreHostKey` — a known gap; don't copy it into new code paths.
- Known authz gap: `GET /api/v1/processes/{id}` is unguarded (INSTALL.md). Don't treat it as the pattern.

## Before you change…

- …auth, roles or the root account → read `INSTALL.md` "Authentication" and "Verifying the installation" (the access matrix).
- …sharing, or make a record shareable → read `docs/sharing.md` "Notes for implementers"; procedure in `/add-api-resource`.
- …anything in `internal/orchestration/` → `.claude/rules/orchestration.md` loads automatically.

## Repo etiquette

- Branch from `master` and open a PR against it; don't push to `master` directly. `.asf.yaml` blocks force-push
  and deletion on `master`, `release/*`, `rel/*` — and so does `.claude/settings.json`.
- Commit subject: imperative, ≤72 chars, optional scope (`fix(slurm): …`); squash-merge adds `(#NNN)`.
- One vertical or concern per PR. Before opening it, run `go test -race ./...` and the `convention-reviewer`
  subagent (its checklist is `REVIEW.md`).
- Personal preferences go in `CLAUDE.local.md` (gitignored), not here.

When compacting, keep: the definition of done, the list of files changed so far, and any failing test names with their errors.
