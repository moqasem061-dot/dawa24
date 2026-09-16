# Architecture Decision Records

One file while the set is small. Split into `NNNN-slug.md` when it outgrows this.

---

## ADR 0001 — Go with templ + HTMX for the Store

**Status:** Accepted · 2026-08-16

### Context

The legacy application is Laravel 12 with **353 Livewire components and 514 Blade
templates**, and roughly 20 HTTP controllers. Business logic lives in the Livewire
components, not in a service layer. The UI is server-rendered and server-reactive,
and it is the largest single artifact to be rebuilt.

Constraints: one developer working with AI agents; Elest.io billing is per
service; Arabic/RTL is a first-class requirement including for SEO; the Gateway
is already Go.

### Options

- **A.** Go + templ + HTMX + Alpine — one deployable, server-rendered
- **B.** Go API + Next.js/React — two deployables, client state
- **C.** TypeScript end-to-end — one language, two runtimes
- **D.** Stay on Laravel, modernise in place — Octane, modular refactor

### Decision

**Option A.**

### Why

1. **Closest conceptual port.** Livewire's model — server holds state, server
   renders, small deltas over the wire — maps almost 1:1 onto templ + HTMX. Onto
   React it maps close to 0:1: client state management would have to be invented,
   because the legacy app has none.
2. **One language with the Gateway.** Shared idioms, shared money handling, one
   toolchain, one CI shape.
3. **Cost.** One container at roughly 128 MB rather than two, on per-service
   billing.
4. **Arabic SSR.** HTML generated in one place; no hydration mismatch on RTL.
5. **Agent determinism.** Compile-checked templates and explicit SQL give an AI
   agent a far narrower failure surface than a framework built on runtime
   convention.

Option D was evaluated seriously and rejected on the *data* problems rather than
the language: two competing order systems, a 137-column `users` table, four
subscription systems and two RBAC systems all require a schema rewrite regardless
of language. Once the schema is being rebuilt, keeping the Laravel code buys
little.

### Consequences

- Rich data grids need work HTMX does not give free. Mitigation: TanStack Table
  and Chart.js as islands, not a React application.
- Smaller hiring pool than PHP or JS. Accepted: the codebase is small,
  conventional and heavily linted.
- **Revisit if** a customer mobile app or third-party vendor SPA becomes a
  requirement. The Go backend already speaks JSON; add Next.js then, against a
  stable API, without touching the domain layer.

---

## ADR 0002 — Modular monolith, not microservices

**Status:** Accepted · 2026-08-16

### Context

The platform is expected to grow. That is not, by itself, a reason to split it.

### Decision

A **modular monolith**: one binary, one database, module boundaries enforced by
the linter and mirrored by PostgreSQL schemas. The worker runs as a separate
*process* from the same image, not a separate service.

The **AI Gateway is the only separate service**, and it earns that on criteria no
Store module currently meets.

### Why the Gateway is separate

- **Different failure domain** — third-party LLM providers, minute-scale
  latencies, independent outages.
- **Different scaling curve** — I/O-bound proxying versus transactional commerce.
- **Shared across products** — it already serves other Muhiya applications.
- **Holds credentials the Store must never see.**

### Extraction triggers

A module leaves the monolith only on a specific, measurable condition:

| Candidate | Trigger |
|---|---|
| Import/ingestion | Sustained ingestion > 100k rows/hour, **or** import work causes > 20% of API P99 latency |
| Search indexer | Catalogue > 250k active SKUs, **or** search P95 > 300 ms after index tuning |
| Notifications | > 1M sends/month, **or** an integration needs an independent release cadence |
| Reporting | Analytical queries measurably degrade OLTP and read replicas are already in use |

"It will scale" is not a trigger. Neither is "the team prefers services".

### Consequences

- One deploy for most changes; coupling must be prevented by tooling rather than
  by network boundaries. `depguard` rules in `.golangci.yml` do this.
- One database is one failure domain for business data. Accepted: PostgreSQL with
  PITR and a tested restore is more reliable than distributed transactions spread
  across services nobody is staffed to operate.

---

## ADR 0003 — Tenant isolation via PostgreSQL row-level security

**Status:** Accepted · 2026-08-16

### Context

This is a **multi-vendor marketplace**: suppliers who compete with each other keep
catalogue, pricing, stock and order data in one database. A leak between tenants
is the most damaging bug the system can have.

The legacy application enforced isolation by hand — `where organization_id = ?`
repeated across 353 Livewire components. Every new query is another chance to
forget. The audit also found **36 `*_id` columns with no foreign key**, so this
was not a codebase where such invariants were reliably maintained.

### Decision

Defence in depth, with the database as backstop:

1. Middleware resolves the active organisation into `context.Context`.
2. `database.InTx` / `InReadTx` issue `SET LOCAL app.current_org_id` per
   transaction.
3. Tenant-owned tables `ENABLE` **and `FORCE`** row-level security, with a policy
   calling `platform.tenant_visible(organization_id)`.
4. Cross-tenant access requires `database.AsSystem(ctx)` — explicit and greppable.

### Why FORCE matters

Without `FORCE ROW LEVEL SECURITY` the table owner bypasses every policy. In a
small deployment the application usually connects as the owner, which would make
the whole mechanism decorative.

### Why SET LOCAL rather than SET

`SET LOCAL` is scoped to the transaction and discarded when the connection
returns to the pool. Session-level `SET` would leak one tenant's id into the next
request that borrowed the same pooled connection — precisely the bug this exists
to prevent.

### Consequences

- A forgotten `WHERE` returns **zero rows** instead of a competitor's data: it
  fails closed, surfacing as a visible bug rather than a silent breach.
- All tenant-scoped access goes through the transaction helpers; `db.Pool()`
  stays out of module code.
- Negligible per-query GUC overhead relative to the queries themselves.
- **Required test:** every tenant-owned table gets a cross-tenant read test
  asserting zero rows. CI gate, not a convention.

---

## ADR 0004 — Migrations run before promotion; rollback is redeploy

**Status:** Accepted · 2026-08-16

### Context

The legacy project had no CI, no containers, no IaC and no version control. Its
schema could not be rebuilt from its migrations: 49 active migrations against 114
models, plus 153 archived ones and a `migrations.zip`.

### Decision

- Migrations run as a **discrete pipeline step**, before the new image is
  promoted. Never on server startup.
- Every migration is **backward-compatible with the running version**
  (expand/contract): add and backfill in one release, remove in a later one.
- **Rollback = redeploy the previous image.** No schema reversal in the hot path.
- Migrations are **embedded in the binary** and **checksummed**; editing an
  applied migration is refused.
- An **advisory lock** serialises concurrent deploys.
- `/ready` reports unready while migrations are pending, so an instance never
  serves traffic against a schema it does not expect.

### Why not migrate on startup

N instances starting together race. Worse, it couples "the schema is ready" to "a
web process started", so a migration failure presents as a crash loop instead of
a failed deployment step with a readable log.

### Consequences

- A destructive change takes two releases. That is the point.
- The pipeline needs database credentials at the migration step.
- `down.sql` files exist for local development. They are **not** the production
  rollback plan; the previous image is.

---

## ADR 0005 — AI lives behind the Gateway; the Store stays provider-agnostic

**Status:** Accepted · 2026-08-16

### Context

The legacy application wired five providers directly into business code, plus a
provider SDK dependency, an `ai_providers` table, two competing traits
(`HasAiServices` and `OldHasAiServices`), and provider API keys in `.env`. Roughly
19 files referenced providers directly.

A mature Gateway already exists (`muhiyallm-gateway`): OpenAI-compatible surface,
40 migrations, exact-money billing, virtual keys, per-model pricing windows,
request logging, and a `client_app` column already designed for multiple
consumers.

### Decision

1. The Store contains **no provider SDK, no provider key, no model name**. CI
   enforces it (`make check-provider-isolation`).
2. The Store requests **capabilities** — `product.match`, `import.detect_columns`,
   `order.optimize` — which the Gateway maps to model aliases.
3. **Do not fork the Gateway.** Deploy the same codebase as a second Elest.io
   service with its own database and keys. A fork means maintaining two diverging
   money implementations forever.
4. **Every capability has a deterministic fallback.** The Gateway being
   unreachable degrades quality, never availability.

### Fallbacks

| Capability | Fallback |
|---|---|
| `import.detect_columns` | Heuristic header matcher (legacy `ColumnDetector` path) |
| `product.match` | Trigram + Arabic-normalised fuzzy match in Postgres |
| `product.enrich` | Skip; leave fields null and flag for review |
| `order.optimize` | Rule engine: price, then stock, then distance |
| `catalog.chat` | Assistant unavailable; ordinary search still works |
| `search.expand_query` | Plain search without expansion |

### Consequences

- Swapping providers is a Gateway admin change with **zero Store deploys**.
- The only Gateway code change needed is logging a tenant header (~20 lines),
  written generically so it lands upstream and benefits every client.
- AI spend is attributed per organisation via `X-Dawa-Org-ID` and capped by
  Gateway plans.
- Store developers cannot experiment with a provider directly. That is the
  intent: the capability boundary is what keeps the Store portable.

---

## ADR 0006 — Content i18n in JSONB vs Embedded UI Chrome vs Admin Translations Table

**Status:** Accepted · 2026-08-16

### Context (D-2)
Domain content (product titles, categories) requires bilingual storage. UI labels (buttons, validation codes) and admin copy (legal policies, banners) have different owners and lifecycles.

### Decision
1. **Content**: JSONB `{"ar","en"}` stored on row columns using `i18n.Text`.
2. **UI Chrome**: Embedded catalog in Go binary keyed by `apperr.Code` and template identifiers.
3. **Admin Copy**: `platform_admin.translations` and policy tables editable through admin surface.

---

## ADR 0007 — Per-Module Admin Routes Under `/api/v1/admin/`

**Status:** Accepted · 2026-08-16

### Context (D-3)
Admin is a permission level, not an isolated bounded context. A central admin module would break architecture boundaries by importing all domain repositories.

### Decision
Each module owns its administrative endpoints mounted under `/api/v1/admin/<module>/...` protected by `identity.RequirePermission`.

---

## ADR 0008 — Keyset (Cursor) Pagination by Default

**Status:** Accepted · 2026-08-16

### Context (D-4)
Large tables (`catalog.products`, `commerce.order_lines`) suffer severe query latency with high offsets.

### Decision
Keyset (cursor) pagination is the standard for all listing APIs (`internal/shared/pagination`), with default limit of 50 and ceiling limit of 200. Offset pagination is used only for specific tabular admin views requiring page number navigation.

---

## ADR 0009 — Direct-to-Storage Presigned Uploads

**Status:** Accepted · 2026-08-16

### Context (D-5)
Large spreadsheet imports and media uploads proxying through application workers tie up worker threads and memory.

### Decision
The backend generates presigned PUT URLs via `storage.PresignPut`. The client uploads directly to MinIO/S3 and confirms the object key to the backend, which performs server-side validation.

---

## ADR 0010 — Session Cookies for Frontend with CSRF Protection

**Status:** Accepted · 2026-08-16

### Context (D-7)
The templ/HTMX frontend is server-rendered and same-origin.

### Decision
The frontend uses Redis-backed session cookies with `SameSite=Lax`, `HttpOnly`, `Secure` flags, and per-session CSRF token validation on mutating HTTP methods (`POST`, `PUT`, `PATCH`, `DELETE`). External vendor API integrations use PASETO tokens.

---

## ADR 0011 — Data Integrity: Partial Unique Indexes, State Machines, Decimal Strings, Idempotent ETL

**Status:** Accepted · 2026-08-16

### Context (D-8, D-9, D-10, D-11)
Soft deletes, order transitions, currency representation, and data migration require strict consistency.

### Decision
1. **Soft Deletes**: Unique indexes on soft-deletable tables are partial: `WHERE deleted_at IS NULL`.
2. **Order Transitions**: Validated by domain state machines and recorded to `order_status_history` in the same transaction.
3. **Money in JSON**: Always formatted as a two-decimal JSON string via `money.Amount`.
4. **ETL**: Built as resumable, chunked, idempotent operations with 2-way sum reconciliation down to the cent.

---

## ADR 0012 — Codebase Structural Bounds and Audience-Gated Architecture

**Status:** Accepted · 2026-08-31

### Context
As the platform expanded to support complex workflows (smart ordering, bulk catalog ingestion, multi-tenant RBAC, promotional bidding, and invoice adjudication), source files and templates accumulated high line counts and intertwined handler responsibilities. Monolithic templates (>1,000 lines) and oversized Go files (>400 lines) degrade maintainability, compile-time performance, and auditability.

### Decision
1. **Source File Line Bounds**: Enforce hard ceilings of $\le 400$ lines per Go file and $\le 1,000$ lines per Templ file across the entire repository.
2. **Template Decomposition**: Split large views into modular subtemplates (`_table.templ`, `_modals.templ`, `_script.templ`, `_subpages.templ`) using declarative component composition (`@SubComponent(...)`).
3. **Audience-Gated UI Partitioning**: Route handlers in `internal/ui` are structured by target audience (`admin_*.go`, `vendor_*.go`, `customer_*.go`, `shared_*.go`, `auth_*.go`), with strict AST regression tests in `test/route_audience_test.go` verifying that all endpoints are mounted within corresponding authenticated Chi middleware groups (`RequireCustomer`, `RequireVendor`, `RequireStaff`, `RequireApproved`).
4. **Deadcode Ratchet Discipline**: Integrate `golang.org/x/tools/cmd/deadcode` into the automated testing harness (`test/deadcode_ratchet_test.go`) to prevent dead code accumulation beyond the measured baseline.

---

## ADR 0013 — Permission Namespaces Are Scoped, and the Scopes Do Not Mix

**Status:** Accepted · 2026-08-31

### Context

The permission registry declares keys in three catalogues:
`catalog_admin.go`, `catalog_vendor.go` and `catalog_pharmacy.go`. Keys beginning
`commerce.`, `catalog.`, `org.`, `identity.`, `platform.` and `billing.` are declared
through `adminPage()` / `adminAct()` and are held only by the staff roles. Keys beginning
`vendor.` and `pharmacy.` are tenant keys, held by members of a supplier or pharmacy
organization.

A supplier cannot hold `commerce.order.fulfil`. Not "does not by default" — **cannot**: the
key is not offered by the vendor catalogue, so no vendor role can ever grant it.

This was learned expensively. A fulfilment handler was secured with:

```go
allowed := actor.IsStaff || actor.Can("commerce.admin") ||
    (shipment.OrganizationID == actor.OrganizationID &&
     actor.CanAny("commerce.order.fulfil", "commerce.order.dispatch", "commerce.order.update"))
```

Every key in that list is real, and every key in that list is admin-scope. `allowed` was
therefore permanently false for every supplier, and supplier shipment fulfilment was broken
in production until it was found. The test that covered the handler passed, because it built
its vendor actor as `Permissions: []string{"commerce.order.fulfil"}` — a permission no real
vendor can hold. The test proved the handler worked for an actor that cannot exist.

### Decision

1. A gate on a tenant route uses a `vendor.*` or `pharmacy.*` key. A gate on a staff route
   uses an admin-scope key. The two never mix, except in an explicit staff-bypass branch
   (`actor.IsStaff || actor.Can("<admin key>")`) sitting beside the tenant check.
2. Tests construct actors from declared roles via `authctxtest.ActorForRole`, not from
   hand-written permission slices. A synthetic slice is permitted only when the test is
   exercising the gate mechanism itself, and must be obviously synthetic at the call site.
3. `test/rbac_guard_test.go` asserts both that every gate key exists and that its scope
   matches the audience of the route it guards. An admin key on a vendor route fails the
   build.

### Consequences

A whole class of "correct-looking authorization that refuses everyone" is now caught by the
build rather than by a supplier calling support. The cost is that a genuinely cross-audience
endpoint cannot be gated by a single middleware key — those routes authorize on
*relationship* (does this shipment belong to the caller's organization) with an explicit
staff bypass, which is the honest shape for them anyway.

---

## ADR 0014 — One Cascade Order, Declared with @layer

**Status:** Accepted · 2026-08-31

### Context

The stylesheets carried **670 `!important` declarations** — 212 in `components.css`, 195 in
`utilities.css`, 181 in `foundations.css`. That is not a styling problem. It is what a
cascade with no defined order produces: each fix had to out-shout the last, and a change on
one page never generalised, which is the mechanical reason the design drifted.

### Decision

One order, declared once, before any rule:

```css
@layer reset, tokens, base, layout, components, utilities;
```

Utilities win by position, not by force. Every stylesheet lives in exactly one layer.

`app.css` is the single deliberate exception and stays **outside** all layers, because
unlayered rules beat every layered rule and that is precisely what a hide/show primitive
needs: a cloaked element must not become visible because a component rule later in the sheet
sets `display:flex`. `app.css` therefore contains hide/show primitives and nothing else.

### Consequences

`!important` fell from 670 to **3**, each with a comment justifying it. `check-important` and
`check-css-layered` hold both properties.

The second gate exists because it was learned that removing `!important` is not sufficient.
Five stylesheets initially shipped without a layer wrapper, so they outranked the entire
system that had just been built — the same problem wearing a different hat. A stylesheet
outside `@layer` is as strong as `!important` and much harder to notice.

`app.css` also once carried `@import` rules for six stylesheets already `<link>`ed from the
layout. The browser parsed all six twice, and the imported copies arrived last and unlayered,
silently outranking the originals. `@import` of a local stylesheet is now forbidden.

---

## ADR 0015 — API Gates Answer 403; Page Gates Answer 404

**Status:** Accepted · 2026-08-31

### Context

The HTML surface answers an authorization refusal with **404**, deliberately: a support agent
must not learn that `/admin/developers` exists, and a vendor must not learn the shape of the
`/customer/*` URL space. That reasoning does not transfer to a JSON client, which is already
authenticated and for which a misleading 404 costs debuggability and buys nothing.

Before this was settled, the JSON API had no declarative authorization at all. Every module
mounted behind `RequireAuth` + `ResolveTenant`, and authorization lived inside handler bodies
— which is why three endpoints in one file had no ownership check while a fourth, four
functions away, did it correctly.

### Decision

- `RequirePagePermission` / `RequireTenantPagePermission` gate HTML routes and answer 404.
- `RequireAPIPermission` / `RequireAPITenantPermission` gate JSON routes and answer 403 via
  `httpx.Error` + `apperr.Forbidden`.
- Both log every refusal through the single `denied()` helper, so a permission problem in
  production is one grep.
- A genuinely multi-audience endpoint (staff, buyer and supplier all legitimately reach it)
  authorizes on relationship inside the handler rather than by middleware key, and says so in
  a comment.

### Consequences

The response-code split is intentional and must not be "harmonised" later. An engineer who
sees a 404 from an HTML route and a 403 from an API route for the same underlying refusal is
looking at the design, not a bug.

---

## ADR 0016 — One Dashboard Header, One Site Header

**Status:** Accepted · 2026-08-31

### Context

Four headers existed. The public marketing navbar and all three dashboard shells rendered the
same class, and one CSS rule listing that class alongside the public one set height, padding,
sticky positioning and shadow for all four at once.

They are not the same component. A marketing bar carries a brand lockup and five nav links
and wants room. A dashboard bar sits directly above a data table and wants to be compact and
quiet. The dashboard bar was wearing the marketing bar's 4.5rem proportions, which is most of
why it felt wrong. Each shell then hand-built its contents; two used helper classes and the
third used inline styles, so it did not even align with the other two.

### Decision

`components.DashboardTopBar` is the only dashboard header, with a contract of three regions:
a lead (drawer toggle and title), an optional context slot for what a surface needs to say
about *where* the caller is, and a fixed action cluster ending in the account menu. The order
is fixed so the sign-out control does not move between dashboards.

`.site-header` is the public header and shares no rules with it.

Density is a token (`is-compact`), never a fork. Admin and vendor set it; the pharmacy surface
does not, because its users are on phones at a counter and need 44px targets. **A component is
never duplicated to serve a second density.**

### Consequences

`check-topbar-impls` fails the build if a shell hand-builds a header again. Chrome changes now
happen once instead of four times, and cannot silently diverge.



---

## ADR 0017 — A Class With No Rule Is a Silent Regression

**Status:** Accepted · 2026-08-31

### Context

Phase 5 converted 136 page templates, removing **2,089 inline `style` attributes** and
replacing them with class names. Every ratchet agreed it went well: inline styles fell from
2,170 to 81, raw `<dialog>` reached zero, `!important` held at 3.

None of those gates could see the actual defect. A conversion can remove a `style` attribute
and put a class name in its place **without anyone writing a rule for that class**. The diff
looks like progress. The gate counts go the right way. The element renders unstyled.

Measured after the conversion: 1,365 distinct class names appear in templates, and **468 had
no matching selector in any stylesheet**. Comparing across refs, 398 of those already existed
at the end of Phase 3 — harmless then, because an inline style was still doing the work.
Removing the inline styles is what converted a latent inconsistency into visible breakage.

### Decision

`check-undefined-classes` compares every class token used in a template against every class
selector defined in the stylesheets, and fails when the difference grows. It prints the
offenders, and its failure message explains what the number means, because the number alone
does not communicate the failure mode.

The ceiling stands at 235. The residue is dominated by Alpine state names — `activeTab`,
`filterTab`, `policyFilter` — which appear inside `:class` bindings and are never CSS
selectors. Lowering the ceiling further requires distinguishing those from real classes,
which is worth doing only if the number starts drifting.

### Consequences

The general lesson is worth more than the fix. **A ratchet measures what it counts, not what
you care about.** `check-inline-styles` counts `style="` and cannot tell a conversion from a
deletion; it read 81 while hundreds of elements were unstyled. Every gate added from here
should be asked the same question: what failure would still pass this?

Related: shell CSS was being tracked against a 90 KB target that was measured while 468
classes' worth of rules were missing. Part of the apparent progress toward it was styling
that had gone absent. The target must be reset against a complete stylesheet, and the
`components.css` / `foundations.css` / `utilities.css` dedupe that would actually reach it
should follow the visual regression suite, not precede it.


---

## ADR 0018 — Arabic Matching Data Is Not Translatable Text

**Status:** Accepted · 2026-08-31

### Context

`check-hardcoded-arabic` counted every Arabic string literal in Go and drove toward zero, on
the reasoning that a string baked into a handler can never be shown in English. That was
right for handler text and wrong for what remained once handler text was gone.

At the end of the conversion the count sat at 226, all of it in `internal/modules`, and none
of it user-facing:

| File | What the Arabic is |
|---|---|
| `ingest/domain.go`, `compare/columns_data.go` | column-header dictionaries used to recognise Arabic headings in an uploaded spreadsheet |
| `catalog/import_rows.go` | dosage-form vocabulary — `"غسول فم"` → mouthwash, `"معجون أسنان"` → toothpaste |
| `compare/comparison.go` | Arabic → Latin brand map — `"بانادول"` → panadol |
| `smartorder/pipeline/query.go` | Arabic tokenisation rules, largely in comments |
| `assistant/prompt.go` | the assistant's Arabic system prompt |

These are **inputs the platform matches against, not output it renders**. A supplier uploads
an Excel file with a column headed `الباركود`; the importer recognises it because that exact
string is in a dictionary. Moving it into the i18n catalogue would not translate anything —
it would break product matching, which is the core feature of the platform.

### Decision

The gate measures **user-facing** Arabic and excludes those files by name, with the reason for
each written beside the exclusion. The ceiling is **0**, not a ratchet, because the remaining
population is genuinely empty: `internal/ui` and `cmd` both measure zero.

Adding a file to the exclusion list requires the same justification — that the strings are
matched against rather than displayed.

### Consequences

The English requirement is complete for user-facing code, and the gate now says so honestly
instead of reporting 226 against a target that could never be reached.

The wider point is the same one ADR 0017 makes from the other direction. A ratchet is a proxy,
and a proxy stops being useful at the boundary where the thing it counts and the thing you
care about come apart. Driving this one to literal zero would have been measurable, defensible
on the number, and a product regression.
