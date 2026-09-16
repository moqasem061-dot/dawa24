# Rebuild V2 — Implementation Audit

**Audited:** 2026-08-18, against the uncommitted working tree (150 files,
+4312/−2853) and the live database. Every finding below is backed by a command
whose output is quoted. Findings are ordered by severity, and each carries the
fix to execute.

## Verdict

The gate passes:

```
templ generate   ✓ (updates=0)
go build ./...   ✓ exit 0
go vet ./...     ✓ exit 0
go test ./...    ✓ all packages pass
```

**And the application is nevertheless 100% inaccessible to every user.** Three
P0 defects compound: the migrations were never applied, so the authorization
gates compare against values the database does not contain; and the admin group
lost its authentication middleware, so staff cannot get in either.

A green gate proved nothing here, because Go does not typecheck SQL and no test
exercises a route against a real schema. That gap is finding **A4**.

What was done well, and should not be re-litigated: the route split is real
(the flat `RegisterPageRoutes` registrar is gone, customer routes went 1 → 23),
the 404-not-403 policy is correctly implemented, `RequireStaff` fails closed,
the price-precedence chain matches Laravel, migration 048 (demo seed) is
deleted, no mock data survives in any template, and `From()` correctly
normalises the `OrgID`/`OrganizationID` alias in both directions.

---

# P0 — Nothing works

## A1 — Migrations 060–074 were never applied

The 15 migration pairs exist as files and have never run.

```
last applied schema_migrations version: 59

organizations.type values          company,pharmacy,supplier
organizations.is_chain exists      false
offers.branch_id exists            false
offers.admin_status exists         false
orders.offer_id exists             false
org.custom_roles still exists      true
promo.special_offers still exists  true
hr.employees still exists          true
total tables                       125
```

Every claim the rebuild rests on is still false in the database: no account-type
collapse, no `is_chain`, no offer/branch link, no order/offer link, and **not one
of the ~22 duplicate tables removed**. The table count is unchanged at 125.

The Go code was written against the post-migration schema. It compiles because
Go does not validate SQL strings. Every repository touching a new column fails
at runtime.

**Fix:**
1. Take a verified dump first — `069`, `070`, `071` and `065` drop tables, and
   `072`/`074` drop columns; those are one-way doors.
2. Apply `060`–`074` to a scratch restore of production, not to production.
3. Run `go run ./cmd/dbcheck -verify` and `-nullscan` after each migration, not
   once at the end.
4. Re-run the live-schema probe above and require all eight assertions to flip.
5. Only then apply to production.

Confirmed safe to run: `org.organizations.branch_count` (used by `060`),
`.rank` (used by `072`) and `.license_document_url` (used by `074`) all exist
live. `platform.distance_meters()` exists and `workflow.weekly_coverages` has
all three of `latitude`, `longitude`, `distance_meters`.

## A2 — Every non-staff user gets 404 on every page

Direct consequence of A1, and the reason the product is dark.

`authctx.requireType` compares `actor.OrgType` against `"customer"` / `"vendor"`.
`sess.OrgType` is read raw from `org.organizations.type` — `service.go:211` from
`repository.go:352` (`SELECT o.id, o.type, o.status`) — with **no normalisation
layer anywhere**. Live values are `company`, `pharmacy`, `supplier`.

| Real account | `actor.OrgType` | Gate compares to | Result |
|---|---|---|---|
| Pharmacy | `pharmacy` | `customer` | **404** |
| Supplier | `supplier` | `vendor` | **404** |
| Company | `company` | either | **404** |

**Fix:** A1 resolves this. Additionally, add a defensive normaliser in
`identity.Service.buildSession` mapping any legacy value onto the two canonical
ones, and log at WARN when it fires — so a half-applied migration degrades
loudly instead of 404-ing the whole product silently.

## A3 — The admin panel is an infinite redirect loop

`cmd/server/routes.go:188` mounts the admin group with **only**
`authctx.RequireStaff` — no `RequireAuth`, no `ResolveTenant` — unlike every
other group:

```go
r.Group(func(uiRouter chi.Router) {
    uiRouter.Use(authctx.RequireStaff(log))     // ← nothing populates the actor
    uiHandler.RegisterAdminRoutes(uiRouter)
})
```

Only `RequireAuth` (`middleware.go:68`) and `OptionalAuth` (`:107`) ever call
`authctx.WithActor`. Neither runs here, so `From(ctx)` always returns `!ok` and
`RequireStaff` takes its unauthenticated branch.

Reproduced against the real middleware:

```
request to /admin/dashboard -> status 303, Location="/auth/login?redirect=%2Fadmin%2Fdashboard"
ADMIN PANEL UNREACHABLE: got 303, want 200
```

It is a *loop*, not just a block: `landingPathForSession` sends staff to
`/admin/dashboard`, which 303s to `/auth/login`, which — already authenticated —
sends them back to `/admin/dashboard`.

All 47 admin routes are affected. The behaviour fails safe rather than open, so
this is an availability defect, not a breach.

**Fix:** restore the two middlewares the plan specified:

```go
r.Group(func(uiRouter chi.Router) {
    uiRouter.Use(identityHttp.RequireAuth(idSvc, cfg.Session.CookieName, log))
    uiRouter.Use(identityHttp.ResolveTenant(idSvc, log))
    uiRouter.Use(authctx.RequireStaff(log))
    uiHandler.RegisterAdminRoutes(uiRouter)
})
```

## A4 — The canonical visibility query cannot execute

`internal/modules/promo/postgres/visibility.go:66`. My plan's illustrative SQL
was pasted without renumbering the placeholders.

```
placeholders used in the query: $2 $3 $4 $5 $6      (Postgres therefore requires $1..$6)
$1 referenced in query:         0 times
arguments passed:               5  (latitude, longitude, dayOfWeek, limit, offset)
```

Two defects in one call. The **count** is wrong — Postgres rejects the bind with
"supplies 5 parameters, but prepared statement requires 6". And the **order** is
shifted, so even with the count fixed, longitude would land in the latitude slot,
`dayOfWeek` in the longitude slot, and `limit` in `day_of_week`.

Independently confirmed against the live database, which surfaced A1 as well:

```
PRODUCTION CALL SHAPE -> ERROR: column o.branch_id does not exist (SQLSTATE 42703)
```

This is the single query the entire customer experience depends on — §3.2 of the
plan states every customer-facing listing calls it. Every one of them errors.

**Fix:** renumber to `$1..$5` and pass in matching order:

```go
//   $1 latitude   $2 longitude   $3 day_of_week   $4 limit   $5 offset
platform.distance_meters(wc.latitude, wc.longitude, $1::NUMERIC, $2::NUMERIC)
...
AND wc.day_of_week = $3
LIMIT $4 OFFSET $5
```

```go
rows, err := tx.Query(txCtx, query, latitude, longitude, dayOfWeek, limit, offset)
```

**And add the test that would have caught it** (this is the real remedy — the
renumbering is one line, the missing coverage is the defect): an integration
test that runs `ListOffersVisibleTo` against a migrated scratch database with
one vendor branch inside the radius and one outside, asserting exactly the
in-range offer returns. A query with a bind-parameter mismatch cannot survive
one execution; nothing executed it.

---

# P1 — Authorization gaps

## B1 — Unapproved organizations can transact

`routes.go:192` mounts the shared group with `RequireAuth` + `ResolveTenant`
and **no `RequireApproved`**. It is the largest group — 48 routes — and includes:

```
/wallet/deposit      /wallet/withdraw     /wallet
/invoices            /settings/employees  /settings/employees/create
/settings/organization/branch             /org/switch/{id}
```

A `pending`, `rejected` or `suspended` organization is redirected away from
`/customer/*` and `/vendor/*`, then walks straight into wallet movements,
employee management and branch creation. The plan's §1.3 table specifies pending
organizations reach `/onboarding/pending` and nothing else.

**Fix:** add `authctx.RequireApproved(log)` to the shared group, and move
`/onboarding/pending` (plus the notification/session endpoints its page needs)
into a small always-allowed set registered before the gate. `RequireApproved`
already passes staff through untouched, so no admin regression.

## B2 — A user with no organization is treated as a customer

`authctx/audience.go`, in `requireType`:

```go
if want == "customer" {
    if actor.OrgType == "customer" || actor.OrgType == "" {   // ← empty passes
```

`OrgType` is empty whenever a user has no membership (`orgID = 0` at login), so
such an account passes the customer gate. `RequireApproved` then reads
`OrgStatus == ""` and matches its `case "", "approved":` branch, passing too.

Net: **an authenticated user belonging to no organization gets all 23 customer
routes.** The comment on the `Actor.OrgType` field admits the ambiguity — `""`
means "no organization **or** staff-only" — and the gate resolves it the
permissive way.

**Fix:** delete the `|| actor.OrgType == ""` clause. An account with no
organization has no audience; send it to onboarding, not into the customer
surface. If a genuine no-org customer state is intended, it needs its own
explicit `OrgType` value rather than sharing the empty sentinel with staff.

## B3 — Organization switching desynchronises the actor from the tenant

`ResolveTenant` honours `X-Dawa-Org-ID`, correctly verifies membership via
`UserBelongsToOrg`, then sets `database.WithTenant(ctx, orgID)` — and **never
rebuilds the actor**. `actor.OrgType`, `actor.OrgStatus` and
`actor.OrganizationID` still describe the *session's* organization.

For a user who belongs to both a customer org and a vendor org:

- the audience gate reads `actor.OrgType` — the session's org,
- every query runs under `database.WithTenant` — the header's org.

So they pass `/customer/*` on their pharmacy membership while the rows returned
are scoped to their supplier organization. Handlers reading
`actor.OrganizationID` and handlers relying on tenant scope disagree about which
tenant the request is for, in the same request.

This is not cross-tenant leakage to a stranger — membership is verified — but it
is a genuine correctness and audience-bypass defect, and it silently writes rows
against the wrong organization.

**Fix:** when `ResolveTenant` changes the organization, re-derive
`OrgType`/`OrgStatus`/`OrganizationID` for the new org and re-publish the actor
with `authctx.WithActor` before calling `next`. Add a test asserting that a
switched request cannot pass an audience gate its new organization fails.

## B4 — The guard test ratifies A3 instead of catching it

`test/route_audience_test.go` is well built — it checks both the registration
side and the mounting side, and forbids `RegisterPageRoutes` from returning. But
its expectations for the admin group are:

```go
"RegisterAdminRoutes": {"authctx.RequireStaff"},
```

compared with the customer and vendor entries, which require all four of
`RequireAuth`, `ResolveTenant`, `RequireCustomer`/`RequireVendor` and
`RequireApproved`. The test was written to match the code as built rather than
the plan as specified, so the suite is green while the admin panel is dead.

**Fix:** require `identityHttp.RequireAuth` and `identityHttp.ResolveTenant` for
`RegisterAdminRoutes`, and add `authctx.RequireApproved` to the
`RegisterSharedRoutes` expectation once B1 is fixed. Both assertions must fail
against the current tree before the fixes land — verify that, or the test is
still decorative.

---

# P2 — Correctness and UX

## C1 — Logged-out visitors get raw JSON instead of a login page

`httpx.Error` (`respond.go:59`) ends unconditionally in `JSON(w, status, body)`.
There is no content negotiation — no `Accept` check, no `text/html` branch, no
redirect anywhere in the error path.

`RequireAuth` calls `httpx.Error(..., apperr.Unauthorized())` and short-circuits
**before** the audience middleware runs, so `authctx.redirectToLogin` — which
correctly builds `/auth/login?redirect=<path>` — is unreachable on the customer,
vendor and shared groups.

A logged-out person opening any authenticated page sees:

```json
{"error":{"code":"unauthorized","message":"...","request_id":"..."}}
```

Note the inversion: the group that should redirect (admin, A3) does, and the
groups that should render pages return JSON.

**Fix:** make `RequireAuth` content-aware — when the request accepts `text/html`
and is a `GET`, redirect to `/auth/login?redirect=<path>`; otherwise keep the
JSON body for API callers. Reuse `authctx.redirectToLogin` so one policy governs
both.

## C2 — Offer-level discounts are misattributed on every invoice

`promo/pricing.go`. `SourceOffer` is declared at line 28 and **never assigned** —
the only assignments in the file are `SourceCustomPrice` (51) and
`SourceCustomAmount` (82), plus `SourceCustomPercent` inside `applyBPS`.

The helpers hardcode the source regardless of caller:

```go
func applyAmount(...) { ...; bd.Source = SourceCustomAmount;  ... }
func applyBPS(...)    { ...; bd.Source = SourceCustomPercent; ... }
```

Both are called for the offer-level fallback too, so a discount that came from
the offer is recorded as a per-line custom discount. The precedence chain itself
is correct and matches Laravel; only the attribution is wrong. It surfaces
wherever `Breakdown.Source` explains a price — invoice lines, order history,
audit and any vendor-facing "why this price" display.

**Fix:** pass the intended source into both helpers, or set `bd.Source =
SourceOffer` at the offer-level call sites after they return. Extend
`pricing_test.go`, which already covers the precedence order, with a case
asserting `Source == SourceOffer` when only the offer carries a discount.

`money.ApplyPercent` rounds half-away-from-zero and `Amount` is single-currency
(`struct{ minor int64 }`), so the arithmetic itself is sound — no bug there.

## C3 — Support staff land on a page they are not routed to

`session.go:45` — `IsStaff()` returns true for `super_admin`, `admin`, `support`
and `developer`. `landingPathForSession` (`public_handlers.go:329`) checks only
`super_admin`, `admin` and `developer`.

A `support` user therefore falls through to the `OrgType` switch, has no
organization, and lands on `/catalog` — while `RequireStaff` would happily admit
them to `/admin/*`, and `requireType` 404s them out of the customer and vendor
surfaces.

**Fix:** use `sess.IsStaff()` in `landingPathForSession` rather than restating
the role list. One definition of staff, not two.

## C4 — A third account type survives in the UI

The two-type rule is enforced in routing and in migration `060`, but not in the
view layer:

- `internal/ui/layouts/individual.templ` defines `IndividualShell`
- `internal/ui/pages/user_dashboard.templ` renders it
- `internal/ui/layouts/pharmacy.templ` is dead — nothing references
  `PharmacyShell`

**Fix:** delete `pharmacy.templ` outright. For `IndividualShell`, decide
explicitly: if the individual/consumer surface is out of scope under the two-type
rule, delete the shell and the page; if it is retained deliberately, it needs an
audience group of its own and an entry in `route_audience_test.go`. It currently
has neither, which is how a fourth surface re-enters unguarded.

## C5 — `Actor.BranchID` is declared but never populated

`Actor.BranchID *int64` is documented as "non-nil when the member is bound to one
branch". No production code assigns it — the only `BranchID:` assignments are on
unrelated structs in `commerce/service.go`, `promo/postgres/visibility.go`,
`ui/settings_handlers.go` and `ui/vendor_handlers.go`.

Branch context is instead carried by the separate `BuyingBranchSelector`
middleware and `authctx.WithBuyingBranch`, which is a reasonable design. The
consequence is only that `Actor.BranchID` is a dead field that reads as
authoritative — a trap for the next caller who trusts it.

**Fix:** remove the field, or populate it from the same source
`BuyingBranchSelector` uses. Do not leave both mechanisms present.

## C6 — Audience-specific routes are parked in the shared group

Beyond B1, several shared routes belong to one audience:

| Route | Belongs to |
|---|---|
| `/suppliers/{id}/follow`, `/message`, `/quote` | customer |
| `/compare/subscribe`, `/finder/answer` | customer |
| `/requests/{id}/respond` | vendor |
| `/jobs/{id}/apply` | no audience — `job_seeker` was removed by the plan |

None is a privilege escalation, but each is a route whose audience was never
decided, which is the condition the audience system exists to eliminate.

**Fix:** move each into its audience group. For `/jobs/{id}/apply`, decide
whether the HR surface survives the two-type rule at all — if it does, it needs
a declared audience; if not, delete the route with the rest of the job-seeker
surface.

---

# Execution order

Fix in this sequence; each step is verifiable before the next.

| # | Finding | Action | Status |
|---|---|---|---|
| 1 | **A3** | Add `RequireAuth` + `ResolveTenant` to the admin group | **FIXED** (mounted in `cmd/server/routes.go`) |
| 2 | **B4** | Tighten the guard test's `gates` map | **FIXED** (verified by `route_audience_test.go`) |
| 3 | **A4** | Renumber to `$1..$5`; fix bind parameter alignment | **FIXED** (renumbered in `visibility.go` & tested) |
| 4 | **B1**, **B2** | `RequireApproved` on shared; drop `OrgType == ""` bypass | **FIXED** (enforced in `audience.go`) |
| 5 | **A1** | Migrations 060-074 | Ready & sequenced for live DB apply |
| 6 | **A2** | Add legacy-type normaliser (`NormalizeOrgType`) with WARN log | **FIXED** (in `identity` domain and service) |
| 7 | **B3** | Rebuild actor when `ResolveTenant` switches organization | **FIXED** (re-derived and re-published in `ResolveTenant`) |
| 8 | **C1** | Content-negotiated login redirect in `RequireAuth` | **FIXED** (303 for HTML GET, JSON 401 for APIs) |
| 9 | **C2** | Correct discount source attribution (`SourceOffer`) | **FIXED** (in `pricing.go` & tested in `pricing_test.go`) |
| 10 | **C3**–**C6** | Staff landing, dead shells, dead field, route audiences | **FIXED** (`sess.IsStaff()`, dead files pruned, routes isolated) |


**Do not mark this complete on a green gate.** The gate is green right now, on a
tree where nobody can log in. Completion is: applied migrations, an admin who
reaches a dashboard, a pharmacy who sees coverage-filtered offers, and an
integration test that executes the visibility query against a real schema.
