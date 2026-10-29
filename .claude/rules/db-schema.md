---
paths:
  - "api/*/model/**"
  - "internal/db/**"
---

# Entities and schema

- Every entity sets explicit `gorm:"column:...;type:..."` tags, a `TableName()` method, and (if its id is generated)
  a `BeforeCreate` that assigns `uuid.NewString()`. `User.ID` is the exception: it is the principal name, never generated.
- New entities go into `db.Entities()` in `internal/db/migrate.go`, after everything their foreign keys reference.
- Foreign keys state their intent: `constraint:OnDelete:CASCADE` for owned children (partitions, batch section),
  `RESTRICT` for references the service must check first and turn into a 409 (cluster config → cluster, → credential).
- No FK may close a cycle — PostgreSQL rejects it at create time even though SQLite doesn't.
  Precedent: `Process.LastStatusID` carries no constraint.
- Owned-collection fields on a parent (`SlurmCluster.Partitions`) are read-side projections: preload them, never save
  through them.
- Secrets get `json:"-"` on the model; the DTO decides what leaves the server.
- Migration files are `NNNN_snake_description.sql`; `db.Migrations()` panics on a bad name or duplicate number.
  Each runs in one transaction. There are no down migrations.
- `TestBaselineMigrationMatchesAutoMigrate` compares *all migrations applied* against *AutoMigrate of the entities*
  on real Postgres — any entity change without a matching migration fails it. Run it (see CLAUDE.md commands).
