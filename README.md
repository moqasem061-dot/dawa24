# Dawa24 Store

B2B pharmaceutical marketplace — Go modular monolith replacing the Laravel 12 +
Livewire application.

Companion documents live in `../docs/rebuild/`:
[master plan](../docs/rebuild/REBUILD_MASTER_PLAN.md) ·
[legacy schema inventory](../docs/rebuild/SCHEMA_INVENTORY.md)

---

## Quick start

```bash
cp .env.example .env
# generate a session secret (must be >= 32 bytes)
printf 'SESSION_SECRET=%s\n' "$(openssl rand -hex 32)" >> .env

make up          # postgres + redis + minio
make migrate     # build the schema
make run         # http://localhost:8080
```

Verify:

```bash
curl -s localhost:8080/health | jq
curl -s localhost:8080/ready  | jq
curl -s localhost:8080/api/v1/status | jq
```

`make help` lists every target.

## Architecture in one paragraph

One Go binary, three entrypoints (`server`, `worker`, `cli`), one PostgreSQL
database, Redis for cache/sessions/rate-limits, MinIO for files. Business code is
organised into modules under `internal/modules/`, each mirroring a PostgreSQL
schema, with boundaries enforced by `golangci-lint`. Tenant isolation is enforced
by PostgreSQL row-level security, not by developer discipline. All AI capability
is behind the MuhiyaLLM Gateway; the Store holds no provider credentials and
functions completely without it.

See [`docs/adr/decisions.md`](docs/adr/decisions.md) for why, and
[`AGENTS.md`](AGENTS.md) for the working rules.

## Layout

```
cmd/server        HTTP process
cmd/worker        River background jobs
cmd/cli           migrations and operational tasks

internal/platform config, database (+RLS), cache, storage, queue,
                  httpx, observability, gateway
internal/shared   money, i18n, arabic, apperr — dependency-free leaves
internal/modules  bounded contexts (identity, org, catalog, ... )
internal/ui       templ layouts, components, static assets

db/migrations     NNN_name.up.sql / .down.sql, embedded in the binary
docs/adr          decision records
docs/modules      one page per module
```

## The rules that are enforced, not suggested

| Rule | Enforced by |
|---|---|
| No AI provider names outside `platform/gateway` | `make check-provider-isolation` |
| No Go file over 400 lines | `make check-file-size` |
| `shared/` imports nothing internal | `depguard` |
| `platform/` never imports `modules/` | `depguard` |
| Applied migrations are immutable | checksum in the migration runner |
| Money is never a float | `money.Amount` is the only monetary type |

`make check` runs all of them — the same set CI runs.

## Current state

**Phase 1–2: platform foundations.** Built: config with fail-fast validation,
PostgreSQL pool with RLS tenant context, Redis cache, Gateway client with circuit
breaker and fallbacks, HTTP middleware, embedded migration runner, health and
readiness endpoints, River worker, and the `identity` + `org` schemas.

**Not built yet:** catalog, inventory, commerce, promo, billing, ingest, workflow
and hr modules; the templ UI layer; the legacy data ETL. Phase order is in
[master plan §13](../docs/rebuild/REBUILD_MASTER_PLAN.md).

## Deployment

Three Elest.io pipelines: PostgreSQL, Gateway (`api.muhiya.com`), and this Store.
Migrations run as their own pipeline step before the image is promoted; rollback
is redeploying the previous image. See ADR 0004.
