# Shipping to Elest.io — exact steps

Three pipelines, in this order. Do not skip the ordering: the Store will not boot
without PostgreSQL and Redis, and the Gateway is optional by design.

> Elest.io's dashboard wording shifts between releases. Where a label is quoted
> below, match it by meaning rather than character-for-character.

---

## Step 0 — Get the code into GitHub (10 minutes)

Nothing here is in version control yet, and the legacy folder contains live
credentials. Do this first.

```bash
cd "F:/Dawa 24/dawa24-store"
git init
git add .
git commit -m "Phase 1-2: platform foundations"
git branch -M main
git remote add origin git@github.com:<you>/dawa24-store.git
git push -u origin main
```

**Before you push, confirm `.env` is not staged.** `git status` must not list it.
The `.gitignore` already excludes `.env` and `.env.*`.

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

6. Migration `222_dawa24_app_role.up.sql` applies comprehensive least-privilege grants across all 21 schemas, sequences, and sets `ALTER ROLE dawa24_app NOBYPASSRLS` automatically when running `go run ./cmd/cli migrate`.
   If running grants manually:

```sql
\c dawa24_store
GRANT USAGE ON SCHEMA identity, profile, org, catalog, inventory, commerce,
                       promo, billing, ingest, workflow, hr, platform, ai,
                       advertising, anti_scraping, attachments, assistant,
                       branches, cart, customer, notifications, orders, payments,
                       reports, reviews, support, system, users
  TO dawa24_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA identity, profile,
      org, catalog, inventory, commerce, promo, billing, ingest, workflow, hr,
      platform, ai TO dawa24_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA identity, profile, org, catalog,
      inventory, commerce, promo, billing, ingest, workflow, hr, platform, ai
  TO dawa24_app;

-- The audit trail is append-only. Revoking these is what makes it evidence
-- rather than just another table.
REVOKE UPDATE, DELETE ON platform.audit_log FROM dawa24_app;
ALTER ROLE dawa24_app NOBYPASSRLS;
```

7. Copy the connection string for `dawa24_app`. It becomes `DATABASE_URL`. **Append `?sslmode=require`** —
   Elest.io terminates TLS and the driver will not use it unless told to.
   Note: The Go application pool verifies on startup that the connected user has `NOBYPASSRLS`. If a superuser or `BYPASSRLS` role is used, a critical security warning is logged.

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

## Pipeline 3 — Object storage (can wait)

Only needed once file uploads and product images land (Phase 5+). Create a
**MinIO** service, make a `dawa24` bucket, generate access keys, and fill in the
`STORAGE_*` variables. Until then leave them at the `.env.example` defaults; the
Store does not touch storage yet.

---

## Pipeline 4 — The Store

1. Elest.io → **Create Service** → **CI/CD** (deploy from a Git repository).
2. Connect the `dawa24-store` GitHub repo, branch `main`.
3. Build: **Dockerfile** at the repository root. Elest.io detects it.
4. Port: **8080**.
5. Health check path: **`/health`**.
6. Environment variables:

```
APP_ENV=prod
APP_NAME=Dawa24
APP_BASE_URL=https://<your-domain>
PORT=8080

DATABASE_URL=postgres://dawa24_app:<password>@<pg-host>:<port>/dawa24_store?sslmode=require
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

GATEWAY_ENABLED=false
GATEWAY_BASE_URL=https://api.muhiya.com
GATEWAY_VIRTUAL_KEY=REPLACE_ME_WITH_VIRTUAL_KEY_FROM_GATEWAY_ADMIN
GATEWAY_CLIENT_APP=dawa24-store

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

7. **Run migrations before the first deploy serves traffic.** The image contains
   the CLI:

```bash
docker run --rm \
  -e DATABASE_URL='postgres://dawa24_app:<pw>@<pg-host>:<port>/dawa24_store?sslmode=require' \
  -e REDIS_URL='redis://:<pw>@<redis-host>:<port>/0' \
  -e SESSION_SECRET='<any 32+ byte string; unused by the CLI but required by config>' \
  <your-image>:<tag> /app/cli migrate
```

Then apply the `GRANT` block from Pipeline 1 step 6.

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

It runs River, migrates River's own schema on boot, and processes the `imports`,
`ai`, `notifications`, `projections` and `maintenance` queues.

You can skip this until Phase 5, when the import pipeline lands. Nothing enqueues
jobs yet.

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
