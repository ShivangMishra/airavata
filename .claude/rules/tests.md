---
paths:
  - "**/*_test.go"
---

# Tests

- Use stdlib `testing` only (`t.Errorf`/`t.Fatalf`); testify is not a direct dependency — don't add it.
- HTTP behaviour (routes, status codes, authz, ownership, sharing) is tested end-to-end through
  `internal/server/server_test.go`: `h := newHarness(t)`, `h.mustDo(method, path, token, body, wantStatus)`,
  `h.do(...)` when asserting a failure, `h.seed*` helpers for fixtures. Add new helpers next to the existing ones.
- Every authz rule gets both sides tested: the allowed caller *and* the refused one (401 anonymous vs 403 wrong user).
- The harness uses in-memory SQLite with `foreign_keys(1)`; Postgres-only behaviour belongs in
  `internal/db/postgres_test.go`, gated by `AIRAVATA_TEST_POSTGRES_DSN`.
- Name tests as the behaviour they pin: `TestDeletingAClusterInUseIsRefused`, not `TestDelete2`.
- If you change a step of `docs/sharing.md`'s worked example, update `TestDocsSharingWorkedExample` with it.
