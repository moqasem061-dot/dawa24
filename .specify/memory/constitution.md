# Dawa24 Constitution

This file summarises the rules in `AGENTS.md`, which is canonical. If the two
ever disagree, `AGENTS.md` wins. Security rules live in
`docs/security/security_constitution.md`. This file adds no new rules.

## Core Principles

### I. Money Is Exact
Money never touches `float64`. Use `internal/shared/money.Amount`; the database
uses `NUMERIC(p,2)`. Money tests assert exact values, never approximations.

### II. Arabic First
Every user-facing string is bilingual `{"ar":"...","en":"..."}`. Design right-to-left first.

### III. AI Behind Capabilities, With Fallbacks
Modules ask the Gateway for a capability (for example `product.match`), never for
a provider or model. Every AI capability ships with a deterministic fallback in the
same change, so ordering and importing keep working when the Gateway is down.

### IV. Tenants Are Isolated In The Database
Tenant-scoped queries run inside `db.InTx` or `db.InReadTx`. Cross-tenant access
requires `database.AsSystem(ctx)`. Every tenant-owned table has row-level security
and a test proving a cross-tenant read returns zero rows.

### V. Module Boundaries Are Enforced
`modules/*` may import `shared/*` and `platform/*`. A module never imports another
module; it uses events or a public interface. `platform/*` never imports `modules/*`.
`shared/*` imports nothing from this repository.

### VI. Small Files, Safe Migrations
Go files stay at 400 lines or fewer. Migrations are `NNN_name.up.sql` and
`.down.sql`, are never edited after they are applied, and stay backward-compatible
with the deployed version.

### VII. This Platform Is The Source Of Truth
The Laravel system is retired and is not a parity baseline. Keep data compatibility
(primary keys, order-number formats, imported records). When behaviour is wrong,
fix it here with a test and record the change in `docs/modules/<module>.md`.

## Development Workflow

- Read `docs/modules/<name>.md` before the code, and work one module at a time.
- Domain tests are table-driven and colocated; repository tests run against real Postgres.
- Run `make check`, which runs every gate CI runs, before merging.

## Governance

`AGENTS.md` is the canonical source. Changes to these principles are made in
`AGENTS.md` first, then mirrored here, with the version below bumped.

**Version**: 1.0.0 | **Ratified**: 2026-11-07 | **Last Amended**: 2026-11-07
