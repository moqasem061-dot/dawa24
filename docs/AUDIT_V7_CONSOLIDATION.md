# Consolidation Audit — the platform as one product

**Date:** 2026-08-20 · **Tree:** `52e8919`
**Scope:** every duplicate system, every disconnected feature, every source of
structural randomness. Written from mechanical scans, not from commit messages.

---

## PART 0 — The numbers

| | Count | Comment |
|---|---:|---|
| Routes | **447** | Laravel, the reference, has ~350 across 5 route files |
| Pages | **97** | Laravel has 353 Livewire components but ~180 real screens |
| Tables | **161** | Laravel has 148, of which ~19 are framework/views |
| Pages rendering zero data | **31** | down from 44, still a third of the admin surface |
| Inline `style="` attributes | **4,938** | this is why the design looks random |
| Duplicate systems found | **9** | detailed below |
| Dead files | see PART 4 | |

The platform has more routes than Laravel while implementing less of it. That is
the whole problem in one line: **growth without consolidation.**

---

## PART 1 — Your reported issues, verified

### 1.1 🔴 Weekly coverage error — ROOT CAUSE FOUND (two bugs, same columns)

The message *"تعذر تحميل جدول التغطية"* is the error state rendering correctly.
The handler is fine. The repository is not.

**`workflow.weekly_coverages`** (migration `010`):
```sql
coverage_from   TIME,     -- nullable
coverage_to     TIME,     -- nullable
```

**`workflow.WeeklyCoverage`** (`domain.go:41`):
```go
CoverageFrom   string    // not a pointer, not a time type
CoverageTo     string
```

**Bug A — the read fails.** `ListCoverageForOrganization`
([repository.go:222](internal/modules/workflow/postgres/repository.go:222)) selects
`wc.coverage_from, wc.coverage_to` and scans them into `*string`. pgx maps
Postgres `TIME` (OID 1083) to `pgtype.Time`, not to `string`. Every row fails to
scan, so the whole list errors — which is exactly the screen you saw.

**Bug B — the write fails too.** `SaveWeeklyCoverage`
([repository.go:91](internal/modules/workflow/postgres/repository.go:91)) passes
`c.CoverageFrom` (a Go `string`) into the `TIME` column. When the form leaves the
time blank, Postgres rejects it: `invalid input syntax for type time: ""`.

**Consequence:** a vendor cannot create coverage, and cannot view it. And because
`ListOffersVisibleTo` INNER JOINs `weekly_coverages`, **the customer offer
marketplace is still returning zero rows.** The Phase 0 fix I previously credited
was never exercised — its integration test skips locally without `DATABASE_URL`,
and in CI it seeds rows via raw SQL with valid times, bypassing both bugs.

**Fix:** `CoverageFrom`/`CoverageTo` become `*string` scanned through
`pgtype.Time` (or the columns become `TEXT` if a `HH:MM` string is what the
domain actually wants — decide once, in the domain). Then a round-trip test
through the **HTTP handler** with a blank time and a filled time.

### 1.2 🔴 Two complete settings systems

| | Route(s) | Template | Screens |
|---|---|---|---|
| **OLD** (you prefer this) | `/settings?tab=…` | `settings_unified.templ` — one tabbed page | 1 |
| **NEW** (duplicate) | `/settings/profile`, `/addresses`, `/security`, `/organization`, `/preferences`, `/payment-methods` | `settings.templ` + its own `settingsTabs` component | 6 |

Seven screens doing one screen's job, with **two different tab implementations**.
`SettingsPaymentMethods(lang, dir)` takes no data at all — it is one of the 31
data-less pages.

The customer sidebar points at the old one (`/settings?tab=wallet`); the vendor
sidebar points at the new one (`/settings/employees`). So the two systems are
both live and reachable, from different places.

### 1.3 🔴 Duplicate branch management — confirmed, three creation paths

```
POST /customer/branches/new          → CustomerBranchNewSubmit    (the real one)
POST /vendor/branches/new            → VendorBranchNewSubmit      (the real one)
POST /settings/organization/branch   → SettingsBranchCreateSubmit (the basic duplicate)
```

`SettingsBranchCreateSubmit` even **invents a branch code** when the form omits
one: `"BR-" + time.Now().UnixNano()%100000`. That is a second, lower-quality
write path into `org.branches` producing rows the real branch screens did not
create and cannot explain.

`SettingsOrganization(lang, dir, o, branches, members)` renders that duplicate
branch list. This is the "extremely basic, poorly designed system" you found
under Organization Data and Membership.

### 1.4 🔴 The cart has no real availability logic

[`AddToCartSubmit`](internal/ui/customer_handlers.go:297):

```go
vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
if vendorOrgID <= 0 {
	vendorOrgID = 1          // ← hardcoded fallback to organization #1
}

if h.catSvc != nil && variantID > 0 {
	if variant, err := h.catSvc.GetVariant(ctx, variantID); err == nil && variant != nil {
		if variant.StockQty > 0 && qty > variant.StockQty {
			qty = variant.StockQty   // ← silently clamps instead of refusing
		}
	}
}
```

Five defects in eleven lines:

1. **`vendorOrgID = 1`** — a missing vendor field silently attributes the item to
   whichever organization has id 1.
2. **Out-of-stock passes.** `StockQty > 0` guards the check, so a variant with
   `StockQty == 0` skips validation entirely and goes in the cart.
3. **Silent clamping.** Asking for 100 of a 3-stock item gives you 3 with no
   message. The user believes they ordered 100.
4. **The error is swallowed** (`err == nil`) — if `GetVariant` fails, *no* stock
   check runs at all.
5. **No coverage check, no branch check, no supplier-eligibility check.** Nothing
   verifies the vendor delivers to the customer's selected branch on any day.

There is also no server-side validation on quantity `+`/`−`, on the product page,
or at checkout. `CheckoutSubmit` does not re-validate stock at order time, so two
customers can buy the same last unit.

### 1.5 🔴 Categories: one table presented as two systems

`catalog.categories` has a **self-referencing `parent_id`**. Root rows
(`parent_id IS NULL`) are "main categories"; children are sub-categories. The
admin surfaces these as if they were separate systems, which is why "Main
Categories" looks like it belongs to nothing.

Worse, for what you actually want:

```sql
CREATE TABLE catalog.brands (
    id, public_id, name, description, image, status, ...
    -- no category_id
);
```

**There is no category→brand relationship in the schema at all.** The
"select a category, then see only its brands" behaviour you described cannot be
built without a migration. `catalog.products` does carry both `category_id` and
`brand_id`, but nothing constrains them to agree.

### 1.6 🔴 Translations: a write-only store

`platform_admin.translations` is read and written by **exactly one place** — the
admin translations screen. No template, no handler, no middleware consumes it.
Every user-facing string is an inline `if lang == "ar"` in the templates.

An administrator can spend an hour editing translations that will never appear
anywhere. You are right to remove it.

### 1.7 🔴 Policies: two editors, one table

| | Screen | Write path |
|---|---|---|
| A | `/admin/settings` tab 3 — inline editor | `POST /admin/settings/policy` |
| B | `/admin/policies` — standalone page | `POST /admin/policies`, `/{id}/publish` |

Both write `platform_admin.policies`. Screen A also embeds a large hardcoded
Arabic default policy text
([admin_settings.templ:150](internal/ui/pages/admin_settings.templ:150)), so the
two screens can disagree about what the policy says. You want B's interface
inside A's tab — that is the right call, and it removes a route.

### 1.8 🟠 Pharmacy dashboard vs cPanel

`/customer/dashboard` and `/customer/cpanel` both exist. Laravel has both too
(`Customer/NewDashboard` and `Customer/Cpanel`), so this is not invented — but in
Go the cPanel is a link hub to pages already in the sidebar, which makes it pure
navigation duplication. **Recommendation: delete it.** Laravel parity is not a
reason to keep a screen whose only function is to link to other screens the
sidebar already exposes.

---

## PART 2 — Duplicates you did not name

| # | Cluster | Routes | Reality |
|---|---|---|---|
| 1 | **User lists** | `/admin/users`, `/full-user`, `/customer-list`, `/vendor-list`, `/admin-list`, `/admins` | six screens, one table |
| 2 | **Organizations** | `/admin/organizations`, `/vendors`, `/suppliers` | three routes → one handler |
| 3 | **Sponsorships** | `/admin/offer-sponsorships`, `/admin/offers-packages/sponsorships` | same screen, two paths |
| 4 | **Saving products** | `/admin/saveing-products`, `/admin/saving-products` | duplicate route, not a redirect |
| 5 | **Trash** | `/admin/deletes-lists/*` (3), `/admin/trash-list/*` (3) | six routes, one purpose |
| 6 | **Analytics** | `/admin/analytics`, `/offers-packages/views`, `/offers-packages/clicks` | overlapping, none connected |
| 7 | **Catalog** | `/admin/products`, `/product-child`, `/adv-products`, `/apis-products` | four catalog screens, two of them data-less |
| 8 | **Settings** | §1.2 | 7 screens → 1 |
| 9 | **Branches** | §1.3 | 3 write paths → 2 |

---

## PART 3 — Why the design looks random

**4,938 inline `style="` attributes** across 97 templates, against two CSS files
(`app.css`, `components.css`).

Consequences you can see:
- every page invents its own spacing, radius, and colour values
- `settings_unified.templ` alone has **245** inline styles; `admin_developers.templ` 230; `admin_settings.templ` 215
- the component library (34 components) exists but is bypassed — new pages
  hand-roll cards and tables instead of using `DataTable`, `EmptyState`, `StatCard`
- the footer is defined once in `customer.templ` with no height constraint, and
  the shell renders it on authenticated pages where it wastes vertical space

There is no design-token discipline, so "fix the spacing on this page" never
generalises. This is a systemic fix, not a per-page one.

---

## PART 4 — Dead weight

| Item | Evidence |
|---|---|
| ~~`internal/ui/layouts/pharmacy.templ`~~ | **Retracted.** It defines `CustomerShell`, which `shell_for.templ` dispatches to. The file name is misleading, not the file dead. |
| 31 data-less pages | `grep -rhoE 'pages\.[A-Za-z0-9_]+\(lang, dir\)'` |
| `platform_admin.translations` | written by one screen, read by nothing |
| 4 catalog screens | two of them (`adv-products`, `apis-products`) render nothing |
| `/admin/notifications` in the sidebar | dead target — it 404s |

---

## PART 5 — What is genuinely right (do not touch)

- `authctx/audience.go` — 404-not-403 audience gating
- `layouts/shell_for.templ` — the shared-page shell dispatcher
- `promo/postgres/visibility.go` — the single canonical coverage query
- `admin_routes_*.go` — `RequirePagePermission` groups
- `test/admin_guard_test.go`, `test/institutional_guard_test.go`
- `cmd/migratecheck`
- `internal/shared/money`
- `internal/ui/components/` — 34 real components; the problem is that pages ignore them

---

## PART 6 — The shape of the fix

Not "build more". In order:

1. **Fix coverage** (§1.1) — the marketplace is dead until this works end to end.
2. **Make the cart real** (§1.4) — stock, coverage, branch eligibility, everywhere.
3. **Merge the duplicates** (§1.2, 1.3, 1.7, PART 2) — nine clusters → nine single screens.
4. **Delete** (§1.6, PART 4) — translations, cPanel, dead layouts, data-less pages that no longer have a purpose.
5. **Fix product classification** (§1.5) — one category tree, a real category→brand relationship.
6. **Restore design discipline** (PART 3) — tokens, components, and a cap on inline styles.

Execution plan: `docs/PLAN_V7/`.

**Target metrics — every one must go down:**

| | Now | Target |
|---|---:|---:|
| Routes | 447 | ≤ 330 |
| Pages | 97 | ≤ 75 |
| Tables | 161 | ≤ 140 |
| Data-less pages | 31 | 0 |
| Inline styles | 4,938 | ≤ 1,000 |
| Duplicate clusters | 9 | 0 |
