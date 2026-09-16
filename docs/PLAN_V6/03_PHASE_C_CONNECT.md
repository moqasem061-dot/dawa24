# PHASE C — Connect: make the data real

**Depends on:** Phase A (test harness, lies removed), Phase B (surface consolidated).
**Principle:** every surviving screen completes the six-link chain
(`00_MASTER.md` §A.5) and proves it with a D1 test.

**Do not start any task here until Phase B has told you the screen survives.**
Connecting a screen that Phase B deletes is wasted work.

---

## C.0 The work unit

For **each** screen below, in this exact order:

1. **Verify it survived Phase B** — check `DECISIONS.md`.
2. **Read the Laravel component** — fields, filters, sort, page size, actions,
   Arabic labels, validation rules. Write them into `CHAIN_AUDIT.md`.
3. **Write the D1 test first** — seed a row, assert the page contains it. Run it.
   **Confirm it fails.** (It will: the page reads nothing.)
4. **Walk the chain from the bottom**: repository → interface → service → handler
   → template signature. Stop at the first link that already exists.
5. **Change the template signature** to take typed data. This is the step that is
   always skipped: `templ XPage(lang, dir string)` → `templ XPage(items []*T, lang, dir string)`.
6. **Run D1.** It must now pass.
7. **Add D2** (if the screen writes), **D3** (tenancy), **D4** (failure surfaces).
8. **Update `CHAIN_AUDIT.md`** with all six links filled.

A screen is not done until its row in `CHAIN_AUDIT.md` has six non-empty cells.

---

## C.1 The complete work list

### C.1.1 The 44 data-less pages

Regenerate the list before starting — Phase B will have removed some:

```bash
grep -rnoE 'pages\.[A-Za-z0-9_]+\(lang, dir\)' internal/ui/*.go | sort -u
```

Baseline at review time, grouped by task:

| Task | Screens | Table(s) to connect |
|---|---|---|
| **C.2** | vendor payments, vendor earnings/order, vendor earnings/offers | `billing.payments`, `billing.payment_histories`, `commerce.orders` |
| **C.3** | admin invoices, admin payments, admin wallets, admin plans-info, admin plans/subscriptions, plan-types, plan-features | `billing.*` incl. 4 dead tables |
| **C.4** | vendor activities, admin employee-activities | `platform_admin.employee_activities` (dead) |
| **C.5** | vendor policies, vendor social-media | `org.organization_policies`, `org.organization_social_media` (dead) |
| **C.6** | admin countries, social-media, highlight-sections, api-integrations | `platform_admin.countries`, `org.organization_social_media`, `promo.highlight_sections`, `platform_admin.api_integrations` (all dead) |
| **C.7** | trash-list, deletes-lists | `information_schema` — no new table |
| **C.8** | all 13 Phase 8 monetisation screens | 9 promo tables (all dead) |
| **C.9** | 2FA (rebuild) | `identity.user_mfa`, `identity.user_sessions` (dead) |
| **C.10** | invoice/order PDF (rebuild) | `billing.invoices`, `commerce.orders` |
| **C.11** | vendor saving-products ×3, customer saving-products ×3, admin saving-products ×4 | `catalog.saving_products` (dead) |
| **C.12** | vendor institutional work, vendor pharmacy-coverage | `org.employee_institutional_works`, `workflow.weekly_coverages` |
| **C.13** | vendor team import, team fast-add, customer add-order | `identity.users`, `org.members` |
| **C.14** | temp warehouses (all admin screens) | 3 dead `inventory.*` tables |
| **C.15** | session plans, session-plan requests, report-issues | `identity.session_plans`, `identity.session_plan_requests`, `workflow.report_issues` |
| **C.16** | admin notifications, first-look, adv/apis products | `notifications.admin_notifications` (dead), `catalog.products` |

### C.1.2 The 32 dead tables — the completion metric

```bash
# regenerate; this number must reach 0 (or each survivor has a written decision)
grep -ohiE "CREATE TABLE (IF NOT EXISTS )?[a-z_]+\.[a-z_0-9]+" db/migrations/*.up.sql \
  | sed -E "s/CREATE TABLE (IF NOT EXISTS )?//I" | sort -u \
  | while read t; do
      [ "$(grep -rl "$t" internal/ --include=*.go | wc -l)" -eq 0 ] && echo "DEAD: $t"
    done
```

Baseline (32):

`billing.payment_histories` · `billing.payment_integrations` · `billing.plan_types` ·
`billing.subscription_histories` · `billing.subscription_users` ·
`billing.user_plan_histories` · `catalog.product_infos` · `catalog.saving_products` ·
`identity.kyc_records` · `identity.session_plan_requests` · `identity.user_identities` ·
`identity.user_session_histories` · `identity.user_sessions` · `ingest.import_batches` ·
`ingest.import_progress` · `inventory.father_user_temparte_warehouses` ·
`inventory.plan_temparte_warehouses` · `inventory.supplier_trackings` ·
`inventory.temp_warehouses` · `inventory.user_plan_temparte_warehouses` ·
`notifications.admin_notifications` · `org.organization_social_media` ·
`org.user_organization_numbers` · `platform_admin.ai_providers` ·
`platform_admin.api_integrations` · `platform_admin.employee_activities` ·
`platform_admin.system_resources` · `profile.user_profiles` · `promo.ad_plans` ·
`promo.offer_package_features` · `promo.offer_promotions` · `promo.offer_views`

**For each, exactly one outcome:**

| Outcome | When | Action |
|---|---|---|
| **Connect** | Laravel has the feature and Phase B kept the screen | complete the chain |
| **Drop** | Laravel does not have it, or it was speculative | `db/migrations/NNN_drop_unused.up.sql`, with the `.down.sql` recreating it, and a `DELETED.md` entry |
| **Defer** | genuinely needed, genuinely later | written justification in `DECISIONS.md` naming the future task — **maximum 3 of these** |

`catalog.product_infos` deserves special attention: it is the 5-column key/value
bag that collides in name with Laravel's read model (now `catalog.product_index`).
If nothing uses it, **drop it** — the name collision alone is a maintenance
hazard.

---

## C.2 Vendor finance — worked example (follow this shape for all others)

This section is written out in full. Every other task in Phase C follows it.

### C.2.1 Read Laravel

```bash
cat "F:/Dawa 24/Laravel/app/Livewire/Employee/EmployeePayments.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Employee/EarningsOrder.php"
cat "F:/Dawa 24/Laravel/app/Livewire/Employee/EarningsOffers.php"
ls "F:/Dawa 24/Laravel/resources/views/livewire/employee/" | grep -iE "payment|earning"
```

Record in `CHAIN_AUDIT.md`: columns, filters (status? date range?), sort, page
size (`->paginate(N)`), row actions, the **earnings formula**, Arabic labels.

**The earnings formula is a business rule.** Write it out explicitly. Admin
earnings (`Admin/EarningsOrder`) must use the same one — implement it **once**
in the service layer.

### C.2.2 Write D1 first, watch it fail

```go
func TestVendorPaymentsPage_ShowsSeededPayment(t *testing.T) {
	db := testDB(t)
	org := seedOrg(t, db, "vendor")
	seedPayment(t, db, org.ID, money.FromMinor(123456), "settled")

	h := newRealUIHandler(t, db)
	rec := doGET(t, h, "/vendor/payments", actorFor(org))

	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Body.String(), "1,234.56")
}
```

Run it. It fails — `VendorPaymentsPage(lang, dir)` renders nothing.
**Paste the failure into the commit message.**

### C.2.3 Walk the chain

| Link | Check | Action |
|---|---|---|
| 1 TABLE | `billing.payments` exists? | yes — nothing to do |
| 2 REPOSITORY | `ListPaymentsByOrganization`? | add to `internal/modules/billing/postgres/payments.go`, inside `db.InReadTx`, **tenant-scoped, no `AsSystem`** |
| 3 INTERFACE | declare it in `internal/modules/billing/repository.go` | |
| 4 SERVICE | `internal/modules/billing/service.go` — validation, `apperr` | |
| 5 HANDLER | `internal/ui/vendor_finance_handlers.go` | replace the empty render |
| 6 TEMPLATE | `pages.VendorPaymentsPage(lang, dir)` → `(payments []*billing.Payment, lang, dir string)` | render the rows |

Handler, correct shape:

```go
func (h *UIHandler) VendorPaymentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/payments", http.StatusSeeOther)
		return
	}

	payments, err := h.billSvc.ListPaymentsByOrganization(ctx, actor.OrganizationID, 50, 0)
	if err != nil {
		h.log.ErrorContext(ctx, "list vendor payments", "error", err, "org", actor.OrganizationID)
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorPaymentsPage(payments, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor payments", "error", err)
	}
}
```

Note: **no `, _ =`**, **no `err == nil`**, and the render error is logged.

### C.2.4 Kill the hardcoded zero

```go
// current — the offers earnings figure is a literal zero
pages.VendorEarningsOffersPage(money.Zero, lang, dir)
```

Replace with the real aggregate. Money via `money.Amount`; assert **exact**
values in tests (never approximate comparison).

### C.2.5 The four tests

- **D1** above
- **D2** n/a (read-only screen)
- **D3** seed a payment for org A; assert org B's page does **not** contain it
- **D4** close the pool (or use a service stub that errors); assert the page
  shows an error state, not an empty table

### C.2.6 Completion

- [ ] `CHAIN_AUDIT.md` row has six non-empty cells
- [ ] `billing.payment_histories` no longer dead
- [ ] Earnings formula implemented once, shared with admin, matching Laravel exactly
- [ ] D1/D3/D4 pass; D1 was shown failing first

---

## C.3–C.16 — the remaining tasks

Each follows §C.2 exactly. Specific notes where the shape differs:

### C.3 Billing & subscriptions admin
Four dead tables (`plan_types`, `subscription_histories`, `subscription_users`,
`user_plan_histories`) were created with no consumer. **First decide whether
Laravel's subscription lifecycle actually needs all four** —
`cat Laravel/app/Models/{Subscription,SubscriptionHistory,SubscriptionUser,PlanType}.php`.
Drop what Laravel does not use.

### C.4 Employee activities
`platform_admin.employee_activities` is dead **and** nothing writes to it. Two
halves: (a) a write hook — Laravel uses `EmployeeActivityObserver`; in Go this is
a service-layer call or a River job fed by domain events; read the observer to
enumerate exactly which actions are recorded. (b) the two read screens.
**A read screen over a table nothing writes is still a fake screen.**

### C.5 / C.6 Organization & platform reference data
These are the four screens Phase A stripped to empty. Connect them now.
`api-integrations` must **never** render a stored key — masked display and a
"replace" action only. Add a test asserting the response body contains no value
from the credential column.

### C.7 Trash & deletes
Replace `defaultModelRegistry` with a real registry built from
`information_schema` — every table with a `deleted_at` column:

```sql
SELECT table_schema, table_name FROM information_schema.columns
WHERE column_name = 'deleted_at' AND table_schema NOT IN ('pg_catalog','information_schema')
```

Counts come from real `COUNT(*)` queries. Restore = `UPDATE … SET deleted_at = NULL`
+ audit row. Purge = hard `DELETE` + its own permission + typed confirmation +
audit row. **FK safety**: restoring a child whose parent is still deleted is
refused with a clear message, not a constraint error.
D2 must prove restore and purge actually change the database.

### C.8 Monetisation (the 13 placeholder screens)
Nine dead promo tables. Before connecting, apply Q1/Q2 per screen — 13 screens
for offer packages is likely more than the feature needs. Laravel has 11 admin +
5 vendor routes here; match that, not more.
Sponsorship ranking must flow through the **existing** `visibility.go` ordering
(`ORDER BY vo.is_sponsored DESC`) — `00_MASTER.md` §A.6 forbids forking that query.
Rotation must be deterministic and unit-testable; **no `random()` in SQL**.

### C.9 2FA (rebuild what Phase A deleted)
Real TOTP (RFC 6238), secret encrypted at rest, QR generated server-side,
single-use hashed recovery codes, rate-limited challenge, and
**`LoginSubmit` honouring `res.RequiresMFA`**.
D2: enable 2FA → `identity.user_mfa` row exists → logout → login → challenge is
required → a **wrong** code is rejected → the correct code issues a session.
The wrong-code assertion is mandatory: the deleted version accepted anything.

### C.10 PDF (rebuild what Phase A deleted)
A real PDF library with embedded Arabic font and correct RTL shaping.
D1: the generated bytes start with `%PDF-`, parse with a PDF reader, and contain
the seeded invoice total.
Plus a recorded manual check that Arabic letters **join** — a PDF with
disconnected Arabic is a failure even if the bytes parse.

### C.11 Saving products
`catalog.saving_products` was reinstated by migration 083 specifically for this,
and then three screens were built that never query it. Connect all of them:
vendor (list/import/detail), customer (list/import/detail), admin (list, per-user,
per-org, import landing). Import goes through the existing ingest pipeline — do
not build a second upload path.

### C.12 Vendor institutional work & pharmacy coverage
`org.employee_institutional_works` is referenced by exactly 1 file. Verify the
chain is complete, not just that the table name appears in a repository.
Pharmacy coverage: read
`Laravel/app/Livewire/Employee/PharmacyEmployeeWeeklyCoverage.php` before
building — do not guess what it shows.

### C.13 Bulk employee upload
Security-sensitive: it creates user accounts. Every created user goes through
the identity service — never a direct insert. No plaintext passwords in the
sheet. D3 must prove a vendor cannot create a user in another organization.

### C.14 Temp warehouses
Three dead `inventory.*` tables. **First establish whether this subsystem is
in scope at all** — it is 14 Laravel admin routes and a 396-line lifecycle
service. If it is deferred, drop the three tables now (they can be recreated)
rather than leaving them dead. Record the decision.

### C.15 Session plans & report issues
`identity.session_plans` is read-only; `session_plan_requests` and
`user_sessions` are dead. `workflow.report_issues` is dead while
`CustomerReportIssueSubmit` claims to write to it (Phase A already fixed the lie;
this task makes the claim true).

### C.16 Notifications & remaining admin
`notifications.admin_notifications` is dead. Note `/admin/notifications` is
currently a **dead target in the sidebar** — Phase B fixes the link, this task
gives it a working page.

---

## PHASE C COMPLETION GATE

```bash
make check
make test-integration          # must actually run
go test ./... -race
```

- [ ] `grep -rnoE 'pages\.[A-Za-z0-9_]+\(lang, dir\)' internal/ui/*.go` returns **0**
- [ ] The dead-table scan returns **0**, or each survivor has a written decision (max 3)
- [ ] `CHAIN_AUDIT.md` has six non-empty cells for every screen
- [ ] Every connected screen has D1 (shown failing first), D3, and D4
- [ ] Every write screen additionally has D2
- [ ] Zero hardcoded data literals remain
- [ ] Money assertions are exact
- [ ] `PROGRESS.md` rows C.2–C.16 complete

**The measure of this phase: delete any handler's body and its D1 test fails.**
