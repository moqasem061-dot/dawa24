# EXECUTION PROMPT — Dawa24 Store, remaining work

You are continuing a large, already-half-finished engineering plan on an existing Go
codebase. Everything you need is on disk. **Do not invent requirements, do not redesign
anything, and do not skip the verification steps.** Where this prompt says "read lines
X–Y of the plan", you must actually open and read them before writing code.

---

## 1. Ground truth

- Working directory (ALL work happens here): `F:\Dawa 24\dawa24-store`
- The master plan: `F:\Dawa 24\dawa24-store\docs\plans\MASTER_EXECUTION_PLAN.md` (3231 lines)
- Go module path: `github.com/muhiya/dawa24-store`
- Other directories under `F:\Dawa 24\` (`Laravel/`, `Dawa24/`, `docs/rebuild/`) are the
  **legacy system being replaced**. Read for reference only. **Never edit them.**

**Before writing any code**, read these sections of the plan in full:

| Lines | What |
|---|---|
| 12–236 | PART 0 — rules, layout, build commands, DB access, live-data facts |
| 236–389 | PART 1 — architecture primer |
| 2761–2868 | PART 6 — verification and definition of done |
| 3013–3231 | PART 8 — review of what shipped, defects, and the remaining-work list |

Product context in one paragraph: Dawa24 is a B2B pharmaceutical marketplace for Egypt.
Pharmacies buy from suppliers **and suppliers buy from other suppliers** — that second case
is first-class, so any code assuming "the buyer is a pharmacy" is wrong. Arabic is the
primary language; every user-facing string is bilingual and RTL-first.

---

## 2. Non-negotiable rules (CI gates enforce these)

1. **Money never touches `float64`.** Use `internal/shared/money.Amount`; DB uses `NUMERIC(p,2)`.
2. **No AI provider or model names outside `internal/platform/gateway/`.** Everything else
   asks for a *capability* or *role*, never a model.
3. **Every AI capability has a deterministic fallback.** Ordering and importing must work
   with the AI gateway switched off.
4. **Tenant-scoped queries run inside `db.InTx` / `db.InReadTx`** (these set the Postgres GUC
   that row-level security reads). Never call `db.Pool()` from a module. Cross-tenant reads
   go through `database.AsSystem(ctx)`.
5. **Module boundaries:** `internal/modules/A` may NOT import `internal/modules/B`. Modules
   may import `internal/shared/*` and `internal/platform/*`. `platform/*` may not import
   `modules/*`. Cross-module needs = an interface declared by the consumer, implemented in
   the composition root (`cmd/server/`). Enforced by golangci-lint depguard.
6. **400 lines maximum per Go file** (excluding `*_templ.go`). 58–71 files already violate
   this; do not make it worse. Every file **you** create or substantially rewrite must be
   under 400 lines.
7. **Never edit an applied migration** (the runner checksums them). Add a new numbered one.
8. **No hardcoded user-facing Arabic in `.go` files.** Add a key to `internal/shared/i18n`
   and call `i18n.T(lang, key)`. Ceiling is 0. Arabic inside `.templ` is tolerated, but new
   work should prefer i18n keys.
9. **No CDN links in templates.** htmx, Alpine and Leaflet are vendored under
   `internal/ui/static/vendor`.
10. **No raw `<dialog>` in page templates** beyond the existing ceiling. Use `components.Modal`.
11. **CSS: this codebase hand-rolls its CSS with Tailwind-*looking* class names.** A class no
    stylesheet defines renders as **nothing** — the page silently loses its design. Never
    invent a class name. Grep `internal/ui/static/css/` for an existing class before using
    one. Canonical vocabulary includes: `glass-panel`, `stat-card-3d`, `data-table`,
    `tab-btn`, `btn-outline-brand`, `btn-outline-success`, `btn-outline-secondary`,
    `badge-slate`, `alert-info`, `b2b-pagination`.
12. **No emoji in templates** (ceiling 8 across all templates; the client explicitly asked).

---

## 3. Build, test and verify

```bash
cd "F:/Dawa 24/dawa24-store"

templ generate          # MANDATORY after editing any .templ; *_templ.go IS committed
go build ./...
go vet ./...
go test -short -count=1 ./...
```

`make` is **not installed**. `bash` is (`/usr/bin/bash`); `templ` is at
`/c/Users/mydwa/go/bin/templ`. `go build ./...` takes ~2 minutes cold — run it in the
background rather than letting it time out.

Gate commands to run manually (from `F:/Dawa 24/dawa24-store`) — see plan lines 2786–2814:

```bash
gofmt -l ./cmd ./internal                                    # must print nothing

# AI provider isolation
grep -rn --include='*.go' -iE '(openai|anthropic|gpt-|claude-|gemini|qwen|gemma|whisper)' \
  ./cmd ./internal | grep -v '/platform/gateway/' | grep -v '_test.go'   # must be empty

# 400-line ceiling — count must not exceed baseline
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + \
  | grep -v ' total$' | awk '$1>400' | wc -l

# Hardcoded Arabic in Go (ceiling 0)
LC_ALL=C grep -rn --include='*.go' '"[^"]*[أ-ي][^"]*"' ./internal/ui ./internal/modules ./cmd \
  | grep -v '_test\|_templ\|/i18n/'

# CDN assets in templates (must be empty)
grep -rn --include='*.templ' -E '(src|href)="https?://(unpkg|cdnjs|cdn\.jsdelivr)' internal/ui

# Raw <dialog> in pages — must not grow
grep -rn '<dialog' internal/ui/pages/*.templ | wc -l

# Emoji in templates (ceiling 8)
LC_ALL=C grep -ro $'\xf0\x9f' --include='*.templ' internal/ui | wc -l

# !important in CSS (ceiling 3)
grep -rn '!important' internal/ui/static/css/*.css | wc -l
```

**Capture a baseline of every gate before your first edit.** Only regressions against that
baseline are yours to fix.

### Database

Live Postgres (read-only for you):

```
postgres://postgres:<REDACTED-ROTATE-THIS-PASSWORD>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require
```

`psql` is not installed. Use the existing probe:

```bash
export PROBE_DSN="postgres://postgres:<REDACTED-ROTATE-THIS-PASSWORD>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require"
go run ./tmp/probe "select count(*) from catalog.product_variants"
```

If `tmp/probe/main.go` is missing, recreate it (~45 lines: connect with `pgx/v5`, run
`strings.Join(os.Args[1:], " ")`, print tab-separated rows). `tmp/` is git-ignored.

**Rules:** READ freely. **Never run ad-hoc `UPDATE` / `DELETE` / `ALTER` against the live
database.** Every schema or data change ships as a numbered migration in `db/migrations/`
(`NNN_name.up.sql` + `NNN_name.down.sql`) applied with `go run ./cmd/cli migrate`. Migrations
197–201 already exist; check the next free number with
`ls db/migrations | sort -V | tail -3` before numbering, because other agents may have added files.

### Live-data facts (so you don't "fix" correct data)

19,996 master products; 12,501 vendor variants of which 3,462 are `status='active'`;
~3,660 active variants with stock across **only two suppliers** (org 187 → 142, org 192 → 1,969);
7 organizations; 14 branches; 805 weekly-coverage rows; 12 orders. Only ~851 master products
have any sellable stock at all.

### Concurrency hazard

GitHub Desktop, a `templ --watch` process, and other agent sessions may mutate this working
tree while you run. Before each work order run `git status --porcelain` and `git stash list`.
If files you did not touch have changed, stop and re-read them. **Never** `git checkout --`
or `git reset --hard` to "clean up".

### Commits

One commit per work order, never bundled. Format:

```
<area>: <what changed>

<why, in prose. name the defect number.>

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
```

### Definition of done (per work order — all must hold)

1. The work order's own acceptance test executed and passing.
2. `templ generate` run if any `.templ` changed, and `*_templ.go` committed alongside.
3. `go build ./...` passes. 4. `go vet ./...` passes. 5. `go test -short -count=1 ./...` passes.
6. New behaviour has a test (table-driven unit tests for rules; integration tests against real
   Postgres for new repository queries).
7. No new file exceeds 400 lines. 8. No new hardcoded Arabic in `.go`.
9. One commit naming the defect number and saying *why*.
10. If module invariants changed, update `docs/modules/<module>.md`.

For any UI work order also screenshot at **1440×900** and **390×844** in Arabic (RTL) and
confirm: no horizontal scroll at either width; modals vertically centred on desktop and
closing on Escape; filters survive an action; pagination counts match rendered rows; no
undefined CSS class rendering as an unstyled element.

---

## 4. EXACT CURRENT STATE — start here

Latest commit: `dede0b77 docs: mark WO-22, WO-24 and WO-25 complete`.
`go build ./...` currently **passes** (exit 0).

`git status` shows exactly two **untracked, uncommitted** files — this is the half-finished
WO-23 (product sponsorship), which is where the previous session stopped:

```
?? internal/modules/promo/admin_sponsorship_rows.go          (168 lines)
?? internal/modules/promo/postgres/admin_sponsorship_rows.go (272 lines)
```

**Read both files before doing anything.** They already contain, and you must NOT rewrite
them from scratch:

- `promo.AdminSponsorshipFilter` (+ `Normalize()`), with fields `Tab, Search,
  OrganizationID, PackageID, TierLevel, StartsFrom, StartsTo, Limit, Offset`. Tabs are
  `all|active|pending|expired|rejected`; default limit 25, max 200.
- `promo.AdminSponsorshipRow` — one listing/detail row (org, product, package, tier, credits
  used vs total, amount, admin status, status, admin notes, reviewed by/at, starts/expires,
  created, impressions, clicks) with helpers `Expired(now)`, `Live(now)`, `ClickRate()`.
- `promo.AdminSponsorshipCounts` — `Total/Active/Pending/Expired/Rejected` tab badges.
- `promo.AdminSponsorshipBackend` — the optional repository interface, plus three
  `*Service` methods (`ListAdminSponsorshipRows`, `AdminSponsorshipCounts`,
  `GetAdminSponsorshipRow`) that type-assert `s.repo` to it and degrade to empty results
  when the assertion fails.
- The Postgres implementation of all three: one joined statement over
  `promo.sponsorship_requests` LEFT JOINed to `org.organizations`, `catalog.products`,
  `promo.offer_packages`, `promo.sponsorship_purchases`, with impressions/clicks from
  `promo.ad_impressions` / `promo.offer_clicks` via LATERAL subqueries, non-overlapping tab
  predicates (rejected → expired → pending → active, in that order so badges sum), and counts
  done in one statement with four `FILTER` clauses.

---

## 5. WORK ITEM 1 (do first) — finish WO-23

**Plan section: lines 1820–1846.** Read it verbatim.

Existing surfaces to change:

| File | Size |
|---|---|
| `internal/ui/admin_adv_products_handlers.go` | 369 lines |
| `internal/ui/pages/admin_adv_products.templ` | 708 lines |
| `internal/ui/admin_adv_products_test.go` | 103 lines |

Existing routes (registered in `internal/ui/admin_routes_catalog.go:42-45`):

```
GET  /admin/adv-products                  h.AdminAdvProductsPage
POST /admin/adv-products/{id}/approve     h.AdminAdvProductApproveSubmit
POST /admin/adv-products/{id}/reject      h.AdminAdvProductRejectSubmit
POST /admin/adv-products/new              h.AdminAdvProductCreateSubmit
```

Sidebar entry: `internal/platform/rbac/nav_admin.go:153` (`adv_products`).

Tables: `promo.sponsorship_requests`, `promo.sponsorship_purchases`,
`promo.offer_sponsorships`, `promo.sponsorship_credit_entries`, `promo.offer_packages`.

**What remains to be built:**

1. **Rewire the page handler** to call `Service.ListAdminSponsorshipRows` /
   `AdminSponsorshipCounts` instead of the current code, which loads 500 requests in one go
   and then resolves the product, organisation and package **one id at a time in a Go loop**.
   Delete that loop.
2. **General filters** parsed from the query string into `AdminSponsorshipFilter`:
   organization, product/search, package tier, status, admin status, date from/to.
3. **A proper `data-table` listing**: product (image + Arabic name + SKU), sponsoring
   organization, package and tier, credits consumed vs total, active window, status,
   impressions, clicks.
4. **A new detail page — `GET /admin/adv-products/{id}`** — register the route in
   `admin_routes_catalog.go` next to the others. It shows the request, the purchase its
   credits draw from, the credit entries (`promo.sponsorship_credit_entries`), the impression
   and click history (`promo.ad_impressions`, `promo.offer_clicks`), and the moderation trail.
   Use `Service.GetAdminSponsorshipRow` for the header; add repository methods for the credit
   entries and the impression/click history in the same style as the existing file (one
   statement, `InReadTx`, `database.AsSystem`).
5. **`B2BPagination`** with the filters carried in `QueryValues` so paging preserves them.
6. **Bind every figure to real data.** Before trusting the template, verify each number with
   the probe against the live database. **Any number that cannot be sourced from a table must
   be removed from the page, not rendered as a zero.** The client's complaint is precisely
   that this page is not connected to its data.
7. Extend `internal/ui/admin_adv_products_test.go` to cover the new filter parsing and the
   detail handler.

**Acceptance:** every number on the list and on the detail page reconciles with a SQL query
you can paste into the probe. Keep new files under 400 lines; if the handler file would grow
past 400, split the detail page into `internal/ui/admin_adv_product_detail_handlers.go`.

---

## 6. WORK ITEM 2 — the repair list (R1, R3, R4, R5)

These are recorded in plan §8.3/§8.5 (lines 3153–3222). **R2 is already DONE — skip it.**

### R1 — Move new hardcoded Arabic in Go into i18n keys

The `check-hardcoded-arabic` ceiling is 0. These seven files added user-facing Arabic string
literals to `.go` sources; move each into `internal/shared/i18n` keys and call
`i18n.T(lang, key)`:

```
internal/modules/commerce/availability_batch.go
internal/modules/platform_admin/postgres/audit_repo.go
internal/ui/admin_decision_memory_handlers.go
internal/ui/admin_employee_activities_handlers.go
internal/ui/customer_order_edit_handlers.go
internal/ui/customer_order_review_handlers.go
internal/modules/billing/subscription_service.go   (the subscription cooldown messages)
```

Scope R1 to **these seven files only**. The tree-wide count is ~179 occurrences, mostly
pre-existing; do not attempt the whole cleanup.

### R3 — `check-undefined-classes` is red at 94 against a ceiling of 64

Six invented classes were already replaced (`btn-outline-primary`→`btn-outline-brand`,
`btn-outline-emerald`→`btn-outline-success`, `btn-outline-sky`→`btn-outline-secondary`,
`badge-subtle`→`badge-slate`, `alert-neutral`→`alert-info`, `match-decisions-page` removed).
Go through the remaining reported names one at a time and decide, per name: it is a genuine
undefined class (fix it by using a class the stylesheet actually defines), or it is an Alpine
state name inside a `:class` binding (which the gate's own comment allows for). Then either
clear the genuine misses or raise the ceiling **with a written argument in the Makefile/gate
comment** listing why each remaining occurrence is legitimate.

### R4 — `gofmt` (low priority)

Run `gofmt -w ./cmd ./internal`. 134 files are unformatted. **Do NOT attempt to split the 58
oversized files** — that is explicitly out of scope. Commit the gofmt pass on its own.

### R5 — Regression test for branch institutional works

`saveBranchInstitutionalWorksTx` deletes then re-inserts a branch's institutional works. This
is correct **only because** all three `UpdateBranch` callers submit `institutional_works` and
all three forms render the field. Write a regression test asserting that editing a branch
**without** touching its works does not clear them.

---

## 7. WORK ITEM 3 — the remaining Level-2 work orders

Do these in this order. For each, **read the plan lines listed and follow them literally.**

### WO-27 — One deletion-requests screen (plan lines 1956–1999)

Two tables exist today: `org.organization_deletion_requests` (screen at
`/admin/organizations/deletion-requests`, handler `internal/ui/admin_org_deletion_handlers.go`,
125 lines) and `identity.account_deletion_requests` (approve/reject actions exist at
`POST /admin/users/deletion/{id}/approve|reject` but **no screen lists them**).

Build `/admin/deletion-requests` as one page with tabs المنشآت / المستخدمين, each with its own
filters (status, organization, requester, date from/to), its own `B2BPagination`, and row
counts on the tabs. 301-redirect the old URL to it and replace the sidebar entry in
`internal/platform/rbac/nav_admin.go`. Standard table and modal vocabulary. **No emoji.**

**The single most important requirement: deletion is a block, never a delete.** Approving a
request must make the account behave as if it does not exist while its rows remain in the
database — terminal `status = 'deleted'` (do **not** add a second column meaning the same as
`deleted_at`); gone from every listing, search, filter and dropdown across admin, vendor,
pharmacy and public surfaces; its products and offers gone from `/catalog`, `/suppliers`,
smart ordering and the compare tool; its users unable to sign in with existing sessions
revoked; `CheckAvailability` refuses it (verify the existing `vendor_unapproved` path covers
this once the status changes); nothing physically removed. Write the change through **one**
service method so no surface can forget a step, and record an audit row.

**Acceptance:** approve a deletion for a test supplier, then walk `/catalog`, `/suppliers`,
`/admin/organizations`, `/admin/orders`, `/customer/smart-order/new` and the compare tool and
confirm it is absent everywhere, while
`SELECT count(*) FROM org.organizations WHERE id = <id>` still returns 1.

### WO-28 — `/admin/finance` (plan lines 2001–2042)

Files: `internal/ui/admin_finance_handlers.go` (377 lines),
`internal/ui/admin_finance_withdrawals_handlers.go`, `internal/ui/pages/admin_finance.templ`,
`internal/ui/pages/admin_finance_subpages.templ`, `internal/ui/admin_finance_test.go`.

1. Tab order exactly: (1) محافظ المنشآت, (2) سجل حركات المحافظ, (3) everything else remaining.
2. **Delete the "تقرير الأرباح والعمولات" tab entirely** — handler, template, route and
   sidebar entry. Verify with
   `grep -rn "profit\|commission" internal/ui/pages/admin_finance*.templ`.
3. De-duplicate the tabs: list every tab, name the question it answers, remove any that
   answers a question another tab already answers.
4. Reduce prose — headings and numbers, not paragraphs ("خليها أهدأ من ناحية كثرة الكلام").
5. **Verify every displayed figure against the database.** For each stat card, put the SQL in
   a code comment next to the handler that computes it and check it with the probe. Tables:
   `billing.wallets` (12), `billing.wallet_transactions` (16), `billing.wallet_deposits` (5),
   `billing.wallet_withdrawals` (2), `billing.invoices`, `billing.invoice_lines` (653),
   `billing.payments`. Any card whose number cannot be derived must be removed.
6. New **"طباعة كشف الحساب"** on the wallet-movements tab: prints the *currently filtered*
   table, taking the organization name from the active organization filter. Reuse the existing
   print stack — `internal/ui/invoice_handlers.go`,
   `internal/ui/static/css/invoice_printable.css`, and
   `internal/ui/pages/credit_statement.templ` (`CreditStatementPage`). The statement carries
   the Dawa24 logo, the organization's logo, the period, opening and closing balance, every
   movement with date, type, reference, debit, credit and running balance, and a total row.
   No emoji, print-safe CSS, correct RTL. **If no organization filter is set, disable the
   button** rather than printing a mixed statement.

### WO-29 — `/admin/jobs` (plan lines 2044–2073)

Files: `internal/ui/jobs_handlers.go`, `internal/ui/job_form_handlers.go`,
`internal/ui/pages/admin_jobs.templ`, `internal/modules/hr/`. Tables: `hr.job_offers`,
`hr.job_applications`, `hr.job_categories`, `hr.job_seeker_profiles`, `hr.work_times`.

Today only `GET /admin/jobs` exists. The vendor side already has the full set in
`internal/ui/vendor_job_handlers.go` — **copy that implementation**. Add to
`admin_routes_platform.go`:

```
GET  /admin/jobs/{id}                                  detail with applications
POST /admin/jobs/new
POST /admin/jobs/{id}/edit                             modal forms
POST /admin/jobs/{id}/toggle                           publish / unpublish
POST /admin/jobs/{id}/delete
POST /admin/jobs/{id}/applications/{appId}/accept
POST /admin/jobs/{id}/applications/{appId}/reject
```

Plus filters (organization, category, status, governorate/city, date from/to), search,
`B2BPagination`, and an audit row per write. All writes run under `database.AsSystem` scoped
to the owning organization.

**Acceptance:** an admin can create, edit, publish, unpublish and delete any vacancy and act
on its applications without impersonating the vendor.

---

## 8. WORK ITEM 4 — the Level-3 systems (WO-31 … WO-42)

These are larger and each has a full specification in the plan. **There is no WO-30** — the
client's own list skips 30, and the numbering follows their labels deliberately.

Suggested order (plan Appendix B, lines 2959–2976): **WO-35 first**, because WO-27, WO-33 and
WO-37 all notify through it. Then WO-15-adjacent lifecycle work, then WO-40 before WO-36
(they share one import mechanism, built once), then the configuration surfaces.

| WO | Plan lines | Subject | Notes / dependencies |
|---|---|---|---|
| WO-31 | 2075–2145 | Editable default roles | migration 201 |
| WO-32 | 2147–2169 | `/admin/plans`: more than four cards without breaking the layout | — |
| WO-33 | 2171–2207 | `/admin/users`: full user administration | depends on WO-17 (done), WO-35 |
| WO-34 | 2209–2214 | `/vendor/quotas`: full review | depends on A7 |
| WO-35 | 2216–2287 | Notifications for every event | **do this first of the Level-3 set** |
| WO-36 | 2289–2329 | `/admin/organizations/import`: use the real import tool | uses WO-40's mechanism |
| WO-37 | 2331–2364 | Refunds from the admin panel | migration 202, WO-35 |
| WO-38 | 2366–2381 | Smart ordering bound to coverage and the branch | depends on A5, A8 |
| WO-39 | 2383–2457 | Per-tool AI model configuration | migration 203; respect rule 2 |
| WO-40 | 2459–2513 | Resumable warehouse-upload sessions, on a page | build the mechanism once |
| WO-41 | 2515–2561 | SEO, keywords and AI-discoverability tab | — |
| WO-42 | 2563–2663 | Make the AI Assistant genuinely capable | largest; respect rules 2 and 3 |

For each: read the plan lines, implement exactly what is written, satisfy the work order's own
acceptance criterion, and commit separately.

Also available in parallel: **PART 5 — database and architecture hygiene**, plan lines
2665–2760. Note especially §5.1: deletion state is currently expressed two ways (`deleted_at`
on most tables, a `status` string on organizations/branches/users). `deleted_at` = soft-deleted
row; `status` = lifecycle state; a blocked account is a `status`, not a `deleted_at`. **WO-27
must not add a third way.**

---

## 9. Open questions — do NOT guess

**PART 7 (plan lines 2868–2910)** lists decisions the client still owes. Where one blocks you,
implement the stated default, mark it in the code with a clearly greppable comment, and report
it. In particular **Q6 — blocked-account semantics** governs WO-27: read it before starting
that work order.

---

## 10. Re-run this after WO-23

Plan §6.4 (lines 2818–2860) is an end-to-end scenario over the unified availability path using
org **192** (vendor, branches 76/81) and org **188** (customer, branches 69/73). **Its earlier
result was invalidated by defect D1 and it must be re-run.** Follow all ten steps as written
and record the result of every one.

---

## 11. Reporting

At the end of each work order, and again at the end of each workstream, produce a short report:
what was changed, what was verified (with the actual command output), what was **not** done and
why, and the state of every gate against the baseline you captured. State failures plainly with
their output. **Do not describe a work order as complete if any part of it was skipped — say
which part and why.**
