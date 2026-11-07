# Database roles, credentials and secrets

This guide covers:

- how Dawa24 connects to PostgreSQL
- which database users exist, and why platform clients (pharmacies, suppliers, staff) do not get one each
- the security layers that together enforce tenant isolation
- every credential that has to be rotated after the 2026-09 exposure, and how to rotate each one
- how to purge the leaked values from git history
- the complete environment-variable reference

Read it before the first production deploy and again after any credential exposure.

---

## 1. The problem that was fixed

Until this change, the server and worker could connect to PostgreSQL as the **schema owner / superuser**:

- `docker-compose.yml` fell back to `postgres:<default password>@postgresql`.
- `DB_REQUIRE_RLS` defaulted to `false`.

With a superuser or `BYPASSRLS` connection, PostgreSQL ignores every row-level security (RLS) policy. Many tenant screens load records by ID and rely on RLS to hide other companies' rows (AGENTS.md rule 4), so every one of them became a cross-tenant leak waiting for a missed `WHERE` clause.

**What the code does now:**

| Where | Behaviour |
|---|---|
| `internal/platform/config` | In production `DB_REQUIRE_RLS` defaults to **true**. `SESSION_SECRET` is **required**. |
| `internal/platform/database/connection.go` | The server and worker **refuse to start** if their role is a superuser, holds `BYPASSRLS`, or **owns application tables**. An owner can `ALTER TABLE … DISABLE ROW LEVEL SECURITY` or drop tables. |
| `cmd/cli/app_role.go` | A production `migrate` fails if `dawa24_app` still accepts the default password from migration 222, or if `DB_APP_PASSWORD` is under 24 characters. |
| `docker-compose.yml` | `MIGRATE_DATABASE_URL`, `APP_DATABASE_URL`, `DB_APP_PASSWORD`, `REDIS_PASSWORD` and `SESSION_SECRET` are **required**, with no defaults. The app port binds to `127.0.0.1` only. |
| `deploy/hostinger/*` | Secrets are required. `init-db.sh` quotes the password safely, sets full role attributes, and revokes `PUBLIC` access. |
| `cmd/cli` | No default admin, seed or load-test passwords remain. `create-admin` reads `ADMIN_PASSWORD` from the environment. |

`DB_REQUIRE_RLS=false` still exists as an emergency-only escape hatch. It logs an ERROR on every start. Do not leave it set.

---

## 2. Database users (roles): who needs one

### 2.1 Do pharmacies, suppliers and staff each need a database user? **No.**

Platform clients are **application users**: rows in `identity.users`, with memberships in `org.members`. They never talk to PostgreSQL directly. Every request goes through the Go server, which already knows the authenticated user and organisation.

The server then opens each transaction with `db.InTx` / `db.InReadTx`, which sets:

- `app.current_org_id = <caller's organisation>`, or
- `app.is_system = on` for deliberate cross-tenant system work (`database.AsSystem`)

The RLS policy `platform.tenant_visible(organization_id)` reads those settings. So one database role (`dawa24_app`) serves every tenant, and PostgreSQL still returns only the calling tenant's rows.

A database user per tenant (or per account type) would be **worse**:

- **Connection pools could not be shared.** Each tenant or role needs its own pool, so thousands of pharmacies would exhaust PostgreSQL's `max_connections`.
- **Privilege sprawl.** Every table added by a migration would need grants re-issued to thousands of roles.
- **More credentials to leak and rotate,** and each must still be stored somewhere the server can read it. The server remains the trust boundary either way.
- **It adds no protection over RLS plus the GUC.** Isolation still depends on the server choosing the right identity per request.

The account types (pharmacy, supplier, employee, platform staff) are separated by the **RBAC and audience layers** in the application (`internal/platform/rbac`, `authctx`). They are not separated by database users.

### 2.2 The database roles that should exist

Each **workload** (process) gets its own credential, with only the privileges that workload needs:

| Role | Login | Used by | Privileges | Env var / URL | Separate? |
|---|---|---|---|---|---|
| **owner** (the role that ran the migrations: `postgres`, or Elestio's generated admin) | yes | `cli migrate` only; human DBA | owns the schemas; DDL; `CREATEROLE` (migrations create roles) | `MIGRATE_DATABASE_URL` | **Yes.** Never given to server or worker. |
| **`dawa24_app`** | yes | server, worker | `NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE`, owns nothing; `SELECT/INSERT/UPDATE/DELETE` on app tables (migration 222); `TRUNCATE` on two index tables; `platform.audit_log` append-only (228) | `APP_DATABASE_URL`, password `DB_APP_PASSWORD` | **Yes** (from owner). Server and worker may share it (see 2.3). |
| `dawa24_assistant_ro` | **no** | assistant dataset engine, via `SET LOCAL ROLE` inside `dawa24_app` sessions | column-level `SELECT` only; `BYPASSRLS` (the compiler adds the tenant predicate) | none | Created by migration 214. No password exists. |
| `dawa24_sql_console` | **no** | admin developer SQL console, via `SET LOCAL ROLE` in a read-only transaction | read-only | none | Created by migration 217. No password exists. |
| **backup** (recommended) | yes | `pg_dump` / backup job | `pg_read_all_data` (PG14+) plus `BYPASSRLS` so the dump includes every row; nothing else | the backup tool's own config | **Yes.** Never in app env vars. |
| **ETL** (`cmd/etl`, one-off legacy import) | yes | legacy migration only | owner-level during the import | `-target-dsn` flag | Use the owner temporarily; do not keep a separate standing role. |
| **read-only analytics** (optional) | yes | BI tool | `pg_read_all_data`, **no** `BYPASSRLS` unless it must see every tenant | tool config | Only if you need one. Grant it read access to specific schemas. |

External systems get **no database access at all:**

- **n8n** (Telegram, WhatsApp, marketing) talks to the app through bridge tokens.
- The **AI Gateway** has no database access.
- **Browsers** have no database access.

### 2.3 Should the server and worker have separate database users?

It's optional, but recommended once you run more than one server. Both need exactly the same table privileges. Separate login roles give each process its own password, connection limit and audit trail (`pg_stat_activity.usename`). Create them as members that inherit `dawa24_app`'s grants:

```sql
-- run as the owner; passwords are generated by you, never committed
CREATE ROLE dawa24_web    LOGIN PASSWORD '<random>' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE INHERIT CONNECTION LIMIT 60 IN ROLE dawa24_app;
CREATE ROLE dawa24_worker LOGIN PASSWORD '<random>' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE INHERIT CONNECTION LIMIT 30 IN ROLE dawa24_app;
-- dawa24_app itself can then be NOLOGIN:
ALTER ROLE dawa24_app NOLOGIN;
```

The assistant and SQL-console `SET ROLE` grants were given to `dawa24_app`. Because `dawa24_app` is `NOINHERIT`, grant those two roles directly to `dawa24_web` as well:

```sql
GRANT dawa24_assistant_ro, dawa24_sql_console TO dawa24_web;
```

If you do this, point the server at `dawa24_web` and the worker at `dawa24_worker`. The startup check treats both the same as `dawa24_app`.

---

## 3. Security layers (defence in depth)

A tenant's data is protected by all of these together. No single layer is relied on alone.

1. **Network.** PostgreSQL must not be reachable from the internet.
   - Hostinger stack: Postgres is on the internal Docker network only.
   - Elestio: keep the database's public port closed, or allow-list only the app host.
   - Redis likewise; the app port binds to loopback and is reached only through Traefik/Caddy.
2. **Transport.** Use `sslmode=require`, or better `verify-full` with the provider CA, whenever the connection crosses a network you do not control.
   - `sslmode=disable` is acceptable only on a private Docker network on one host, as in `deploy/hostinger`.
3. **Authentication.**
   - SCRAM passwords (`password_encryption = scram-sha-256`), at least 24 random characters, unique per role and environment.
   - `pg_hba.conf` allows each role only from the app network: `host dawa24_store dawa24_app 172.16.0.0/12 scram-sha-256`, and `reject` for everything else.
4. **Role privilege.**
   - The runtime role is `NOSUPERUSER NOBYPASSRLS`, owns nothing, and has DML only.
   - DDL belongs to the owner, used only by `migrate`.
   - This is enforced at startup (section 1).
5. **Row-level security.**
   - Every tenant table is `ENABLE` + `FORCE ROW LEVEL SECURITY` with `platform.tenant_visible(organization_id)`.
   - The per-transaction GUC is set by `db.InTx`/`InReadTx`.
   - `database.AsSystem` is the only, deliberately greppable, bypass.
6. **Application authorisation.**
   - RBAC permissions, audience gates (pharmacy / supplier / staff), approval gates, and per-object party checks (for example `actorMayViewOrder`, `AuthorizeUserOrgLink`).
   - These are needed wherever code reads as system.
7. **Query safety.**
   - Parameterised SQL only; dynamic identifiers come from allowlists (audited 2026-09).
   - This matters because a SQL injection running as `dawa24_app` could call `set_config('app.is_system','on')` and turn RLS off for its own transaction. Layers 5 and 6 depend on layer 7.
8. **Resource limits.**
   - `statement_timeout` (`DB_STATEMENT_TIMEOUT`), pool size (`DB_MAX_CONNS`), and per-role `CONNECTION LIMIT`.
   - One bad request cannot exhaust the database.
9. **Audit and monitoring.**
   - `platform.audit_log` is append-only for `dawa24_app`.
   - Enable `log_connections` and `log_disconnections`, and alert on login failures.
   - The server logs an ERROR at start if its role is unsafe.
10. **Backups.** A dedicated backup role, encrypted off-host storage, and a tested restore (DEPLOY.md, "Restore drill").

### Verification: run on the production database

```sql
-- 1. As the APP connection (APP_DATABASE_URL): must be false, false, false
SELECT r.rolsuper, r.rolbypassrls,
       EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
               WHERE c.relowner=r.oid AND n.nspname NOT IN ('pg_catalog','information_schema','public')) AS owns_tables
  FROM pg_roles r WHERE r.rolname = current_user;

-- 2. Every tenant table has RLS forced (as owner): must return no rows
SELECT n.nspname, c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 JOIN pg_attribute a ON a.attrelid=c.oid AND a.attname='organization_id'
 WHERE c.relkind='r' AND n.nspname NOT IN ('pg_catalog','information_schema')
   AND NOT (c.relrowsecurity AND c.relforcerowsecurity);
-- Known today: billing.user_plan_histories, billing.wallet_withdrawals,
-- catalog.decision_memory_preferences, identity.account_deletion_requests,
-- inventory.user_plan_temparte_warehouses, platform.import_runs(_rows).
-- Their current queries filter in SQL; add RLS in a new migration after testing.

-- 3. Who can log in, and with what powers
SELECT rolname, rolcanlogin, rolsuper, rolbypassrls, rolcreaterole, rolconnlimit
  FROM pg_roles WHERE rolname NOT LIKE 'pg\_%' ORDER BY rolname;

-- 4. The default password is gone (from any shell; must FAIL to connect)
--    psql "postgres://dawa24_app:dawa24_app_change_me@<host>/<db>"
```

### Switching an existing deployment (Elestio / root `docker-compose.yml`)

1. Generate the app password: `openssl rand -base64 36 | tr -d '/+=' | cut -c1-40`
2. In the hosting panel, set:
   - `MIGRATE_DATABASE_URL=postgres://<owner>:<NEW owner password>@postgresql:5432/<db>?sslmode=require` (after rotating the owner password; see section 4)
   - `DB_APP_PASSWORD=<step 1>`
   - `APP_DATABASE_URL=postgres://dawa24_app:<step 1>@postgresql:5432/<db>?sslmode=require`
   - `SESSION_SECRET`, `REDIS_PASSWORD`, and the others in section 6.
3. Deploy. `migrate` runs as the owner, applies migrations, and sets `dawa24_app`'s password. The server and worker then start as `dawa24_app`, and refuse to start if anything is still wrong.
4. Run the verification queries above.
5. Remove any old `DATABASE_URL` that still names the owner from the server and worker environment.

On a brand-new `deploy/hostinger` stack, `init-db.sh` creates `dawa24_app` from `DB_APP_PASSWORD` on first start, and everything else is the same.

---

## 4. Credentials to rotate

**Why:** commit `55502b6` (2026-09-18) pushed a test file containing production values to `github.com/moqasem061-dot/dawa24`. Deleting the file later did not remove the values from history. Several other weak defaults also lived in the repository.

**Rule:** treat every value below as known to an attacker. Rotate it at the provider, and only then clean history (section 5). Cleaning history without rotating protects nothing.

Generate random values with `openssl rand -hex 32` (64 hex characters), or with `openssl rand -base64 36 | tr -d '/+=' | cut -c1-40` for passwords. Never reuse a value between production, staging and development.

| # | Credential | Exposure | How to regenerate | Where to put the new value |
|---|---|---|---|---|
| 1 | **PostgreSQL owner password** (Elestio user starting `UhSl…`, from the leaked `DATABASE_URL`) | git history | Elestio dashboard → PostgreSQL service → change the admin password, or `ALTER ROLE "<owner>" PASSWORD '<new>';` as that role | `MIGRATE_DATABASE_URL` only. Remove it from server and worker. |
| 2 | **`dawa24_app` password** | default `dawa24_app_change_me` is in migration 222 | new random value | `DB_APP_PASSWORD` and `APP_DATABASE_URL`, then deploy (`migrate` applies it) |
| 3 | **`SESSION_SECRET`** | git history | `openssl rand -hex 32` | `SESSION_SECRET`. This also rotates the MFA pending-cookie and image-proxy keys, which are derived from it. Everyone is logged out of in-flight MFA, and outstanding reset links die. |
| 4 | **All live sessions** | could have been created through the `super_admin` self-registration bug or the forged-reset path | in Redis: `redis-cli -a "$REDIS_PASSWORD" --scan --pattern 'session:*' \| xargs -r redis-cli -a "$REDIS_PASSWORD" del`, and the same for `user_sessions:*` | Sessions are random tokens in Redis, so a new `SESSION_SECRET` alone does not end them. |
| 5 | **SMTP password** (`no-reply@dawa24.com`) | git history | Hostinger hPanel → Emails → the mailbox → Change password. Then review sent-mail logs and forwarding rules. | `SMTP_PASSWORD` |
| 6 | **Telegram bot token** (`TELEGRAM_BOT_TOKEN`) | git history (the leaked `TELEGRAM_BRIDGE_TOKEN` had the same value) | Telegram @BotFather → `/revoke` → choose the bot → new token. Update the n8n Telegram credential. | `TELEGRAM_BOT_TOKEN` and n8n |
| 7 | **`TELEGRAM_BRIDGE_TOKEN`** | git history | `openssl rand -hex 32`. It must **not** equal the bot token. | the app env, plus the n8n HTTP node's `Authorization: Bearer` header |
| 8 | **Meta / Facebook access token** (the leaked `MARKETING_BRIDGE_TOKEN` was a Meta `EAAG…` token) | git history | Meta Business Settings → System users → revoke and generate a new token with the minimum permissions. Check Page and app activity logs. | n8n Meta credential only |
| 9 | **`MARKETING_BRIDGE_TOKEN`** | git history | `openssl rand -hex 32`, separate from any Meta token | the app env, plus the n8n marketing workflow header |
| 10 | **Cloudflare Turnstile secret** | git history | Cloudflare dashboard → Turnstile → widget → Settings → **Rotate secret key**. The site key is public and needs no change. | `TURNSTILE_SECRET_KEY` |
| 11 | **Redis password** | repository default in `docker-compose.yml` | `openssl rand -hex 32` | `REDIS_PASSWORD`; `REDIS_URL` is built from it |
| 12 | **Postgres container superuser** (`POSTGRES_PASSWORD`, self-hosted stacks) | repository default in `docker-compose.yml` | `ALTER ROLE postgres PASSWORD '<new>';` | `POSTGRES_PASSWORD` |
| 13 | **Super-admin account** (`admin@dawa24.net`) | older builds set a repository default (`Admin!…`) when `ADMIN_PASSWORD` was empty | sign in and change it, or `ADMIN_PASSWORD=<new> cli create-admin admin@dawa24.net`. Enable MFA. | Remove `ADMIN_PASSWORD` from the environment afterwards. |
| 14 | **Unexpected staff accounts** | `POST /api/v1/auth/register` accepted `role=super_admin` before the fix | `SELECT id,email,role,created_at FROM identity.users WHERE role IN ('super_admin','admin','support','developer','moderator') ORDER BY created_at;` Suspend and delete any you did not create, then review `platform.audit_log` for what they did. | — |
| 15 | **Seed and test accounts** on staging (`*@dawa24.test`) | repository default `Dawa24!…` | delete them, or re-seed with a new `SEED_PASSWORD` | `SEED_PASSWORD`, staging only |
| 16 | **AI Gateway keys** (`GATEWAY_VIRTUAL_KEY`, `GATEWAY_ADMIN_KEY`, and the `api_key` stored in `platform_admin.system_settings`) | returned by `GET /api/v1/platform/settings/{key}` to any member whenever `MODULE_API_ENABLED` was on | Gateway admin console: revoke and reissue. Re-save the key on the admin AI settings screen. Your local `.env` has `GATEWAY_VIRTUAL_KEY=true`, which is a misconfiguration: set the real key. | the env vars and the admin settings screen |
| 17 | **Google Maps API key** | local `.env` only (not found in git), but browser keys are public by nature | Google Cloud → Credentials → restrict to HTTP referrers `https://dawa24.com/*` and the Maps JS API only. Regenerate if unrestricted. | `GOOGLE_MAPS_API_KEY` |
| 18 | **`TELEGRAM_GATEWAY_TOKEN`, `WHATSAPP_BRIDGE_TOKEN`** | not found in git; rotate if `.env` was ever shared | Telegram Gateway dashboard; `openssl rand -hex 32` for the bridge token | env and n8n |
| 19 | **Object-storage keys** (production S3/MinIO) | only the dev default `minioadmin` is in git | if production MinIO still uses `minioadmin`, create a bucket-scoped access key and delete the root key from app use | `STORAGE_ACCESS_KEY_ID` / `STORAGE_SECRET_ACCESS_KEY` |
| 20 | **Per-tenant Gateway keys** | not exposed | no action | — |

After rotating, confirm each old value fails:

- Connect to the database with the old password.
- Call the bridge with the old token (expect 401).
- Check that Telegram `getMe` with the old token returns 401.

---

## 5. Removing the leaked values from GitHub

`scripts/purge-leaked-secrets.sh` rewrites every historical version of `internal/platform/config/http_test.go`, replacing the leaked values with `***REMOVED***`.

- It reads the **old** values from a local file (default `.env`) and never prints them.
- It verifies afterwards, and refuses to continue if any value remains.
- It was dry-run successfully on a throwaway clone.

1. **Rotate first** (section 4).
2. Commit or stash all work. The script refuses to run on a dirty tree.
3. `bash scripts/purge-leaked-secrets.sh .env`
4. Review `git log`, then push the rewritten history:
   `git push --force-with-lease origin --all && git push --force-with-lease origin --tags`
5. Tell every collaborator to **re-clone**. Pulling into an old clone would bring the history back.
6. Ask GitHub to purge cached views of commit `55502b6`: GitHub Support → "Remove sensitive data". Check whether the repository has forks; each fork keeps the old history.
7. Turn on GitHub **secret scanning and push protection** (Settings → Code security) to block the next leak at `git push` time.

---

## 6. Environment variables

`.env.example` is the complete, annotated list. It is generated from every `get*` call in `internal/platform/config` and every `os.Getenv` in `cmd/` and `internal/`. The ones that must be set in production:

| Variable | Secret | Notes |
|---|---|---|
| `APP_ENV=prod` | | enables every production check |
| `APP_BASE_URL` | | `https://…` (enforced) |
| `APP_DATABASE_URL` → server/worker `DATABASE_URL` | ✔ | `dawa24_app` (or `dawa24_web`/`dawa24_worker`), `sslmode=require` off-host |
| `MIGRATE_DATABASE_URL` → migrate `DATABASE_URL` | ✔ | owner. Only the migrate job sees it. |
| `DB_APP_PASSWORD` | ✔ | ≥ 24 characters; must match `APP_DATABASE_URL` |
| `DB_REQUIRE_RLS` | | leave unset or `true` |
| `REDIS_PASSWORD` / `REDIS_URL` | ✔ | |
| `SESSION_SECRET` | ✔ | ≥ 32 bytes (use 64 hex); required |
| `SESSION_SECURE=true` | | enforced |
| `TRUSTED_PROXIES` | | the proxy's Docker network CIDR. **New.** |
| `SMTP_PASSWORD` (+ host/user) | ✔ | when `SMTP_ENABLED=true` |
| `TURNSTILE_SECRET_KEY` / `TURNSTILE_SITE_KEY` | ✔ / public | Turnstile is enabled by default |
| `GATEWAY_VIRTUAL_KEY` | ✔ | when `GATEWAY_ENABLED=true` |
| `TELEGRAM_*`, `WHATSAPP_*`, `MARKETING_BRIDGE_TOKEN` | ✔ | only for the integrations you run; an empty bridge token disables that bridge |
| `STORAGE_ACCESS_KEY_ID` / `STORAGE_SECRET_ACCESS_KEY` | ✔ | when `STORAGE_ENDPOINT` is set (enforced) |
| `POSTGRES_PASSWORD` | ✔ | self-hosted Postgres container only |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | ✔ | first deploy only (≥ 14 characters); remove afterwards |

New or changed in this change set:

- `APP_DATABASE_URL` and `MIGRATE_DATABASE_URL` are now required by compose.
- `DB_REQUIRE_RLS` defaults to true in production.
- `SESSION_SECRET` is required in production.
- `TRUSTED_PROXIES` is now honoured.
- `SEED_PASSWORD` and `LOADSEED_PASSWORD` are now required by their tools; there are no defaults.
- `ADMIN_PASSWORD` has no default and needs at least 14 characters.

### Where to keep secrets

- **Production:** the hosting panel's environment or secret store (Elestio / Hostinger), or Docker secrets. Only the owner and deploy operators should be able to read them.
- **Local:** `.env`. It is git-ignored and docker-ignored; keep it that way (`git check-ignore .env` must print `.env`).
- **Never** in: the repository, test files, migrations, compose defaults, CI logs, chat messages or screenshots.
- `.github/workflows/ci.yml` uses only throwaway values for its own ephemeral Postgres and Redis. Keep it that way; it needs no repository secrets.
