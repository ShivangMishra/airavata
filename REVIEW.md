# Review checklist

What a reviewer (human, `/code-review`, managed Code Review, or `.claude/agents/convention-reviewer.md`) should flag
in this repo. Style that `gofmt`/`go vet` enforce is out of scope. Report only what the diff introduces.

## Must fix

1. **Missing or wrong authz.** Every new/changed service method that isn't a public catalogue read starts with
   `auth.Require*` or the vertical's `access` check. Admin-only unfiltered listings; owner-only credential access
   (admins included); READ vs WRITE grants respected. A guard in a controller instead of the service is a finding.
2. **Secret leakage.** `PrivateKey`, `Passphrase`, tokens, client secrets in a response DTO, a `json` tag without
   `"-"`, an error message, or a `slog` call.
3. **Shell / script injection.** A value reaching an SSH command without `ssh.ShellQuote`, or a new `#SBATCH`
   value not in `buildSlurmScript`'s line-break check.
4. **Schema drift.** Entity tags changed without a new `internal/db/migrations/NNNN_*.sql`, an edited existing
   migration, or a new entity missing from `db.Entities()`. An FK that forms a cycle.
5. **Orphaned shares.** Deleting a shareable record without `DeleteByResources` in the same transaction.
6. **Partial writes.** Multi-row writes outside `Transaction` / `WithTx`, or writes through a GORM association.
7. **Opaque errors.** A caller-caused failure returned as a bare error (becomes 500) instead of an `httpx.*` error;
   `gorm.ErrRecordNotFound` not mapped to 404.

## Should fix

8. Authz rules tested on one side only (allowed caller but no 401/403 case) in `internal/server/`.
9. Route or payload change without `docs/api.md` and `docs/openapi.yaml` updates; sharing change without
   `docs/sharing.md`.
10. Side effects (SSH, DB writes, `time.Now`) added inside a go-workflows workflow function instead of an activity.
11. New `.go` file without the ASF license header; new dependency in `go.mod` without a stated reason.
12. New comments longer than one short line, or comments that restate the code (CLAUDE.md: default to none).
