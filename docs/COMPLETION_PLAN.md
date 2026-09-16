# Dawa24 Store — Completion Plan

**Written:** 2026-08-16 · **Against repo state:** 20 migrations, 82 tables, 101 routes, 14 modules
**Supersedes:** `docs/AUDIT_2026-08-16.md` (findings C1–C3 now resolved; C6 outstanding)

This is the full remaining scope. Part 1 settles the open engineering decisions —
read it before executing anything, because several later tasks depend on those
answers. Parts 2–6 are the work. Part 7 sequences it.

---

# PART 0 — Verified current state

| Dimension | Value | Assessment |
|---|---|---|
| Migrations / tables | 20 / 82 | ~75% of target schema |
| API routes | 101 | vs ~580 legacy declarations |
| Modules | 14 | All bounded contexts have a home |
| RLS-protected tables | 31 (`ENABLE`+`FORCE`+policy) | Good; must stay at 100% of tenant tables |
| Workers | Real implementations | Fixed since audit |
| RLS suite in CI | `DATABASE_URL` set, runs | Fixed since audit |
| Frontend | 733 lines, one static page | **~3% — not started** |
| ETL | No MySQL driver, no extract/load | **Not started** |
| `aicapabilities` wiring | Not called by any module | **Outstanding (C6)** |

**Backend ~70%. Frontend ~3%. Overall ~45%.**

---

# PART 1 — Open engineering decisions

Each of these is currently unresolved or resolved inconsistently. Decide once,
record in `docs/adr/decisions.md`, then execute.

## D-1. Frontend technology — commit to templ, delete the static page

**Problem.** ADR 0001 specifies templ + HTMX + Alpine + Tailwind. What exists is a
hand-written 90-line `index.html` with inline `onclick` handlers that never calls
the API. Two incompatible directions.

**Decision: honour ADR 0001. Delete `web/templates/index.html`.**

**Why.** The reasoning in ADR 0001 has not changed: 353 server-reactive Livewire
components port ~1:1 to templ+HTMX and ~0:1 to a client-state framework; one
language with the Gateway; one container; native SSR for Arabic RTL and SEO.
Keeping a static page alongside would create a second rendering path that drifts.

**Consequence.** `templ generate` becomes a build step. Add it to the Dockerfile
builder stage and to CI, and commit generated `*_templ.go` files or generate in
CI — pick one and enforce it, because a stale generated file that compiles is a
silent bug.

## D-2. Content i18n — database table, not embedded catalogues

**Problem.** `translations` and `languages` exist in the legacy schema.
`languages` and `currencies` now exist in the target; `translations` does not.
Meanwhile all *content* (product names, category names) is already JSONB
`{"ar","en"}` on the row.

**Decision: two distinct mechanisms, and never mix them.**

| Kind of string | Mechanism |
|---|---|
| **Content** — product names, category names, org descriptions | JSONB `{"ar","en"}` on the row. Already done. Keep. |
| **UI chrome** — button labels, validation messages, emails | Go message catalogue embedded in the binary, keyed by `apperr.Code` and template id |
| **Admin-editable copy** — policy pages, banners, email templates | `platform_admin.translations` table |

**Why.** Putting UI chrome in the database means a deploy cannot change a label
and a cache miss can blank the interface. Putting admin copy in the binary means
a lawyer needs an engineer to fix a privacy page. The split follows who owns the
string.

**Consequence.** `apperr.Error.Msg` currently carries an English sentence. Change
the contract: `Code` is the key, and the HTTP layer resolves the message for the
request language. This is a small refactor now and a large one after 500 error
sites exist.

## D-3. Admin surface — per-module routes behind admin middleware, not an admin module

**Problem.** Laravel has 275 admin route declarations. The target has zero. The
tempting move is one big `admin` module.

**Decision: each module owns its own admin routes**, mounted under
`/api/v1/admin/<module>/…` and gated by `RequirePermission`. No `admin` module.

**Why.** An admin module would import every other module to do its job, which
`depguard` forbids and which would collapse the boundaries the architecture
exists to maintain. Admin is a *permission level*, not a bounded context.

**Consequence.** Every module gains an `http/admin.go` alongside `http/handlers.go`.
Platform-wide screens (dashboards, cross-tenant reports) live in
`platform_admin` and consume other modules through their published service
interfaces, never their repositories.

## D-4. Pagination — keyset by default, offset only where a page number is shown

**Problem.** No pagination standard exists. `internal/shared/pagination` was
planned and never written. Listing endpoints currently return unbounded results.

**Decision: keyset (cursor) pagination as the default.** Offset pagination only
for admin tables that genuinely need "page 7 of 24".

**Why.** `catalog.products` and `commerce.order_lines` will be the largest tables.
`OFFSET 50000` makes PostgreSQL walk 50,000 rows before discarding them; keyset
uses the index. Vendor catalogues will hit this.

**Contract:**

```json
{ "data": [...], "next_cursor": "eyJpZCI6MTIzfQ", "has_more": true }
```

Cursor is base64 of the sort key. **Every list endpoint gets a default and a
maximum limit** (50 / 200). An unbounded list endpoint is a denial-of-service
primitive.

## D-5. File uploads — presigned direct-to-storage, never proxied

**Problem.** `POST /api/v1/ingest/uploads` exists. Vendors upload spreadsheets of
hundreds of thousands of rows, and product images in bulk.

**Decision: the API issues a presigned PUT; the browser uploads directly to
MinIO/S3; the client then calls a confirm endpoint with the object key.**

**Why.** Proxying a 200 MB upload through the app holds a request goroutine and a
connection for minutes, competes with checkout traffic, and puts the file in the
container's memory or disk. `storage.PresignPut` already exists — use it.

**Consequence.** `ingest.file_uploads` stores the key and a status; a confirm
endpoint validates content-type and size server-side **after** upload. Never
trust the client's declared type.

## D-6. Search — stay on PostgreSQL; the trigger for Typesense is numeric

**Decision: `pg_trgm` + `platform.normalize_arabic` as-is.**

**Add Typesense only when** the catalogue exceeds **250,000 active variants**, or
search P95 exceeds **300 ms** after index tuning, or faceted search across more
than three dimensions becomes a product requirement.

**Why.** Zero extra infrastructure, and Arabic normalisation already happens
inside the index. Adding a search engine now means a sync pipeline, a consistency
problem, and another Elest.io service before there is evidence it is needed.

## D-7. Auth for the frontend — session cookies, not tokens

**Decision: the templ/HTMX UI uses the existing Redis-backed session cookie.
PASETO tokens are for the vendor/partner API only.**

**Why.** The UI is server-rendered and same-origin; a cookie is the simplest
correct answer and matches the legacy behaviour, which means no forced re-login at
cutover. Tokens in browser storage would add XSS exposure for no benefit here.

**Consequence.** CSRF protection is now required — same-site cookies plus a
per-session token on every mutating form. `SameSite=Lax` alone does not cover
top-level POST navigation.

## D-8. Soft delete — keep, but make it consistent and indexed

**Problem.** `deleted_at` is used inconsistently; some tables have it, some don't,
and partial indexes exist only in places.

**Decision:** tenant-owned business entities get `deleted_at`. Ledger tables
(`stock_movements`, `wallet_transactions`, `order_status_history`,
`platform.audit_log`) **never** get it — they are append-only by design.

**Every unique index on a soft-deletable table must be partial:**
`WHERE deleted_at IS NULL`. Otherwise a deleted SKU permanently blocks its reuse.

## D-9. Order status — one state machine, in the domain layer

**Decision:** transitions are validated by an explicit table of allowed
`(from, to)` pairs in `commerce/domain.go`, and every transition writes
`order_status_history` **in the same transaction** as the status column update.

**Why.** The legacy system had 13 statuses on `adv_orders` and 5 on `orders` with
no validation anywhere. Without a state machine, a cancelled order can be marked
delivered by a race between two vendor clicks.

## D-10. Money in the API — always a decimal string

Already correct in `money.MarshalJSON`. **Codify it:** no endpoint may emit a JSON
number for a monetary value. Add a CI grep for `float` in any `http/` package.

## D-11. ETL strategy — blocked on U1, but build for the harder case

**Decision: build the ETL as resumable and idempotent regardless of which
strategy is chosen.** That costs little extra and is a prerequisite for the
strangler approach if U1 comes back "yes, live".

---

# PART 2 — Remaining backend

## 2.1 Schema — 20 tables, migrations 021–026

| Migration | Tables | Notes |
|---|---|---|
| `021_platform_content` | `translations`, `privacy_policies`, `contact_us`†, `system_resources`, `api_integrations` | †`contact_us` endpoints exist but the table does not — verify and reconcile |
| `022_ingest_batches` | `import_batches`, `import_progress` | Required before the import wizard UI |
| `023_promo_completion` | `ad_plans`, `offer_promotions`, `offer_location_covers` | Completes the revenue engine |
| `024_billing_history` | `payment_histories` | Payment audit trail |
| `025_hr_jobs` | `job_offers`, `job_applications`, `job_categories` | Jobs board |
| `026_misc` | `user_favorites`, `admin_notifications`, `supplier_trackings`, `ask_fors`, `request_offers`, `institutional_works` | `ask_fors`/`request_offers` may already be covered by `commerce/quotes` — **verify before creating** |

**Rules for every one of these:** tenant-owned → `organization_id` +
`ENABLE`/`FORCE ROW LEVEL SECURITY` + `platform.tenant_visible()` policy + an RLS
test. Arabic comments carried across. Partial unique indexes per D-8.

**Do not create a table without stating in `docs/modules/<module>.md` why it
earns its place.** Several legacy tables were correctly consolidated away; do not
reintroduce them by reflex.

## 2.2 API — the exact gaps, per module

Current: 101 routes. Below is what is missing, module by module.

### identity — the largest gap relative to its importance
Only `login`, `logout`, `register` exist.

```
POST   /api/v1/auth/password/forgot
POST   /api/v1/auth/password/reset
POST   /api/v1/auth/email/verify
POST   /api/v1/auth/email/resend
POST   /api/v1/auth/mfa/enroll          → returns TOTP secret + QR payload
POST   /api/v1/auth/mfa/confirm
POST   /api/v1/auth/mfa/disable
POST   /api/v1/auth/mfa/verify          → second step of login
GET    /api/v1/me                       → user + profile + active org + permissions
PUT    /api/v1/me
GET    /api/v1/me/sessions
DELETE /api/v1/me/sessions/{id}
GET    /api/v1/me/addresses
POST   /api/v1/me/addresses
PUT    /api/v1/me/addresses/{id}
DELETE /api/v1/me/addresses/{id}
PUT    /api/v1/me/preferences
GET    /api/v1/me/favorites
POST   /api/v1/me/favorites
DELETE /api/v1/me/favorites/{productId}
```

`GET /api/v1/me` is the highest-value single endpoint in this list — every
frontend shell needs it on first paint, and without it the UI cannot decide what
to render.

### org
```
PUT    /api/v1/org/organizations/{id}
DELETE /api/v1/org/organizations/{id}
PUT    /api/v1/org/organizations/{id}/branches/{bid}
DELETE /api/v1/org/organizations/{id}/branches/{bid}
PUT    /api/v1/org/organizations/{id}/members/{uid}      → change role
DELETE /api/v1/org/organizations/{id}/members/{uid}
POST   /api/v1/org/organizations/{id}/invitations
POST   /api/v1/org/invitations/{token}/accept
DELETE /api/v1/org/organizations/{id}/follow
PUT    /api/v1/org/organizations/{id}/policies
PUT    /api/v1/org/organizations/{id}/social
```

### catalog
```
PUT    /api/v1/catalog/products/{id}/variants/{vid}
DELETE /api/v1/catalog/products/{id}/variants/{vid}
PUT    /api/v1/catalog/categories/{id}
DELETE /api/v1/catalog/categories/{id}
PUT    /api/v1/catalog/brands/{id}
DELETE /api/v1/catalog/brands/{id}
GET    /api/v1/catalog/products                  → vendor's own list, keyset paged
POST   /api/v1/catalog/products/bulk-status      → activate/deactivate many
GET    /api/v1/catalog/search/facets             → category, brand, price band counts
```

### inventory
```
PUT    /api/v1/inventory/warehouses/{id}
DELETE /api/v1/inventory/warehouses/{id}
GET    /api/v1/inventory/stocks/low               → below min_threshold
GET    /api/v1/inventory/transfers
POST   /api/v1/inventory/transfers/{id}/receive
GET    /api/v1/inventory/movements                → org-wide ledger, keyset paged
```

### commerce
```
PATCH  /api/v1/commerce/cart/items/{variantId}    → quantity change; DELETE-then-POST loses the snapshot
POST   /api/v1/commerce/orders/{id}/cancel
POST   /api/v1/commerce/shipments/{id}/status
POST   /api/v1/commerce/orders/{id}/rate
GET    /api/v1/commerce/orders/{id}/history
POST   /api/v1/commerce/returns
GET    /api/v1/commerce/returns
```

### ingest — cannot build the wizard without these
```
GET    /api/v1/ingest/sessions
GET    /api/v1/ingest/sessions/{id}/rows          → paged, filter by match status
POST   /api/v1/ingest/sessions/{id}/mapping       → column mapping confirmation
POST   /api/v1/ingest/sessions/{id}/start
POST   /api/v1/ingest/sessions/{id}/commit
POST   /api/v1/ingest/sessions/{id}/cancel
PUT    /api/v1/ingest/sessions/{id}/rows/{rid}    → manual match override
GET    /api/v1/ingest/sessions/{id}/events        → SSE progress stream
```

### billing, promo, hr, workflow, notifications, platform_admin
Same pattern: every `POST` needs its `PUT`/`DELETE`; every list needs keyset
paging and filters. Plus:

```
GET    /api/v1/promo/sponsorships
POST   /api/v1/promo/sponsorships
POST   /api/v1/promo/sponsorships/{id}/review     → admin approve/reject
PUT    /api/v1/notifications/preferences
POST   /api/v1/notifications/read-all
GET    /api/v1/platform/translations
PUT    /api/v1/platform/translations/{key}
```

### admin surface (D-3) — per module, under `/api/v1/admin/`
Organizations pending approval; users; cross-tenant order search; platform
dashboards; reference-data CRUD for countries, cities, currencies, languages;
audit-log viewer; system settings. **This is the largest single API gap** — the
legacy app has 275 admin routes.

## 2.3 Wiring and correctness

1. **Wire `aicapabilities` (C6, still open).** `ingest` currently does its own
   inline matching. Route it through `aicapabilities` so trigram runs first and
   the Gateway is called only for rows below `min_similarity_score`. Also wire
   `catalog` search expansion and `commerce` order optimisation.
2. **Add the black-hole test**: `GATEWAY_BASE_URL` pointed at an unroutable
   address; full import and checkout must still complete.
3. **`internal/shared/pagination`** per D-4, applied to every list endpoint.
4. **CSRF middleware** per D-7.
5. **Rate limiting** — Redis token bucket: per-IP on auth, per-org on ingest and
   AI, per-key on the API.
6. **Localise `apperr` messages** per D-2 before the error surface grows.
7. **Audit-log writes** on every state-changing admin action, in-transaction.

---

# PART 3 — Frontend

**The largest remaining body of work. Currently 733 lines and one static page.**

## 3.1 Stack

templ (compiled templates) · HTMX 2 (interactivity) · Alpine.js (local state) ·
Tailwind 4 (styling) · Chart.js (dashboards only). **No React, no SPA router.**

Budget: **< 40 KB JS total**, FCP < 1.2 s on 3G, no layout shift.

## 3.2 Build order — foundations before screens

### Step 1 — Toolchain
`templ generate` in the Dockerfile builder stage and in CI. Tailwind build
producing `web/static/css/app.css`. Decide generated-file policy per D-1.
Self-host the Cairo font — the current page loads it from a CDN the CSP blocks.

### Step 2 — Design tokens, defined once
6 spacing steps · 5 type sizes · 3 weights · one accent · one radius · one border
colour · one shadow level for overlays. **No arbitrary Tailwind values in
templates.** RTL is the default direction; LTR is the variant.

### Step 3 — Component library (`internal/ui/components/`)
Button · Input · Select · Checkbox · Radio · Textarea · FormField (label, hint,
error) · **DataTable** (sticky header, sort, keyboard nav, empty/loading/error
states) · Pagination (keyset-aware) · Modal · Drawer · Toast · Badge · Card ·
Tabs · Dropdown · DatePicker · FileDropzone (presigned per D-5) · MoneyDisplay
(`tabular-nums`, always 2 decimals + currency) · Avatar · Skeleton.

**Every component ships with all four states designed: loading, empty, error,
partial.** This is the single biggest determinant of whether the app feels
finished.

### Step 4 — Three shells
`AdminShell` · `VendorShell` · `CustomerShell`. Same components, different
navigation and permission gates. Each fetches `GET /api/v1/me` once on load.

## 3.3 Screens, in dependency order

| # | Screen | Depends on | Notes |
|---|---|---|---|
| 1 | Login + MFA challenge | identity API gap | The gate to everything else |
| 2 | Password reset flow | identity | |
| 3 | Registration + org onboarding | identity, org | Vendor signs up, creates organization |
| 4 | Vendor: product list | catalog list + keyset paging | The DataTable's first real use |
| 5 | Vendor: product editor | catalog PUT, variants | Bilingual fields side by side |
| 6 | Vendor: inventory + stock adjust | inventory | |
| 7 | Vendor: import wizard | **ingest endpoints + SSE** | Upload → map columns → review matches → commit |
| 8 | Customer: catalogue + search | catalog search + facets | Arabic search, the core buyer experience |
| 9 | Customer: product detail | catalog | |
| 10 | Customer: cart | commerce cart + PATCH | |
| 11 | Customer: checkout | commerce checkout | Multi-vendor split shown clearly |
| 12 | Customer: order history + detail | commerce | |
| 13 | Vendor: order fulfilment | commerce shipments | Status transitions per D-9 |
| 14 | Vendor: offers and sponsorships | promo | |
| 15 | Admin: organization approval queue | org admin | |
| 16 | Admin: users and roles | identity admin | |
| 17 | Admin: reference data | platform_admin admin | |
| 18 | Admin: dashboards | reporting endpoints | Chart.js |
| 19 | Notifications centre | notifications | SSE badge |
| 20 | Public landing page + policy pages | platform_admin | Rebuild in templ; SEO, Arabic-first |

## 3.4 Anti-slop rules

Flat surfaces. One border colour. No gradients, no glassmorphism, no decorative
icons, no emoji. Numbers right-aligned and tabular. Money never abbreviated in a
table. Density over whitespace — this is a tool people use for hours, not a
marketing site. Test every screen with real Arabic product names, which change
line height and truncation behaviour.

---

# PART 4 — ETL

**Not started. Blocked on U5 and, for strategy, U1.**

Per `REBUILD_MASTER_PLAN.md` §11, as `cmd/cli migrate-data`:

1. **Add a MariaDB driver** — none exists in `go.mod` today.
2. **Extract** → newline-delimited JSON per table, chunked, resumable.
3. **Validate** → orphan sweep across the 36 unconstrained `*_id` columns;
   invalid JSON; out-of-range enums; duplicate emails; negative money. Emit a
   **pre-migration defect report** and stop.
4. **Transform** → type mapping, `users` decomposition, order-system unification,
   entitlement consolidation, blobs to object storage, UTC→`TIMESTAMPTZ`.
5. **Load** → `COPY`, FKs deferred, indexes created after.
6. **Verify** → the gate computes **both sides itself**. Row counts exact;
   checksums; money sums **to the cent**; zero FK orphans; 100% JSON parse;
   timestamp min/max after conversion; business invariants
   (`SUM(lines.line_total) = order.final_price`).
7. **Reconcile** → per-table go/no-go report.

**Preserve legacy primary keys.** User 4417 stays 4417.

**Resolve U5 first** — which legacy order system is authoritative. The target
model assumed `main_orders`+`adv_orders`. Verify by row counts and recent
`created_at` in both before migrating a single row.

---

# PART 5 — Testing and quality gates

| Gate | Current | Target |
|---|---|---|
| Unit tests on domain logic | Partial | 90% coverage on every `domain.go` |
| Repository integration tests | Thin | Every repository method, real PostgreSQL |
| **RLS cross-tenant tests** | Runs in CI | **100% of tenant tables — no exceptions** |
| Handler tests | Routing only (nil repos) | Real service, auth, permission and validation paths |
| Gateway black-hole test | Missing | Full import + checkout with AI unreachable |
| Money property tests | Good | Extend to order totals and multi-vendor allocation |
| E2E | None | 12 critical journeys, Playwright, Arabic + English |
| Load | None | k6: catalogue search, checkout, bulk import |
| Parity vs Laravel | None | 500 search queries; 1,000 order repricings **to the cent**; full permission matrix |

**Ship gate: 100% parity on money and permissions.** Anything less is a blocker.

---

# PART 6 — Operations

1. **Object storage** — MinIO service, bucket, keys. Required by D-5 and by
   `APP_ENV=prod`.
2. **Move to `APP_ENV=prod`** once storage exists.
3. **Backups** — PITR on `dawa24_store`, plus a **tested restore into a scratch
   database**. An untested backup is not a backup.
4. **Metrics** — Prometheus endpoint: RED per module, queue depth, gateway
   latency and fallback rate, pool saturation.
5. **Alerts** — `/ready` failing, queue depth growing, error rate, DB connections
   above 80%.
6. **Secrets rotation** — the Redis password and the Gateway virtual key were both
   exposed in chat transcripts.
7. **Lock down the stray Gateway** deployed at the `dawa24-store` domain against
   production Gateway data.

---

# PART 7 — Sequenced roadmap

Each phase ends with a gate. **Do not start the next until the gate passes.**

| Phase | Scope | Gate | Est. |
|---|---|---|---|
| **T** | Decisions D-1…D-11 recorded as ADRs. `pagination` package. `apperr` message-key refactor. CSRF. Rate limiting. | ADRs merged; every list endpoint paged; CSRF on all mutations | 1 wk |
| **U** | Migrations 021–026 (20 tables) + RLS + tests | All tables have RLS tests; migrations idempotent | 1.5 wk |
| **V** | identity API gap (esp. `GET /api/v1/me`), org CRUD, catalog/inventory/commerce CRUD completion | ~90 new endpoints; handler tests with real services | 3 wk |
| **W** | ingest endpoints + SSE; wire `aicapabilities`; black-hole test | Import runs end-to-end with Gateway unreachable | 2 wk |
| **X** | Admin surface across all modules (D-3) | Admin can approve orgs, manage users, edit reference data | 2.5 wk |
| **Y** | **Frontend foundations** — toolchain, tokens, components, three shells | Component gallery renders; all four states per component; RTL verified | 3 wk |
| **Z1** | Screens 1–7 (auth, vendor catalogue, import wizard) | Vendor can log in, manage products, run an import | 4 wk |
| **Z2** | Screens 8–14 (customer journey, fulfilment) | A pharmacy can search, cart, checkout; vendor fulfils | 4 wk |
| **Z3** | Screens 15–20 (admin, notifications, landing) | Admin operable; public pages SEO-ready | 3 wk |
| **AA** | ETL | Defect report clean; all verification gates pass on a full dataset | 3 wk |
| **AB** | Parity, load, E2E | 100% money and permission parity; SLOs met | 2.5 wk |
| **AC** | Cutover | Reverse-ETL tested; two staging rehearsals; runbook | 1.5 wk |

**Total ≈ 31 weeks solo**, ~19 with two developers working backend/frontend in
parallel from Phase Y.

**Parallelisation:** Y can start as soon as V is done — the frontend needs stable
APIs, not complete ones. AA can run alongside Z1–Z3 by a second person.

---

# PART 8 — Risks

| Risk | Impact | Mitigation |
|---|---|---|
| **U5 unresolved** — wrong order system assumed | ETL migrates the wrong data; discovered after cutover | Resolve in Phase T with row counts. **Highest-value hour in the project.** |
| **U2 unresolved** — payment settlement unknown | Phase V billing work is guesswork | Inspect `payment_integrations` rows and vendor payment Livewire components |
| **U3** — live LLM-generated SQL | Active vulnerability in the Laravel app | Investigate; report; do not port |
| Frontend underestimated | The 3→100% jump is the biggest unknown | Build the component library first; screens then go fast |
| Parity impossible to prove | Legacy has zero tests | Characterisation tests against Laravel **before** rewriting each rule |
| RLS gap on a new table | Cross-tenant data leak | RLS test is a CI gate; no table ships without one |
| Schema drift from `docs/modules/` | Next agent re-derives wrongly | Update the module doc in the same commit as the migration |

---

# PART 9 — Definition of done for the whole project

- [ ] Every legacy feature either implemented, or explicitly recorded as
      deliberately dropped with a reason in `docs/modules/`
- [ ] 100% of tenant-owned tables have `ENABLE`+`FORCE` RLS and a passing
      cross-tenant test
- [ ] Money parity with Laravel to the cent on 1,000 historical orders
- [ ] Permission matrix identical for every (role, resource, action) triple
- [ ] Every AI capability works with the Gateway unreachable
- [ ] All three shells complete; every screen has loading, empty, error and
      partial states
- [ ] ETL verification gates pass on a full production-sized dataset
- [ ] Reverse-ETL written and rehearsed
- [ ] SLOs met under k6 load
- [ ] `HANDOFF.md` accurate
