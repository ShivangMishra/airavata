---
name: schema-change
description: Add or change a persistent entity in api/*/model and write the matching versioned migration in internal/db/migrations. Use whenever a GORM struct tag, entity field, index, constraint or table changes.
---

# Schema change

The schema reaches databases two ways — `AutoMigrate` (dev, tests) and `internal/db/migrations/*.sql` (production) —
and `TestBaselineMigrationMatchesAutoMigrate` fails if they disagree. Both must change together.

## 1. Change the entity

- Edit the struct in `api/<vertical>/model/`. Follow `.claude/rules/db-schema.md` (column/type tags, `TableName`,
  `BeforeCreate`, FK `OnDelete` intent, no cycles).
- New table → add it to `db.Entities()` in `internal/db/migrate.go` after everything it references.
- Run the SQLite suite: `go test ./internal/db/ ./internal/server/`.

## 2. Generate the migration body

Needs Postgres (`docker compose -f dev-tools/compose/compose.yml up -d postgres`). Then:

```bash
export AIRAVATA_TEST_POSTGRES_DSN='postgres://airavata:123456@localhost:15432/airavata?sslmode=disable'
go run ./.claude/skills/schema-change/ddldelta
```

It applies every committed migration to a throwaway schema, runs AutoMigrate over your entities, and prints the
DDL AutoMigrate needed — exactly what the new file must contain. It drops its schema on exit.

## 3. Write the file

- Name: next number, `internal/db/migrations/NNNN_snake_case_description.sql` (`ls internal/db/migrations` for the last one).
- Start with a `--` comment explaining *why* the schema changes (see `0002_unified_resource_sharing.sql`).
- Paste the DDL from step 2 unchanged. Add data backfill (`INSERT … SELECT`, `UPDATE`) after it if rows must move —
  `0002` is the precedent, and `internal/db/sharing_migration_test.go` shows how to test a backfill on SQLite.
- Never edit an existing migration (a hook blocks it). A wrong migration is corrected by the next one.
- Don't use `IF NOT EXISTS` guards to paper over drift; the baseline test needs the exact DDL.

## 4. Verify

```bash
go test ./internal/db/                         # with AIRAVATA_TEST_POSTGRES_DSN still exported
go run ./.claude/skills/schema-change/ddldelta # must now report "no schema changes"
```

Both Postgres tests must **run** (not skip) and pass — check `-v` output for `SKIP` if unsure.
If Postgres isn't available, say so explicitly in your summary; don't claim the migration is verified.
