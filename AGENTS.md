# Conventions for AI coding agents (and humans)

Read this before changing anything. It is short on purpose.

> **Continuing this project?** Start with [`HANDOFF.md`](HANDOFF.md) — it records
> exactly what is built, what is not, every reference path, and the remaining
> phases in order.

## What this is

Dawa24 is a **B2B pharmaceutical marketplace** for the Egyptian market: pharmacies
buy from suppliers. It is being rebuilt from a Laravel 12 + Livewire monolith
(141 tables, 353 Livewire components, zero tests) into this Go modular monolith.

**Arabic is the primary language.** Every user-facing string is bilingual
`{"ar":"...","en":"..."}`. Design RTL first.

## Non-negotiable rules

1. **Money never touches `float64`.** Use `internal/shared/money.Amount`. The
   database uses `NUMERIC(p,2)`. If you find yourself writing `float`, stop.

2. **No AI provider names outside `internal/platform/gateway/`.** The Store asks
   the Gateway for a *capability* (`product.match`), never for a provider or a
   model. `make check-provider-isolation` fails the build otherwise.

3. **Every AI capability has a deterministic fallback.** A pharmacy must be able
   to order and a supplier must be able to import when the Gateway is down. If
   you add a capability, you add its fallback in the same change.

4. **Tenant-scoped queries run inside `db.InTx` / `db.InReadTx`.** Those set the
   Postgres GUC that row-level security reads. Never reach for `db.Pool()`
   directly in a module. Cross-tenant access requires `database.AsSystem(ctx)`,
   which is deliberately greppable.

5. **Module boundaries are enforced by the linter.**
   - `modules/*` may import `shared/*` and `platform/*`
   - `modules/A` may **not** import `modules/B` — use events or a public interface
   - `platform/*` may **not** import `modules/*`
   - `shared/*` imports nothing from this repo
   `golangci-lint` (depguard) will reject violations.

6. **400 lines per Go file, maximum.** `make check-file-size` enforces it. Split
   by concern: `domain.go`, `service.go`, `repository.go`, `http/`, `jobs/`.

7. **Preserve legacy behaviour during migration.** Business rules, primary key
   values, order-number formats and money semantics are ported *exactly*, even
   where they look wrong. We are proving parity against the old system. Fixes
   happen after cutover, deliberately, with tests. If something looks like a bug,
   write it down in `docs/modules/<module>.md` — do not "improve" it.

## Layout

```
cmd/{server,worker,cli}          entrypoints; one image, three binaries
internal/platform/               infrastructure: config, database, cache,
                                 storage, queue, httpx, observability, gateway
internal/shared/                 dependency-free leaves: money, i18n, arabic,
                                 apperr, pagination
internal/modules/<context>/      one bounded context, mirroring a DB schema
  domain.go                      types + rules, zero I/O
  service.go                     use cases
  repository.go                  interface
  postgres/                      SQL implementation
  http/                          handlers + routes
  views/                         templ templates
  jobs/                          River workers
db/migrations/                   NNN_name.up.sql / .down.sql, embedded
docs/adr/                        why decisions were made
docs/modules/                    one page per module: entities, invariants
```

Postgres schemas mirror module names: `identity`, `org`, `catalog`, `inventory`,
`commerce`, `promo`, `billing`, `ingest`, `workflow`, `hr`, `platform`, `ai`.

## Working effectively here

- **Read `docs/modules/<name>.md` before the code.** It states the entities,
  invariants and known legacy quirks in one page.
- **Work one module at a time.** The boundaries make this natural and keep the
  context you need small.
- **Let the build catch errors.** `make check` runs every gate CI runs. A failing
  pipeline is cheaper than a long conversation.
- **Never hand-write what can be generated.** Migrations follow a template; SQL
  becomes typed Go via sqlc; templates become Go via templ.
- **Do not re-derive the legacy schema.** It is documented in
  `../docs/rebuild/SCHEMA_INVENTORY.md` (141 tables, every column and index).

## Migrations

- `NNN_name.up.sql` + `NNN_name.down.sql`, wrapped in `BEGIN; ... COMMIT;`
- **Never edit an applied migration.** The runner checksums them and refuses.
  Add a new one.
- Every migration must be backward-compatible with the currently deployed
  version (expand/contract), because migrations run *before* the new image is
  promoted and rollback is "redeploy the old image".
- Tenant-owned tables get `organization_id`, `ENABLE`/`FORCE ROW LEVEL SECURITY`,
  and a policy using `platform.tenant_visible(organization_id)`.
- Carry the Arabic column comments across from the legacy schema. They are the
  only documentation that exists for some columns.

## Testing

- Domain logic: table-driven unit tests, colocated.
- Repositories: integration tests against real Postgres.
- **Tenant isolation: every tenant-owned table needs a test proving a
  cross-tenant read returns zero rows.** This is a CI gate.
- AI features: one test with the Gateway disabled, asserting the fallback path
  produces a usable result.
- Money: exact-value assertions. No approximate comparison, ever.

## Current state

Phase 1–2 of the rebuild: platform foundations. Built so far — config, database
with RLS, cache, gateway client, HTTP middleware, migration runner, health
endpoints, worker with River, and the `identity`/`org` schemas.

Not built yet: catalog, inventory, commerce, promo, billing, ingest, workflow,
hr modules; the templ UI layer; the legacy data ETL. See
`../docs/rebuild/REBUILD_MASTER_PLAN.md` §13 for the phase order.
