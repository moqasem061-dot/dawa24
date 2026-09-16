# Test deployment on Elest.io — Dawa24 Store

Reusing the **existing PostgreSQL and Redis** rather than provisioning new ones.
AI Gateway **off** (external, `api.muhiya.com`).

Verified against the repository at the time of writing: `go build` clean,
`go test` passing, `docker compose config` valid, all ports consistent on 8070.

---

## Method: docker-compose, not Dockerfile alone

Use **docker-compose** as the Elest.io stack type. Both files matter and do
different jobs:

| File | Job |
|---|---|
| `Dockerfile` | How to **build** the image — one image containing three binaries (`server`, `worker`, `cli`) |
| `docker-compose.yml` | What to **run** — `migrate` → `server` → `worker` |

If you deploy with the Dockerfile method alone, Elest.io generates a single
`app` service that starts the server and **never applies the migrations**. The
server then boots, reports itself unready, and refuses traffic permanently. The
compose file in the repo root is what prevents that:

```
migrate   runs to completion, creates 64 tables across 13 schemas, exits
server    starts only if migrate succeeded, publishes 172.17.0.1:8070
worker    starts only if migrate succeeded
```

---

## Gotcha that will stop your first boot

**Set `APP_ENV=staging`, not `prod`.**

`config.Load()` applies extra strictness in production, including:

```go
if env.IsProd() {
    if cfg.Storage.AccessKeyID == "" || cfg.Storage.SecretAccessKey == "" {
        fail("STORAGE_ACCESS_KEY_ID and STORAGE_SECRET_ACCESS_KEY are required in production")
    }
}
```

You have no MinIO/S3 yet, so `APP_ENV=prod` makes the container **exit
immediately** with a configuration error. `staging` still forces secure cookies
and HTTPS behaviour but does not demand object storage, which no code path uses
yet.

---

## STEP 1 — Create the Store database on the Gateway's PostgreSQL

Run `dawa24-ops/postgres/10-store-db-on-gateway-instance.sql`, **Steps 1 and 2
only** for now. Step 3 comes after the first deploy.

Summary of what it does:

1. Connected to `postgres` as superuser: creates database `dawa24_store`,
   creates role `dawa24_app` (deliberately **not** a superuser — a superuser
   bypasses row-level security and would void tenant isolation), and revokes
   `CONNECT` from `PUBLIC` so the Store role cannot reach the Gateway's database
   and vice versa.
2. **Reconnect pgAdmin to `dawa24_store`**, still as superuser: creates
   `pg_trgm`, `unaccent`, `pgcrypto`, `citext`. Extensions need superuser
   rights that `dawa24_app` does not have, so creating them here turns the
   migration's `CREATE EXTENSION IF NOT EXISTS` into a harmless no-op instead of
   a permission error that aborts the whole run.

Before running Step 2, confirm you are on the right database:

```sql
SELECT current_database();   -- must print dawa24_store
```

Then note the instance host and port from the Elest.io Postgres service. You
need them for `DATABASE_URL`.

---

## STEP 2 — Redis

The Store **requires** Redis; `config.Load()` refuses to boot without
`REDIS_URL`. Reuse the existing service at
`redis2-u74003.vm.elestio.app:26381`.

Use a **database index the Gateway is not using**. The Store takes `/0`:

```
redis://:<REDIS_PASSWORD>@redis2-u74003.vm.elestio.app:26381/0
```

If the Gateway already uses `/0` on that instance, move the Store to `/2`.
Keys are prefixed `dawa24:<env>:` regardless, so a collision would be unlikely
— but a shared `FLUSHDB` would not care about prefixes.

---

## STEP 3 — Create the Elest.io pipeline

1. **Create Service → CI/CD**, connected to `github.com/muhiyatools/dawa24-store`,
   branch `main`.
2. Stack type: **docker-compose**.
3. Compose file path: `docker-compose.yml` (repository root).
4. Exposed port: **8070**.
5. Health check path: `/health`.

**Name the service something that is not `dawa24-store`** if that name is
already taken by the service currently running the Gateway. Two services with
misleading names is how the last mix-up happened.

---

## STEP 4 — Environment variables

Paste into the Elest.io environment panel. Replace every `CHANGE_ME`.

```env
APP_ENV=staging
APP_NAME=Dawa24
APP_BASE_URL=https://CHANGE_ME.vm.elestio.app
PORT=8070

# PostgreSQL — the new database on the Gateway's existing instance.
# sslmode=require is not optional: Elest.io terminates TLS and the driver will
# not use it unless told to.
DATABASE_URL=postgres://dawa24_app:CHANGE_ME_DB_PASSWORD@CHANGE_ME_PG_HOST:CHANGE_ME_PG_PORT/dawa24_store?sslmode=require
DB_MAX_CONNS=20
DB_MIN_CONNS=2
DB_STATEMENT_TIMEOUT=30s

# Redis — note the leading colon; Redis has no username.
REDIS_URL=redis://:CHANGE_ME_REDIS_PASSWORD@redis2-u74003.vm.elestio.app:26381/0
REDIS_POOL_SIZE=10

# Sessions — minimum 32 bytes or the process refuses to boot.
# Generate with: openssl rand -hex 32
SESSION_SECRET=CHANGE_ME_openssl_rand_hex_32
SESSION_COOKIE_NAME=dawa24_session
SESSION_TTL=720h
SESSION_SECURE=true

# AI Gateway — OFF. External instance, no virtual key issued yet.
# Every AI capability falls back to a deterministic path (Arabic trigram
# similarity for product matching, heuristics for column detection), so the
# application is fully functional this way. This is the configuration CI runs
# against, not a degraded mode.
GATEWAY_ENABLED=false
GATEWAY_BASE_URL=https://api.muhiya.com
GATEWAY_CLIENT_APP=dawa24-store
GATEWAY_TIMEOUT=60s

# Object storage — no code path uses it yet. Leave blank; this is why
# APP_ENV must be staging rather than prod.
STORAGE_ENDPOINT=
STORAGE_BUCKET=dawa24
STORAGE_ACCESS_KEY_ID=
STORAGE_SECRET_ACCESS_KEY=
STORAGE_USE_PATH_STYLE=true

LOG_LEVEL=info

# Worker pool sizes. The workers are currently stubs (see docs/AUDIT_2026-08-16.md
# finding C3), so these have no practical effect yet.
WORKER_IMPORTS=2
WORKER_AI=4
WORKER_NOTIFICATIONS=4
WORKER_PROJECTIONS=2
WORKER_MAINTENANCE=1
```

**Do not set `GATEWAY_VIRTUAL_KEY`.** With `GATEWAY_ENABLED=false` it is unused,
and setting `GATEWAY_ENABLED=true` without a key is rejected at boot.

---

## STEP 5 — Reverse proxy

| Listen | Target | Path |
|---|---|---|
| HTTPS 443 | HTTP · `172.17.0.1` · **8070** | `/` |

The target port must be **8070** and match `docker-compose.yml` line 92 and
`PORT` exactly. A mismatch here is what produces a bare `502 Bad Gateway`.

**Do not enable Basic Auth** on this row — the application has its own login.

---

## STEP 6 — Deploy and verify

Deploy. Then, in order:

```bash
curl -s https://<domain>/health | jq
```
Expect `{"status":"ok","service":"dawa24-store","version":"..."}`.

```bash
curl -s https://<domain>/ready | jq
```
Expect **200** with `"pending_migrations": 0` and every component `"ok"`.

```bash
curl -s https://<domain>/api/v1/status | jq
```
Expect `ai_gateway` reported as `"disabled"` — that is correct and intended.

```bash
curl -s https://<domain>/api/v1/catalog/categories | jq
curl -s https://<domain>/api/v1/platform/countries | jq
```
Expect empty arrays. The schema exists; no data has been loaded.

Open `https://<domain>/` in a browser for the Arabic RTL landing page. Note it
is a static page that does not call the API — see the caveats below.

---

## STEP 7 — Post-deploy grants

Once `/ready` returns 200, run **Step 3 and Step 4** of
`10-store-db-on-gateway-instance.sql` against `dawa24_store`.

Step 4's first query is the one that matters. Every tenant-owned table must show
`rowsecurity = true` **and** `forcerowsecurity = true`. `dawa24_app` owns those
tables because it ran the migrations, and **a table owner bypasses RLS unless the
table was declared FORCE**. If any row shows false, tenant isolation is not
actually in effect.

---

## Troubleshooting

| Symptom | Cause |
|---|---|
| Container exits immediately, logs list config problems | `config.Load()` did its job. Every problem is listed in the first log line. Most likely `APP_ENV=prod` without storage keys, or `SESSION_SECRET` under 32 bytes. |
| `502 Bad Gateway` | No container running, or the reverse proxy target is not `172.17.0.1:8070`. Check `docker ps` on the service first. |
| `/ready` returns 503, `migrations: down`, `pending > 0` | The `migrate` container did not run or failed. Check its logs. This is the safety net working. |
| `/ready` 503 with `database: connecting` | The server is up and retrying. Wrong host/port/password, or `sslmode=require` missing. |
| `permission denied for schema ...` | Step 3 grants not run yet. |
| `extension "pg_trgm" is not available` | Step 2 was run against the wrong database. |
| Landing page loads but the font looks wrong | Known issue: the page requests Google Fonts, which the CSP in `internal/platform/httpx/middleware.go` blocks (`font-src 'self'`). Cosmetic. |

---

## What you are actually deploying — be clear on this

This is a **test deployment of a partially built system**. From
`docs/AUDIT_2026-08-16.md`:

- **Backend ~55%, frontend ~3%, overall ~35%.**
- **63 API endpoints exist** against roughly 580 in the Laravel app.
- **The landing page is static HTML** and does not call the API. There is no
  login screen, no product table, no cart, no admin panel.
- **The four background workers are stubs** that log and return.
- **`cli migrate-data` is fabricated** — it reports a successful ETL having read
  and written nothing. Do not run it, and do not treat it as a migration path.
- **The migrations have never been executed anywhere.** This deployment is their
  first real run; expect to fix errors.

What this deployment proves: the image builds, the schema applies, the service
connects to PostgreSQL and Redis, RLS is enforced, and the API answers. That is
worth confirming now. It is not a working marketplace and the Laravel app remains
the only functioning one.
