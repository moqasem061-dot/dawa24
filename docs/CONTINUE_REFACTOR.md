# Continue the Dawa24 refactor

Hand this file to whoever picks the work up next, human or model. It is written
to be pasted as a prompt on its own.

---

## The prompt

You are continuing a platform-wide refactor of **Dawa24 Store**, an Arabic-first
B2B pharmaceutical marketplace for the Egyptian market: a Go modular monolith
(chi + templ + pgx + River) on PostgreSQL 18, replacing a legacy Laravel 12 +
Livewire application. The repository is `F:\Dawa 24\dawa24-store`.

**Read these three files before writing any code, in this order:**

1. `AGENTS.md` — the non-negotiable rules. Money never touches `float64`; no AI
   provider names outside `internal/platform/gateway/`; every AI capability has
   a deterministic fallback; tenant queries run inside `db.InTx` / `db.InReadTx`;
   module boundaries are linted; 400 lines per Go file; legacy behaviour is
   ported exactly, and anything that looks like a bug is written down rather
   than "improved".
2. `docs/REFACTOR_2026-08-30.md` — everything two previous passes changed, why,
   and what was measured either side. Its "What was not done" and "Still not
   done" sections are your backlog.
3. This file.

### Working agreement

- **Do not commit or push.** The user reviews the diff themselves. The tree is
  currently uncommitted on `main` at `91709d6` with ~300 files changed.
- **Verify continuously.** `go build ./...`, `go vet ./...`, `gofmt -l .` and
  `go test -short -count=1 ./...` must stay green. 69 packages pass today.
  After any `.templ` edit run `templ generate` **from the repository root** with
  no `-path` flag — a `-path` run rewrites the `FileName` in every generated
  file and produces a 130-file diff of pure noise.
- **A test run must leave the tree clean.** `git status --porcelain` after
  `go test ./...` should be empty. This was broken and was fixed; do not
  reintroduce writes into `internal/ui/data/`.
- **Ratchets only go down.** `make check` runs 14 gates, six of which are
  numeric ceilings. When you improve one, lower its ceiling in the same change.
  Never raise one. (`make` is not installed on the user's machine — run the
  recipe bodies directly in bash to check them.)
- **Report honestly.** If a finding turns out to be wrong, correct it in
  `docs/REFACTOR_2026-08-30.md` rather than dropping it. Two findings were
  overstated in the original audit and were corrected in place; do the same.

### Current state, measured

| Metric | Value | Ceiling in `make check` |
|---|---|---|
| Go files over 400 lines | 93 | 93 |
| Inline `style=` attributes (pages + layouts) | 4,042 | 4,042 |
| Arabic string literals in Go | 2,507 | 2,507 |
| Emoji in templates | 43 (byte-count metric) | 43 |
| Components no page uses | 50 | 50 |
| CDN asset hosts in templates | 0 | zero tolerance |

Database: 151 tables, 911 indexes, RLS enabled **and** forced on all 92 tenant
tables, no unindexed foreign keys, no duplicate indexes.

---

## The backlog, in priority order

### 1. Split `internal/ui` into packages by audience — the largest remaining job

156 files in one flat `package ui`, behind a single `UIHandler` struct with 25
service dependencies, serving admin, vendor, customer, public and print alike.
The files are now split by concern (largest is 543 lines, down from 3,149) but
the package boundary is untouched.

Target: `ui/admin`, `ui/vendor`, `ui/customer`, `ui/public`, `ui/shared`, each
with its own handler type holding only the services it uses.

**Do the prerequisite first.** The repository layer has no tests at all — every
`internal/modules/*/postgres` package, plus `platform/config`, `cache`,
`features`, `observability`. That is precisely the safety net this cut needs.
Write characterisation tests against current behaviour for `catalog`,
`commerce`, `billing` and `org` before moving anything.

`internal/ui/render.go`, `request_helpers.go` and `handler_deps.go` are the
natural contents of `ui/shared`.

### 2. Move the 2,507 Arabic literals out of Go

An English-language user is currently served Arabic from the backend regardless
of locale. The engine exists and works: `i18n.T(lang, "key")`, with keys
declared by `addKey(e, key, namespace, textAR, textEN, description)` in
`internal/shared/i18n/catalog_*.go`. 475 keys exist; 39 are used in templates.

One worked example was done: `admin.jobs_unknown_company`, used from
`internal/ui/admin_commerce_handlers.go`.

The user's decision on scope: **strict on the customer and vendor surfaces,
best-effort on admin.** Worst offenders: `admin_handlers.go` and its descendants
(202 literals originally), `vendor_handlers.go` (103), `visitor.go` (63),
`customer_handlers.go` (62), `compare_handlers.go` (61).

Do it file by file while you are in a file for another reason, and lower
`check-hardcoded-arabic` each time.

### 3. Convert pages onto the design system — customer surface first

The user chose audience-by-audience, customer first. Per page: replace
hand-rolled markup with the shared components, replace inline styles with
tokens or utility classes, add real loading / empty / error states, verify RTL
and the responsive scale, and remove decoration that carries no meaning.

**The component library is the problem, not the solution yet.** 50 of 148
components are referenced by zero pages — including every button, every form
field, the tabs, the toast, all seven skeletons, the pagination and the
`DataTable`. Either a page should be using each one, or it should not exist.
Decide per component; do not simply delete them to move the number.

Also outstanding here:
- **4,042 inline styles.** The repeated exact-match patterns were already swept
  onto utility classes; what remains is a long tail needing per-page judgement.
- **12 ad-hoc breakpoints** (420/640/700/720/768/860/900/980/992/1024/1100 px).
  Define a scale of three or four and delete the rest.
- **100 inline `<svg>` blocks** bypassing `internal/ui/components/icons.templ`.
- **43 emoji left.** Mostly status dots (🟢 🔴) where no icon means the same
  thing. These want a design decision — a status pill component — not a sweep.
- **56 inline `<script>` blocks** in templates. Moving them into `app.js` is
  what lets `'unsafe-inline'` come out of the CSP.

### 4. Two emoji jobs that a sweep must not touch

Both were deliberately left, and the reasons matter:

- `internal/ui/visitor.go` stores country names with flag emoji (`"مصر 🇪🇬"`),
  and `internal/modules/platform_admin/postgres/visitors.go` has SQL comparing
  against those exact strings. Changing one without the other silently stops
  matching rows already in the database. Fix both together, with a migration
  for the stored values.
- Structs with an `Icon string` field holding an emoji — `pages/wizard.go`,
  `pages/admin_import_wizard.go`, `modules/ingest/catalog_import.go`,
  `modules/identity/domain.go`. Converting them means changing every render
  site, which is a change per site rather than a sweep.

### 5. Pagination

17 handlers fetch with a hardcoded limit and no pagination surface. **Most of
them are fine**: `ListOrganizations(sysCtx, nil, nil, 500, 0)` populates filter
dropdowns and genuinely wants the whole list — paginating those would break the
dropdown.

The three that are page-driven and do need real pagination, because each
renders rows and each will grow:

- `internal/ui/customer_saving_handlers.go:43` — `ListSavingProductsEnriched(…, 500, 0)`
- `internal/ui/vendor_saving_handlers.go:37` — same
- `internal/ui/admin_catalog_handlers.go:216` — `ListAllSavingProductsAdmin(…, 500, 0)`

`h.pageLimit(r)` and `h.pageOffset(r)` already exist in
`internal/ui/request_helpers.go`, and `components.B2BPagination` exists and is
used on three pages. Each conversion needs the page template to render the
control, so treat it as template work, not a parameter swap.

### 6. Optimisation, once there is evidence

`pg_stat_statements` is **not** installed: `shared_preload_libraries` is empty
on the Elestio instance and only a server restart changes that. The user has
been asked. Until it is there:

- Thirty indexes have never been scanned since the database was created. That
  is suggestive but not conclusive — a feature nobody has exercised cannot have
  used its index. Do not drop them on this evidence.
- No N+1 sites remain (all five from the original audit are resolved; one of
  the five turned out to be bounded by an explicit guard, not an N+1).

### 7. Things only the user can do — remind them if still open

- **Rotate the PostgreSQL password and the Gateway administrator credential.**
  The production superuser password had been stored in
  `platform_admin.system_settings['gateway_configuration'].api_key` and sent as
  HTTP Basic auth to `api.muhiya.com` on every management call. The stored value
  was cleared and the code now refuses to hand out a credential that fails
  validation, but the password itself has already left the building.
- **Enter a real Gateway administrator credential** in the admin settings
  screen — Gateway provisioning is disabled until they do.
- **Enable `pg_stat_statements`.**
- **Decide about migration 48.** `seed_realistic_data` was applied to production
  on 17 August and its file no longer exists, so a fresh environment cannot
  reproduce production. `go run ./cmd/cli migrate-status` now reports it and
  exits 3. Either reconstruct the file or write the divergence off in
  `docs/adr/`.

---

## Environment notes that will cost you time otherwise

- **Windows.** `make` is not installed; run gate recipe bodies directly in bash.
  `psql` is not installed either — to query the database, write a small `pgx`
  program under `tmp/` (gitignored) and `go run` it.
- **The server cannot be booted here.** It requires Redis and object storage,
  and the machine has neither Redis nor Docker. Everything so far is verified by
  compilation, tests, template rendering and direct database queries — *not* by
  clicking through the running product. Say so plainly rather than implying a
  page was seen working.
- **`goimports` and `templ` are installed** at `~/go/bin`.
- **Answered questions, do not re-ask:** the production database has no real
  customer data; the application connects as `dawa24_app`, not `postgres`;
  frontend conversion is audience-by-audience, customer first; bilingual
  strictness is customer/vendor strict, admin best-effort; there is no deadline.

## Useful tooling left behind

Under the previous session's scratchpad, and worth recreating if gone: a
declaration-level Go file splitter driven by a JSON plan (moves top-level decls
verbatim between files in the same package, then `goimports` prunes), and an
emoji-to-icon converter that distinguishes an emoji that is an element's *only*
content — which must become an icon, never be deleted — from one sitting beside
text that already says the same thing.

The lesson from that second script is worth keeping: a blanket delete of
decoration broke the build, because an `if` branch containing only an emoji
became an empty branch (templ rejects those) and a delete button containing only
a wastebasket became an invisible control.
