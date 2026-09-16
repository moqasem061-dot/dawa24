# Dawa24 — Build Phases

**Measured:** 2026-08-17 against the live database and the 141-table legacy inventory.
**Supersedes:** `FINISH_THE_SYSTEM.md`, `AUDIT_2026-08-16_FULL.md`, `REMAINING_WORK.md`.

This is a **build** plan. Testing is deferred to Phase 8 on purpose — write the
system first, harden it at the end. Do not stop between tasks to ask what is
next; the next task is the next heading. Work top to bottom.

---

# PART 0 — Why the system looks weak

> **STATUS (2026-08-17, after the build run):** Phases 1–6 are built and gated
> green (`templ generate && go build ./... && go vet ./...`, `gofmt` clean, full
> short test suite passes). Account types, three dashboards, the account surface,
> public/content pages, all ten Phase-4 modules, the component library, and all
> fourteen admin screens now exist. **Phase 7 (ETL) is still blocked on the two
> owner decisions** (authoritative order system; legacy company/agency mapping).
> Phase 8 hardening is partial (unit tests + CI coverage floor done; repository/
> integration tests, infra rotation, a11y/perf measurement remain). The legacy
> Laravel app is still the only deployed system.

It is not a rendering problem. **The backend now covers the full legacy feature
set for Phases 1–6, and the frontend exposes the account surface, the public
marketplace, the supplier/pharmacy dashboards and the admin surface.**

| | Legacy | Now (after build) | Gap |
|---|---|---|---|
| Tables | 141 | 98 + 10 new migrations | legacy ETL tables not yet loaded |
| Page routes | ~60 screens | 60+ | account surface now present |
| Dashboards | admin + supplier + pharmacy | **all three** | none |
| Registration | supplier / pharmacy / company | **supplier / pharmacy / chain** | none |

## The single most important defect

`internal/ui/public_handlers.go` — registration ignores account type entirely:

```go
_, sess, err := h.idSvc.Register(ctx, identity.RegisterInput{
	Email: email, Password: password,
	NameAr: name, NameEn: name,
	Role:  "customer",   // hardcoded; no supplier, no pharmacy
	Phone: phone,
})
```

No organization is created, no membership, no role. Everyone who registers is a
customer with **no organization**, so every tenant-scoped screen shows nothing.
That is why the product feels empty: the front door does not work. **Phase 1

> **FIXED (Phase 1).** `RegisterSubmit` now reads `account_type` and calls
> `identity.RegisterOrganization`, which creates the user, organization, owner
> membership and main branch in one transaction and routes to the right
> dashboard (`/vendor/dashboard`, `/pharmacy/dashboard`, `/onboarding/pending`).

fixes this and everything else depends on it.**

## Features present in Laravel with no table, module or screen here

| Legacy tables | Feature | Status |
|---|---|---|
| `chat_conversations`, `chat_messages` | Buyer↔supplier messaging | ✅ built (`chat` schema, `/messages`) |
| `compare_discount_plans` ×6 | Paid discount-comparison subscriptions | ✅ built (`/compare`, entitlements) |
| `ask_fors` | Document/action requests between parties | ✅ built (`workflow.requests`, `/requests`) |
| `trees`, `tree_options`, `tree_results` | Guided product-finder questionnaire | ✅ built (`catalog.finder_*`, `/finder`, `/admin/finder`) |
| `institutional_works` | Institutional service catalogue | ✅ built (`workflow.services`, `/services`) |
| `session_plans` | Concurrent-session licensing | ✅ built (enforcement + `/settings/security`) |
| `visitors` | Traffic analytics | ✅ built (middleware + `/admin/analytics`) |
| `organization_highlight_sections` ×2 | Per-org merchandising | ✅ built (`org.highlight_sections`, `/vendor/storefront`) |
| `user_address_histories` | Address audit trail | ✅ built (append-only + timeline) |
| `what_in_contents` | CMS content blocks | ✅ built (`platform_admin.content_blocks`) |

## Features with a table and API but **no screen**

These are already built server-side and invisible to users — the cheapest wins
in the whole plan:

`identity.user_favorites` · `platform_admin.languages` + `translations` ·
`platform_admin.contact_messages` · `identity.user_addresses` ·
`profile.user_profiles` + `user_preferences` · `commerce.wishlists` ·
`commerce.quote_requests` · `promo.offers` · `hr.job_offers` ·
`org.organization_reviews` · `org.organization_followers` ·
`platform_admin.privacy_policies` · `billing.wallets` · `billing.invoices`

> **FIXED.** Every item above now has a screen: favourites (`/favorites`), the language selector, contact (`/contact` + `/admin/messages`), addresses (`/settings/addresses`), profile/preferences (`/settings/profile` + `/settings/preferences`), wallet/invoices (`/wallet`, `/invoices`), offers (`/offers`), jobs (`/jobs`), supplier reviews/follows (`/suppliers/{id}`), and privacy/terms (DB-driven).

---

# PART 1 — Rules (short, and non-negotiable)

1. **Identity from `authctx`.** Never a query parameter, body field or header.
2. **Money is `money.Amount`** — int64 minor units. Never `float64`.
3. **Bilingual text is `i18n.Text`** — JSONB `{ar,en}`. Arabic is primary, not a
   translation.
4. **Tenant scope in the `WHERE` clause.** The app connects as `postgres`, so
   all 53 RLS policies are inert. RLS is not a guard; your predicate is.
5. **Every `/api/v1/admin/` group wraps in `RequirePermission`.** Three modules
   once shipped without it and were open to any logged-in user.
6. **A handler taking an org id from the URL calls `authctx.SameOrgOrForbidden`.**
7. **Admin mutations write `platform.audit_log` in the same transaction**, via
   `database.WriteAudit`.
8. **Nullable text columns:** scan into a pointer, or `NOT NULL DEFAULT ''`. Run
   `go run ./cmd/dbcheck -nullscan "$DATABASE_URL"` after any migration adding
   text columns.
9. **CHECK constraints must match the Go domain.** Registering a pharmacy once
   failed because the constraint still held the legacy enum.
10. **Components use `app.css` / `components.css` tokens.** Tailwind is not
    loaded and never has been — utility classes render as nothing.
11. **No provider name outside `internal/platform/gateway/`.**

**Gate before calling a task done:**

```bash
templ generate && go build ./... && go vet ./...
```

---

# PHASE 1 — Account types, roles and the three dashboards

**The foundation. Nothing else works properly until this lands.**

## 1.1 — Registration chooses an account type

**Migration `035_account_types.up.sql`**

`identity.users.role` currently holds a platform role. Organization membership
carries the real capability, so add nothing to `users`; instead ensure these
role keys exist in `identity.roles` (scope `organization`):

| key | Arabic | For |
|---|---|---|
| `org_owner` | مالك المؤسسة | Whoever registered the organization |
| `org_manager` | مدير | Day-to-day management |
| `org_employee` | موظف | Limited operational access |
| `org_accountant` | محاسب | Billing and invoices only |
| `org_warehouse` | أمين مخزن | Inventory and transfers only |

Seed `identity.permissions` and `identity.role_permissions` for each, keyed
`<module>.<action>` — `catalog.product.create`, `commerce.order.dispatch`,
`billing.invoice.read`, `inventory.transfer.approve`, and so on.

**`internal/ui/pages/auth.templ` — rewrite the registration form**

Step 1 is a choice, rendered as two large cards, Arabic-first:

- **مورّد / شركة أدوية** (`supplier`) — sells to pharmacies
- **صيدلية** (`pharmacy`) — buys from suppliers
- **سلسلة صيدليات** (`chain_pharmacy`) — multi-branch pharmacy

Step 2 collects, per type:

| Field | supplier | pharmacy | chain |
|---|---|---|---|
| `legal_name` | ✔ required | ✔ | ✔ |
| `trade_name_ar` / `trade_name_en` | ✔ | ✔ | ✔ |
| `commercial_register` | ✔ required | ✔ required | ✔ |
| `tax_number` | ✔ required | optional | ✔ |
| `pharmacist_license` | — | ✔ required | ✔ |
| `city_id` | ✔ | ✔ | ✔ |
| `branch_count` | — | — | ✔ |

**`internal/modules/identity/service.go` — `RegisterOrganization`**

One transaction:

1. Create the user, `role = 'customer'` (platform role stays low).
2. Create `org.organizations` with the chosen `type` and `status = 'pending'`.
3. Create `org.members` with `role_key = 'org_owner'`, `status = 'active'`.
4. Create the main `org.branches` row.
5. Issue the session with `ActiveOrgID` set to the new organization.
6. Write `platform.audit_log`.

**`internal/ui/public_handlers.go`** — `RegisterSubmit` reads `account_type` and
calls the above. Validation failures re-render the form **with the entered
values still in the fields**; a registration form that empties itself on error
is the fastest way to lose a signup.

**Done when:** registering as each of the three types produces a user, an
organization of the right type, an owner membership, and a session whose
`ActiveOrgID` is that organization — and the new account lands on its own
dashboard.

## 1.2 — Post-registration routing

`identity.Session` already carries `Role` and `ActiveOrgID`. Add the
organization type to the session so the shell can branch without a query per
request. After login or registration:

| Organization type | Lands on |
|---|---|
| `supplier` | `/vendor/dashboard` |
| `pharmacy`, `chain_pharmacy` | `/pharmacy/dashboard` |
| none (individual) | `/catalog` |
| platform `super_admin` / `admin` | `/admin/dashboard` |

Approval gate: an organization with `status = 'pending'` reaches a
`/onboarding/pending` screen explaining what happens next, **not** a dashboard
full of zeroes. Rejected organizations see the reason and a contact route.

## 1.3 — Supplier dashboard — `/vendor/dashboard`

New: `internal/ui/pages/vendor_dashboard.templ` + handler.

**Metric row** (`components.SkeletonStats(5)` while loading):

| Metric | Source |
|---|---|
| المنتجات النشطة | `COUNT catalog.products WHERE organization_id AND status='active'` |
| طلبات قيد التنفيذ | `COUNT commerce.order_shipments WHERE organization_id AND status IN ('pending','confirmed')` |
| مبيعات الشهر | `SUM order_lines.line_total` for the month — `money.Amount` |
| رصيد المحفظة | `billing.wallets` |
| أصناف تحت الحد الأدنى | `inventory.stocks WHERE quantity <= reorder_level` |

**Panels:** latest 10 shipments awaiting dispatch with a one-click dispatch
action; low-stock table linking to inventory; last import session with its
progress; active offers with view/click counts from `promo.offer_views` and
`offer_clicks`; unread quote requests.

## 1.4 — Pharmacy dashboard — `/pharmacy/dashboard`

New: `internal/ui/pages/pharmacy_dashboard.templ` + handler + a
`layouts.PharmacyShell`.

**Metrics:** open orders, this month's spend, wallet balance, saved favourites,
active discount offers available to this pharmacy.

**Panels:** reorder suggestions from `catalog.saving_products` and past order
lines; order tracker showing each order's live `order_status_history`; offers
targeted at this pharmacy's city via `promo.offer_location_covers`; followed
suppliers from `org.organization_followers`.

**Navigation** (`PharmacyShell` sidebar): الرئيسية · تصفح المنتجات · السلة ·
طلباتي · المفضلة · العروض · الموردون · المحفظة والفواتير · الإعدادات

## 1.5 — Permission-aware navigation

The sidebar renders only what the member may reach. `authctx.Actor.Can(perm)`
already exists — use it per link. A warehouse clerk must not see billing.

**Done when:** three account types each land on a distinct dashboard with real
figures, and the sidebar differs by role.

---

# PHASE 2 — The account surface

Everything a signed-in user needs about themselves. All of it has tables and
most has an API; none of it has a screen.

## 2.1 — Settings — `/settings`

`internal/ui/pages/settings.templ`, tabbed via `components.Tabs` with `URL` set
so each tab is a real, shareable route.

### `/settings/profile`
`profile.user_profiles` + `identity.users`. Name (ar/en), phone with
verification state, avatar upload **via `storage.PresignPut`** — never proxied.
Email change requires the current password.

### `/settings/addresses`
`identity.user_addresses` — full CRUD. Title, recipient, phone, city
(`platform_admin.cities`), address, building, floor, apartment, default flag.
Setting a new default clears the old one **in the same transaction**. Deletion
goes through `components.ConfirmModal`. Write `user_address_histories`
(Phase 4.7) on every change.

### `/settings/security`
Password change, MFA enrol/disable with a QR code, active sessions with
individual revoke, and login history from `identity.user_security`.

### `/settings/organization` — owners and managers only
Organization profile, branches CRUD, members with role assignment and invites,
`org.organization_social_media`, `org.organization_policies`.

### `/settings/preferences`
`profile.user_preferences` — language, timezone, notification channels per
event type.

## 2.2 — Favourites — `/favorites`

`identity.user_favorites` exists with a repository and no screen.

- Heart toggle on every product card and detail page, HTMX `hx-post`
  `/favorites/{productID}/toggle`, swapping only the icon.
- `/favorites` lists them as a product grid with an empty state that links to
  the catalogue.
- The count appears in the header beside the cart.
- `commerce.wishlists` is the **shared, named** list — a pharmacy building a
  monthly order — distinct from personal favourites. Give it
  `/favorites?tab=lists`.

## 2.3 — Language selector

`platform_admin.languages` and `translations` exist and nothing reads them.

- Header control switching ar ⇄ en, persisted to
  `profile.user_preferences.language` for signed-in users and a `dawa24_lang`
  cookie otherwise.
- Replace `localeAndDir` in `internal/ui/handlers.go`: precedence becomes
  **query `?lang=` → user preference → cookie → `Accept-Language` → ar**.
- `dir` flips to `ltr` for English and every layout must survive it — the CSS is
  already logical-direction-first, so check for hardcoded `left`/`right`.
- Admin CRUD for `translations` at `/admin/translations`.

## 2.4 — Notifications as a dropdown

Delete the full-page notification screen as the primary surface.

- Bell in the header with an unread badge from
  `notifications.GetUnreadCount`, polled with
  `hx-trigger="load, every 60s"`.
- `components.Dropdown` panel, 400px, showing the latest 10: icon by type,
  Arabic relative time ("منذ ٥ دقائق"), unread strip, click marks read and
  navigates to the entity.
- "تعليم الكل كمقروء" and a link to `/notifications` for the full archive,
  which stays for history and filtering.
- Endpoints: `GET /notifications/dropdown` (partial),
  `POST /notifications/{id}/read`, `POST /notifications/read-all`.

## 2.5 — Wallet and invoices — `/wallet`, `/invoices`

`billing.wallets`, `wallet_transactions`, `invoices`, `invoice_lines`,
`payment_histories`, `user_payment_methods` — all present, none exposed.

Balance card, paginated transaction ledger with running balance, top-up flow,
payment-method management (including the **delete** endpoint that has no route),
invoice list with status badges and a print view.

---

# PHASE 3 — Public and content pages

## 3.1 — Contact us — `/contact`

`platform_admin.contact_messages` exists.

Form: name, email, phone, subject, message, plus organization type when signed
out. Rate-limited by IP. On submit, write the row, notify admins, and reply with
a success state that does not clear the page. Admin inbox at `/admin/messages`
with status (`new`/`read`/`replied`), assignment and a reply box.

## 3.2 — Content pages driven by the database

`platform_admin.privacy_policies` and `documents` exist; `what_in_contents` must
be added (Phase 4.8).

- `/privacy`, `/terms` render from the database, not hardcoded markup, with an
  effective date and version history.
- `/about`, `/how-it-works`, `/faq`, `/help` render CMS blocks.
- `/admin/content` edits them with a bilingual side-by-side editor.

## 3.3 — Supplier directory — `/suppliers`, `/suppliers/{id}`

`org.organizations` + `organization_reviews` + `followers` + `social_media` +
`policies` all exist with no public surface.

Directory with city and type filters. Profile shows the supplier's catalogue,
rating and reviews, policies, social links, a follow button
(`organization_followers`), and a "طلب عرض سعر" action creating a
`commerce.quote_requests` row.

## 3.4 — Offers — `/offers`, `/offers/{id}`

`promo.offers`, `offer_products`, `offer_views`, `offer_clicks`,
`offer_promotions`, `offer_location_covers`, `highlight_sections`.

Public offers page filtered by the viewer's city through
`offer_location_covers`. Record a view on render and a click on navigation —
the supplier dashboard already promises those numbers. Home page renders
`highlight_sections` as merchandised rows.

## 3.5 — Jobs — `/jobs`, `/jobs/{id}`

`hr.job_offers`, `job_applications`, `job_categories` exist with no screen.
Listing, detail, apply with a CV upload via presigned PUT, and
`/vendor/jobs` for the supplier to manage postings and review applicants.

---

# PHASE 4 — The modules Laravel had and this system does not

Each needs migration → domain → repository → service → HTTP → UI. Follow the
existing module shape exactly: `internal/modules/<name>/{domain.go, repository.go,
service.go, postgres/, http/}`.

## 4.1 — Messaging (`chat`) — highest value

Legacy `chat_conversations`, `chat_messages`.

**Migration `036_chat.up.sql`** — schema `chat`:

```
conversations   id, public_id, organization_id, counterparty_org_id,
                subject i18n, context_type ('order'|'quote'|'product'|'general'),
                context_id, status, last_message_at, created_by_user_id
messages        id, conversation_id, sender_user_id, sender_org_id,
                body TEXT NOT NULL DEFAULT '', attachments JSONB,
                read_at, created_at
participants    conversation_id, user_id, organization_id, last_read_at
```

RLS on `organization_id` **and** `counterparty_org_id` — a conversation is
visible to both sides, which the standard `tenant_visible` helper does not
express. Write the policy explicitly.

**UI:** `/messages` two-pane (list + thread), `components.Drawer` on mobile.
SSE for live delivery — **set `X-Accel-Buffering: no`** or the proxy buffers
every event. Unread badge in the header. "راسل المورّد" from supplier profiles,
order detail and quote requests, carrying the context.

## 4.2 — Requests (`ask_fors`)

Legacy `ask_fors`: type (`document`|`action`|`approval`), title, description,
status (`pending`/`accepted`/`declined`/`cancelled`), `action_url`, between a
user and an organization.

A supplier asks a pharmacy for its licence; a pharmacy asks a supplier for a
certificate. Schema `workflow.requests`. Inbox at `/requests` with status tabs,
attachments via presigned upload, accept/decline writing an audit row.

## 4.3 — Discount comparison plans

Legacy `compare_discount_plans`, `_features`, `_requests`, `_subscriptions`,
`_subscription_users`, `compare_discount_user_sessions`. Monthly / yearly /
lifetime pricing — a **revenue feature**, so it is not optional.

Schema `billing.compare_plans` + `compare_plan_features` +
`compare_subscriptions`. Pricing page, subscribe flow through the existing
payment path, entitlement checks gating the comparison tool, and admin plan
CRUD. Money in `money.Amount` throughout.

## 4.4 — Guided product finder (`trees`)

Legacy `trees` (bilingual question, type `choice`/`text`/`number`, `is_first`),
`tree_options`, `tree_results`.

A questionnaire walking a pharmacist to the right product. Schema
`catalog.finder_questions` / `_options` / `_results`. `/finder` renders one
question at a time over HTMX; admin builds the tree at `/admin/finder` with a
preview.

## 4.5 — Institutional works

Legacy `institutional_works` (bilingual title/description, icon, `pricing_type`
free/paid/subscription, `parent_id` hierarchy), `institutional_work_connections`,
`employee_institutional_works`.

A hierarchical catalogue of institutional services. Schema `workflow.services`.
Public listing, detail, request flow, admin CRUD with drag-ordering.

## 4.6 — Session plans

Legacy `session_plans` (`max_login_sessions`, price, days, `is_free`),
`session_plan_requests`, `user_session_histories`.

Licensing on concurrent sign-ins. `identity.user_security.max_login_sessions`
already exists — enforce it in `SessionStore.Create` by counting live sessions
and refusing or evicting the oldest. Plans, purchase, and a session history
screen under `/settings/security`.

## 4.7 — Address history

Legacy `user_address_histories`. Append-only rows on every address change,
shown as a timeline under `/settings/addresses`.

## 4.8 — CMS blocks

Legacy `what_in_contents`. Schema `platform_admin.content_blocks`: key, bilingual
title and body, position, active flag. Backs Phase 3.2.

## 4.9 — Visitor analytics

Legacy `visitors`: ip, user agent, browser, device, os, country, city.

Middleware recording one row per session per day — **not** per request, or the
table grows unusably. `/admin/analytics` shows traffic, device split and
geography. Store the IP truncated to /24 and say so in the privacy policy.

## 4.10 — Per-organization merchandising

Legacy `organization_highlight_sections`, `organization_highlight_section_items`.
Only the platform-level equivalent exists. Lets a supplier curate rows on its own
profile page. Schema `org.highlight_sections` + `_items`, managed from
`/vendor/storefront`.

---

# PHASE 5 — Finish the component library and the screens using it

The library is on the real design system as of the last round, but most of it is
used nowhere.

## 5.1 — Wire what exists

`SkeletonGrid` on the catalogue, `SkeletonTable` on every data table,
`SkeletonStats` on dashboards, `SkeletonList` on messages and notifications.
Every HTMX-loading region gets `hx-indicator` pointing at a
`components.LoadingIndicator`.

**Rule:** a screen that fetches has all four states — loading, empty, error,
partial. Retrofitting means touching every screen twice.

## 5.2 — Components still missing

| Component | For |
|---|---|
| `Combobox` | Searchable product / city / supplier pickers |
| `DateRangePicker` | Report and analytics filters |
| `Stepper` | Registration, checkout, import wizard |
| `Timeline` | Order status history, address history |
| `StatCard` | Dashboard metric, with delta and sparkline |
| `Rating` | Supplier reviews, read and write |
| `QuantityStepper` | Cart and order lines, honouring pack size |
| `ImageGallery` | Product detail with zoom |
| `CommandPalette` | ⌘K navigation for power users |
| `Breadcrumbs` | Deep catalogue navigation |

## 5.3 — Modals to add

Quick-view product, add-to-cart with quantity and pack size, quote request,
shipment dispatch with carrier and tracking, stock adjustment with reason,
member invite, address picker at checkout, image cropper for avatars and logos,
bulk-action confirmation with an affected-row count.

## 5.4 — Depth on existing screens

**Catalogue:** faceted filters (category, brand, dosage form, price band,
availability), sorting, saved searches, grid/list toggle, compare up to four,
pagination that keeps filters in the URL.

**Product detail:** gallery, variants with per-pack pricing, customer-specific
pricing from `catalog.customer_product_mappings`, stock by warehouse,
alternatives by `scientific_name`, price-drop alert
(`catalog.product_alerts`), reviews.

**Cart:** per-supplier grouping (an order splits by supplier), minimum-order
warnings per supplier, applied offers, live totals.

**Checkout:** a real `components.Stepper` — address → shipping → payment →
review; saved addresses; wallet or payment method; per-supplier delivery
estimate; final total that **equals the sum of its lines to the cent**.

---

# PHASE 6 — Admin surface completion

`/admin/dashboard` with real platform metrics and charts · `/admin/organizations`
with approve/reject/suspend and document review · `/admin/users` (done) ·
`/admin/orders` cross-tenant search and refunds · `/admin/products` moderation ·
`/admin/offers` and ad approval · `/admin/content` · `/admin/translations` ·
`/admin/messages` · `/admin/analytics` · `/admin/audit` reading
`platform.audit_log` with actor, action and before/after · `/admin/settings`
(done) · `/admin/jobs` for River queue depth and failures.

Every mutation: `RequirePermission`, `database.AsSystem` with a comment, and an
audit row in the same transaction.

---

# PHASE 7 — ETL

Unchanged and still blocking cutover. 1,263 lines, 17 statements, 141 tables.

**Two decisions needed from the owner before transform code is written:**

1. **Which legacy order system is authoritative** — `orders` + `order_items`, or
   `main_orders` + `adv_orders`? Report row counts and recent `created_at` for
   both.
2. **How do legacy `company` and `agency` map** onto supplier / pharmacy /
   chain_pharmacy? Migration 034 accepts all five so nothing is blocked, but the
   ETL cannot translate them until this is answered. Report the row count per
   legacy value.

Stages: extract to NDJSON (chunked, resumable) → validate and **stop** on defects
(orphans across the 36 unenforced `*_id` columns, invalid JSON, bad enums,
duplicate emails, negative money) → transform (user decomposition, order
unification, blobs to object storage, **explicit UTC → `TIMESTAMPTZ`**) → load
with `COPY`, FKs deferred, indexes after → **verify by computing both sides
itself** → per-table reconciliation. **Preserve legacy primary keys.**

---

# PHASE 8 — Hardening (last, deliberately)

Only now: repository tests for every new module, service coverage above 60%,
handler authorization tests, CI failing on `0.0%` coverage, RTL verified with
real Arabic product names, 40 KB JS budget, FCP under 1.2s on 3G, accessibility
pass, `dawa24_app` switch, **credential rotation** (PostgreSQL, Redis, Gateway
key — all have appeared in transcripts), MinIO, backups with a *tested* restore,
metrics.

---

# PART 2 — Execution order

Phases 1 → 2 → 3 are sequential; 1 unblocks everything.
Phase 4 items are independent — 4.1 (chat) first, it carries the most value.
Phase 5 runs alongside 2–4 as screens land.
Phase 6 after 4. Phase 7 anytime once its two questions are answered.
Phase 8 last.

**Rough scale:** ~11 migrations, ~7 new modules, ~45 new page routes, ~120 new
endpoints, ~30 new components and modals.

# PART 3 — Reporting

Per session: tasks completed, files changed, **commands run with real output**,
anything contradicting this document with evidence, what is blocked and on what,
and Part 0 updated to match reality. Measure before writing the summary.
