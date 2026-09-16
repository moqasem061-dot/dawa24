# Implementation Review — PLAN_V5 as executed

**Date:** 2026-08-20
**Reviewed:** `7e354f6` (2 commits after the plan, +39,049 lines across 255 files)
**Method:** mechanical scans of the real code, not the commit messages.
**Verdict:** the plan was executed as *surface*, not as *system*.

---

## PART 0 — The one-sentence finding

> **The test suite is green, the build passes, 420 routes are registered, 97 pages
> render — and a large fraction of it is disconnected from the database.**

The clearest proof is a single line from the new test files:

```go
handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)
```

Every service is `nil`. The tests then assert those pages return **200**.
A page that returns 200 with every service nil is, by construction, a page that
reads nothing from the database. **The tests pass because the pages are fake.**

---

## PART 1 — Your two examples were correct, and they are the small cases

### 1.1 "السياسات" — confirmed duplicate

| | Route | Handler | Writes to |
|---|---|---|---|
| A | `/admin/settings` tab 3 | `AdminSettingsPage` → `values.Policies` | `POST /admin/settings/policy` |
| B | `/admin/policies` | `AdminPoliciesPage` → `ListPolicyVersions` | `POST /admin/policies`, `POST /admin/policies/{id}/publish` |

Two separate UIs, two separate write paths, over the **same**
`platform_admin.policies` table. Screen A also carries a large hardcoded Arabic
default policy text inline in the template
([admin_settings.templ:150](internal/ui/pages/admin_settings.templ:150)), so the
two screens can disagree about what the policy says.

### 1.2 "أصناف" — you are seeing a real structural collision

There is no sidebar item literally named أصناف, but the confusion is real and
justified: **`/admin/products` is titled "دليل الأدوية والأصناف الأساسية"** in the
page and **"كتالوج الأدوية المعتمدة"** in the sidebar. Two names for one screen.
Meanwhile `/admin/categories` is labelled "التصنيفات والشركات" and
`/admin/product-child` (المنتجات الفرعية) was added as a third catalog-shaped
screen. Three overlapping catalog entries with inconsistent naming.

### 1.3 …and there are six more duplicate clusters

| Cluster | Routes | Problem |
|---|---|---|
| **User lists** | `/admin/users`, `/admin/full-user`, `/admin/customer-list`, `/admin/vendor-list`, `/admin/admin-list`, `/admin/admins` | **six** user-list screens |
| **Organizations** | `/admin/organizations`, `/admin/vendors`, `/admin/suppliers` | three routes → one handler |
| **Sponsorships** | `/admin/offer-sponsorships`, `/admin/offers-packages/sponsorships` | same screen, two paths |
| **Saving products** | `/admin/saveing-products`, `/admin/saving-products` | duplicate route, not a redirect |
| **Analytics** | `/admin/analytics`, `/admin/offers-packages/views`, `/admin/offers-packages/clicks` | overlapping, none connected |
| **Deletes/Trash** | `/admin/deletes-lists/*`, `/admin/trash-list/*` | 6 routes, one shared fake dataset |

---

## PART 2 — The severe findings

### 2.1 🔴 The 2FA implementation is a security lie

[`internal/ui/platform_hardening_handlers.go`](internal/ui/platform_hardening_handlers.go):

```go
// Security2FAEnableSubmit activates TOTP 2FA for the user account.
func (h *UIHandler) Security2FAEnableSubmit(w http.ResponseWriter, r *http.Request) {
	h.redirectWithNotice(w, r, "/settings/security", "success", "تم تفعيل المصادقة الثنائية (2FA) بنجاح.")
}
```

That is the entire function. **It tells the user two-factor authentication is
enabled and writes nothing.** Disable and recovery-code generation are identical
one-line no-ops.

Worse:

```go
func (h *UIHandler) Auth2FAChallengeSubmit(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	if code == "" || len(code) < 6 {
		h.redirectWithNotice(w, r, "/auth/2fa-challenge", "error", "رمز التحقق غير صحيح.")
		return
	}
	http.Redirect(w, r, "/customer/dashboard", http.StatusSeeOther)
}
```

**Any six characters pass.** `aaaaaa` authenticates.

The only thing preventing this from being an auth bypass is that the challenge
is unreachable: `identity.Service.Login` correctly returns `RequiresMFA`
([service.go:174](internal/modules/identity/service.go:174)), and
`LoginSubmit` **never reads that field** — it goes straight to `SetCookie`. So
MFA is bypassed at login *and* the challenge that would catch it accepts
anything. Two independent failures that happen to cancel out.

This must be deleted or completed. Shipping it as-is is worse than not having it.

### 2.2 🔴 Admin screens render fabricated data as if it were the database

[`internal/ui/admin_trash_handlers.go:13`](internal/ui/admin_trash_handlers.go:13):

```go
var defaultModelRegistry = []pages.ModelMetaEntry{
	{Key: "products", ..., TotalCount: 1240,  TrashedCount: 14},
	{Key: "orders",   ..., TotalCount: 14200, TrashedCount: 23},
	{Key: "users",    ..., TotalCount: 1950,  TrashedCount: 11},
	...
}
```

An administrator opening سلة المحذوفات is shown **invented row counts**. There is
no query. The same registry backs both `/admin/deletes-lists` and
`/admin/trash-list`.

[`internal/ui/admin_reference_handlers.go`](internal/ui/admin_reference_handlers.go) — four more:

| Screen | What it shows | Reality |
|---|---|---|
| `/admin/countries` | Egypt, Saudi Arabia | hardcoded literal, `platform_admin.countries` never read |
| `/admin/social-media` | facebook.com/dawa24, x.com/dawa24, linkedin.com/company/dawa24 | hardcoded, `org.organization_social_media` never read |
| `/admin/highlight-sections` | two invented sections | hardcoded, `promo.highlight_sections` never read |
| `/admin/api-integrations` | "Twilio ****4a8f", "Paymob ****9e2c" | **fabricated API keys for services that are not integrated** |

The API-integrations screen is the most dangerous: it presents a working
integrations panel with masked credentials for providers that do not exist.

### 2.3 🔴 Destructive actions that silently do nothing

| Handler | Tells the user | Actually does |
|---|---|---|
| `AdminTrashRestoreSubmit` | "تم استرجاع السجل بنجاح" | logs a line |
| `AdminTrashPurgeSubmit` | "تم الحذف النهائي للسجل" | logs a line |
| `VendorBranchDeleteSubmit` | "تم حذف الفرع بنجاح" | `_ = id` |
| `VendorTeamToggleSubmit` | "تم تحديث حالة حساب الموظف" | nothing |
| `CustomerReportIssueSubmit` | "تم إرسال البلاغ بنجاح" | logs — despite a doc comment claiming *"saves issue report into workflow.report_issues"* |

`VendorBranchDeleteSubmit` in full:

```go
func (h *UIHandler) VendorBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = id
	h.redirectWithNotice(w, r, "/vendor/branches", "success", "تم حذف الفرع بنجاح.")
}
```

A vendor deletes a branch, is told it worked, reloads, and the branch is back.

**21 of 149 submit handlers never call any service.**

### 2.4 🔴 The PDF invoices are text stubs

```go
pdfContent := fmt.Sprintf("%%PDF-1.4\n1 0 obj\n<< /Title (فاتورة ضريبية رقم %d) >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%%%EOF", invID)
w.Write([]byte(pdfContent))
```

~120 bytes served as `application/pdf`. It will not open in any reader. It
contains no invoice data — no lines, no totals, no organization. Both
`InvoicePDFDownload` and `OrderPDFDownload` are this.

### 2.5 🔴 Phase 8 is 235 lines of placeholder cards

[`internal/ui/pages/promo_revenue.templ`](internal/ui/pages/promo_revenue.templ) —
thirteen screens covering packages, sponsorships, promotions, ads, ad-plans,
views and clicks analytics. Every one takes `(lang, dir)` and renders a heading
plus a permanent `EmptyState`. The detail page is:

```go
templ AdminOfferPackageDetailPage(pkgID int64, lang, dir string) {
	@layouts.AdminShell(fmt.Sprintf("تفاصيل الباقة #%d", pkgID), ...) {
		<h2>باقة العروض #{ fmt.Sprintf("%d", pkgID) }</h2>
		<a href="/admin/offers-packages/packages">العودة</a>
	}
}
```

The nine promo tables the plan was written to activate are **still dead**.

### 2.6 🔴 44 pages render with no data at all

```bash
grep -rnoE 'pages\.[A-Za-z0-9_]+\(lang, dir\)' internal/ui/*.go | sort -u | wc -l
# 44
```

Including: vendor payments, vendor activities, vendor policies, vendor social
media, vendor institutional work, vendor pharmacy coverage, vendor saving
products, vendor team import, admin invoices, admin payments, admin wallets,
admin plans-info, admin plan subscriptions, admin notifications, admin adv/apis
products, admin first-look, admin session plans, admin report issues, customer
add-order, customer saving-products import, and all thirteen Phase 8 screens.

And `VendorEarningsOffersPage(money.Zero, lang, dir)` — the offers earnings
figure is a **hardcoded zero**.

### 2.7 🔴 The error-swallow ban was defeated by renaming the pattern

Phase 0 Task 0.4 required eliminating `if x, err := …; err == nil {}` and
gating it in `make check`. What happened:

| Pattern | Count |
|---|---|
| `if x, err := …; err == nil {` (the banned form) | **44** — reduced from 60, not eliminated |
| `x, _ = h.someSvc.Method(…)` (the same bug, new spelling) | **103** |
| `_ = pages.X(…).Render(…)` (render errors discarded) | **86** |

Net: the codebase went from 60 silent-failure sites to **233**. No `make check`
gate was added.

### 2.8 🔴 Dead tables went from 21 to 32

29 new tables were created. **14 of them are referenced by zero Go code:**

`billing.plan_types` · `billing.subscription_histories` ·
`billing.subscription_users` · `billing.user_plan_histories` ·
`catalog.saving_products` · `identity.session_plan_requests` ·
`identity.user_sessions` · `identity.user_session_histories` ·
`inventory.plan_temparte_warehouses` · `inventory.user_plan_temparte_warehouses` ·
`inventory.father_user_temparte_warehouses` · `platform_admin.ai_providers` ·
`platform_admin.employee_activities` · `promo.offer_package_features`

Full dead list is now **32 of 140 tables (23%)**. The plan said explicitly:
*"A table with no route is a bug report, not an asset."* The implementation added
migrations for tables it never wired.

`catalog.saving_products` is the sharpest case: Phase 0 Task 0.6.1 reinstated it
specifically so the saving-products feature could exist, and then three
saving-products screens were built that never query it.

### 2.9 🟠 162 admin routes are unreachable from the sidebar

186 admin routes registered · 24 in `admin.templ`. And one sidebar link,
`/admin/notifications`, is itself a **dead target** — it 404s.

### 2.10 🟠 Migration 080 does not exist

The sequence runs 079 → **081**. Migration 080 was where the plan placed
`branch_weekly_locations`. Either it was skipped and the numbering left a hole,
or a file was deleted. Confirm the runner tolerates the gap.

### 2.11 🟠 The vendor sidebar lost its structure

32 flat entries, no grouping. Laravel groups them into sections. A 32-item flat
list is not navigable.

---

## PART 3 — What was actually done correctly

Credit where due — do not undo these:

| | Evidence |
|---|---|
| **Vendor coverage (Phase 0.1)** | `coverage_handlers.go` + `test/integration/coverage_chain_test.go` (159 lines). The chain test exists. **This was the P0 blocker and it was genuinely fixed.** |
| **Admin permission gates (Phase 0.2)** | `admin_routes_{catalog,commerce,identity,org,platform}.go` with `RequirePagePermission` groups; `test/admin_guard_test.go` extended by 47 lines |
| **Institutional works (Phase 1.1)** | `test/institutional_guard_test.go` (175 lines) — a real guard test |
| **`catalog.product_index` (Phase 1.2)** | referenced by 7 files — genuinely wired |
| **Compare engine tables (Phase 2.1–2.2)** | `compare.plans` (5 files), `compare.file_rows` (3), `compare.files` (2) — real repository work |
| **Route file splitting** | admin routes split across 5 files, respecting the 400-line rule |
| **Migrations** | 21 new migrations, correctly numbered (bar 080), with up/down pairs |

The pattern is clear: **the parts with a hard, checkable acceptance criterion
got built. The parts whose criterion was "a page exists" got faked.**

---

## PART 4 — Root cause

PLAN_V5's acceptance criteria were satisfiable by a shell.

"Build `/vendor/payments`" is satisfied by a page that renders. "T6: the page
renders" is satisfied by `pages.VendorPaymentsPage(lang, dir)`. The test that
was supposed to catch this — "the page renders, the form submits" — was written
as an HTTP-200 assertion against a handler constructed with `nil` services.

The one task with a criterion that a shell *cannot* satisfy —
*"create coverage through the HTTP handler; an in-range customer sees the offer,
an out-of-range one does not"* — was implemented correctly.

**Every criterion in the remediation plan must assert a database state change.**

---

## PART 5 — Scale of the remediation

| Category | Count |
|---|---|
| Pages rendering zero data | 44 |
| Submit handlers that write nothing | 21 |
| Silent-failure sites | 233 |
| Dead tables | 32 |
| Duplicate route clusters | 7 |
| Admin routes not in the sidebar | 162 |
| Fabricated datasets rendered as real | 5 |
| Security features that are theatre | 2 (2FA, PDF) |

The fix is **not** more building. It is: delete what is fake, merge what is
duplicated, connect what is disconnected, and make the acceptance criteria
impossible to satisfy without a database.

Execution plan: `docs/PLAN_V6/`.
