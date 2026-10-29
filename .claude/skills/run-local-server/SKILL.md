---
name: run-local-server
description: Start Postgres and airavata-server locally, get an auth token, and exercise endpoints with curl to confirm a change works end-to-end. Use when asked to run the server, smoke-test an endpoint, or reproduce an HTTP-level bug outside the test harness.
---

# Run the server locally

Prefer a test in `internal/server/` for anything you want to keep. Use this for a real-process smoke test.

## Start

```bash
docker compose -f dev-tools/compose/compose.yml up -d postgres      # host port 15432, user/pass airavata/123456
go build -o bin/airavata-server ./cmd/airavata-server
```

Run it in the background (Bash `run_in_background: true`) with a pinned token so it survives restarts:

```bash
AIRAVATA_ROOT_ACCOUNT_TOKEN=local-dev-root ./bin/airavata-server
```

Wait for it, then confirm:

```bash
until curl -sf localhost:9095/health; do sleep 1; done
```

## Expected noise at startup

- `Failed to start email monitor` — no `AIRAVATA_EMAIL_MONITOR_ADDRESS`; job monitoring by email is off. Harmless.
- The workflow backend is a SQLite file at `/tmp/airavataorchestrator.sqlite` that persists across runs. If old
  workflow instances misbehave, stop the server and delete that file.

## Call it

```bash
TOKEN=local-dev-root
curl -s localhost:9095/api/v1/slurm-clusters                                   # anonymous catalogue read
curl -s -H "Authorization: Bearer $TOKEN" localhost:9095/api/v1/ssh-credentials # owned: root's own only
```

Request bodies for every endpoint are in `docs/api.md`. `root` is `SUPER_ADMIN` and already has a `users` row.
To act as a non-admin you need CILogon (`CILOGON_CLIENT_ID`/`SECRET`) — locally, prefer the test harness for that.

## Schema

`AIRAVATA_DB_AUTO_MIGRATE=true` (default) applies entity changes on start. To test the production path instead:
`AIRAVATA_DB_AUTO_MIGRATE=false ./bin/airavata-server migrate status|up`.
Resetting the dev DB (`docker volume rm compose_postgres_data` after `down`) deletes the user's local data and is
denied in `.claude/settings.json` — tell the user the command and let them run it.

## Stop

Stop the background task. Leave Postgres running unless asked.
