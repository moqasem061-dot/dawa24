# Shipping to Elest.io — exact steps

Six pipelines, in this order. Do not skip the ordering: the Store will not boot
without PostgreSQL and Redis. Object storage and the Gateway are optional.

> Elest.io's dashboard wording shifts between releases. Where a label is quoted
> below, match it by meaning rather than character-for-character.

---

## Step 0 — The repository

The code lives at <https://github.com/moqasem061-dot/dawa24> (branch `main`).
Elest.io builds from it directly; to work locally:

```bash
git clone https://github.com/moqasem061-dot/dawa24.git
```

`.env` is ignored by `.gitignore` and must never be committed: `git status`
must not list it.

Separately, and independently of this project: rotate every credential that
appears in `F:\Dawa 24\Laravel\.env`, `.env_clean`, `.env_new`, `.env_temp` —
MySQL, SMTP, AWS, and the AI provider keys. They have been sitting in plaintext
in a folder with no version control.

---

## Pipeline 1 — PostgreSQL

1. Elest.io → **Create Service** → **PostgreSQL 17** → same region you will use
   for the Store (cross-region adds latency to every query).
2. Size: **2 vCPU / 4 GB / 80 GB** is the starting point from the plan. Storage is
   the thing to grow first.
3. Enable **automatic backups** with point-in-time recovery.
4. Once running, open the database console and create the two databases and the
   application role:

```sql
-- Two databases on one instance: one bill, no coupling between them.
CREATE DATABASE dawa24_store;
CREATE DATABASE dawa24_gateway;

CREATE ROLE dawa24_app WITH LOGIN PASSWORD '<generate a long random password>';
GRANT CONNECT ON DATABASE dawa24_store TO dawa24_app;
```

5. Connect to `dawa24_store` and enable the extensions the migrations require:

```sql
\c dawa24_store
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
```

Migration `001_foundation` also issues these, but extension creation needs
superuser rights that `dawa24_app` will not have. Creating them here as the admin
user means the migration's `IF NOT EXISTS` becomes a no-op.

6. Grants are the migrations' job, not yours. Migration `222` gives
   `dawa24_app` least-privilege access to every application schema and sets
   `NOBYPASSRLS`; migration `228` adds the two runtime `TRUNCATE`s and makes
   `platform.audit_log` append-only. Nothing to run by hand.

7. **Two connection strings, two roles.** Append `?sslmode=require` to both —
   Elest.io terminates TLS and the driver will not use it unless told to.

   Every binary reads `DATABASE_URL`; what differs is which role it names.

   | Process | `DATABASE_URL` names | Notes |
   |---|---|---|
   | `cli migrate` | the owner/admin user | also reads `DB_APP_PASSWORD` and sets `dawa24_app`'s password to it |
   | server, worker | `dawa24_app` | in `docker-compose.yml` this comes from `APP_DATABASE_URL` |

   `docker-compose.yml` requires both `MIGRATE_DATABASE_URL` (owner, for
   `migrate` only) and `APP_DATABASE_URL` (`dawa24_app`, for server and worker);
   it refuses to start without them. In production the server and worker also
   refuse a connection that is a superuser, holds `BYPASSRLS`, or owns the
   application tables (`DB_REQUIRE_RLS` defaults to `true`; `false` is a logged,
   emergency-only opt-out). A production `cli migrate` refuses to finish while
   `dawa24_app` still accepts the default password migration 222 created it with,
   or when `DB_APP_PASSWORD` is shorter than 24 characters.

   `SESSION_SECRET` is required in production (`openssl rand -hex 32`); the
   server refuses to start without it.

   The full model — roles, why tenants do not get their own database users, TLS,
   `pg_hba`, rotation of every credential — is in
   [docs/SECURITY_DATABASE_AND_SECRETS.md](docs/SECURITY_DATABASE_AND_SECRETS.md).

   Switching an existing deployment over:
   1. Set `DB_APP_PASSWORD` to a new long random value.
   2. Run `cli migrate` with the owner `DATABASE_URL` and that `DB_APP_PASSWORD`.
   3. Point the server's and worker's `DATABASE_URL` (compose: `APP_DATABASE_URL`)
      at `postgres://dawa24_app:<that password>@<pg-host>:<port>/dawa24_store?sslmode=require`.

**Restore drill, before you put real data in:** take a backup, restore it into a
scratch database, and confirm it comes back. An untested backup is not a backup.

---

## Pipeline 2 — Redis

1. Elest.io → **Create Service** → **Redis 7** → **1 GB**, same region.
2. Copy the connection URI → `REDIS_URL`.
3. Set the eviction policy to **`allkeys-lru`**. Cache, sessions and rate-limit
   counters share this instance; under memory pressure you want eviction, not
   write errors.

> Sessions live here. Evicting a session logs a user out. If that becomes a real
> problem, move sessions to Redis database `1` with `noeviction` and leave the
> cache on `0` — the config already takes a full URI with a database index.

---

## Pipeline 3 — Object storage (optional)

Uploads (product images, licences, receipts, documents) are written under
`DATA_DIR` on local disk, which must be a persistent volume shared by the server
and worker. To keep them in S3-compatible storage instead, create a **MinIO**
service (or use S3), make a `dawa24` bucket, generate access keys and fill in the
`STORAGE_*` variables; with `STORAGE_ENDPOINT` set, production refuses to start
without both keys.

---

## Pipeline 4 — The Store

1. Elest.io → **Create Service** → **CI/CD** (deploy from a Git repository).
2. Connect the `dawa24` GitHub repo, branch `main`.
3. Build: **Dockerfile** at the repository root. Elest.io detects it.
4. Port: **8080**.
5. Health check path: **`/health`**.
6. Environment variables:

```
APP_ENV=prod
APP_NAME=Dawa24
APP_BASE_URL=https://<your-domain>
PORT=8080

# The server connects as dawa24_app, never as the owner (Pipeline 1, step 7).
DATABASE_URL=postgres://dawa24_app:<DB_APP_PASSWORD>@<pg-host>:<port>/dawa24_store?sslmode=require
DB_MAX_CONNS=20
DB_MIN_CONNS=2

REDIS_URL=redis://:<password>@<redis-host>:<port>/0

SESSION_SECRET=<openssl rand -hex 32>
SESSION_SECURE=true
SESSION_TTL=720h

# Optional: Telegram bridge (docs/modules/telegram.md). Set both or neither.
TELEGRAM_BOT_USERNAME=<bot username without @>
TELEGRAM_BRIDGE_TOKEN=<openssl rand -hex 32, same value in the n8n credential>

# Optional: WhatsApp bridge (docs/modules/whatsapp.md). Number and token both or neither.
WHATSAPP_BUSINESS_NUMBER=<business number, international form without +>
WHATSAPP_BRIDGE_TOKEN=<openssl rand -hex 32, same value in the n8n credential>
WHATSAPP_NOTIFICATION_TEMPLATE=<approved template name, optional>

# Microsoft Clarity (heatmaps and session recordings on every page). Empty
# disables it and keeps Clarity out of the CSP. Verify in the browser console:
# typeof clarity === "function" and requests to *.clarity.ms/collect return 204.
CLARITY_PROJECT_ID=ymd15b26tc

GATEWAY_ENABLED=false
GATEWAY_BASE_URL=https://api.muhiya.com
GATEWAY_VIRTUAL_KEY=REPLACE_ME_WITH_VIRTUAL_KEY_FROM_GATEWAY_ADMIN
GATEWAY_CLIENT_APP=dawa24

# Google Maps Embed API key (optional). Without it the location picker renders a
# coordinate-entry fallback. Get a key in Google Cloud Console → APIs & Services
# → enable "Maps Embed API" → Credentials. The key is visible in iframe URLs, so
# restrict it to your domain via "HTTP referrers" application restrictions.
GOOGLE_MAPS_API_KEY=

LOG_LEVEL=info
LOG_FORMAT=json
```

`config.Load()` refuses to boot on anything missing or incoherent, so a
misconfigured deploy fails immediately with a list of every problem, rather than
half-working. In particular: `APP_ENV=prod` requires HTTPS URLs and
`SESSION_SECURE=true`, and `GATEWAY_ENABLED=true` requires a real virtual key.

7. **Run migrations before the new version serves traffic.** The image contains
   the CLI. It connects as the owner, applies the application's and the job
   queue's migrations, and sets `dawa24_app`'s password:

```bash
docker run --rm \
  -e APP_ENV=prod \
  -e DATABASE_URL='postgres://<admin-user>:<pw>@<pg-host>:<port>/dawa24_store?sslmode=require' \
  -e DB_APP_PASSWORD='<the dawa24_app password>' \
  -e ADMIN_EMAIL='<first admin email>' -e ADMIN_PASSWORD='<first admin password>' \
  <your-image>:<tag> /app/cli migrate
```

It exits non-zero on any failure; do not promote the new image until it
succeeds. `ADMIN_*` only create the first super_admin: a deploy never changes an
existing admin's password.

If Elest.io's pipeline supports a pre-deploy hook, put `/app/cli migrate` there.
Otherwise run it manually on each deploy that contains a new migration —
`/ready` will report `pending_migrations > 0` and refuse traffic until you do,
which is the safety net for forgetting.

8. Deploy. Then verify:

```bash
curl -s https://<domain>/health           # {"status":"ok","version":"..."}
curl -s https://<domain>/ready            # 200 + pending_migrations: 0
curl -s https://<domain>/api/v1/status    # per-component detail
```

`/ready` returning 503 with `"migrations": {"status":"down"}` means step 7 has not
been run.

---

## Pipeline 5 — The worker

The worker is the **same image, different command**. Create a second CI/CD
service from the same repository with:

- Command / entrypoint override: **`/app/worker`**
- The same environment variables as the Store
- No public port, no health check path

It runs River and processes the `imports`, `ai`, `notifications`,
`projections`, `smartorder` and `maintenance` queues. River's schema is migrated
by `cli migrate` (step 7), not by the worker. Without the worker, imports and
notifications queue up and are never processed.

---

## Pipeline 6 — The Gateway (later, deliberately)

Deploy the **existing `muhiyallm-gateway` repository unchanged** as a second
Elest.io service pointed at `dawa24_gateway`, with its own provider keys. Do not
fork it — see ADR 0005.

When you get there:

1. Deploy the Gateway service; run its own migrations (it self-migrates on boot).
2. In the Gateway admin, create a user and a plan for Dawa24, then issue a
   **virtual key** per environment.
3. Create the model aliases the Store asks for: `dawa24-fast`, `dawa24-quality`.
4. On the Store service, set `GATEWAY_ENABLED=true` and paste the virtual key.
5. Redeploy the Store. `/api/v1/status` will show `ai_gateway: ok`.

Until then the Store runs with `GATEWAY_ENABLED=false` and every AI capability
serves its deterministic fallback. **This is a supported configuration, not a
degraded one** — it is exactly what CI runs against.

---

## Environments

| | dev | staging | prod |
|---|---|---|---|
| Postgres | local docker | Elest.io, small | Elest.io, 2/4/80 |
| Redis | local docker | Elest.io, small | Elest.io, 1 GB |
| Store | `make run` | CI/CD from `main` | CI/CD from tags |
| Gateway | disabled | shared staging key | own virtual key |

Never point staging at the production database. Staging should run on an
anonymised copy.

---

## Local development note

`make` is not installed on this Windows machine. Either install it (Git Bash ships
with `mingw32-make`, or use `choco install make`), or run the targets directly:

```bash
go run ./cmd/cli migrate      # make migrate
go run ./cmd/server           # make run
go test -race ./...           # make test
go vet ./...                  # make vet
```

CI runs on Ubuntu where `make` is present, so the Makefile targets work there.
