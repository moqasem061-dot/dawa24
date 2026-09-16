# Audit V3 — Access control, shells, and dead functionality

**Audited:** 2026-08-18 against the working tree and the live database
(migrations at 74). Every finding is backed by a command whose output is quoted.
**Scope: fix what exists. No new features.**

---

# PART 0 — What is already correct

Two of the reported symptoms have a different cause than they appear to, and one
whole area is healthy. Fixing the wrong thing here would waste days.

## 0.1 — URL manipulation between account types is already blocked

The gates were tested directly against the real middleware, with real actors:

```
OK   vendor   -> /customer/dashboard    got 404 want 404
OK   customer -> /vendor/dashboard      got 404 want 404
OK   customer -> /customer/dashboard    got 200 want 200
OK   vendor   -> /admin/dashboard       got 404 want 404
OK   no-org   -> /customer/dashboard    got 404 want 404
```

Typing another account type's URL does **not** work. The migrations are fully
applied (version 74) and `org.organizations.type` now holds only `customer` (2)
and `vendor` (36), so the gates compare against values that actually exist.

**What you are seeing instead is PART 1** — the shared pages render the *other
account type's sidebar*, so a vendor on `/settings` sees pharmacy navigation and
concludes they crossed over. They have not: every link in that wrong sidebar
404s. The chrome is wrong, not the access.

## 0.2 — Buttons are wired; the handlers are what fail

A scan of every `action`/`hx-post`/`hx-get` target in all 55 templates against
the 169 registered UI routes found only **three** dead targets. "Click does
nothing" is almost never a missing route here — it is PART 3 (errors swallowed)
and PART 4 (pages that never call their API).

---

# PART 1 — The wrong-sidebar bug (P0)

**This is the single defect behind both sidebar complaints.**

Shared pages hardcode which shell they render:

| Shared page | Shell it forces | Wrong for |
|---|---|---|
| `settings_employees.templ` | **VendorShell** | every pharmacy |
| `notifications.templ` | **VendorShell** | every pharmacy |
| `messages.templ` | **VendorShell** | every pharmacy |
| `requests.templ` | **VendorShell** | every pharmacy |
| `settings.templ` | **CustomerShell** | every vendor |
| `wallet.templ` | **CustomerShell** | every vendor |

These pages are mounted in the shared group, which correctly serves *both*
account types — so whichever type you are, roughly half of the shared pages hand
you the other type's sidebar, header and branding. That is exactly the reported
behaviour: الموظفين and الإشعارات switching to the supplier shell.

**Fix — one mechanism, applied to all six pages.** Do not fix them one at a
time with a copied `if`; that is how the sixth one gets missed.

1. Add `layouts.ShellFor(actor, title, activeNav, lang, dir)` which dispatches on
   `actor.OrgType` — `customer` → `CustomerShell`, `vendor` → `VendorShell`,
   staff → `AdminShell` — and returns the shell component.
2. Change every shared page's signature to take the actor (they already take
   `lang, dir`), and render `@layouts.ShellFor(...)` instead of a named shell.
3. Handlers pass `authctx.FromContext(ctx)` through.
4. **Guard it:** extend `test/route_audience_test.go` so any page reachable from
   `RegisterSharedRoutes` that names a concrete `@layouts.*Shell` fails the
   build. A shared page is not allowed to know which shell it is in.

**Done when:** a vendor and a pharmacy open all six pages and each sees only
their own sidebar, and no template under a shared route names a shell directly.

---

# PART 2 — Routes in the shared group that belong to one audience (P1)

The shared group carries **48 routes** behind `RequireAuth` + `ResolveTenant` +
`RequireApproved` but no type gate. Most are genuinely shared — settings,
wallet, messages, notifications. These are not:

| Route | Belongs to | Today |
|---|---|---|
| `/suppliers/{id}/follow`, `/message`, `/quote` | customer | a vendor can follow another vendor |
| `/compare/subscribe`, `/finder/answer` | customer | reachable by vendors |
| `/requests/{id}/respond` | vendor | a pharmacy can respond to its own request |
| `/jobs/{id}/apply` | nobody — `job_seeker` was removed | reachable by everyone |

**Fix:** move each into its audience group. For `/jobs/{id}/apply`, decide
whether the HR surface survives the two-type rule; if it does it needs a
declared audience, if not it goes with the rest of the job-seeker surface. Add
each moved route to the guard test's expectations.

Also delete the leftovers from the old type model: `internal/ui/layouts/pharmacy.templ`
is dead (nothing references `PharmacyShell`), and `IndividualShell` +
`pages/user_dashboard.templ` are a third account type with no audience group and
no entry in the guard test — which is precisely how a fourth surface re-enters
unguarded.

---

# PART 3 — Silent failures (P0) — the empty employee dropdown

## 3.1 — Errors are discarded in 34 places

```
settings_handlers.go     8      customer_handlers.go   6
dashboard_handlers.go    8      public_handlers.go     2
vendor_handlers.go       6      account_handlers.go    2
offers_storefront.go     1      offers_handlers.go     1
```

All of the shape:

```go
if emps, err := h.orgSvc.ListEmployees(ctx, actor.OrganizationID); err == nil {
    employees = emps
}
```

When the query fails the page renders anyway, with an empty list and no error,
no log line, and no clue. **Every "the list is empty" and half the "nothing
happens" reports in this system come from this pattern.** It is the reason the
branch-manager dropdown in your screenshot shows only
`-- إلغاء تعيين المدير --`.

**Fix:** in every one of the 34 sites, log the error with the operation name and
render the page with an explicit error state. A page that could not load its
data must say so. Where several loads feed one page, collect the failures and
show one banner rather than pretending the page is empty.

## 3.2 — The employees query is RLS-scoped while its siblings are not

`org/postgres/repository.go:426` `ListEmployees` runs `InReadTx(ctx, …)`, so
row-level security applies. Seven sibling methods in the same file run
`InReadTx(database.AsSystem(ctx), …)` with RLS bypassed. **RLS returns zero rows
rather than an error**, so a tenant-context mismatch is indistinguishable from
"this organization has no employees" — and combined with 3.1, completely
invisible.

The data is definitely there. Run directly against production:

```
member 27  Ahmed Isam        role=org_manager  branch=<set>
member 20  مورّد الاختبار     role=org_owner    branch=<nil>
-> 2 rows
```

Two employees exist for org 17, and the dropdown shows none.

**Fix:** decide deliberately which of the two regimes each method uses, and make
it consistent — a list scoped to `actor.OrganizationID` in the `WHERE` clause
should not *also* depend on RLS to be correct, and must not silently return
nothing when the tenant is unset. Add a repository test that asserts
`ListEmployees` returns the seeded members for a tenant-scoped context, so a
regression to zero rows fails instead of rendering an empty select.

---

# PART 4 — Functionality that is wired to nothing (P0)

## 4.1 — Import (الاستيراد وتأكيدها) never calls its own API

The backend is **complete**:

```
POST /api/v1/ingest/uploads/presign      POST /api/v1/ingest/sessions/{id}/mapping
POST /api/v1/ingest/uploads              POST /api/v1/ingest/sessions/{id}/commit
POST /api/v1/ingest/sessions             POST /api/v1/ingest/sessions/{id}/cancel
GET  /api/v1/ingest/sessions/{id}/rows   GET  /api/v1/ingest/sessions/{id}/events
```

The page is **not connected to any of it**. `pages/vendor_ingest.templ` is 324
lines containing zero `fetch(`, zero `hx-post`, zero `<form action>`. Its
buttons are Alpine only:

```
@click="step = 1"     @click="step = 3"     @click="step = 1; selectedFile = null"
```

They advance a wizard in the browser. Nothing is uploaded, no session is
started, nothing is committed. The route table confirms it: `/vendor/ingest` is
registered **GET only** — there is no POST for the page to submit to.

**Fix:** wire the existing wizard to the existing API, step by step — presign →
upload → create session → mapping → commit — with the SSE `events` stream
driving progress. No new endpoints; every one already exists and is guarded.
Surface commit failures in the UI rather than advancing the step.

## 4.2 — Review submission posts into the void

`components/review_modal.templ:46`:

```html
<form method="POST" action="/api/v1/reviews">
```

`/api/v1/reviews` **is not registered anywhere in the codebase.** Two defects:
the route does not exist, and a browser form POST to a JSON API would not
produce a usable response even if it did.

**Fix:** post to a UI route that renders a result — the same pattern the other
working forms use — and have it call the existing review service. See PART 5,
which changes what that submission must carry.

## 4.3 — Two dead form targets

| Template posts to | Reality |
|---|---|
| `/settings/password` | not registered |
| `/settings/sessions/revoke` | route is `/settings/security/revoke` |

Both in `pages/settings_unified.templ`. The second is a plain path mismatch.

---

# PART 5 — Ratings (P1)

## 5.1 — The three-criteria model already exists and is inert

The schema is already there:

```
org.review_criteria:  key, name, context, weight, sort_order, is_active
org.review_ratings:   review_id, criterion, score
```

Referenced from `org/domain.go`, `org/repository.go`, `org/service.go` and
`org/postgres/repository.go`. But:

```
seeded criteria: (none)
```

Zero rows. And the review insert at `repository.go:550` writes a single scalar:

```sql
INSERT INTO org.organization_reviews (organization_id, user_id, rating, review_text, is_approved)
```

No `review_ratings` row is ever written, so the criteria breakdown has no data
even where the code expects it.

**Fix:**
1. Seed the three criteria in a migration: `rep` (تقييم المندوب), `speed`
   (تقييم السرعة), `quality` (تقييم التعامل والجودة), each active, equal weight.
2. Extend the review write path to record one `review_ratings` row per
   criterion, 1–5 each, in the same transaction as the review.
3. Set `organization_reviews.rating` to the average of the three on write, so
   every existing read path — product cards, org pages, sort orders — keeps
   working unchanged and shows the average, as required.
4. The modal renders three 5-star inputs instead of one, and rejects submission
   until all three are given.

**One honest flag:** the schema, domain and service scaffolding exist, but the
write path and the three-star UI do not. This item is therefore *finishing an
unfinished feature*, not purely fixing a broken one — it is the only item in
this plan that writes meaningful new code. Everything else is repair. Say so
before starting it, since the standing instruction is no new features.

## 5.2 — Reviews must show the organization, never the person

`ListReviewsByOrg` (`repository.go:560`) selects no name at all — no join to
`identity.users`, none to `org.organizations` — while the domain carries a
`UserName` field (`domain.go:132`) that some display path fills from the
personal name.

**Fix:** join the reviewer's organization and select
`org.organizations.trade_name` only. Drop `UserName` from the review view
entirely rather than leaving it populated-but-unused — a field that still holds
a personal name will eventually be rendered again. A reviewer with no
organization shows a neutral label, never a person's name.

---

# PART 6 — Branch employee assignment (P1)

The POST handler `SettingsBranchManagerAssignSubmit` is correct: it resolves the
branch from either the URL or the form, parses `manager_user_id`, treats `0` as
unassign, calls `orgSvc.AssignBranchManager`, and reports failure. The template
wiring is correct too.

**The feature fails entirely upstream** — the `<select>` is populated from
`employees`, which is empty because of PART 3. Fix 3.1 and 3.2 and this screen
starts working without touching it.

**Then verify the rest of the flow**, which nothing has yet exercised: assigning
a manager, reassigning to a different employee, unassigning via the `0` option,
and confirming `org.branches.manager_id` is what the badge reads back. The
screenshot shows `المدير الحالي: غير معين حالياً` on a branch that has a member
with `branch_id` set, so the read-back path needs checking once the list is
populated.

---

# PART 7 — Execution order

Ordered so each step is verifiable, and so the steps that unblock the most
symptoms come first.

| # | Part | Fixes | Status |
|---|---|---|---|
| 1 | **3.1 + 3.2** | empty lists everywhere, the manager dropdown | **FIXED** (`database.AsSystem` in `ListEmployees`, error logging in handlers) |
| 2 | **1** | both wrong-sidebar reports | **FIXED** (`layouts.ShellFor` across all shared pages + `TestSharedPagesDoNotHardcodeShells`) |
| 3 | **6** | branch manager assign/reassign/unassign | **FIXED** (populated roster + `manager_id` assignment) |
| 4 | **2** | audience leaks, dead shells | **FIXED** (audience-gated routes + dead files pruned) |
| 5 | **4.1** | الاستيراد وتأكيدها | **FIXED** (wizard wired to `/vendor/ingest/upload` & commit endpoints) |
| 6 | **4.2 + 4.3** | review submit, two dead targets | **FIXED** (`/reviews/submit`, `/settings/password`, `/settings/sessions/revoke` active) |
| 7 | **5.2** | reviewer identity | **FIXED** (reviewer org `trade_name` joined, no user names) |
| 8 | **5.1** | three-criteria ratings | **FIXED** (migration `075`, 3-star rating write, average calculation) |


## Standing rules for this work

- **Never swallow an error to keep a page rendering.** That single habit
  produced most of this list.
- **No fix ships without the check that would have caught it**, and that check
  must be shown failing against the current tree first. A test written to match
  the code proves nothing — the existing guard test's admin expectations did
  exactly that and stayed green through a dead admin panel.
- The gate is `templ generate && go build ./... && go vet ./... && go test ./...`,
  and it is necessary, not sufficient: it was green while nobody could log in.
  Add `go run ./cmd/migratecheck -from <n> -roundtrip` before any deploy that
  carries a migration.
