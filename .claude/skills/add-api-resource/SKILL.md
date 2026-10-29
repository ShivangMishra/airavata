---
name: add-api-resource
description: Add a new REST resource or endpoint to an api/<vertical>, or make an existing record shareable. Covers model, repository, service authz, DTO, controller, wiring in internal/app and internal/server, tests and docs.
---

# Add an API resource

Work in this order; each step names the file to copy from. Read the reference before writing the new one.

## 1. Model and schema
Entity in `api/<vertical>/model/` → follow the `/schema-change` skill (entity + migration together).

## 2. Repository — copy `api/compute/repository/cluster.go`
- `type XRepository struct{ db *gorm.DB }`, `NewXRepository(db)`, `WithTx(tx)`.
- Methods take `ctx` first and call `r.db.WithContext(ctx)`. `FindByID` returns `gorm.ErrRecordNotFound` untouched.
- Lookups of sub-resources are scoped by parent id (`FindByIDAndClusterID`) so ids can't be reached through another parent's path.

## 3. DTO — copy `api/compute/dto/cluster.go`
- `XRequest` with `Validate() []httpx.FieldError` that collects *every* violation via `httpx.Constraints`;
  `httpx.Bind` calls it and turns the result into a 400. Field names are JSON paths (`inputs[0].name`).
- `ApplyXRequest(model, req)` and `ToXResponse(model)`. Responses never carry secrets.

## 4. Service — copy `api/compute/service/cluster.go` (catalogue) or `config.go` (owned + shareable)
- First line of every method is the authz decision. Pick one, deliberately:
  - catalogue write → `auth.RequireAdmin(ctx)`; catalogue read → open (no guard);
  - self-service create → `auth.RequireAuthenticated(ctx)` and set `OwnerID = &principal.Name`; the caller must have
    a `users` row (`404 No user record found…` otherwise — reuse the vertical's existing lookup);
  - owned read/update/delete → the vertical's `access` check (READ for get, WRITE for update, owner for delete/share).
  - unfiltered list of owned records → `auth.RequireAdmin`; per-caller list → only owned + shared rows.
- Errors: `notFoundAs(err, "X not found: %s", id)`, `httpx.Conflict` for "still in use" (check before the FK refuses).
- Multi-row writes in `s.db.WithContext(ctx).Transaction` with `repo.WithTx(tx)`.

## 5. Controller — copy `api/compute/controller/cluster.go`
- `Register(mux)` with method+path patterns under `/api/v1/<kebab-plural>/{xId}`. Path param names are camelCase ids.
- Handlers: `httpx.Bind` → service → `httpx.WriteJSON` (201 on create, 204 on delete) / `httpx.WriteError`. Nothing else.

## 6. Wire it
- Repository field + constructor in `Repositories` / `NewRepositories`; service in `Services` / `NewServices`
  (`internal/app/app.go`).
- One `Register` line in `internal/server/server.go`.

## 7. Making a record shareable
Read `docs/sharing.md` "Notes for implementers" first. Then:
- Add a `ResourceType…` constant **and** its case in `ResourceType.Valid()` (`api/sharing/model/sharing.go`).
- Implement a `ResourceGuard` (`ResourceType`, `Label`, `RequireControlled`) — copy `configGuard` in
  `api/compute/service/guards.go` — and a `NewXSharingService` constructor.
- Add a `Services` field and a `sharingctl.New(svc, "/api/v1/<plural>", "<xId>").Register(mux)` line.
- In the record's `Delete`, call `sharing.WithTx(tx).DeleteByResources(...)` in the same transaction.

## 8. Tests — in `internal/server/server_test.go` (or a sibling `*_test.go` in `package server_test`)
- Happy path via `h.mustDo`, plus every refusal: anonymous → 401, wrong user → 403, missing → 404, in use → 409,
  bad body → 400 with the expected `firstFieldError`.
- Shareable: owner, READ grantee, WRITE grantee, group member, outsider.

## 9. Docs
- `docs/api.md`: a section with the route list and one worked `curl` example per endpoint, matching neighbours.
- `docs/openapi.yaml`: paths + schemas, copying the neighbouring resource's shape (OpenAPI 3.1; nothing generates or checks it, so diff it by eye).
- Access-matrix change → `INSTALL.md` "Verifying the installation".

## Done when
`gofmt -l .` empty, `go vet ./...`, `go test ./...` pass, and the docs list the new routes.
