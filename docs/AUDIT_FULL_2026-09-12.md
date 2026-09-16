# Dawa24 Store — full platform audit

**Date:** 2026-09-12
**Scope:** `dawa24-store` Go modular monolith (1,993 Go files, ~550k LOC, 786 templ
templates) audited against the live database `dawa24_store` @ `postgres-u74003.vm.elestio.app`.
**Database:** PostgreSQL 18.6 — 20 schemas, 171 tables, 380 foreign keys, 104 RLS
policies, 209 applied migrations.

## Method

Every finding below was verified against the live database, not inferred from the
source.

1. **Schema truth.** Full catalogue dump: columns, types, nullability, defaults,
   foreign keys, indexes, RLS flags, policy expressions, functions.
2. **Migration integrity.** `cmd/migratecheck` (checksum drift + transactional
   dry-run) plus a file-vs-`schema_migrations` reconciliation.
3. **Code↔schema linkage.** 1,834 SQL literals were extracted from the Go source
   and `PREPARE`d against the live schema inside a transaction that was never
   committed. **1,543 parsed and bound cleanly. 52 failed for a real reason**
   (undefined table, undefined column, type mismatch); the remaining 148 were
   `fmt.Sprintf` fragments that are never a whole statement on their own.
4. **Tenant isolation.** Policy expressions were read and compared against the
   GUCs the application actually sets, and against the real ownership shape of
   the data.
5. **Data integrity.** Generated orphan-row and cross-tenant-consistency scans
   over every `*_id` column, plus business-invariant checks.
6. **Security.** Secret scanning (repo, git history, live database), SQL
   injection sinks, authn/authz, session and cookie handling, CSRF, XSS sinks,
   security headers, file upload, and the admin SQL console.
7. **Build gates.** `go build ./...`, `go vet ./...`, `go test ./internal/...`.

**Build, vet and the full unit-test suite are green.** That is itself a finding:
the repository layer is broken in eleven places and no gate catches it, because
repositories are not integration-tested against a real schema.

---

# P0 — Critical

## P0-1. The production database superuser password is a plaintext, widely-copied secret

The value stored in `platform_admin.system_settings` under **both** the
`ai_configuration` and `gateway_configuration` keys is:

```
{"api_key": "king:<PRODUCTION_POSTGRES_SUPERUSER_PASSWORD>", ...}
```

The password half is byte-for-byte the password in the production `DATABASE_URL`.
It is exposed on five independent paths:

| # | Path | Location |
|---|---|---|
| 1 | Plaintext row in the database | `platform_admin.system_settings` (2 keys) |
| 2 | **Rendered into admin page HTML, unmasked** | `internal/ui/pages/admin_developers_ai.templ:73` — `fmt.Sprintf("adminAIManager(%q, %q)", …, values.GatewaySettings.APIKey)` |
| 3 | **Transmitted to a third-party host** | Sent to `api.muhiya.com` on every gateway provisioning call, plan listing and usage query |
| 4 | Committed to the git repository | `scratch/db_audit.go:13` (full DSN), `internal/modules/platform_admin/gateway_credential.go:19`, `gateway_credential_test.go:8,45`, 5 files under `docs/` |
| 5 | Present throughout git history | `git log -S` matches across many commits |

The repository already contains a guard, `ValidateAdminCredential`, written after
a previous incident found this exact value stored as `postgres:<password>`. The
live value is now `king:<password>` — **same secret, different prefix, guard
bypassed**. `MatchesDatabaseSecret` would flag it, but by design it only warns.

The credential is also baked into the committed binary `bin/server.exe`.

**Impact:** anyone with the admin developers page, anyone who can read one
database row, anyone with repository access or a clone of its history, and the
operator of `api.muhiya.com` all hold full superuser control of production.

## P0-2. The application connects as `postgres`, so all 103 RLS-protected tables have no tenant isolation

Live connection state:

```
usename  | application_name | count
---------+------------------+------
postgres | dawa24-store     |   3
```

`postgres` is `rolsuper = true, rolbypassrls = true`. Row-level security is
bypassed unconditionally for a superuser — `FORCE ROW LEVEL SECURITY` does not
apply to one either.

The bypass is doubled by the platform's own predicate:

```sql
CREATE FUNCTION platform.is_system() RETURNS boolean AS $$
  SELECT CURRENT_USER IN ('postgres', 'supabase_admin')
      OR pg_has_role(CURRENT_USER, 'pg_database_owner', 'MEMBER')
      OR pg_has_role(CURRENT_USER, 'postgres', 'MEMBER')
      OR COALESCE(current_setting('app.is_system', true) = 'on', false);
$$;
```

`platform.tenant_visible()` short-circuits on `is_system()`, so **every policy on
every table evaluates to `true`**. 103 tables carry RLS; 104 policies exist; none
of them currently restrict anything.

The least-privilege role `dawa24_app` (`NOSUPERUSER`, `NOBYPASSRLS`) exists in the
database, fully granted by `cmd/dbcheck -provision`, and is not used by anything.

The code's own comment in `cmd/dbcheck/provision.go` states the consequence
precisely: *"an application connecting as one has no tenant isolation at all,
however many policies exist."*

There is a second consequence. Because the app is a superuser, the admin SQL
console (`POST /admin/developers/sql`, gated by `platform.developer.sql`) grants
arbitrary read of files on the database host. Verified live:

```sql
SELECT left(pg_read_file('postgresql.conf'), 50);  -- returns file contents
```

`pg_read_file` is superuser-only, is a plain `SELECT`, and therefore passes the
console's prefix allowlist and its read-only transaction. This escalation
disappears entirely under `dawa24_app`.

## P0-3. RLS cannot simply be switched on — the policies contradict the data model

This is why P0-2 has not been fixed by flipping the connection string, and it
must be solved before it can be.

**Master catalogue.** `catalog.products` carries
`USING (platform.tenant_visible(organization_id))`. In production:

```
product_org | distinct_variant_orgs | rows
------------+-----------------------+-------
        185 |                     3 | 63,183
      88411 |                     1 |      1
```

All 60,912 products belong to organisation **185**, the master catalogue owner.
Variants belong to the three vendor organisations that sell them. Under
`dawa24_app`, a pharmacy whose `organization_id` is not 185 sees **zero
products**. The catalogue, search, offers and smart-order all go blank.

**Buyer/seller orders.** `commerce.order_lines` carries
`USING (platform.tenant_visible(organization_id))`, where the line's
`organization_id` is the *vendor*:

```
order_org | line_org | rows
----------+----------+------
      188 |      187 | 1192
      186 |      187 |  690
      188 |      248 |  389
```

2,854 of 2,854 order lines have a line org different from the order org. The
buyer cannot read the lines of their own order. `commerce.orders`'s policy is
likewise single-sided.

Three tables are worse still — their policies read a GUC the application never
sets:

```sql
-- catalog.saving_products, org.employee_institutional_works,
-- workflow.purchase_priority_engines
USING (organization_id = NULLIF(current_setting('app.current_tenant', true),'')::bigint
       OR current_setting('app.is_system', true) = 'true')
```

The application sets `app.current_org_id` (not `app.current_tenant`) and
`app.is_system = 'on'` (not `'true'`). Under a non-superuser role these three
tables return **zero rows to everyone, including system callers**.

Twelve further policies are `USING (true)` — present but enforcing nothing:
`chat.conversations`, `chat.messages`, `chat.participants`,
`hr.job_seeker_profiles`, `org.branch_institutional_works`, `org.review_criteria`,
`org.review_ratings`, `platform_admin.document_requests`,
`platform_admin.feature_flags`, `promo.special_offers`,
`promo.special_offer_locations`, `promo.special_offer_products`.

Four tenant-owned tables have no RLS at all:
`billing.wallet_withdrawals`, `catalog.decision_memory_preferences`,
`identity.account_deletion_requests`, `platform.import_runs`.

And `identity.users`, `identity.user_sessions`, `identity.user_security` and
`org.organizations` carry neither RLS nor a policy. `user_sessions` holds IP
addresses, user agents, device names and geolocation.

## P0-4. The main login path issues the session cookie without `Secure`, and there is no HSTS

`internal/modules/identity/http/handlers.go:154` sets `Secure: h.secureCookie`
correctly. But the browser login path users actually hit is in `internal/ui`,
and all six cookie writes there omit the flag entirely:

| File | Lines |
|---|---|
| `internal/ui/auth_handlers.go` | 161, 281 |
| `internal/ui/auth_submit_handlers.go` | 28, 230, 290, 331 |

```go
http.SetCookie(w, &http.Cookie{
    Name:     "dawa24_session",
    Value:    res.Session.Token,
    Path:     "/",
    HttpOnly: true,                    // Secure is absent
    SameSite: http.SameSiteLaxMode,
    MaxAge:   maxAge,                  // up to 30 days
})
```

No `Strict-Transport-Security` header is set anywhere in the codebase
(`internal/platform/httpx/middleware.go` sets six other headers but not this
one). A single plaintext HTTP request to the domain — a user typing the bare
hostname, a stale bookmark, an image on a subdomain — transmits a 30-day session
token in cleartext.

---

# P1 — High

## P1-1. The `chat` module is written against a schema that does not exist

`internal/modules/chat/postgres/repository.go` is wired into the live router at
`cmd/server/routes_ui_builder.go:214`. Every one of its statements fails.

| Code expects | Live column |
|---|---|
| `conversations.subject` | `title` |
| `conversations.counterparty_org_id` | *(absent)* |
| `conversations.status` | *(absent)* |
| `conversations.last_message_at` | *(absent)* |
| `conversations.created_by_user_id` | `created_by` |
| `messages.sender_user_id` | `sender_id` |
| `messages.attachments` (jsonb) | `attachment_url`, `attachment_type` |
| `messages.read_at` | *(absent — `participants.last_read_at`)* |

Affected: `repository.go:27, 43, 75, 123, 132, 142, 176, 189`. Chat is
non-functional end to end.

## P1-2. Three dropped tables are still queried

| Table | Dropped by | Still queried at | Failure mode |
|---|---|---|---|
| `identity.kyc_records` | migration `152_retire_dead_tables` | `identity/postgres/admin_repo.go:293, 319, 359` | National ID silently reads as `""` at :293 (error discarded); the upsert at :319 and `GetNationalID` at :359 return hard errors |
| `hr.employees` | migration `069_merge_employees` (merged into `org.members`) | `hr/postgres/jobs.go:310` | `_, _ = tx.Exec(...)` — **`job_title` and `base_salary` are silently discarded every time an applicant is hired** |
| `assistant.tool_audits` | never existed — the table is `assistant.tool_audit` | `assistant/postgres/read_stage3_admin.go:247` | The Security Events admin projection errors outright |

Migration 152's own header claims these tables were *"referenced nowhere in the
Go or templ source"*. For `identity.kyc_records` that was not true.

## P1-3. The catalogue reindex job cannot run

`internal/modules/catalog/jobs/reindex_sql.go:50` and `:94` select `b.city` from
`org.branches`. The column is **`city_id`** (there is no `city`). Both the parent
and variant projections are affected, so `catalog.product_index` — 20,462 rows,
the table catalogue search reads — cannot be rebuilt. The index silently ages
out of sync with the catalogue.

## P1-4. Compare-file retention purge never deletes anything

`internal/modules/compare/postgres/matching_repo.go:148-151`
(`PurgeExpiredCompareFiles`) reads `p.features->>'compare_file_retention_days'`
from `billing.plans`. **`billing.plans` has no `features` column.** The statement
errors on every run.

Consequence: uploaded supplier price lists (`compare.files`, 736 rows;
`compare.file_rows`, 376,916 rows) are never purged. This is a retention and
data-minimisation failure as much as a storage one — these files are
commercially sensitive third-party pricing.

## P1-5. Assistant pharmacy financial projection errors

`internal/modules/assistant/postgres/read_stage3_pharmacy_ext.go:61-62` reads
`billing.invoices.payment_status`. The column on `billing.invoices` is `status`
— `payment_status` exists on `commerce.orders`, a different table. Both the
unpaid-invoice total and count fail.

## P1-6. Vendor dashboard silently reports a zero wallet balance

`internal/modules/commerce/postgres/dashboard.go:276-280`:

```sql
SELECT COALESCE(balance, 0) FROM billing.wallets
WHERE organization_id = $1
   OR user_id IN (SELECT id FROM identity.users WHERE organization_id = $1)
```

Two errors in one statement: `billing.wallets` has **no `balance` column**
(balance is derived from `wallet_transactions.balance_after`), and
`identity.users` has **no `organization_id`** (membership lives in `org.members`).
The result is discarded with `_ = tx.QueryRow(...)`, so the dashboard shows
`0.00` rather than failing.

`dashboard.go:74` additionally has a type error: `COALESCE(cust_org.name->>'ar',
cust_org.name->>'en', u.name, …)` mixes `text` and `jsonb` (`identity.users.name`
is `jsonb`) — `COALESCE types text and jsonb cannot be matched`.

## P1-7. Other confirmed schema mismatches

| Location | Problem |
|---|---|
| `cmd/worker/builtin_workers.go:53` | `logs.event_type` does not exist |
| `cmd/worker/builtin_workers.go:188` | `import_rows.match_confidence` does not exist |
| `cmd/cli/seed.go:34` | `roles.is_platform_role` does not exist |
| `cmd/etl/loader.go:122,136` | `products.slug`, `product_variants.stock` do not exist |
| `internal/ui/testsupport_test.go:336,365,395,511` | `products.type`, `offers.min_order_value`, `orders.customer_org_id`, `policies.key` |
| `internal/ui/vendor_reviews_e2e_test.go:80` | `members.role` (column is `role_key`) |
| `test/integration/rls_billing_commerce_test.go:86` | `quote_requests.vendor_org_id` |
| `test/integration/coverage_chain_test.go:41,56,75,94` | `public_id` is `uuid`, bound as `text` |

The last five mean the integration suite — including the RLS isolation test that
AGENTS.md designates a CI gate — cannot currently run.

## P1-8. Migration 48 is applied to production but its file no longer exists

`schema_migrations` records 209 applied versions with a maximum of 210. Version
80 was skipped by numbering. **Version 48, `seed_realistic_data`, is recorded as
applied but has no file in `db/migrations/`.**

Two consequences: the database cannot be rebuilt from the repository, and
`migratecheck`'s drift detector cannot check a file it cannot find — the one
missing migration is the one it is blind to.

It also explains the shape of the data: 60 users on `@example.com`/`@gmail.com`
addresses, organisations numbered 88101–88902, all created 2026-09-04 onward.
Confirm whether this environment is intended to hold seed data before cutover.

---

# P2 — Medium

| # | Finding | Evidence |
|---|---|---|
| P2-1 | **No CSRF on the public route group**, which carries ~15 state-changing POSTs (`/compare/upload`, `/compare/files/{id}/delete`, `/rename`, `/archive`, `/mapping`, `/compare/run`, `/compare/subscribe`). `SameSite=Lax` limits exploitation, but the double-submit gate every other POST has is simply absent here. Ownership *is* enforced in the handlers (`checkFileOwnership`), so there is no IDOR. | `internal/ui/public_routes.go:41-49` vs `cmd/server/routes.go:86` |
| P2-2 | **`SESSION_COOKIE_NAME` is a dead configuration knob.** The name is hardcoded as `"dawa24_session"` in 9 places, including `OptionalAuth` and the CSRF exemption check. Changing the env var silently breaks authentication on all public routes. | `public_routes.go:43`, `httpx/csrf.go:42`, `auth_handlers.go:161,281`, `auth_submit_handlers.go:28,230,290,331` |
| P2-3 | **Raw `err.Error()` returned to end users** on 12+ non-admin paths, leaking internal structure. | `customer_user_org_handlers.go:163,192,218`, `settings_payment_handlers.go:37,87`, `organization_profile_handlers.go:134`, `coverage_write_*`, others |
| P2-4 | **Orphaned rows.** `org.members.role_id` → 1 row points at a non-existent role (that member's permission resolution silently yields nothing). `ai.usage_events.user_id` → 22 orphans. `platform.audit_log.actor_user_id` → 286 orphans (acceptable for an audit trail, but undocumented). | Generated orphan scan |
| P2-5 | **10 organisations have no owner** (`owner_id IS NULL`), including two `approved` ones (88411 vendor, 88412 customer) and one `suspended` vendor. | `org.organizations` |
| P2-6 | **17 active users belong to no organisation**; **7 organisations have no branch**. Expected for pending signups given the no-auto-branch policy — worth an explicit invariant so it stays intentional. | |
| P2-7 | **`float64` in a money path**, against the project's first non-negotiable rule: `salFloat := float64(in.BaseSalary.Minor()) / 100.0`. | `hr/postgres/jobs.go` (same function as P1-2) |
| P2-8 | **12 foreign-key columns have no supporting index** — `billing.wallet_deposits.refunded_by`, `billing.wallet_transactions.refunded_by`, `billing.wallet_withdrawals.reviewed_by`, `catalog.decision_memory_preferences.updated_by`, `catalog.match_decisions.promoted_by`, `commerce.order_shipments.courier_assigned_by`, `commerce.variant_branch_quota_releases.{branch_id,released_by}`, `org.organization_deletion_requests.reviewed_by`, `org.profile_change_requests.{requested_by,reviewed_by}`, `platform_admin.ai_role_models.updated_by`. | |
| P2-9 | **No integration-test gate.** `go build`, `go vet` and `go test ./internal/...` are all green while eleven repository methods cannot execute. AGENTS.md requires repository integration tests and a per-table cross-tenant read test; neither runs. | |

---

# P3 — Low

- **RLS enabled but not FORCEd** on `promo.special_offers`,
  `promo.special_offer_locations`, `promo.special_offer_products` — inconsistent
  with the other 100 tables.
- **~70 MB of never-scanned indexes**, largest: `idx_compare_file_rows_norm_name`
  (29 MB), `compare_file_rows_org_norm_idx` (12 MB), `idx_compare_file_rows_sku`
  (6 MB), `catalog_import_rows_search` (5.8 MB), plus duplicate trigram indexes on
  `catalog.brands` (`idx_brands_name_en_trgm` and `brands_name_trgm_idx`, 2.6 MB
  each). Statistics have never been reset, so these counts cover the table's full
  lifetime.
- **11 nullable `text` columns scanned into non-pointer Go strings** — all safe to
  make `NOT NULL DEFAULT ''` (none carries a unique index). Full list from
  `go run ./cmd/dbcheck -nullscan`.
- **15 live tables are referenced by no Go SQL**, including
  `inventory.temp_warehouses`, `inventory.father_user_temparte_warehouses`,
  `promo.special_offer_products`, `promo.special_offer_locations`,
  `promo.ad_plans`, `billing.subscription_users`, `hr.job_categories`,
  `ingest.import_progress`, `platform_admin.employee_activities`.
- **CSP carries `unsafe-inline` and `unsafe-eval`** — already documented in
  `middleware.go` with the reason (56 inline script blocks; Alpine 3 uses
  `new Function`). Not a regression, but it caps what the CSP can do.

## Verified clean

Worth recording, so nobody re-audits it: **no SQL injection was found.** All 35
`fmt.Sprintf`-into-SQL sites either interpolate `$n` placeholder *numbers* or
allowlisted identifiers (`billing/postgres/admin_repo.go:336` uses a closed
`switch`; `platform_admin/postgres/trash.go` validates against a regex and
`information_schema` before `%q`). Passwords use bcrypt. Session and CSRF tokens
use `crypto/rand`. `math/rand` appears once, for shuffling ads. Only three
`templ.Raw` sinks exist and all three pass `json.Marshal` output, which escapes
`<`, `>` and `&`. Account lockout and an auth rate limiter are wired. Six
security headers are set. `go vet` is clean. The one-live-subscription-per-org
invariant holds. Order totals reconcile exactly
(`subtotal − discount_amount = Σ lines.total_price`). No negative prices, no
negative stock, no user with an empty password hash, no orphaned variants.

---

# Remediation plan

Ordered so that nothing later is blocked by something earlier, and so that no
step can be deployed in a state that breaks production.

## Phase 0 — Credential containment (today, before anything else)

Nothing else in this plan matters while the superuser password is public.

1. **Rotate the PostgreSQL password.** Assume it is compromised — it is in git
   history, in a committed binary, in an HTML page, and has been sent to a
   third-party host.
2. **Run `go run ./cmd/dbcheck -provision <admin-dsn> -role dawa24_app -password <new>`**
   and point the application `DATABASE_URL` at `dawa24_app`.
   → **Do not deploy this yet.** It is inert until Phase 2, and deploying it
   before Phase 2 blanks the catalogue. Provision the role and hold.
3. **Issue a separate AI Gateway credential** from `api.muhiya.com`. It must
   share nothing with any database password.
4. **Purge the secret from the database:**
   ```sql
   UPDATE platform_admin.system_settings
      SET value = jsonb_set(value, '{api_key}', '""')
    WHERE key IN ('ai_configuration', 'gateway_configuration');
   ```
   then set the new gateway key through the admin UI.
5. **Purge it from the repository:** delete `scratch/db_audit.go`; replace the
   literal in `gateway_credential.go:19` and `gateway_credential_test.go:8,45`
   with an obviously-fake placeholder; scrub the five `docs/` files; delete
   `bin/server.exe` and add `bin/` to `.gitignore`. History rewrite
   (`git filter-repo`) is the complete fix if the remote permits it; rotation
   (step 1) is what actually neutralises the exposure.
6. **Mask the key in the UI.** `admin_developers_ai.templ:73` must never receive
   the secret. Pass a boolean "configured" flag plus a 4-character suffix; keep
   the blank-means-keep-existing save behaviour that is already there.
7. **Consider upgrading `MatchesDatabaseSecret` from a warning to a refusal.**
   The current design — warn, never veto — was a deliberate choice, and the
   rationale in `gateway_credential.go` is sound in the abstract. In practice the
   warning was bypassed by changing `postgres:` to `king:`. At minimum, log an
   audit event when an operator saves past the warning.

## Phase 1 — Repair the code↔schema breaks

These are independent of each other and of the RLS work. Each is small, and each
one is a live defect today.

| Order | Fix | File |
|---|---|---|
| 1.1 | Rewrite the `chat` repository against the real schema (`title`, `created_by`, `sender_id`, `attachment_url`/`attachment_type`, `participants.last_read_at`) — or, if chat is not shipping, unmount it at `routes_ui_builder.go:214` and say so | `modules/chat/postgres/repository.go` |
| 1.2 | `b.city` → `b.city_id` (join `platform_admin.cities` if the name is wanted) — **unblocks catalogue reindex** | `catalog/jobs/reindex_sql.go:50,94` |
| 1.3 | Replace `billing.plans.features->>'compare_file_retention_days'` with a real source — add the column, or move retention to `billing.plan_features` — **unblocks retention purge** | `compare/postgres/matching_repo.go:148-151` |
| 1.4 | `i.payment_status` → `i.status`, with the invoice status vocabulary | `assistant/postgres/read_stage3_pharmacy_ext.go:61-62` |
| 1.5 | Derive wallet balance from `wallet_transactions.balance_after`; resolve org membership via `org.members`, not `identity.users.organization_id`; fix the `text`/`jsonb` COALESCE at `:74` | `commerce/postgres/dashboard.go:74,276-280` |
| 1.6 | Write `job_title`/`base_salary` to `org.members` (where migration 069 moved them) instead of the dropped `hr.employees`; use `money.Amount`, not `float64`; **stop discarding the error** | `hr/postgres/jobs.go:310` |
| 1.7 | Restore `identity.kyc_records` in a new migration, or move `national_id` onto `identity.users` and update all 4 call sites | `identity/postgres/admin_repo.go:293,319,359` |
| 1.8 | `assistant.tool_audits` → `assistant.tool_audit` | `assistant/postgres/read_stage3_admin.go:247` |
| 1.9 | Fix the remaining seven mismatches in P1-7 (worker, CLI, ETL) | see table |
| 1.10 | Fix the five test-file mismatches — the integration suite cannot run until then | see table |

**Stop discarding errors.** `_, _ = tx.Exec(...)` and `_ = tx.QueryRow(...)` are
why P1-2, P1-6 and part of P1-2 have been invisible. Every one of them should
either handle the error or log it.

## Phase 2 — Make row-level security real

This is the largest piece and the one that must not be rushed. It is safe
precisely because P0-2 means nothing depends on RLS today.

1. **Decide the visibility model per table**, and write it down. Three shapes are
   needed, not one:
   - *Platform-global read* — the master catalogue (`catalog.products`,
     `catalog.categories`, `catalog.brands`, `billing.plans`). Readable by every
     authenticated tenant; writable only by the owning organisation or system.
   - *Two-sided* — `commerce.orders`, `commerce.order_lines`,
     `commerce.purchase_requests`, `billing.invoices`. Visible to **both** the
     buyer org and the vendor org. `billing.invoices` already models this
     (`tenant_visible(organization_id) OR tenant_visible(customer_org_id)`) —
     that is the pattern to copy.
   - *Single-tenant* — everything else; the existing predicate is correct.
2. **Fix the three broken policies** on `catalog.saving_products`,
   `org.employee_institutional_works`, `workflow.purchase_priority_engines`:
   replace `app.current_tenant` / `= 'true'` with `platform.tenant_visible(...)`
   so there is exactly one predicate in the system.
3. **Replace the twelve `USING (true)` policies** with real predicates, or drop
   them and document those tables as platform-global. A policy that enforces
   nothing is worse than no policy — it reads as protection in a review.
4. **Add RLS** to `billing.wallet_withdrawals`,
   `catalog.decision_memory_preferences`,
   `identity.account_deletion_requests`, `platform.import_runs`,
   `identity.user_sessions`, `identity.user_security`.
   Decide explicitly for `identity.users` and `org.organizations` — a public
   supplier directory may justify global read, but say so in the migration.
5. **Harden `platform.is_system()`.** Drop the `CURRENT_USER IN ('postgres', …)`
   and `pg_has_role` clauses. System access should be the explicit
   `app.is_system` GUC that `database.AsSystem` sets, and nothing else. Leaving
   role-name clauses in means the next operator who runs as a superuser silently
   disables the whole model again.
6. **Audit the 1,054 `AsSystem` call sites.** That is more than the 950
   `InTx`/`InReadTx` calls in the codebase. `AsSystem` is described in AGENTS.md
   as "deliberately greppable" cross-tenant access; at this density it is the
   default path, not the exception. Classify each: legitimately cross-tenant
   (admin, jobs, matching) vs. reaching for it because scoped access did not
   work.
7. **Cut over to `dawa24_app` in staging first**, with the RLS isolation tests
   from step 8 passing, then production.
8. **Write the isolation tests AGENTS.md already requires**: for every
   tenant-owned table, a test proving a cross-tenant read returns zero rows.
   Make it a CI gate.

## Phase 3 — Web security hardening

1. Add `Secure: cfg.Session.SecureOnly` to all six cookie writes in
   `internal/ui/`, or better, route them through the single helper in
   `identity/http/handlers.go` that already does it correctly.
2. Add `Strict-Transport-Security: max-age=31536000; includeSubDomains` to
   `httpx.SecurityHeaders`, gated on `cfg.Env.IsProd()`.
3. Replace all 9 hardcoded `"dawa24_session"` literals with
   `cfg.Session.CookieName`.
4. Apply `httpx.CSRF` to the public route group's state-changing POSTs.
5. Replace raw `err.Error()` in user-facing responses with `h.safeMessage(err,
   lang)` — the helper already exists and is used elsewhere in the same package.
6. Re-scope the admin SQL console once Phase 2 lands: under `dawa24_app` the
   `pg_read_file` escalation is gone, but consider also denying `pg_*` and
   `information_schema` function calls and redacting `identity.users.password_hash`
   from results.

## Phase 4 — Data and schema cleanup

1. Reconstruct or formally retire migration 48. If the seed data should not be
   in this environment, plan its removal before cutover; if it should, commit the
   file.
2. Repair the orphans: the one bad `org.members.role_id`, the 22
   `ai.usage_events.user_id`, the 10 ownerless organisations. Add the FK
   constraints that would have prevented them, or document why each is absent
   (`platform.audit_log` has a legitimate reason — record it).
3. Add the 12 missing FK indexes.
4. Drop the confirmed-unused indexes and the duplicate `catalog.brands` trigram
   pair (~70 MB).
5. Apply `NOT NULL DEFAULT ''` to the 11 nullable text columns from
   `dbcheck -nullscan`.
6. Triage the 15 unreferenced tables — a second `152_retire_dead_tables`, but
   this time verified against the source with the SQL extractor used for this
   audit, which is what would have caught `identity.kyc_records`.
7. Add `FORCE ROW LEVEL SECURITY` to the three `promo.special_offer*` tables.

## Phase 5 — Gates, so none of this recurs

The whole class of finding in Phase 1 exists because Go does not typecheck SQL
and nothing else did either. Three cheap gates close it:

1. **SQL prepare check in CI.** Extract every SQL literal, `PREPARE` it against a
   migrated throwaway Postgres, fail on `42P01`/`42703`/`42883`/`42804`. This is
   what found eleven live defects in one pass; it runs in seconds. The extractor
   and checker written for this audit are a starting point.
2. **Repository integration tests** against real Postgres — already mandated by
   AGENTS.md §Testing, not currently enforced.
3. **Cross-tenant isolation test per tenant-owned table** — also already
   mandated, also not enforced. It is the only thing that will keep Phase 2 true.

Add a secret-scanning pre-commit hook (gitleaks or equivalent) while you are
there.

---

## Suggested sequencing

| Window | Work |
|---|---|
| Immediately | Phase 0 (1–6). Rotate, re-provision, purge, mask. |
| Week 1 | Phase 1 + Phase 3. Both are small, local, independently shippable. |
| Week 1–2 | Phase 5.1 — the SQL prepare gate, so Phase 1 fixes cannot regress. |
| Week 2–4 | Phase 2. The model decision (2.1) is the hard part; the SQL is mechanical once it is made. |
| Week 4 | Phase 2.7 cutover to `dawa24_app`, staging then production. |
| Ongoing | Phase 4, Phase 5.2–5.3. |

**The platform is not production-ready today**, and the single reason is Phase 0:
the production database superuser credential is public in five places and
currently in transit to a third party. Everything else on this list is fixable on
a normal engineering schedule. That one is not a schedule item.
